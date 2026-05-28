package providerutil

import (
	"fmt"
	"math/rand"

	"github.com/SurveyController/SurveyConsole/internal/models"
)

// ParseConfigIndex parses a string config index, returning 0 on malformed input.
func ParseConfigIndex(s string) int {
	var idx int
	fmt.Sscanf(s, "%d", &idx)
	return idx
}

// ProviderConfigIndex resolves a config entry by full provider key before bare provider question id.
func ProviderConfigIndex(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta) (string, bool) {
	if cfg == nil || meta.ProviderQuestionID == "" {
		return "", false
	}
	if key := models.MakeProviderQuestionKey(meta.Provider, meta.ProviderPageID, meta.ProviderQuestionID); key != "" {
		if idx, ok := cfg.ProviderQuestionConfigIndexMap[key]; ok {
			return idx, true
		}
	}
	idx, ok := cfg.ProviderQuestionConfigIndexMap[meta.ProviderQuestionID]
	return idx, ok
}

// AllZero reports whether every probability is zero.
func AllZero(probs []float64) bool {
	for _, p := range probs {
		if p != 0 {
			return false
		}
	}
	return true
}

// Float64Slice converts common JSON-decoded numeric slices to []float64.
func Float64Slice(v any) ([]float64, bool) {
	if v == nil {
		return nil, false
	}
	switch sl := v.(type) {
	case []float64:
		return sl, true
	case []any:
		result := make([]float64, 0, len(sl))
		for _, item := range sl {
			if f, ok := float64Value(item); ok {
				result = append(result, f)
			}
		}
		return result, true
	}
	return nil, false
}

func float64Value(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// MatrixRowProbabilities returns the probability vector for one matrix row.
func MatrixRowProbabilities(raw any, rowIndex, optionCount int) []float64 {
	probs := make([]float64, optionCount)
	switch sl := raw.(type) {
	case [][]float64:
		if rowIndex < len(sl) {
			copy(probs, sl[rowIndex])
		}
	case []any:
		if rowIndex < len(sl) {
			if row, ok := Float64Slice(sl[rowIndex]); ok {
				copy(probs, row)
			}
			break
		}
		if row, ok := Float64Slice(sl); ok {
			copy(probs, row)
		}
	default:
		if row, ok := Float64Slice(raw); ok {
			copy(probs, row)
		}
	}
	return probs
}

// SampleAnswerDurationSeconds samples a configured duration or falls back to an inclusive range.
func SampleAnswerDurationSeconds(cfg *models.ExecutionConfig, fallbackMin, fallbackMax int) int {
	if cfg != nil {
		min := cfg.AnswerDurationRangeSeconds[0]
		max := cfg.AnswerDurationRangeSeconds[1]
		if min > 0 || max > 0 {
			if min < 0 {
				min = 0
			}
			if max < min {
				max = min
			}
			if max == min {
				return max
			}
			return min + rand.Intn(max-min+1)
		}
	}
	if fallbackMax < fallbackMin {
		fallbackMax = fallbackMin
	}
	if fallbackMax == fallbackMin {
		return fallbackMax
	}
	return fallbackMin + rand.Intn(fallbackMax-fallbackMin+1)
}
