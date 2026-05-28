package tests

import (
	"testing"

	"github.com/SurveyController/SurveyController-Go/internal/questions"
)

func TestWeightedIndex(t *testing.T) {
	probs := []float64{1.0, 0.0, 0.0, 0.0}
	// With 100% weight on index 0, should always return 0
	for i := 0; i < 100; i++ {
		idx := questions.WeightedIndex(probs, 4)
		if idx != 0 {
			t.Errorf("Expected 0, got %d", idx)
			break
		}
	}
}

func TestWeightedIndexUniform(t *testing.T) {
	probs := []float64{0.25, 0.25, 0.25, 0.25}
	counts := make([]int, 4)
	for i := 0; i < 1000; i++ {
		idx := questions.WeightedIndex(probs, 4)
		counts[idx]++
	}
	// Each should be roughly 250
	for i, c := range counts {
		if c < 100 || c > 400 {
			t.Errorf("Option %d: count %d outside expected range", i, c)
		}
	}
}

func TestWeightedSampleWithoutReplacement(t *testing.T) {
	probs := []float64{1.0, 1.0, 1.0, 1.0}
	selected := questions.WeightedSampleWithoutReplacement(probs, 4, 2)
	if len(selected) != 2 {
		t.Errorf("Expected 2, got %d", len(selected))
	}
	if selected[0] == selected[1] {
		t.Error("Should not select same index twice")
	}
}

func TestDistributionTracker(t *testing.T) {
	tracker := questions.NewDistributionTracker()

	// Record some choices
	tracker.RecordChoice(1, 0, 3, nil)
	tracker.RecordChoice(1, 0, 3, nil)
	tracker.RecordChoice(1, 1, 3, nil)

	total, counts := tracker.Snapshot(1, 3, nil)
	if total != 3 {
		t.Errorf("Total = %d, want 3", total)
	}
	if counts[0] != 2 {
		t.Errorf("counts[0] = %d, want 2", counts[0])
	}
	if counts[1] != 1 {
		t.Errorf("counts[1] = %d, want 1", counts[1])
	}
}

func TestConsistencyContext(t *testing.T) {
	rules := []questions.AnswerRule{
		{
			ConditionQuestionNum:   1,
			ConditionMode:          "selected",
			ConditionOptionIndices: []int{0},
			TargetQuestionNum:      2,
			ActionMode:             "must_not_select",
			TargetOptionIndices:    []int{1},
		},
	}

	ctx := questions.NewConsistencyContext(rules)

	// Record answer for question 1
	ctx.RecordAnswer(1, 0)

	// Apply consistency to question 2
	probs := []float64{0.25, 0.25, 0.25, 0.25}
	result := ctx.ApplySingleConsistency(probs, 2)

	if result[1] != 0 {
		t.Errorf("Option 1 should be zeroed out, got %f", result[1])
	}
	if result[0] == 0 {
		t.Error("Option 0 should not be zeroed out")
	}
}

func TestConsistencyNotTriggered(t *testing.T) {
	rules := []questions.AnswerRule{
		{
			ConditionQuestionNum:   1,
			ConditionMode:          "selected",
			ConditionOptionIndices: []int{0},
			TargetQuestionNum:      2,
			ActionMode:             "must_not_select",
			TargetOptionIndices:    []int{1},
		},
	}

	ctx := questions.NewConsistencyContext(rules)

	// Record different answer for question 1
	ctx.RecordAnswer(1, 2)

	probs := []float64{0.25, 0.25, 0.25, 0.25}
	result := ctx.ApplySingleConsistency(probs, 2)

	// Should not be modified
	for i, p := range result {
		if p != 0.25 {
			t.Errorf("probs[%d] = %f, want 0.25", i, p)
		}
	}
}

func TestMultipleConstraint(t *testing.T) {
	rules := []questions.AnswerRule{
		{
			ConditionQuestionNum:   1,
			ConditionMode:          "selected",
			ConditionOptionIndices: []int{0},
			TargetQuestionNum:      2,
			ActionMode:             "must_select",
			TargetOptionIndices:    []int{2},
		},
	}

	ctx := questions.NewConsistencyContext(rules)
	ctx.RecordAnswer(1, 0)

	mustSelect, mustNotSelect := ctx.GetMultipleConstraint(2, 4)
	if len(mustSelect) != 1 || mustSelect[0] != 2 {
		t.Errorf("mustSelect = %v, want [2]", mustSelect)
	}
	if len(mustNotSelect) != 0 {
		t.Errorf("mustNotSelect = %v, want []", mustNotSelect)
	}
}

func TestPsychometricPlan(t *testing.T) {
	items := []questions.PsychometricItem{
		{Kind: "single", QuestionIndex: 1, OptionCount: 5, Bias: "center"},
		{Kind: "single", QuestionIndex: 2, OptionCount: 5, Bias: "center"},
		{Kind: "single", QuestionIndex: 3, OptionCount: 5, Bias: "center"},
	}

	plan := questions.BuildPsychometricPlan(items, 0.85)
	if plan == nil {
		t.Fatal("Plan should not be nil")
	}

	// All items should have choices
	for _, item := range items {
		choice := plan.GetChoice(item.QuestionIndex, nil)
		if choice == nil {
			t.Errorf("No choice for question %d", item.QuestionIndex)
		}
		if *choice < 0 || *choice >= item.OptionCount {
			t.Errorf("Choice %d out of range for question %d", *choice, item.QuestionIndex)
		}
	}
}

func TestDimensionPlan(t *testing.T) {
	grouped := map[string][]questions.PsychometricItem{
		"dim1": {
			{Kind: "single", QuestionIndex: 1, OptionCount: 5, Bias: "center"},
			{Kind: "single", QuestionIndex: 2, OptionCount: 5, Bias: "center"},
		},
		"dim2": {
			{Kind: "single", QuestionIndex: 3, OptionCount: 5, Bias: "center"},
			{Kind: "single", QuestionIndex: 4, OptionCount: 5, Bias: "center"},
		},
	}

	plan := questions.BuildDimensionPsychometricPlan(grouped, 0.85)
	if plan == nil {
		t.Fatal("Plan should not be nil")
	}

	choice1 := plan.GetChoice(1, nil)
	choice3 := plan.GetChoice(3, nil)
	if choice1 == nil || choice3 == nil {
		t.Error("Should have choices for all dimensions")
	}
}

func TestNormalizeDroplistProbs(t *testing.T) {
	probs := []float64{0.5, 0.5}
	result := questions.NormalizeDroplistProbs(probs, 4)
	if len(result) != 4 {
		t.Errorf("Length = %d, want 4", len(result))
	}
	if result[0] != 0.5 || result[1] != 0.5 {
		t.Error("First two should be preserved")
	}
	if result[2] != 1.0 || result[3] != 1.0 {
		t.Error("Remaining should be 1.0")
	}
}
