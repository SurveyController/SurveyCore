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
		RandomUserAgentKeys:    []string{"pc"},
		UserAgentRatios:        map[string]int{"pc": 100},
	}
	ua := sampleUserAgent(cfg)
	if ua == "" {
		t.Fatal("random UA should be selected when enabled")
	}
	if ua != userAgentProfiles["pc"] {
		t.Fatalf("UA = %q, want pc profile", ua)
	}
	profile := sampleUserAgentProfile(cfg)
	if profile.Category != "pc" || profile.UserAgent != userAgentProfiles["pc"] {
		t.Fatalf("profile = %#v, want pc profile", profile)
	}
}

func TestSampleUserAgentSkipsZeroWeightProfiles(t *testing.T) {
	cfg := &execution.ExecutionConfig{
		RandomUserAgentEnabled: true,
		RandomUserAgentKeys:    []string{"pc", "mobile"},
		UserAgentRatios:        map[string]int{"pc": 0, "mobile": 5},
	}
	for i := 0; i < 20; i++ {
		if got := sampleUserAgent(cfg); got != userAgentProfiles["mobile"] {
			t.Fatalf("UA = %q, want mobile only", got)
		}
	}
}

func TestSampleUserAgentReturnsEmptyWhenAllConfiguredWeightsAreZero(t *testing.T) {
	cfg := &execution.ExecutionConfig{
		RandomUserAgentEnabled: true,
		RandomUserAgentKeys:    []string{"pc", "mobile"},
		UserAgentRatios:        map[string]int{"pc": 0, "mobile": 0},
	}
	if got := sampleUserAgent(cfg); got != "" {
		t.Fatalf("UA = %q, want empty", got)
	}
}
