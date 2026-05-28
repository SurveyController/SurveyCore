package questions

import (
	"math"
	"math/rand"
)

// WeightedIndex selects an index based on probability weights.
func WeightedIndex(probs []float64, optionCount int) int {
	if optionCount <= 0 {
		return 0
	}
	total := 0.0
	for i := 0; i < optionCount && i < len(probs); i++ {
		total += math.Max(0, probs[i])
	}
	if total <= 0 {
		return rand.Intn(optionCount)
	}
	r := rand.Float64() * total
	cumulative := 0.0
	for i := 0; i < optionCount && i < len(probs); i++ {
		cumulative += math.Max(0, probs[i])
		if r <= cumulative {
			return i
		}
	}
	return optionCount - 1
}

// WeightedSampleWithoutReplacement selects numSelect indices without replacement.
func WeightedSampleWithoutReplacement(probs []float64, optionCount, numSelect int) []int {
	if numSelect >= optionCount {
		result := make([]int, optionCount)
		for i := range result {
			result[i] = i
		}
		return result
	}

	selected := make([]int, 0, numSelect)
	remaining := make([]float64, optionCount)
	copy(remaining, probs)

	for len(selected) < numSelect {
		idx := WeightedIndex(remaining, optionCount)
		selected = append(selected, idx)
		remaining[idx] = 0
	}
	return selected
}

// NormalizeDroplistProbs normalizes droplist probabilities.
func NormalizeDroplistProbs(probs []float64, optionCount int) []float64 {
	if len(probs) >= optionCount {
		return probs[:optionCount]
	}
	result := make([]float64, optionCount)
	copy(result, probs)
	for i := len(probs); i < optionCount; i++ {
		result[i] = 1.0
	}
	return result
}

// ToFloat64Slice converts various types to []float64.
func ToFloat64Slice(v any) ([]float64, bool) {
	if v == nil {
		return nil, false
	}
	switch sl := v.(type) {
	case []float64:
		return sl, true
	case []any:
		result := make([]float64, 0, len(sl))
		for _, item := range sl {
			if f, ok := item.(float64); ok {
				result = append(result, f)
			}
		}
		return result, true
	}
	return nil, false
}
