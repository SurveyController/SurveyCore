package restapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestV1HealthAndNotFound(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Config{DBPath: dir + "\\test.db"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	resp, err := http.Get(server.URL + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status=%d", resp.StatusCode)
	}
	resp, err = http.Get(server.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("legacy health status=%d, want 404", resp.StatusCode)
	}
	resp, err = http.Get(server.URL + "/api/v1/tasks/missing")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status=%d", resp.StatusCode)
	}
}

func TestV1CreateTaskRejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Config{DBPath: dir + "\\test.db"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	body := `{"unknown":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != "invalid_json" {
		t.Fatalf("code=%v", payload["code"])
	}
}
