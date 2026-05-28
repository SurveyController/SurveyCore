package wjx

import (
	"testing"

	"github.com/SurveyController/SurveyConsole/internal/models"
)

func TestBuildSingleActionUsesWjxTypeCodeMapping(t *testing.T) {
	textCfgIdx := "0"
	singleCfgIdx := "1"
	sliderCfgIdx := "2"
	orderCfgIdx := "3"
	cfg := &models.ExecutionConfig{
		Texts:         [][]string{{"hello"}, nil, nil, nil},
		SingleProb:    []any{nil, []float64{0, 1}, nil, nil},
		SliderTargets: []float64{0, 0, 42, 0},
		QuestionsMetadata: map[int]models.SurveyQuestionMeta{
			1: {Num: 1, TypeCode: "1", IsTextLike: true},
			2: {Num: 2, TypeCode: "3", Options: 2, OptionTexts: []string{"A", "B"}},
			3: {Num: 3, TypeCode: "8"},
			4: {Num: 4, TypeCode: "11", Options: 3},
		},
		QuestionConfigIndexMap: map[int]string{
			1: textCfgIdx,
			2: singleCfgIdx,
			3: sliderCfgIdx,
			4: orderCfgIdx,
		},
	}
	state := models.NewExecutionState()

	actions, err := buildAnswerActions(cfg, state, "")
	if err != nil {
		t.Fatalf("buildAnswerActions returned error: %v", err)
	}
	if len(actions) != 4 {
		t.Fatalf("actions length = %d, want 4: %#v", len(actions), actions)
	}
	if actions[0].Kind != "text" || len(actions[0].TextValues) != 1 || actions[0].TextValues[0] != "hello" {
		t.Fatalf("type 1 action = %#v, want text hello", actions[0])
	}
	if actions[1].Kind != "choice" || len(actions[1].SelectedIndices) != 1 || actions[1].SelectedIndices[0] != 1 {
		t.Fatalf("type 3 action = %#v, want single choice index 1", actions[1])
	}
	if actions[2].Kind != "slider" || actions[2].SliderValue == nil || *actions[2].SliderValue != 42 {
		t.Fatalf("type 8 action = %#v, want slider 42", actions[2])
	}
	if actions[3].Kind != "order" || len(actions[3].SelectedIndices) != 3 {
		t.Fatalf("type 11 action = %#v, want order of 3 options", actions[3])
	}
}

func TestBuildSubmitDataFormatsCommonActions(t *testing.T) {
	sliderValue := 66.5
	got := buildSubmitData([]AnswerAction{
		{QuestionNum: 1, Kind: "choice", SelectedIndices: []int{0}},
		{QuestionNum: 2, Kind: "choice", SelectedIndices: []int{0, 2}},
		{QuestionNum: 3, Kind: "text", TextValues: []string{"甲", "乙"}},
		{QuestionNum: 4, Kind: "matrix", MatrixIndices: []int{1, 2}},
		{QuestionNum: 5, Kind: "slider", SliderValue: &sliderValue},
	}, nil)

	want := "1$1}2$1|3}3$甲^乙}4$1!2,2!3}5$66.5"
	if got != want {
		t.Fatalf("submitdata = %q, want %q", got, want)
	}
}

func TestBuildSubmitDataWithSkippedUsesFrontendPlaceholders(t *testing.T) {
	cfg := &models.ExecutionConfig{
		QuestionsMetadata: map[int]models.SurveyQuestionMeta{
			1: {Num: 1, Title: "单选", TypeCode: "3", Options: 2},
			2: {Num: 2, Title: "排序", TypeCode: "11", Options: 3},
			3: {Num: 3, Title: "量表", TypeCode: "5", Options: 2},
			4: {Num: 4, Title: "填空", TypeCode: "1", Options: 1},
			5: {Num: 5, Title: "下拉", TypeCode: "7", Options: 2},
		},
	}

	got := buildSubmitDataWithSkipped([]AnswerAction{
		{QuestionNum: 1, Kind: "choice", SelectedIndices: []int{1}},
	}, cfg, []int{2, 3, 4, 5})

	want := "1$2}2$-3,-3,-3}3$-3}4$(跳过)}5$-3"
	if got != want {
		t.Fatalf("submitdata = %q, want %q", got, want)
	}
}

func TestBuildAnswerPlanAppliesDisplayConditions(t *testing.T) {
	cfg := &models.ExecutionConfig{
		SingleProb: []any{[]float64{1, 0}, nil},
		Texts:      [][]string{nil, []string{"should skip"}},
		QuestionConfigIndexMap: map[int]string{
			1: "0",
			2: "1",
		},
		QuestionsMetadata: map[int]models.SurveyQuestionMeta{
			1: {Num: 1, Title: "单选", TypeCode: "3", Options: 2},
			2: {
				Num:                 2,
				Title:               "条件填空",
				TypeCode:            "1",
				IsTextLike:          true,
				TextInputCount:      1,
				HasDisplayCondition: true,
				DisplayConditions: []map[string]any{
					{
						"condition_question_num":   1,
						"condition_mode":           "selected",
						"condition_option_indices": []any{float64(1)},
					},
				},
			},
		},
	}

	plan, err := buildAnswerPlan(cfg, models.NewExecutionState(), "")
	if err != nil {
		t.Fatalf("buildAnswerPlan returned error: %v", err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].QuestionNum != 1 {
		t.Fatalf("actions = %#v, want only question 1", plan.Actions)
	}
	if len(plan.SkippedNums) != 1 || plan.SkippedNums[0] != 2 {
		t.Fatalf("skipped = %#v, want question 2", plan.SkippedNums)
	}
}

func TestBuildAnswerPlanAppliesForwardJumpRules(t *testing.T) {
	cfg := &models.ExecutionConfig{
		SingleProb: []any{[]float64{1, 0}, nil, nil, nil},
		Texts:      [][]string{nil, []string{"skip 2"}, []string{"skip 3"}, []string{"answer 4"}},
		QuestionConfigIndexMap: map[int]string{
			1: "0",
			2: "1",
			3: "2",
			4: "3",
		},
		QuestionsMetadata: map[int]models.SurveyQuestionMeta{
			1: {
				Num:      1,
				Title:    "跳题",
				TypeCode: "3",
				Options:  2,
				HasJump:  true,
				JumpRules: []map[string]any{
					{"option_index": 0, "jumpto": 4},
				},
			},
			2: {Num: 2, Title: "填空2", TypeCode: "1", IsTextLike: true, TextInputCount: 1},
			3: {Num: 3, Title: "填空3", TypeCode: "1", IsTextLike: true, TextInputCount: 1},
			4: {Num: 4, Title: "填空4", TypeCode: "1", IsTextLike: true, TextInputCount: 1},
		},
	}

	plan, err := buildAnswerPlan(cfg, models.NewExecutionState(), "")
	if err != nil {
		t.Fatalf("buildAnswerPlan returned error: %v", err)
	}
	if len(plan.Actions) != 2 || plan.Actions[0].QuestionNum != 1 || plan.Actions[1].QuestionNum != 4 {
		t.Fatalf("actions = %#v, want questions 1 and 4", plan.Actions)
	}
	if len(plan.SkippedNums) != 2 || plan.SkippedNums[0] != 2 || plan.SkippedNums[1] != 3 {
		t.Fatalf("skipped = %#v, want questions 2 and 3", plan.SkippedNums)
	}
}

func TestSampleKtimesAllowsFixedConfiguredRange(t *testing.T) {
	cfg := &models.ExecutionConfig{AnswerDurationRangeSeconds: [2]int{12, 12}}
	if got := sampleKtimes(cfg); got != 12 {
		t.Fatalf("sampleKtimes fixed range = %d, want 12", got)
	}
}

func TestNormalizeSurveyURLAddsScheme(t *testing.T) {
	got := normalizeSurveyURL("www.wjx.cn/vm/demo.aspx")
	if got != "https://www.wjx.cn/vm/demo.aspx" {
		t.Fatalf("normalizeSurveyURL without scheme = %q", got)
	}
	already := "http://www.wjx.cn/vm/demo.aspx"
	if got := normalizeSurveyURL(already); got != already {
		t.Fatalf("normalizeSurveyURL with scheme = %q, want %q", got, already)
	}
}
