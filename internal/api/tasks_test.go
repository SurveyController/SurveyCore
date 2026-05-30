package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/SurveyController/SurveyConsole/internal/models"
)

func TestNewProxyPoolFromRuntimeConfigUsesOfficialRandomIPConfig(t *testing.T) {
	var gotDeviceID string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotDeviceID = r.Header.Get("X-Device-ID")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"host":"1.2.3.4","port":"8080","account":"u","password":"p","expire_at":"2099-01-01T00:00:00Z"}]}`))
	}))
	defer server.Close()

	area := "110000"
	cfg := &models.RuntimeConfig{
		ProxySource:         "default",
		ProxyAreaCode:       &area,
		RandomIPUserID:      77,
		RandomIPDeviceID:    "device-77",
		IPExtractEndpoint:   server.URL,
		RandomIPLeaseMinute: 3,
	}

	pool := NewProxyPoolFromRuntimeConfig(cfg)
	leases, err := pool.FetchBatch(1)
	if err != nil {
		t.Fatalf("FetchBatch failed: %v", err)
	}
	if len(leases) != 1 || leases[0].Address != "u:p@1.2.3.4:8080" {
		t.Fatalf("leases = %#v, want one configured official lease", leases)
	}
	if gotDeviceID != "device-77" {
		t.Fatalf("device header = %q, want device-77", gotDeviceID)
	}
	if gotBody["user_id"] != float64(77) || gotBody["minute"] != float64(3) || gotBody["area"] != "110000" {
		t.Fatalf("request body = %#v, want configured user/minute/area", gotBody)
	}
}

func TestStorePersistsTaskAndLogs(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	task := &TaskRecord{ID: "task-1", Status: TaskRunning, Config: &models.RuntimeConfig{URL: "https://example.com"}}
	if err := store.SaveTask(task); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendLog("task-1", TaskLog{Level: "INFO", Message: "hello"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "task-1.json")); err != nil {
		t.Fatalf("task json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "task-1.logs.jsonl")); err != nil {
		t.Fatalf("task log missing: %v", err)
	}
}

func TestTaskManagerLoadMarksRunningInterruptedAndSkipsBadJSON(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTask(&TaskRecord{ID: "running", Status: TaskRunning, Config: &models.RuntimeConfig{URL: "x"}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{bad"), 0644); err != nil {
		t.Fatal(err)
	}

	manager := NewTaskManager(store, nil)
	errs := manager.Load()
	if len(errs) == 0 {
		t.Fatal("expected bad json error")
	}
	task, ok := manager.Get("running")
	if !ok {
		t.Fatal("running task not loaded")
	}
	if task.Status != TaskInterrupted {
		t.Fatalf("status = %q, want interrupted", task.Status)
	}
}
