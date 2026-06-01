package main

import (
	"path/filepath"
	"testing"

	"github.com/SurveyController/SurveyCore/internal/appconfig"
)

func TestServiceConfigUsesSurveyPort(t *testing.T) {
	t.Setenv("SURVEY_PORT", "8080")

	cfg, err := appconfig.Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := cfg.ListenAddr(); got != "127.0.0.1:8080" {
		t.Fatalf("ListenAddr() = %q, want %q", got, "127.0.0.1:8080")
	}
}

func TestServiceConfigRejectsInvalidSurveyPort(t *testing.T) {
	t.Setenv("SURVEY_PORT", "abc")

	if _, err := appconfig.Load(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Fatal("Load() error = nil, want invalid port error")
	}
}
