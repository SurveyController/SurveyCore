package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseProxyFromNestedJSON(t *testing.T) {
	payload := `{
		"data": {
			"items": [
				{"ip": "1.1.1.1", "port": 8000, "account": "user", "password": "pass"},
				{"proxy": "http://2.2.2.2:9000"},
				{"nested": {"list": ["3.3.3.3:7000"]}}
			]
		}
	}`

	leases, err := parseProxyFromJSON(payload)
	if err != nil {
		t.Fatalf("parseProxyFromJSON failed: %v", err)
	}
	if len(leases) != 3 {
		t.Fatalf("leases length = %d, want 3: %#v", len(leases), leases)
	}
	if leases[0].Address != "user:pass@1.1.1.1:8000" {
		t.Fatalf("first proxy = %q", leases[0].Address)
	}
}

func TestFetchFromCustomChecksHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusForbidden)
	}))
	defer server.Close()

	_, err := fetchFromCustom(server.URL, 1)
	if err == nil {
		t.Fatal("expected error for HTTP 403")
	}
}

func TestFetchFromCustomParsesCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":["user:pass@4.4.4.4:8080"]}`))
	}))
	defer server.Close()

	leases, err := fetchFromCustom(server.URL, 1)
	if err != nil {
		t.Fatalf("fetchFromCustom failed: %v", err)
	}
	if len(leases) != 1 || leases[0].Address != "user:pass@4.4.4.4:8080" {
		t.Fatalf("leases = %#v", leases)
	}
	if got := ExtractProxyAddress(leases[0].Address); got != "http://user:pass@4.4.4.4:8080" {
		t.Fatalf("normalized proxy = %q", got)
	}
}

func TestFetchFromOfficialPostsSessionAndParsesBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("X-Device-ID"); got != "device-1" {
			t.Fatalf("X-Device-ID = %q, want device-1", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["user_id"].(float64) != 33 || body["minute"].(float64) != 3 || body["num"].(float64) != 2 {
			t.Fatalf("request body numeric fields = %#v", body)
		}
		if body["pool"] != "quality" || body["upstream"] != "default" || body["area"] != "110100" {
			t.Fatalf("request body string fields = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"provider":"default",
			"items":[
				{"host":"1.1.1.1","port":8000,"account":"user","password":"pass","expire_at":"2030-01-02T03:04:05Z"},
				{"host":"2.2.2.2","port":9000,"account":"user2","password":"pass2","expire_at":"2030-01-02T03:05:05Z"}
			]
		}`))
	}))
	defer server.Close()

	leases, err := fetchFromOfficial("default", 2, officialOptions{
		Endpoint: server.URL,
		UserID:   33,
		DeviceID: "device-1",
		AreaCode: "110100",
		Minute:   3,
		Pool:     "quality",
	})
	if err != nil {
		t.Fatalf("fetchFromOfficial failed: %v", err)
	}
	if len(leases) != 2 {
		t.Fatalf("leases length = %d, want 2", len(leases))
	}
	if leases[0].Address != "user:pass@1.1.1.1:8000" || leases[0].Source != "default" || !leases[0].Poolable {
		t.Fatalf("first lease = %#v", leases[0])
	}
	if leases[0].ExpireTS <= 0 {
		t.Fatalf("first lease ExpireTS = %v, want parsed timestamp", leases[0].ExpireTS)
	}
}

func TestFetchFromOfficialBenefitUsesIdiotUpstreamAndSinglePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["upstream"] != "idiot" || body["minute"].(float64) != 1 {
			t.Fatalf("benefit request body = %#v", body)
		}
		if _, hasNum := body["num"]; hasNum {
			t.Fatalf("single request should omit num: %#v", body)
		}
		_, _ = w.Write([]byte(`{"provider":"idiot","host":"3.3.3.3","port":7000,"account":"u","password":"p"}`))
	}))
	defer server.Close()

	leases, err := fetchFromOfficial("benefit", 1, officialOptions{
		Endpoint: server.URL,
		UserID:   44,
		DeviceID: "device-2",
		Minute:   5,
	})
	if err != nil {
		t.Fatalf("fetchFromOfficial failed: %v", err)
	}
	if len(leases) != 1 || leases[0].Address != "u:p@3.3.3.3:7000" || leases[0].Source != "benefit" {
		t.Fatalf("leases = %#v", leases)
	}
	if leases[0].Poolable {
		t.Fatalf("lease without expire_at should not be poolable: %#v", leases[0])
	}
}

func TestFetchFromOfficialRequiresCredentials(t *testing.T) {
	t.Setenv("WJX_RANDOM_IP_USER_ID", "")
	t.Setenv("RANDOM_IP_USER_ID", "")
	t.Setenv("WJX_RANDOM_IP_DEVICE_ID", "")
	t.Setenv("RANDOM_IP_DEVICE_ID", "")

	_, err := fetchFromOfficial("default", 1, officialOptions{Endpoint: "http://example.invalid"})
	if err == nil {
		t.Fatal("expected missing credentials error")
	}
}

func TestFetchFromOfficialReportsHTTPDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"detail":"token_rate_limited"}`))
	}))
	defer server.Close()

	_, err := fetchFromOfficial("default", 1, officialOptions{
		Endpoint: server.URL,
		UserID:   55,
		DeviceID: "device-3",
	})
	if err == nil || !strings.Contains(err.Error(), "token_rate_limited") {
		t.Fatalf("error = %v, want token_rate_limited", err)
	}
}
