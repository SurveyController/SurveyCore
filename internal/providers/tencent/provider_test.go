package tencent

import (
	"reflect"
	"testing"

	"github.com/SurveyController/SurveyConsole/internal/models"
	"github.com/SurveyController/SurveyConsole/internal/providers/providerutil"
)

func TestBuildSubmitBodyCoversChoiceTextAndMatrix(t *testing.T) {
	rawQuestions := []map[string]any{
		{
			"id":      "q1",
			"type":    "radio",
			"page_id": "p1",
			"options": []any{
				map[string]any{"id": "o1", "text": "A"},
				map[string]any{"id": "o2", "text": "B"},
			},
		},
		{
			"id":      "q2",
			"type":    "text",
			"page_id": "p1",
		},
		{
			"id":      "q3",
			"type":    "matrix_radio",
			"page_id": "p2",
			"options": []any{
				map[string]any{"id": "m1", "text": "差"},
				map[string]any{"id": "m2", "text": "好"},
			},
			"sub_titles": []any{
				map[string]any{"id": "r1", "text": "外观"},
			},
		},
	}
	actions := []TencentAnswerAction{
		{QuestionID: "q1", QuestionType: "radio", SelectedIDs: []string{"o2"}},
		{QuestionID: "q2", QuestionType: "text", TextValue: "hello"},
		{QuestionID: "q3", QuestionType: "matrix_radio", MatrixAnswers: []TencentMatrixRow{{RowID: "r1", OptionIDs: []string{"m1"}}}},
	}

	body := buildSubmitBody("123", "hash", rawQuestions, actions, 33, "UA")
	answerSurvey := body["answer_survey"].(map[string]any)
	if answerSurvey["duration"] != 33 {
		t.Fatalf("duration = %v, want 33", answerSurvey["duration"])
	}
	pages := answerSurvey["pages"].([]map[string]any)
	if len(pages) != 2 {
		t.Fatalf("pages length = %d, want 2", len(pages))
	}

	firstPageQuestions := pages[0]["questions"].([]map[string]any)
	choiceOptions := firstPageQuestions[0]["options"].([]map[string]any)
	if choiceOptions[1]["checked"] != 1 {
		t.Fatalf("choice options = %#v, want second option checked", choiceOptions)
	}
	if firstPageQuestions[1]["text"] != "hello" {
		t.Fatalf("text answer = %#v, want hello", firstPageQuestions[1])
	}

	secondPageQuestions := pages[1]["questions"].([]map[string]any)
	rows := secondPageQuestions[0]["sub_titles"].([]map[string]any)
	rowOptions := rows[0]["options"].([]map[string]any)
	if rowOptions[0]["id"] != "m1" || rowOptions[0]["checked"] != 1 || rowOptions[1]["checked"] != 0 {
		t.Fatalf("matrix row options = %#v, want top-level option ids with first checked", rowOptions)
	}
}

func TestSampleAnswerDurationSecondsAllowsFixedConfiguredRange(t *testing.T) {
	cfg := &models.ExecutionConfig{AnswerDurationRangeSeconds: [2]int{45, 45}}
	if got := providerutil.SampleAnswerDurationSeconds(cfg, 60, 60); got != 45 {
		t.Fatalf("sampleAnswerDurationSeconds fixed range = %d, want 45", got)
	}
}

func TestProviderConfigIndexPrefersFullKeyAndFallsBackToBareID(t *testing.T) {
	meta := models.SurveyQuestionMeta{
		Provider:           ProviderName,
		ProviderPageID:     "page-1",
		ProviderQuestionID: "q1",
	}
	fullKey := models.MakeProviderQuestionKey(ProviderName, "page-1", "q1")
	cfg := &models.ExecutionConfig{
		ProviderQuestionConfigIndexMap: map[string]string{
			"q1":    "1",
			fullKey: "2",
		},
	}

	if got, ok := providerutil.ProviderConfigIndex(cfg, meta); !ok || got != "2" {
		t.Fatalf("providerConfigIndex full key = %q/%v, want 2/true", got, ok)
	}
	delete(cfg.ProviderQuestionConfigIndexMap, fullKey)
	if got, ok := providerutil.ProviderConfigIndex(cfg, meta); !ok || got != "1" {
		t.Fatalf("providerConfigIndex bare id = %q/%v, want 1/true", got, ok)
	}
}

func TestStandardizeQuestionsExtractsTencentDisplayAndJumpLogic(t *testing.T) {
	rawQuestions := []map[string]any{
		{
			"id":      "q-1",
			"type":    "radio",
			"title":   "来源题",
			"page_id": "p-1",
			"page":    1,
			"options": []any{
				map[string]any{"id": "o-1", "text": "显示后续题", "display": map[string]any{"targets": []any{"q-2", "q-3"}}},
				map[string]any{"id": "o-2", "text": "跳到第二页", "goto": map[string]any{"page": "p-2"}},
			},
		},
		{"id": "q-2", "type": "text", "title": "条件题一", "page_id": "p-1", "page": 1, "hidden": true, "refer": map[string]any{"source": "q-1"}},
		{"id": "q-3", "type": "text", "title": "条件题二", "page_id": "p-1", "page": 1, "hidden": true, "refer": "q-1"},
		{"id": "q-4", "type": "text", "title": "第二页首题", "page_id": "p-2", "page": 2},
	}

	questions := standardizeQuestions(rawQuestions)
	if len(questions) != 4 {
		t.Fatalf("questions length = %d, want 4", len(questions))
	}
	source := questions[0]
	if !source.HasDependentDisplayLogic {
		t.Fatalf("source HasDependentDisplayLogic = false, want true")
	}
	wantControls := []map[string]any{
		{"target_question_num": 2, "condition_option_indices": []int{0}, "condition_mode": "selected"},
		{"target_question_num": 3, "condition_option_indices": []int{0}, "condition_mode": "selected"},
	}
	if !reflect.DeepEqual(source.ControlsDisplayTargets, wantControls) {
		t.Fatalf("controls = %#v, want %#v", source.ControlsDisplayTargets, wantControls)
	}
	wantJump := []map[string]any{
		{"option_index": 1, "jumpto": 4, "option_text": "跳到第二页"},
	}
	if !source.HasJump || !reflect.DeepEqual(source.JumpRules, wantJump) {
		t.Fatalf("jump = %v/%#v, want %#v", source.HasJump, source.JumpRules, wantJump)
	}
	if source.LogicParseStatus != models.LogicParseStatusComplete {
		t.Fatalf("source logic status = %q, want complete", source.LogicParseStatus)
	}

	for _, target := range questions[1:3] {
		if !target.HasDisplayCondition {
			t.Fatalf("question %d HasDisplayCondition = false, want true", target.Num)
		}
		wantConditions := []map[string]any{
			{"condition_question_num": 1, "condition_mode": "selected", "condition_option_indices": []int{0}},
		}
		if !reflect.DeepEqual(target.DisplayConditions, wantConditions) {
			t.Fatalf("question %d conditions = %#v, want %#v", target.Num, target.DisplayConditions, wantConditions)
		}
		if target.LogicParseStatus != models.LogicParseStatusComplete {
			t.Fatalf("question %d logic status = %q, want complete", target.Num, target.LogicParseStatus)
		}
	}
}

func TestBuildAnswerActionsAppliesTencentDisplayConditions(t *testing.T) {
	cfg := &models.ExecutionConfig{
		SingleProb: []any{[]float64{0, 1}, nil, nil},
		QuestionConfigIndexMap: map[int]string{
			1: "0",
			2: "1",
			3: "2",
		},
		QuestionsMetadata: map[int]models.SurveyQuestionMeta{
			1: {Num: 1, TypeCode: "3", Options: 2, ProviderQuestionID: "q-1", ProviderType: "radio", Page: 1},
			2: {
				Num:                 2,
				TypeCode:            "1",
				ProviderQuestionID:  "q-2",
				ProviderType:        "text",
				IsTextLike:          true,
				HasDisplayCondition: true,
				DisplayConditions: []map[string]any{
					{"condition_question_num": 1, "condition_mode": "selected", "condition_option_indices": []int{0}},
				},
				Page: 1,
			},
			3: {Num: 3, TypeCode: "1", ProviderQuestionID: "q-3", ProviderType: "text", IsTextLike: true, Page: 1},
		},
	}
	rawQuestions := []map[string]any{
		{"id": "q-1", "type": "radio", "page_id": "p-1", "options": []any{map[string]any{"id": "o-1"}, map[string]any{"id": "o-2"}}},
		{"id": "q-2", "type": "text", "page_id": "p-1"},
		{"id": "q-3", "type": "text", "page_id": "p-1"},
	}

	actions := buildAnswerActions(cfg, models.NewExecutionState(), rawQuestions, "")
	if len(actions) != 2 {
		t.Fatalf("actions length = %d, want 2: %#v", len(actions), actions)
	}
	if actions[0].QuestionID != "q-1" || actions[1].QuestionID != "q-3" {
		t.Fatalf("actions = %#v, want q-1 and q-3", actions)
	}
}

func TestBuildAnswerActionsAppliesTencentForwardJumpRules(t *testing.T) {
	cfg := &models.ExecutionConfig{
		SingleProb: []any{[]float64{1, 0}, nil, nil},
		QuestionConfigIndexMap: map[int]string{
			1: "0",
			2: "1",
			3: "2",
		},
		QuestionsMetadata: map[int]models.SurveyQuestionMeta{
			1: {
				Num:                1,
				TypeCode:           "3",
				Options:            2,
				ProviderQuestionID: "q-1",
				ProviderType:       "radio",
				HasJump:            true,
				JumpRules:          []map[string]any{{"option_index": 0, "jumpto": 3}},
				Page:               1,
			},
			2: {Num: 2, TypeCode: "1", ProviderQuestionID: "q-2", ProviderType: "text", IsTextLike: true, Page: 1},
			3: {Num: 3, TypeCode: "1", ProviderQuestionID: "q-3", ProviderType: "text", IsTextLike: true, Page: 1},
		},
	}
	rawQuestions := []map[string]any{
		{"id": "q-1", "type": "radio", "page_id": "p-1", "options": []any{map[string]any{"id": "o-1"}, map[string]any{"id": "o-2"}}},
		{"id": "q-2", "type": "text", "page_id": "p-1"},
		{"id": "q-3", "type": "text", "page_id": "p-1"},
	}

	actions := buildAnswerActions(cfg, models.NewExecutionState(), rawQuestions, "")
	if len(actions) != 2 {
		t.Fatalf("actions length = %d, want 2: %#v", len(actions), actions)
	}
	if actions[0].QuestionID != "q-1" || actions[1].QuestionID != "q-3" {
		t.Fatalf("actions = %#v, want q-1 and q-3", actions)
	}
}
