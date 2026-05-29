package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	pool := newProxyPoolFromRuntimeConfig(cfg)
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

func TestApplyRunOverridesConnectsReverseFillAndProxyFlags(t *testing.T) {
	cfg := models.NewDefaultRuntimeConfig()

	applyRunOverrides(&cfg, runOverrides{
		URL:                   "https://www.wjx.cn/vm/abc.aspx",
		Target:                5,
		Threads:               2,
		RandomIPEnabled:       true,
		ProxySource:           "default",
		CustomProxyAPI:        "http://proxy.example.com",
		ProxyAreaCode:         "110000",
		RandomIPUserID:        77,
		RandomIPDeviceID:      "device-77",
		IPExtractEndpoint:     "http://extract.example.com",
		RandomIPLeaseMinute:   3,
		ReverseFillSourcePath: "samples.xlsx",
		ReverseFillFormat:     models.ReverseFillFormatWJXText,
		ReverseFillStartRow:   2,
		ReverseFillThreads:    4,
	})

	if cfg.URL != "https://www.wjx.cn/vm/abc.aspx" || cfg.Target != 5 || cfg.Threads != 2 {
		t.Fatalf("basic run overrides = url %q target %d threads %d", cfg.URL, cfg.Target, cfg.Threads)
	}
	if !cfg.RandomIPEnabled {
		t.Fatal("random IP flag was not enabled")
	}
	if cfg.ProxySource != "custom" || cfg.CustomProxyAPI != "http://proxy.example.com" {
		t.Fatalf("custom proxy overrides = source %q api %q", cfg.ProxySource, cfg.CustomProxyAPI)
	}
	if cfg.ProxyAreaCode == nil || *cfg.ProxyAreaCode != "110000" {
		t.Fatalf("proxy area = %#v, want 110000", cfg.ProxyAreaCode)
	}
	if cfg.RandomIPUserID != 77 || cfg.RandomIPDeviceID != "device-77" || cfg.IPExtractEndpoint != "http://extract.example.com" || cfg.RandomIPLeaseMinute != 3 {
		t.Fatalf("official random IP overrides not applied: %#v", cfg)
	}
	if !cfg.ReverseFillEnabled {
		t.Fatal("reverse fill should be enabled when a source path is provided")
	}
	if cfg.ReverseFillSourcePath != "samples.xlsx" || cfg.ReverseFillFormat != models.ReverseFillFormatWJXText {
		t.Fatalf("reverse fill source/format = %q/%q", cfg.ReverseFillSourcePath, cfg.ReverseFillFormat)
	}
	if cfg.ReverseFillStartRow != 2 || cfg.ReverseFillThreads != 4 {
		t.Fatalf("reverse fill row/threads = %d/%d", cfg.ReverseFillStartRow, cfg.ReverseFillThreads)
	}
}
