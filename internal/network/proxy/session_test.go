package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestRandomIPServiceActivatesTrialAndPersistsSession(t *testing.T) {
	var gotDeviceID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotDeviceID = r.Header.Get("X-Device-ID")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_id":88,"remaining_quota":9,"total_quota":10,"used_quota":1}`))
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "random-ip.json")
	service := NewRandomIPService(path, RandomIPEndpoints{TrialEndpoint: server.URL})

	snapshot, err := service.ActivateTrial(t.Context())
	if err != nil {
		t.Fatalf("ActivateTrial failed: %v", err)
	}
	if !snapshot.Authenticated || snapshot.UserID != 88 || snapshot.DeviceID == "" || !snapshot.QuotaKnown {
		t.Fatalf("snapshot = %#v, want authenticated trial session", snapshot)
	}
	if gotDeviceID == "" || gotDeviceID != snapshot.DeviceID {
		t.Fatalf("X-Device-ID = %q, want generated device id %q", gotDeviceID, snapshot.DeviceID)
	}

	reloaded := NewRandomIPService(path, RandomIPEndpoints{TrialEndpoint: server.URL})
	reloadedSnapshot, err := reloaded.Snapshot()
	if err != nil {
		t.Fatalf("reloaded Snapshot failed: %v", err)
	}
	if reloadedSnapshot.UserID != 88 || reloadedSnapshot.DeviceID != snapshot.DeviceID {
		t.Fatalf("reloaded snapshot = %#v, want persisted user/device", reloadedSnapshot)
	}
}

func TestRandomIPServiceSyncQuotaKeepsSessionAndUpdatesQuota(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"user_id":88,"remaining_quota":9,"total_quota":10,"used_quota":1}`))
			return
		}
		_, _ = w.Write([]byte(`{"user_id":88,"remaining_quota":7,"total_quota":10,"used_quota":3}`))
	}))
	defer server.Close()

	service := NewRandomIPService(filepath.Join(t.TempDir(), "random-ip.json"), RandomIPEndpoints{TrialEndpoint: server.URL})
	if _, err := service.ActivateTrial(t.Context()); err != nil {
		t.Fatalf("ActivateTrial failed: %v", err)
	}
	snapshot, err := service.SyncQuota(t.Context())
	if err != nil {
		t.Fatalf("SyncQuota failed: %v", err)
	}
	if snapshot.UserID != 88 || snapshot.RemainingQuota != 7 || snapshot.UsedQuota != 3 {
		t.Fatalf("snapshot = %#v, want refreshed quota", snapshot)
	}
}

func TestRandomIPServiceRedeemCardPostsAuthenticatedUser(t *testing.T) {
	var redeemBody map[string]any
	trial := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"user_id":88,"remaining_quota":1,"total_quota":1,"used_quota":0}`))
	}))
	defer trial.Close()
	redeem := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&redeemBody); err != nil {
			t.Fatalf("decode redeem body: %v", err)
		}
		_, _ = w.Write([]byte(`{"redeemed":true,"card_quota":5,"remaining_quota":6,"total_quota":6,"used_quota":0}`))
	}))
	defer redeem.Close()

	service := NewRandomIPService(filepath.Join(t.TempDir(), "random-ip.json"), RandomIPEndpoints{
		TrialEndpoint:      trial.URL,
		CardRedeemEndpoint: redeem.URL,
	})
	if _, err := service.ActivateTrial(t.Context()); err != nil {
		t.Fatalf("ActivateTrial failed: %v", err)
	}
	result, err := service.RedeemCard(t.Context(), "CARD-1")
	if err != nil {
		t.Fatalf("RedeemCard failed: %v", err)
	}
	if redeemBody["user_id"] != float64(88) || redeemBody["card_code"] != "CARD-1" {
		t.Fatalf("redeem body = %#v", redeemBody)
	}
	if result["redeemed"] != true || result["card_quota"] != float64(5) {
		t.Fatalf("result = %#v, want redeemed quota", result)
	}
}
