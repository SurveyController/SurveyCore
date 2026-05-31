package appconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SurveyController/SurveyCore/internal/models"
)

func TestLoadUsesDefaultsWhenFileMissing(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != defaultPort {
		t.Fatalf("port = %d, want %d", cfg.Server.Port, defaultPort)
	}
	if cfg.Storage.DBPath != defaultDBPath {
		t.Fatalf("db_path = %q, want %q", cfg.Storage.DBPath, defaultDBPath)
	}
	if cfg.RandomIP.APIURL != defaultRandomIPAPIURL {
		t.Fatalf("random ip api = %q, want default", cfg.RandomIP.APIURL)
	}
	if cfg.AI.BaseURL != defaultAIBaseURL || cfg.AI.Model != defaultAIModel {
		t.Fatalf("ai defaults = %q/%q, want defaults", cfg.AI.BaseURL, cfg.AI.Model)
	}
}

func TestLoadReadsTOML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "surveycore.toml")
	writeTestFile(t, path, `
[server]
port = 19999

[storage]
db_path = "data/from-file.db"

[random_ip]
api_url = "https://proxy.example.test/extract"

[ai]
base_url = "https://ai.example.test/v1"
model = "test-model"
api_key = "test-key"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != 19999 {
		t.Fatalf("port = %d, want file value", cfg.Server.Port)
	}
	if cfg.Storage.DBPath != "data/from-file.db" {
		t.Fatalf("db_path = %q, want file value", cfg.Storage.DBPath)
	}
	if cfg.RandomIP.APIURL != "https://proxy.example.test/extract" {
		t.Fatalf("random ip api = %q, want file value", cfg.RandomIP.APIURL)
	}
	if cfg.AI.BaseURL != "https://ai.example.test/v1" || cfg.AI.Model != "test-model" || cfg.AI.APIKey != "test-key" {
		t.Fatalf("ai config = %#v, want file values", cfg.AI)
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "surveycore.toml")
	writeTestFile(t, path, `
[server]
port = 19178
unknown = true
`)

	if _, err := Load(path); err == nil {
		t.Fatal("Load() error = nil, want unknown key error")
	}
}

func TestListenAddrUsesFixedLocalhost(t *testing.T) {
	cfg := Default()
	cfg.Server.Port = 18080
	if got := cfg.ListenAddr(); got != "127.0.0.1:18080" {
		t.Fatalf("ListenAddr() = %q, want localhost addr", got)
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "surveycore.toml")
	writeTestFile(t, path, `
[server]
port = 70000
`)

	if _, err := Load(path); err == nil {
		t.Fatal("Load() error = nil, want invalid port error")
	}
}

func TestApplyRuntimeDefaultsFillsOnlyEmptyValues(t *testing.T) {
	cfg := Config{
		RandomIP: RandomIPConfig{APIURL: "https://proxy.example.test/extract"},
		AI:       AIConfig{BaseURL: "https://ai.example.test/v1", Model: "test-model", APIKey: "test-key"},
	}
	runtime := models.RuntimeConfig{
		AIModel: "custom-model",
	}

	cfg.ApplyRuntimeDefaults(&runtime)

	if runtime.IPExtractEndpoint != "https://proxy.example.test/extract" {
		t.Fatalf("ip endpoint = %q, want config default", runtime.IPExtractEndpoint)
	}
	if runtime.AIBaseURL != "https://ai.example.test/v1" {
		t.Fatalf("ai base url = %q, want config default", runtime.AIBaseURL)
	}
	if runtime.AIModel != "custom-model" {
		t.Fatalf("ai model = %q, want request value preserved", runtime.AIModel)
	}
	if runtime.AIAPIKey != "test-key" {
		t.Fatalf("ai key = %q, want config default", runtime.AIAPIKey)
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}
