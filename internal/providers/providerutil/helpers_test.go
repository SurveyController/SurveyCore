package providerutil

import (
	"testing"

	"github.com/SurveyController/SurveyConsole/internal/models"
)

func TestFloat64SliceAcceptsTypedAndJSONSlices(t *testing.T) {
	if got, ok := Float64Slice([]float64{1, 2}); !ok || len(got) != 2 || got[1] != 2 {
		t.Fatalf("Float64Slice typed = %#v/%v, want [1 2]/true", got, ok)
	}
	got, ok := Float64Slice([]any{float64(1.5), int(2), "skip"})
	if !ok || len(got) != 2 || got[0] != 1.5 || got[1] != 2 {
		t.Fatalf("Float64Slice JSON = %#v/%v, want [1.5 2]/true", got, ok)
	}
}

func TestMatrixRowProbabilitiesHandlesNestedAndFlatRows(t *testing.T) {
	nested := []any{
		[]any{float64(0.1), float64(0.9)},
		[]any{float64(0.8), float64(0.2)},
	}
	if got := MatrixRowProbabilities(nested, 1, 2); got[0] != 0.8 || got[1] != 0.2 {
		t.Fatalf("MatrixRowProbabilities nested = %#v, want second row", got)
	}
	if got := MatrixRowProbabilities([]any{float64(0.3), float64(0.7)}, 5, 2); got[0] != 0.3 || got[1] != 0.7 {
		t.Fatalf("MatrixRowProbabilities flat fallback = %#v, want flat row", got)
	}
}

func TestProviderConfigIndexPrefersFullKey(t *testing.T) {
	meta := models.SurveyQuestionMeta{
		Provider:           models.ProviderQQ,
		ProviderPageID:     "page-1",
		ProviderQuestionID: "q1",
	}
	fullKey := models.MakeProviderQuestionKey(models.ProviderQQ, "page-1", "q1")
	cfg := &models.ExecutionConfig{
		ProviderQuestionConfigIndexMap: map[string]string{
			"q1":    "1",
			fullKey: "2",
		},
	}
	if got, ok := ProviderConfigIndex(cfg, meta); !ok || got != "2" {
		t.Fatalf("ProviderConfigIndex full key = %q/%v, want 2/true", got, ok)
	}
	delete(cfg.ProviderQuestionConfigIndexMap, fullKey)
	if got, ok := ProviderConfigIndex(cfg, meta); !ok || got != "1" {
		t.Fatalf("ProviderConfigIndex bare id = %q/%v, want 1/true", got, ok)
	}
}

func TestSampleAnswerDurationSecondsUsesConfiguredRange(t *testing.T) {
	cfg := &models.ExecutionConfig{AnswerDurationRangeSeconds: [2]int{12, 12}}
	if got := SampleAnswerDurationSeconds(cfg, 1, 3); got != 12 {
		t.Fatalf("SampleAnswerDurationSeconds fixed = %d, want 12", got)
	}
	for i := 0; i < 10; i++ {
		got := SampleAnswerDurationSeconds(nil, 1, 3)
		if got < 1 || got > 3 {
			t.Fatalf("SampleAnswerDurationSeconds fallback = %d, want 1..3", got)
		}
	}
}
