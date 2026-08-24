package credamo

import (
	"encoding/json"
	"testing"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

func TestBuildAnswerItemsMatchesEntryByProviderQuestionID(t *testing.T) {
	raw := []map[string]any{
		{
			"qstNo":        "Q2",
			"qstId":        102,
			"sortNo":       2,
			"questionType": 2,
			"selector":     1,
			"choices":      []any{map[string]any{"choiceId": 1}, map[string]any{"choiceId": 2}},
		},
	}
	request := &model.SubmissionRequest{Context: model.SubmissionContext{Actions: []model.AnswerAction{
		{QuestionNum: 99, QuestionID: "102", Kind: model.QuestionKindSingle, SelectedIndices: []int{1}},
	}}}

	items, err := buildAnswerItems(raw, request)
	if err != nil {
		t.Fatal(err)
	}
	choice := items[0]["answerQstChoice"].(map[string]any)
	if choice["choiceId"] != 2 {
		t.Fatalf("choice = %#v", choice)
	}
}

func TestBuildAnswerItemsUsesJSONProbabilityValues(t *testing.T) {
	var entry model.QuestionStrategy
	if err := json.Unmarshal([]byte(`{"question_type":"multiple","probabilities":{"options":[100,0,100]}}`), &entry); err != nil {
		t.Fatal(err)
	}
	questionNum := 1
	entry.QuestionNum = &questionNum
	raw := []map[string]any{
		{
			"qstNo":        "Q1",
			"qstId":        101,
			"questionType": 2,
			"selector":     2,
			"choices": []any{
				map[string]any{"choiceId": 1},
				map[string]any{"choiceId": 2},
				map[string]any{"choiceId": 3},
			},
		},
	}
	request := &model.SubmissionRequest{Context: model.SubmissionContext{Actions: []model.AnswerAction{{QuestionNum: 1, Kind: model.QuestionKindMultiple, SelectedIndices: []int{0, 2}}}}}

	items, err := buildAnswerItems(raw, request)
	if err != nil {
		t.Fatal(err)
	}
	choices := items[0]["answerQstChoiceList"].([]map[string]any)
	if len(choices) != 2 || choices[0]["choiceId"] != 1 || choices[1]["choiceId"] != 3 {
		t.Fatalf("choices = %#v", choices)
	}
}

func TestBuildAnswerItemsCoversOrderAndMatrixDefaults(t *testing.T) {
	request := &model.SubmissionRequest{Context: model.SubmissionContext{Actions: []model.AnswerAction{
		{QuestionNum: 1, Kind: model.QuestionKindOrder, SelectedIndices: []int{0, 1}},
		{QuestionNum: 2, Kind: model.QuestionKindMatrix, MatrixIndices: []int{0, 1}},
	}}}
	items, err := buildAnswerItems([]map[string]any{
		{
			"qstNo":        "Q1",
			"qstId":        101,
			"sortNo":       2,
			"questionType": 6,
			"choices": []any{
				map[string]any{"choiceId": 1},
				map[string]any{"choiceId": 2},
			},
		},
		{
			"qstNo":        "Q2",
			"qstId":        102,
			"sortNo":       1,
			"questionType": 4,
			"choices":      []any{map[string]any{"choiceId": 3}, map[string]any{"choiceId": 4}},
			"answers":      []any{map[string]any{"answerId": 5}, map[string]any{"answerId": 6}},
		},
	}, request)
	if err != nil {
		t.Fatal(err)
	}
	if items[0]["qstId"] != 102 || items[1]["qstId"] != 101 {
		t.Fatalf("items not sorted by sortNo: %#v", items)
	}
	matrixRows := items[0]["answerQstChoiceList"].([]map[string]any)
	if len(matrixRows) != 2 {
		t.Fatalf("matrix rows = %#v", matrixRows)
	}
	orderRows := items[1]["answerChoiceContent"].([]map[string]any)
	if len(orderRows) != 2 || orderRows[0]["choiceId"] != 1 || orderRows[1]["choiceId"] != 2 {
		t.Fatalf("order rows = %#v", orderRows)
	}
}

func TestBuildAnswerItemsUsesMatrixRowProbabilities(t *testing.T) {
	questionNum := 1
	request := &model.SubmissionRequest{Context: model.SubmissionContext{Actions: []model.AnswerAction{{QuestionNum: questionNum, Kind: model.QuestionKindMatrix, MatrixIndices: []int{1, 0}}}}}
	items, err := buildAnswerItems([]map[string]any{
		{
			"qstNo":        "Q1",
			"qstId":        102,
			"questionType": 4,
			"choices":      []any{map[string]any{"choiceId": 3}, map[string]any{"choiceId": 4}},
			"answers":      []any{map[string]any{"answerId": 5}, map[string]any{"answerId": 6}},
		},
	}, request)
	if err != nil {
		t.Fatal(err)
	}
	rows := items[0]["answerQstChoiceList"].([]map[string]any)
	first := rows[0]["choiceAnswerList"].([]map[string]any)[0]
	second := rows[1]["choiceAnswerList"].([]map[string]any)[0]
	if first["answerId"] != 6 || second["answerId"] != 5 {
		t.Fatalf("matrix rows = %#v", rows)
	}
}

func TestBuildAnswerItemsAppliesAnswerRules(t *testing.T) {
	request := &model.SubmissionRequest{Context: model.SubmissionContext{Actions: []model.AnswerAction{
		{QuestionNum: 1, Kind: model.QuestionKindSingle, SelectedIndices: []int{1}},
		{QuestionNum: 2, Kind: model.QuestionKindSingle, SelectedIndices: []int{2}},
	}}}
	items, err := buildAnswerItems([]map[string]any{
		{
			"qstNo":        "Q1",
			"qstId":        101,
			"sortNo":       1,
			"questionType": 2,
			"selector":     1,
			"choices":      []any{map[string]any{"choiceId": 1}, map[string]any{"choiceId": 2}},
		},
		{
			"qstNo":        "Q2",
			"qstId":        102,
			"sortNo":       2,
			"questionType": 2,
			"selector":     1,
			"choices":      []any{map[string]any{"choiceId": 3}, map[string]any{"choiceId": 4}, map[string]any{"choiceId": 5}},
		},
	}, request)
	if err != nil {
		t.Fatal(err)
	}
	first := items[0]["answerQstChoice"].(map[string]any)
	second := items[1]["answerQstChoice"].(map[string]any)
	if first["choiceId"] != 2 || second["choiceId"] != 5 {
		t.Fatalf("items = %#v", items)
	}
}

func TestUnknownAndDescriptionQuestionsAreNotSubmitted(t *testing.T) {
	request := &model.SubmissionRequest{Context: model.SubmissionContext{Actions: []model.AnswerAction{{QuestionNum: 2, Kind: model.QuestionKindSingle, SelectedIndices: []int{0}}}}}
	items, err := buildAnswerItems([]map[string]any{
		{"qstNo": "Q1", "qstId": "intro", "questionType": 0, "qstTitle": "说明"},
		{"qstNo": "Q2", "qstId": "known", "questionType": 2, "selector": 1, "choices": []any{map[string]any{"choiceId": 1}}},
		{"qstNo": "Q3", "qstId": "unknown", "questionType": 99, "choices": []any{map[string]any{"choiceId": 2}}},
	}, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0]["qstId"] != "known" {
		t.Fatalf("items = %#v", items)
	}
	unknown := normalizeQuestion(rawToNormalizedInput(map[string]any{"qstNo": "Q3", "questionType": 99, "choices": []any{map[string]any{"choiceId": 2}}}, 3), 3)
	if !unknown.Unsupported || unknown.TypeCode == "3" || isAnswerableQuestion(unknown) {
		t.Fatalf("unknown = %#v", unknown)
	}
}

func TestBuildAnswerItemsRejectsOutOfRangeIndices(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]any
		act  model.AnswerAction
	}{
		{"choice", map[string]any{"qstNo": "Q1", "questionType": 2, "selector": 1, "choices": []any{map[string]any{"choiceId": 1}}}, model.AnswerAction{QuestionNum: 1, Kind: model.QuestionKindSingle, SelectedIndices: []int{2}}},
		{"matrix", map[string]any{"qstNo": "Q1", "questionType": 4, "choices": []any{map[string]any{"choiceId": 1}}, "answers": []any{map[string]any{"answerId": 2}}}, model.AnswerAction{QuestionNum: 1, Kind: model.QuestionKindMatrix, MatrixIndices: []int{-1}}},
		{"matrix missing row", map[string]any{"qstNo": "Q1", "questionType": 4, "choices": []any{map[string]any{"choiceId": 1}}, "answers": []any{map[string]any{"answerId": 2}}}, model.AnswerAction{QuestionNum: 1, Kind: model.QuestionKindMatrix}},
		{"order", map[string]any{"qstNo": "Q1", "questionType": 6, "choices": []any{map[string]any{"choiceId": 1}}}, model.AnswerAction{QuestionNum: 1, Kind: model.QuestionKindOrder, SelectedIndices: []int{3}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildAnswerItems([]map[string]any{tc.raw}, &model.SubmissionRequest{Context: model.SubmissionContext{Actions: []model.AnswerAction{tc.act}}})
			if err == nil {
				t.Fatal("expected controlled bounds error")
			}
		})
	}
}
