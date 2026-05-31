package engine

import (
	"testing"
	"time"

	"github.com/SurveyController/SurveyCore/internal/execution"
)

func TestSampleIntervalDelayUsesRange(t *testing.T) {
	for i := 0; i < 100; i++ {
		delay := sampleIntervalDelay([2]int{1, 3})
		if delay < time.Second || delay > 3*time.Second {
			t.Fatalf("delay = %s, want between 1s and 3s", delay)
		}
	}
}

func TestSampleIntervalDelaySwapsBounds(t *testing.T) {
	for i := 0; i < 100; i++ {
		delay := sampleIntervalDelay([2]int{4, 2})
		if delay < 2*time.Second || delay > 4*time.Second {
			t.Fatalf("delay = %s, want between 2s and 4s", delay)
		}
	}
}

func TestSampleUserAgentHonorsDisabledAndRatios(t *testing.T) {
	disabled := sampleUserAgent(&execution.ExecutionConfig{})
	if disabled != "" {
		t.Fatalf("disabled UA = %q, want empty provider default", disabled)
	}

	cfg := &execution.ExecutionConfig{
		RandomUserAgentEnabled: true,
		RandomUserAgentKeys:    []string{"pc_web"},
		UserAgentRatios:        map[string]int{"pc": 100},
	}
	ua := sampleUserAgent(cfg)
	if ua == "" {
		t.Fatal("random UA should be selected when enabled")
	}
	if ua != userAgentProfiles["pc_web"] {
		t.Fatalf("UA = %q, want pc profile", ua)
	}
}

func TestSampleUserAgentKeepsLegacyKeys(t *testing.T) {
	cfg := &execution.ExecutionConfig{
		RandomUserAgentEnabled: true,
		RandomUserAgentKeys:    []string{"mobile"},
		UserAgentRatios:        map[string]int{"mobile": 100},
	}
	ua := sampleUserAgent(cfg)
	if ua != userAgentProfiles["mobile"] {
		t.Fatalf("UA = %q, want legacy mobile profile", ua)
	}
}
