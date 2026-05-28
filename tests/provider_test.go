package tests

import (
	"testing"

	"github.com/SurveyController/SurveyController-Go/internal/providers"
)

func TestDetectSurveyProvider(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://www.wjx.cn/vm/xxxxx.aspx", "wjx"},
		{"https://ks.wjx.com/vm/xxxxx.aspx", "wjx"},
		{"https://wj.qq.com/s/xxxxx", "qq"},
		{"https://www.credamo.com/s/xxxxx", "credamo"},
		{"https://example.com/survey", "wjx"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := providers.DetectSurveyProvider(tt.url, providers.ProviderWJX)
			if got != tt.expected {
				t.Errorf("DetectSurveyProvider(%s) = %s, want %s", tt.url, got, tt.expected)
			}
		})
	}
}

func TestIsWJXSurveyURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"https://www.wjx.cn/vm/xxxxx.aspx", true},
		{"https://ks.wjx.com/vm/xxxxx.aspx", true},
		{"https://wj.qq.com/s/xxxxx", false},
		{"https://example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := providers.IsWJXSurveyURL(tt.url)
			if got != tt.expected {
				t.Errorf("IsWJXSurveyURL(%s) = %v, want %v", tt.url, got, tt.expected)
			}
		})
	}
}

func TestIsQQSurveyURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"https://wj.qq.com/s/xxxxx", true},
		{"https://www.wjx.cn/vm/xxxxx.aspx", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := providers.IsQQSurveyURL(tt.url)
			if got != tt.expected {
				t.Errorf("IsQQSurveyURL(%s) = %v, want %v", tt.url, got, tt.expected)
			}
		})
	}
}

func TestNormalizeSurveyProvider(t *testing.T) {
	tests := []struct {
		value    string
		def      string
		expected string
	}{
		{"wjx", "", "wjx"},
		{"WJX", "", "wjx"},
		{" qq ", "", "qq"},
		{"invalid", "", "wjx"},     // falls back to default
		{"invalid", "qq", "qq"},    // falls back to custom default
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got := providers.NormalizeSurveyProvider(tt.value, tt.def)
			if got != tt.expected {
				t.Errorf("NormalizeSurveyProvider(%q, %q) = %q, want %q", tt.value, tt.def, got, tt.expected)
			}
		})
	}
}

func TestRegistry(t *testing.T) {
	registry := providers.NewRegistry()

	// Should have WJX registered
	adapter, err := registry.Get("wjx")
	if err != nil {
		t.Fatalf("Get(wjx) failed: %v", err)
	}
	if adapter.ProviderName() != "wjx" {
		t.Errorf("ProviderName() = %s, want wjx", adapter.ProviderName())
	}

	// Should fail for unknown provider
	_, err = registry.Get("unknown")
	if err == nil {
		t.Error("Get(unknown) should return error")
	}

	// GetByURL should detect WJX
	adapter, err = registry.GetByURL("https://www.wjx.cn/vm/test.aspx")
	if err != nil {
		t.Fatalf("GetByURL failed: %v", err)
	}
	if adapter.ProviderName() != "wjx" {
		t.Errorf("ProviderName() = %s, want wjx", adapter.ProviderName())
	}
}
