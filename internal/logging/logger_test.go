package logging

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestStructuredLogIncludesLevel(t *testing.T) {
	var buf bytes.Buffer
	oldColor := os.Getenv("NO_COLOR")
	t.Setenv("NO_COLOR", "")
	SetOutput(&buf)
	t.Cleanup(func() {
		SetOutput(os.Stderr)
		_ = os.Setenv("NO_COLOR", oldColor)
	})

	InfoFields("hello", F("task_id", "abc"))

	output := buf.String()
	if !strings.Contains(output, "[INFO]") {
		t.Fatalf("output = %q, want [INFO]", output)
	}
	if !strings.Contains(output, "task_id=") {
		t.Fatalf("output = %q, want task_id field", output)
	}
}

func TestNoColorDisablesANSI(t *testing.T) {
	var buf bytes.Buffer
	t.Setenv("NO_COLOR", "1")
	SetOutput(&buf)
	t.Cleanup(func() {
		SetOutput(os.Stderr)
	})

	InfoFields("hello", F("task_id", "abc"))

	output := buf.String()
	if strings.Contains(output, "\033[") {
		t.Fatalf("output = %q, want no ANSI", output)
	}
	if !strings.Contains(output, "task_id=abc") {
		t.Fatalf("output = %q, want plain field", output)
	}
}

func TestFieldKeyColorDoesNotColorValue(t *testing.T) {
	var buf bytes.Buffer
	t.Setenv("NO_COLOR", "")
	SetOutput(&buf)
	t.Cleanup(func() {
		SetOutput(os.Stderr)
	})

	InfoFields("hello", F("task_id", "abc"))

	output := buf.String()
	if !strings.Contains(output, "\033[36mtask_id=\033[0mabc") {
		t.Fatalf("output = %q, want only field key colored", output)
	}
}
