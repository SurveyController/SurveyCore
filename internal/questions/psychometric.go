package questions

import (
	"fmt"
	"math"
	"math/rand"
)

// PsychometricItem describes one item in a psychometric plan.
type PsychometricItem struct {
	Kind            string    // "single", "matrix"
	QuestionIndex   int
	RowIndex        *int
	OptionCount     int
	Bias            string    // "left", "center", "right"
	IsReversed      bool
	ScoreByChoice   []float64 // score mapping for each choice
}

// PsychometricPlan holds pre-generated answers for a single dimension.
type PsychometricPlan struct {
	Items    []PsychometricItem
	Theta    float64
	SigmaE   float64
	Choices  map[string]int // key -> choice index
}

// GetChoice returns the pre-generated choice for a question.
func (p *PsychometricPlan) GetChoice(questionIndex int, rowIndex *int) *int {
	key := choiceKey(questionIndex, rowIndex)
	if choice, ok := p.Choices[key]; ok {
		return &choice
	}
	return nil
}

// DimensionPsychometricPlan holds plans per dimension.
type DimensionPsychometricPlan struct {
	Plans map[string]*PsychometricPlan
}

// GetChoice delegates to the correct dimension's plan.
func (d *DimensionPsychometricPlan) GetChoice(questionIndex int, rowIndex *int) *int {
	for _, plan := range d.Plans {
		if choice := plan.GetChoice(questionIndex, rowIndex); choice != nil {
			return choice
		}
	}
	return nil
}

// BuildPsychometricPlan creates a psychometric plan for a set of items.
func BuildPsychometricPlan(items []PsychometricItem, targetAlpha float64) *PsychometricPlan {
	if len(items) < 2 {
		return nil
	}
	if targetAlpha <= 0 {
		targetAlpha = 0.85
	}

	k := len(items)
	rho := computeRhoFromAlpha(targetAlpha, k)
	sigmaE := computeSigmaEFromRho(rho)
	theta := rand.NormFloat64()

	choices := make(map[string]int)
	for _, item := range items {
		score := generatePsychoAnswer(theta, item.OptionCount, item.Bias, sigmaE, item.IsReversed)
		// Apply score_by_choice mapping if available
		choice := mapScoreToChoice(score, item)
		key := choiceKey(item.QuestionIndex, item.RowIndex)
		choices[key] = choice
	}

	return &PsychometricPlan{
		Items:   items,
		Theta:   theta,
		SigmaE:  sigmaE,
		Choices: choices,
	}
}

// BuildDimensionPsychometricPlan creates per-dimension plans.
func BuildDimensionPsychometricPlan(groupedItems map[string][]PsychometricItem, targetAlpha float64) *DimensionPsychometricPlan {
	plans := make(map[string]*PsychometricPlan)
	for dimension, items := range groupedItems {
		if len(items) >= 2 {
			plan := BuildPsychometricPlan(items, targetAlpha)
			if plan != nil {
				plans[dimension] = plan
			}
		}
	}
	if len(plans) == 0 {
		return nil
	}
	return &DimensionPsychometricPlan{Plans: plans}
}

func computeRhoFromAlpha(alpha float64, k int) float64 {
	// rho = alpha / (k - alpha*(k-1))
	denom := float64(k) - alpha*(float64(k)-1)
	if denom <= 0 {
		return 0.5
	}
	return alpha / denom
}

func computeSigmaEFromRho(rho float64) float64 {
	if rho <= 0 {
		return 1.0
	}
	// sigma_e = sqrt(1/rho - 1)
	return math.Sqrt(1.0/rho - 1.0)
}

func generatePsychoAnswer(theta float64, optionCount int, bias string, sigmaE float64, isReversed bool) int {
	biasShift := 0.0
	switch bias {
	case "left":
		biasShift = -0.5
	case "right":
		biasShift = 0.5
	}

	effectiveTheta := theta
	if isReversed {
		effectiveTheta = -theta
	}

	z := effectiveTheta + biasShift + sigmaE*rand.NormFloat64()
	return zToCategory(z, optionCount)
}

// mapScoreToChoice maps a raw score to a choice index using ScoreByChoice if available.
func mapScoreToChoice(score int, item PsychometricItem) int {
	if len(item.ScoreByChoice) == 0 {
		return score
	}
	// ScoreByChoice maps choice index -> score value
	// Find the choice index whose score matches
	for choiceIdx, targetScore := range item.ScoreByChoice {
		if int(targetScore) == score {
			return choiceIdx
		}
	}
	// Fallback: find closest score
	bestIdx := 0
	bestDiff := 999
	for choiceIdx, targetScore := range item.ScoreByChoice {
		diff := abs(int(targetScore) - score)
		if diff < bestDiff {
			bestDiff = diff
			bestIdx = choiceIdx
		}
	}
	return bestIdx
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func zToCategory(z float64, optionCount int) int {
	if optionCount <= 1 {
		return 0
	}
	// Use inverse normal quantile thresholds (matching Python algorithm)
	// Divide [0,1] into optionCount equal bins, use inverse-normal thresholds
	m := float64(optionCount)
	phi := normalCDF(z)
	// Find which bin z falls into
	idx := int(math.Floor(phi * m))
	if idx >= optionCount {
		idx = optionCount - 1
	}
	if idx < 0 {
		idx = 0
	}
	return idx
}

// normalCDF computes the cumulative distribution function of the standard normal.
func normalCDF(x float64) float64 {
	return 0.5 * (1 + math.Erf(x/math.Sqrt2))
}

func choiceKey(questionIndex int, rowIndex *int) string {
	if rowIndex != nil {
		return fmt.Sprintf("q:%d:%d", questionIndex, *rowIndex)
	}
	return fmt.Sprintf("q:%d", questionIndex)
}
