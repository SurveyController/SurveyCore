package tencent

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

func liveRatingQuestions(t *testing.T) []map[string]any {
	t.Helper()
	// Public question metadata from the CI-authorized survey 26070328/fa89.
	data, err := os.ReadFile("testdata/live-rating-questions.json")
	if err != nil {
		t.Fatal(err)
	}
	var questions []map[string]any
	if err := json.Unmarshal(data, &questions); err != nil {
		t.Fatal(err)
	}
	return questions
}

func TestRatingAnswersUseScoresAndProviderIDs(t *testing.T) {
	questions := liveRatingQuestions(t)
	if len(questions) != 3 {
		t.Fatalf("rating questions=%d", len(questions))
	}
	for _, test := range []struct {
		raw       map[string]any
		index     int
		id, score string
	}{
		{questions[1], 0, "q-15-a899-0", "0"},
		{questions[2], 10, "q-18-b757-10", "10"},
		{map[string]any{"id": "star", "type": "star", "star_num": 5}, 0, "star-1", "1"},
		{map[string]any{"id": "custom", "type": "star", "star_num": 5, "star_begin_num": 3}, 4, "custom-7", "7"},
	} {
		actual, err := questionAnswer(test.raw, model.AnswerAction{QuestionNum: 8, SelectedIndices: []int{test.index}})
		expected := map[string]any{"id": test.id, "type": test.raw["type"], "answer": test.score}
		if err != nil || !reflect.DeepEqual(actual, expected) {
			t.Fatalf("actual=%#v expected=%#v error=%v", actual, expected, err)
		}
	}
	matrix, err := questionAnswer(questions[0], model.AnswerAction{QuestionNum: 4, MatrixIndices: []int{0, 1, 2, 3, 4}})
	expected := []map[string]any{
		{"id": "q-13-7d39-g-101-ae7c-1", "type": "matrix_star", "answer": "on"},
		{"id": "q-13-7d39-g-102-68b1-2", "type": "matrix_star", "answer": "on"},
		{"id": "q-13-7d39-g-103-2bc8-3", "type": "matrix_star", "answer": "on"},
		{"id": "q-13-7d39-g-104-a1bb-4", "type": "matrix_star", "answer": "on"},
		{"id": "q-13-7d39-g-105-d55c-5", "type": "matrix_star", "answer": "on"},
	}
	if err != nil || !reflect.DeepEqual(matrix, expected) {
		t.Fatalf("matrix=%#v error=%v", matrix, err)
	}
}

func TestRatingsRejectMissingAndOutOfRangeAnswers(t *testing.T) {
	questions := liveRatingQuestions(t)
	for _, action := range []model.AnswerAction{
		{}, {SelectedIndices: []int{-1}}, {SelectedIndices: []int{11}}, {SelectedIndices: []int{0, 1}},
	} {
		if _, err := ratingAnswer(questions[1], action); err == nil {
			t.Fatalf("accepted invalid rating: %#v", action)
		}
	}
	for _, action := range []model.AnswerAction{
		{}, {MatrixIndices: []int{0}}, {MatrixIndices: []int{0, 1, 2, 3, 5}},
	} {
		if _, err := matrixRatingAnswer(questions[0], action); err == nil {
			t.Fatalf("accepted invalid matrix rating: %#v", action)
		}
	}
}

func TestLiveRatingsParseWithoutFalseDisplayConstraints(t *testing.T) {
	definition, err := buildDefinition(liveRatingQuestions(t), "CI rating fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, question := range definition.Questions {
		if question.Unsupported || question.HasDisplayCondition || question.UnsupportedReason != "" {
			t.Fatalf("supported visible rating was blocked: %#v", question)
		}
	}
	if definition.Questions[0].Rows != 5 || definition.Questions[0].Options != 5 || definition.Questions[1].OptionTexts[0] != "0" || definition.Questions[1].OptionTexts[10] != "10" {
		t.Fatalf("rating bounds=%#v", definition.Questions)
	}
}

func TestHiddenLogicDistinguishesFalseFromUnresolvedConstraints(t *testing.T) {
	for _, value := range []any{nil, false, "", 0, []any{}, map[string]any{}} {
		if hasHiddenLogic(value) {
			t.Fatalf("empty hidden value became a display constraint: %#v", value)
		}
	}
	for _, value := range []any{true, "expression", []any{"rule"}, map[string]any{"rule": true}} {
		if !hasHiddenLogic(value) {
			t.Fatalf("unresolved hidden condition was ignored: %#v", value)
		}
	}
}
