package questions

import (
	"math"
	"math/rand"

	"github.com/SurveyController/SurveyCore/internal/execution"
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
		weight := math.Max(0, probs[i])
		if weight <= 0 {
			continue
		}
		cumulative += weight
		if r < cumulative {
			return i
		}
	}
	return optionCount - 1
}

// ChooseTextCandidate selects one configured text candidate by weights.
func ChooseTextCandidate(texts []string, probs []float64) string {
	if len(texts) == 0 {
		return ""
	}
	if len(probs) == len(texts) {
		return texts[WeightedIndex(probs, len(texts))]
	}
	return texts[rand.Intn(len(texts))]
}

// ChooseConfiguredTextCandidate selects one text candidate from an ExecutionConfig.
func ChooseConfiguredTextCandidate(cfg *execution.ExecutionConfig, configIdx int) (string, bool) {
	if cfg == nil || configIdx < 0 || configIdx >= len(cfg.Texts) || len(cfg.Texts[configIdx]) == 0 {
		return "", false
	}
	var probs []float64
	if configIdx < len(cfg.TextsProb) {
		probs = cfg.TextsProb[configIdx]
	}
	return ChooseTextCandidate(cfg.Texts[configIdx], probs), true
}

// WeightedSampleWithoutReplacement selects numSelect indices without replacement.
func WeightedSampleWithoutReplacement(probs []float64, optionCount, numSelect int) []int {
	if optionCount <= 0 || numSelect <= 0 {
		return nil
	}
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
		if allNonPositive(remaining) {
			for i := 0; i < optionCount && len(selected) < numSelect; i++ {
				alreadySelected := false
				for _, idx := range selected {
					if idx == i {
						alreadySelected = true
						break
					}
				}
				if !alreadySelected {
					selected = append(selected, i)
				}
			}
			break
		}
		idx := WeightedIndex(remaining, optionCount)
		if remaining[idx] <= 0 {
			continue
		}
		selected = append(selected, idx)
		remaining[idx] = 0
	}
	return selected
}

func allNonPositive(values []float64) bool {
	for _, value := range values {
		if value > 0 {
			return false
		}
	}
	return true
}
