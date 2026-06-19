package surveycore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SurveyController/SurveyCore/internal/engine"
)

func TestDefaultConfigEmptyURL(t *testing.T) {
	cfg, err := New().DefaultConfig(context.Background(), "")
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("DefaultConfig returned nil config")
	}
	if cfg.Target != 1 {
		t.Fatalf("Target = %d, want 1", cfg.Target)
	}
	if cfg.Threads != 1 {
		t.Fatalf("Threads = %d, want 1", cfg.Threads)
	}
}

func TestRunRejectsNilConfig(t *testing.T) {
	_, err := New().Run(context.Background(), nil)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Run error = %v, want ErrInvalidConfig", err)
	}
}

func TestRunRejectsEmptyURL(t *testing.T) {
	_, err := New().Run(context.Background(), &RuntimeConfig{})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Run error = %v, want ErrInvalidConfig", err)
	}
}

func TestMapEvent(t *testing.T) {
	now := time.Now()
	got := mapEvent(engine.StatusEvent{
		ThreadName: "Worker-1",
		StatusText: "提交成功",
		Success:    true,
		Current:    1,
		Total:      2,
		Timestamp:  now,
	})
	if got.Worker != "Worker-1" || got.Message != "提交成功" || !got.Success || got.Current != 1 || got.Total != 2 || !got.Time.Equal(now) {
		t.Fatalf("mapped event mismatch: %+v", got)
	}
}
