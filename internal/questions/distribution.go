package questions

import (
	"fmt"
	"math"
	"sync"
)

// DistributionTracker tracks answer distribution statistics at runtime.
type DistributionTracker struct {
	mu    sync.RWMutex
	counts map[string][]int // key -> per-option counts
}

// NewDistributionTracker creates a new tracker.
func NewDistributionTracker() *DistributionTracker {
	return &DistributionTracker{
		counts: make(map[string][]int),
	}
}

// RecordChoice records a chosen option for distribution tracking.
func (d *DistributionTracker) RecordChoice(questionIndex, optionIndex, optionCount int, rowIndex *int) {
	key := distributionKey(questionIndex, rowIndex)
	d.mu.Lock()
	defer d.mu.Unlock()

	counts, ok := d.counts[key]
	if !ok {
		counts = make([]int, optionCount)
		d.counts[key] = counts
	}
	if optionIndex >= 0 && optionIndex < len(counts) {
		counts[optionIndex]++
	}
}

// Snapshot returns total count and per-option counts.
func (d *DistributionTracker) Snapshot(questionIndex, optionCount int, rowIndex *int) (int, []int) {
	key := distributionKey(questionIndex, rowIndex)
	d.mu.RLock()
	defer d.mu.RUnlock()

	counts, ok := d.counts[key]
	if !ok {
		return 0, make([]int, optionCount)
	}
	total := 0
	for _, c := range counts {
		total += c
	}
	return total, counts
}

func distributionKey(questionIndex int, rowIndex *int) string {
	if rowIndex != nil {
		return fmt.Sprintf("q:%d:%d", questionIndex, *rowIndex)
	}
	return fmt.Sprintf("q:%d", questionIndex)
}

// ResolveDistributionProbabilities adjusts probabilities based on observed vs target distribution.
func ResolveDistributionProbabilities(target []float64, optionCount int, tracker *DistributionTracker, questionIndex int, rowIndex *int) []float64 {
	if optionCount <= 0 {
		return target
	}

	// Normalize target
	total := 0.0
	for i := 0; i < optionCount && i < len(target); i++ {
		total += math.Max(0, target[i])
	}
	if total <= 0 {
		result := make([]float64, optionCount)
		for i := range result {
			result[i] = 1.0 / float64(optionCount)
		}
		return result
	}

	normalized := make([]float64, optionCount)
	for i := 0; i < optionCount && i < len(target); i++ {
		normalized[i] = math.Max(0, target[i]) / total
	}

	// Get runtime stats
	sampleCount, counts := tracker.Snapshot(questionIndex, optionCount, rowIndex)
	if sampleCount < 12 {
		// Not enough data for correction
		return normalized
	}

	// Apply correction (matching Python algorithm)
	const (
		warmup    = 12
		gain      = 4.2
		minFactor = 0.45
		maxFactor = 2.2
		gapLimit  = 0.42
	)

	sampleFactor := math.Min(1.0, float64(sampleCount)/float64(warmup))

	adjusted := make([]float64, optionCount)
	for i := 0; i < optionCount; i++ {
		targetRatio := normalized[i]
		actualRatio := float64(counts[i]) / float64(sampleCount)
		gap := targetRatio - actualRatio
		if gap > gapLimit {
			gap = gapLimit
		}
		if gap < -gapLimit {
			gap = -gapLimit
		}

		factor := math.Exp(gain * sampleFactor * gap)
		if factor < minFactor {
			factor = minFactor
		}
		if factor > maxFactor {
			factor = maxFactor
		}
		adjusted[i] = normalized[i] * factor
	}

	// Re-normalize
	total = 0
	for _, v := range adjusted {
		total += v
	}
	if total > 0 {
		for i := range adjusted {
			adjusted[i] /= total
		}
	}

	return adjusted
}
