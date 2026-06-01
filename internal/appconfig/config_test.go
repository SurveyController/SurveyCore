package appconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SurveyController/SurveyCore/internal/execution"
)

func TestLoadUsesDefaultsWhenFileMissing(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != defaultPort {
		t.Fatalf("port = %d, want %d", cfg.Server.Port, defaultPort)
	}
	if cfg.Storage.DBPath != defaultDBPath {
		t.Fatalf("db_path = %q, want %q", cfg.Storage.DBPath, defaultDBPath)
	}
	if cfg.AI.BaseURL != defaultAIBaseURL || cfg.AI.Model != defaultAIModel {
		t.Fatalf("ai defaults = %#v, want defaults", cfg.AI)
	}
}

func TestLoadReadsConfigFileAndEnvOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "surveycore.toml")
	if err := os.WriteFile(path, []byte(`
[server]
port = 19999

[storage]
db_path = "data/from-file.db"

[ai]
base_url = "https://ai.example.test/v1"
model = "test-model"
api_key = "file-key"
`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SURVEY_PORT", "20000")
	t.Setenv("AI_API_KEY", "env-key")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 20000 {
		t.Fatalf("port = %d, want env override", cfg.Server.Port)
	}
	if cfg.Storage.DBPath != "data/from-file.db" {
		t.Fatalf("db_path = %q, want file value", cfg.Storage.DBPath)
	}
	if cfg.AI.BaseURL != "https://ai.example.test/v1" || cfg.AI.Model != "test-model" || cfg.AI.APIKey != "env-key" {
		t.Fatalf("ai config = %#v, want file values with env key", cfg.AI)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "surveycore.toml")
	if err := os.WriteFile(path, []byte(`
[ai]
unknown = "x"
`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load() error = nil, want unknown field error")
	}
}

func TestApplyExecutionDefaultsFillsOnlyEmptyAIValues(t *testing.T) {
	cfg := Config{
		AI: AIConfig{BaseURL: "https://ai.example.test/v1", Model: "test-model", APIKey: "test-key"},
	}
	execCfg := &execution.ExecutionConfig{AIModel: "custom-model"}

	cfg.ApplyExecutionDefaults(execCfg)

	if execCfg.AIBaseURL != "https://ai.example.test/v1" {
		t.Fatalf("ai base url = %q, want config default", execCfg.AIBaseURL)
	}
	if execCfg.AIModel != "custom-model" {
		t.Fatalf("ai model = %q, want request value preserved", execCfg.AIModel)
	}
	if execCfg.AIAPIKey != "test-key" {
		t.Fatalf("ai key = %q, want config default", execCfg.AIAPIKey)
	}
}
