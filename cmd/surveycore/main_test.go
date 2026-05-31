package main

import "testing"

func TestListenAddrUsesLocalhostAndDefaultPort(t *testing.T) {
	t.Setenv("SURVEY_PORT", "")

	got, err := listenAddr()
	if err != nil {
		t.Fatalf("listenAddr() error = %v", err)
	}
	if got != "localhost:19178" {
		t.Fatalf("listenAddr() = %q, want %q", got, "localhost:19178")
	}
}

func TestListenAddrUsesSurveyPort(t *testing.T) {
	t.Setenv("SURVEY_PORT", "8080")

	got, err := listenAddr()
	if err != nil {
		t.Fatalf("listenAddr() error = %v", err)
	}
	if got != "localhost:8080" {
		t.Fatalf("listenAddr() = %q, want %q", got, "localhost:8080")
	}
}

func TestListenAddrRejectsInvalidPort(t *testing.T) {
	t.Setenv("SURVEY_PORT", "abc")

	if _, err := listenAddr(); err == nil {
		t.Fatal("listenAddr() error = nil, want error")
	}
}
