package questions

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/SurveyController/SurveyCore/internal/execution"
	runstate "github.com/SurveyController/SurveyCore/internal/runtime"

	"github.com/SurveyController/SurveyCore/internal/models"
)

func TestRunContextAppliesConsistencyRules(t *testing.T) {
	cfg := &execution.ExecutionConfig{
		AnswerRules: []map[string]any{
			{
				"condition_question_num":   1,
				"condition_mode":           "selected",
				"condition_option_indices": []any{float64(0)},
				"target_question_num":      2,
				"action_mode":              "must_not_select",
				"target_option_indices":    []any{float64(1)},
			},
		},
	}
	state := runstate.NewExecutionState()
	runtime := NewRunContext(cfg, state)

	first := runtime.ChooseSingle(models.SurveyQuestionMeta{Num: 1, Options: 2}, 0, 2, []float64{1, 0}, nil)
	if first != 0 {
		t.Fatalf("first choice = %d, want 0", first)
	}
	second := runtime.ChooseSingle(models.SurveyQuestionMeta{Num: 2, Options: 2}, 1, 2, []float64{1, 1}, nil)
	if second == 1 {
		t.Fatalf("consistency rule should block option 1")
	}
}

func TestRunContextAppliesMultipleConstraints(t *testing.T) {
	cfg := &execution.ExecutionConfig{
		AnswerRules: []map[string]any{
			{
				"condition_question_num":   1,
				"condition_mode":           "selected",
				"condition_option_indices": []any{float64(0)},
				"target_question_num":      2,
				"action_mode":              "must_select",
				"target_option_indices":    []any{float64(2)},
			},
		},
	}
	runtime := NewRunContext(cfg, runstate.NewExecutionState())

	runtime.ChooseSingle(models.SurveyQuestionMeta{Num: 1, Options: 2}, 0, 2, []float64{1, 0}, nil)
	selected := runtime.ChooseMultiple(models.SurveyQuestionMeta{Num: 2, Options: 4}, 1, 4, 1, 2, []float64{1, 1, 0, 1})

	found := false
	for _, idx := range selected {
		if idx == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("must_select option 2 missing from %v", selected)
	}
}

func TestRunContextUsesServerAIForEnabledText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"AI答案"}}]}`))
	}))
	defer server.Close()

	cfg := &execution.ExecutionConfig{
		AIAPIKey:    "test-key",
		AIBaseURL:   server.URL,
		AIModel:     "test-model",
		TextAIFlags: []bool{true},
	}
	runtime := NewRunContext(cfg, runstate.NewExecutionState())

	got := runtime.GenerateText(models.SurveyQuestionMeta{Num: 1, Title: "评价"}, 0, "fallback", 1)
	if got != "AI答案" {
		t.Fatalf("AI text = %q, want AI answer", got)
	}
}

func TestAIClientRetriesTransientHTTPFailure(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "busy", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"补偿答案"}}]}`))
	}))
	defer server.Close()

	client := NewAIClient(AIConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
	})

	got, err := client.GenerateAnswer("评价", "1", 1)
	if err != nil {
		t.Fatalf("GenerateAnswer returned error: %v", err)
	}
	if got != "补偿答案" || attempts != 2 {
		t.Fatalf("answer=%q attempts=%d, want answer and one retry", got, attempts)
	}
}

func TestAIClientClassifiesConfigErrorWithoutRetry(t *testing.T) {
	client := NewAIClient(AIConfig{})

	_, err := client.GenerateAnswer("评价", "1", 1)
	var aiErr *AIError
	if !errors.As(err, &aiErr) || aiErr.Kind != AIErrorConfig {
		t.Fatalf("error = %#v, want config AIError", err)
	}
}

func TestRunContextGeneratesConfiguredRandomTextModes(t *testing.T) {
	cfg := &execution.ExecutionConfig{
		TextRandomModes:     []string{models.TextRandomMobile, models.TextRandomInteger, ""},
		TextRandomIntRanges: [][]int{nil, []int{12, 10}, nil},
	}
	runtime := NewRunContext(cfg, runstate.NewExecutionState())

	mobile := runtime.GenerateText(models.SurveyQuestionMeta{Num: 1, Title: "电话"}, 0, "fallback", 1)
	if !regexp.MustCompile(`^1\d{10}$`).MatchString(mobile) {
		t.Fatalf("mobile text = %q, want 11-digit mobile", mobile)
	}

	integer := runtime.GenerateText(models.SurveyQuestionMeta{Num: 2, Title: "数量"}, 1, "fallback", 1)
	value, err := strconv.Atoi(integer)
	if err != nil || value < 10 || value > 12 {
		t.Fatalf("integer text = %q, want 10..12", integer)
	}

	name := runtime.GenerateText(models.SurveyQuestionMeta{Num: 3, Title: "姓名"}, 2, models.TextRandomNameToken, 1)
	if name == models.TextRandomNameToken || strings.TrimSpace(name) == "" {
		t.Fatalf("name token resolved to %q", name)
	}
}

func TestRunContextGeneratesConfiguredMultiTextAndLocation(t *testing.T) {
	cfg := &execution.ExecutionConfig{
		MultiTextBlankModes:     [][]string{{models.TextRandomNone, models.TextRandomMobile, models.TextRandomInteger}},
		MultiTextBlankIntRanges: [][][]int{{nil, nil, []int{5, 5}}},
		LocationParts:           map[int][]string{9: []string{"上海", "浦东新区"}},
	}
	runtime := NewRunContext(cfg, runstate.NewExecutionState())

	got := runtime.GenerateText(models.SurveyQuestionMeta{Num: 1, Title: "多项填空"}, 0, "原值", 3)
	parts := strings.Split(got, "|")
	if len(parts) != 3 {
		t.Fatalf("multi-text parts = %#v, want 3 parts", parts)
	}
	if parts[0] != "原值" {
		t.Fatalf("first blank = %q, want fallback", parts[0])
	}
	if !regexp.MustCompile(`^1\d{10}$`).MatchString(parts[1]) {
		t.Fatalf("second blank = %q, want mobile", parts[1])
	}
	if parts[2] != "5" {
		t.Fatalf("third blank = %q, want random integer 5", parts[2])
	}

	location := runtime.GenerateText(models.SurveyQuestionMeta{Num: 9, IsLocation: true}, 0, "fallback", 1)
	if location != "上海 浦东新区" {
		t.Fatalf("location = %q, want joined location text", location)
	}
	locationParts := runtime.GenerateText(models.SurveyQuestionMeta{Num: 9, IsLocation: true}, 0, "fallback", 2)
	if locationParts != "上海|浦东新区" {
		t.Fatalf("location parts = %q, want pipe-delimited parts", locationParts)
	}
}

func TestRunContextUsesReverseFillSampleForThread(t *testing.T) {
	choice := 1
	cfg := &execution.ExecutionConfig{
		TargetNum: 1,
		ReverseFillSpec: &models.ReverseFillSpec{
			Samples: []models.ReverseFillSampleRow{
				{
					DataRowNumber: 1,
					Answers: map[int]models.ReverseFillAnswer{
						1: {QuestionNum: 1, Kind: models.ReverseFillKindChoice, ChoiceIndex: &choice},
						2: {QuestionNum: 2, Kind: models.ReverseFillKindText, TextValue: "source text"},
						3: {QuestionNum: 3, Kind: models.ReverseFillKindMatrix, MatrixChoiceIndexes: []int{2, 0}},
					},
				},
			},
		},
	}
	state := runstate.NewExecutionState()
	state.Config = cfg
	state.InitializeReverseFillRuntime()
	if acquired := state.AcquireReverseFillSample("Worker-1"); acquired.Status != "acquired" {
		t.Fatalf("AcquireReverseFillSample status = %s, want acquired", acquired.Status)
	}
	runtime := NewRunContextForThread(cfg, state, "Worker-1")

	gotChoice := runtime.ChooseSingle(models.SurveyQuestionMeta{Num: 1, Options: 2}, 0, 2, []float64{1, 0}, nil)
	if gotChoice != 1 {
		t.Fatalf("reverse-fill choice = %d, want 1", gotChoice)
	}
	gotText := runtime.GenerateText(models.SurveyQuestionMeta{Num: 2, Options: 0}, 1, "fallback", 1)
	if gotText != "source text" {
		t.Fatalf("reverse-fill text = %q, want source text", gotText)
	}
	rowIndex := 0
	gotMatrix := runtime.ChooseSingle(models.SurveyQuestionMeta{Num: 3, Options: 3, Rows: 2}, 2, 3, []float64{1, 1, 0}, &rowIndex)
	if gotMatrix != 2 {
		t.Fatalf("reverse-fill matrix row = %d, want 2", gotMatrix)
	}
}

func TestBuildPsychometricPlanFromConfigUsesBiasAndOrdinalScores(t *testing.T) {
	dim := "体验"
	cfg := &execution.ExecutionConfig{
		PsychoTargetAlpha: 0.85,
		QuestionDimensionMap: map[int]*string{
			1: &dim,
			2: &dim,
		},
		QuestionOrdinalScoreMap: map[int][]int{
			1: []int{2, 1, 0},
		},
		QuestionPsychoBiasMap: map[int]string{
			1: "left",
			2: "right",
		},
		QuestionsMetadata: map[int]models.SurveyQuestionMeta{
			1: {Num: 1, TypeCode: "3", Options: 3},
			2: {Num: 2, TypeCode: "5", Options: 3},
		},
	}

	plan := buildPsychometricPlanFromConfig(cfg)

	if plan == nil || plan.Plans[dim] == nil {
		t.Fatalf("psychometric plan = %#v, want dimension plan", plan)
	}
	items := plan.Plans[dim].Items
	if len(items) != 2 {
		t.Fatalf("items length = %d, want 2", len(items))
	}
	if items[0].Bias != "left" || items[1].Bias != "right" {
		t.Fatalf("biases = %q/%q, want left/right", items[0].Bias, items[1].Bias)
	}
	if got := items[0].ScoreByChoice; len(got) != 3 || got[0] != 2 || got[2] != 0 {
		t.Fatalf("ordinal score map = %#v, want [2 1 0]", got)
	}
}

func TestPsychometricOrientationMarksOppositeTargetAsReversed(t *testing.T) {
	items := []PsychometricItem{
		{QuestionIndex: 1, OptionCount: 5, Bias: "center", TargetProb: []float64{0.05, 0.05, 0.1, 0.3, 0.5}},
		{QuestionIndex: 2, OptionCount: 5, Bias: "center", TargetProb: []float64{0.05, 0.05, 0.1, 0.3, 0.5}},
		{QuestionIndex: 3, OptionCount: 5, Bias: "center", TargetProb: []float64{0.5, 0.3, 0.1, 0.05, 0.05}},
	}

	orientation := inferDimensionOrientation(items)

	if !orientation.ReversedKeys["q:3"] {
		t.Fatalf("reversed keys = %#v, want q:3 marked reversed", orientation.ReversedKeys)
	}
	if orientation.ReversedKeys["q:1"] || orientation.ReversedKeys["q:2"] {
		t.Fatalf("reversed keys = %#v, q:1/q:2 should not be reversed", orientation.ReversedKeys)
	}
}

func TestWeightedSampleWithoutReplacementFallsBackWithoutDuplicates(t *testing.T) {
	selected := WeightedSampleWithoutReplacement([]float64{1, 0, 0}, 3, 2)
	if len(selected) != 2 {
		t.Fatalf("selected length = %d, want 2", len(selected))
	}
	if selected[0] == selected[1] {
		t.Fatalf("duplicate selection: %v", selected)
	}
}

func TestWeightedIndexSkipsZeroWeightChoices(t *testing.T) {
	for i := 0; i < 100; i++ {
		if got := WeightedIndex([]float64{0, 1}, 2); got != 1 {
			t.Fatalf("WeightedIndex returned %d, want 1", got)
		}
	}
}

func TestChooseTextCandidateUsesConfiguredProbabilities(t *testing.T) {
	for i := 0; i < 100; i++ {
		got := ChooseTextCandidate([]string{"A", "B"}, []float64{0, 1})
		if got != "B" {
			t.Fatalf("ChooseTextCandidate = %q, want B", got)
		}
	}
}
