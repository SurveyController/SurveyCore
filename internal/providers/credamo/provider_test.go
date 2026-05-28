package credamo

import (
	"strings"
	"testing"

	"github.com/SurveyController/SurveyConsole/internal/models"
	"github.com/SurveyController/SurveyConsole/internal/providers/providerutil"
)

func TestStandardizeQuestionsSupportsRawCredamoAPIShape(t *testing.T) {
	raw := []map[string]any{
		{
			"qstId":        "101",
			"questionType": float64(2),
			"selector":     float64(2),
			"qstNo":        "2",
			"qstTitle":     "多选题",
			"choices": []any{
				map[string]any{"choiceId": "21", "display": "A"},
				map[string]any{"choiceId": "22", "display": "B"},
			},
		},
		{
			"qstId":        "102",
			"questionType": float64(4),
			"qstNo":        "3",
			"qstTitle":     "矩阵题",
			"choices": []any{
				map[string]any{"choiceId": "31", "display": "外观"},
				map[string]any{"choiceId": "32", "display": "功能"},
			},
			"answers": []any{
				map[string]any{"answerId": "41", "display": "差"},
				map[string]any{"answerId": "42", "display": "好"},
			},
		},
	}

	got := standardizeQuestions(raw)
	if len(got) != 2 {
		t.Fatalf("questions length = %d, want 2: %#v", len(got), got)
	}
	if got[0].Num != 2 || got[0].ProviderQuestionID != "101" || got[0].TypeCode != "4" || got[0].Options != 2 {
		t.Fatalf("first question = %#v, want raw multiple question", got[0])
	}
	if got[1].Num != 3 || got[1].TypeCode != "6" || got[1].Rows != 2 || got[1].Options != 2 {
		t.Fatalf("matrix question = %#v, want 2x2 matrix", got[1])
	}
	if got[1].RowTexts[0] != "外观" || got[1].OptionTexts[1] != "好" {
		t.Fatalf("matrix texts = rows %#v options %#v", got[1].RowTexts, got[1].OptionTexts)
	}
}

func TestExtractShortURLSupportsCredamoURLForms(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://www.credamo.com/s/A73QR3ano", "A73QR3ano"},
		{"https://www.credamo.com/s/A73QR3ano/", "A73QR3ano"},
		{"www.credamo.cn/s/A73QR3ano/", "A73QR3ano"},
		{"https://www.credamo.com/answer.html#/s/Bvyyaaano", "Bvyyaaano"},
		{"https://www.credamo.com/answer.html#/s/Bvyyaaano/", "Bvyyaaano"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := extractShortURL(tt.url); got != tt.want {
				t.Fatalf("extractShortURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestBuildSubmitBodyCoversRawCredamoAPIShape(t *testing.T) {
	rawQuestions := []map[string]any{
		{"qstId": "101", "questionType": 2, "selector": 1, "choices": []any{map[string]any{"choiceId": "11"}, map[string]any{"choiceId": "12"}}},
		{"qstId": "102", "questionType": 2, "selector": 2, "choices": []any{map[string]any{"choiceId": "21"}, map[string]any{"choiceId": "22"}}},
		{"qstId": "103", "questionType": 6, "choices": []any{map[string]any{"choiceId": "31"}, map[string]any{"choiceId": "32"}}},
		{
			"qstId":        "104",
			"questionType": 4,
			"choices":      []any{map[string]any{"choiceId": "41"}, map[string]any{"choiceId": "42"}},
			"answers":      []any{map[string]any{"answerId": "51"}, map[string]any{"answerId": "52"}},
		},
		{"qstId": "105", "questionType": 1},
	}
	actions := []CredamoAnswerAction{
		{QuestionID: "101", QuestionType: "single", SelectedIndices: []int{1}},
		{QuestionID: "102", QuestionType: "multiple", SelectedIndices: []int{0, 1}},
		{QuestionID: "103", QuestionType: "order", OrderIndices: []int{1, 0}},
		{QuestionID: "104", QuestionType: "matrix", MatrixAnswers: [][]int{{0}, {1}}},
		{QuestionID: "105", QuestionType: "text", TextValue: "你好"},
	}

	body := buildSubmitBody("demo", rawQuestions, actions, &models.ExecutionConfig{}, 1000, 50)
	if body["answerStartTime"] != int64(1000) || body["answerEndTime"] != int64(51000) {
		t.Fatalf("body timing = %#v/%#v, want 1000/51000", body["answerStartTime"], body["answerEndTime"])
	}
	items := body["answerQstList"].([]map[string]any)
	if len(items) != 5 {
		t.Fatalf("answerQstList length = %d, want 5", len(items))
	}
	if items[0]["answerTime"] != int64(10000) {
		t.Fatalf("answerTime = %#v, want 10000", items[0]["answerTime"])
	}

	single := items[0]["answerQstChoice"].(map[string]any)
	if single["choiceId"] != 12 {
		t.Fatalf("single choice = %#v, want choiceId 12", single)
	}
	multiple := items[1]["answerQstChoiceList"].([]map[string]any)
	if multiple[0]["choiceId"] != 21 || multiple[1]["choiceId"] != 22 {
		t.Fatalf("multiple choices = %#v, want 21/22", multiple)
	}
	order := items[2]["answerChoiceContent"].([]map[string]any)
	if order[0]["choiceId"] != 32 || order[0]["choiceContent"] != 1 || order[1]["choiceContent"] != 2 {
		t.Fatalf("order choices = %#v, want selected ids with rank numbers", order)
	}
	matrix := items[3]["answerQstChoiceList"].([]map[string]any)
	firstRowAnswers := matrix[0]["choiceAnswerList"].([]map[string]any)
	if matrix[0]["choiceId"] != 41 || firstRowAnswers[0]["answerId"] != 51 {
		t.Fatalf("matrix row = %#v, want row choiceId 41 answerId 51", matrix[0])
	}
	if items[4]["answerContent"] != "你好" {
		t.Fatalf("text answer = %#v, want 你好", items[4])
	}
}

func TestCredamoForcedChoicePrefersTextWhenAPIChoiceOrderChanges(t *testing.T) {
	raw := []map[string]any{
		{
			"qstId":        "108",
			"questionType": 2,
			"selector":     1,
			"qstTitle":     "请选择 200",
			"choices": []any{
				map[string]any{"choiceId": "6787", "display": "300"},
				map[string]any{"choiceId": "6788", "display": "500"},
				map[string]any{"choiceId": "6789", "display": "200"},
				map[string]any{"choiceId": "6790", "display": "600"},
			},
		},
	}
	questions := standardizeQuestions(raw)
	if questions[0].ForcedOptionIndex == nil || *questions[0].ForcedOptionIndex != 2 || questions[0].ForcedOptionText != "200" {
		t.Fatalf("forced choice = index %v text %q, want index 2 text 200", questions[0].ForcedOptionIndex, questions[0].ForcedOptionText)
	}

	cfg := &models.ExecutionConfig{
		QuestionsMetadata: map[int]models.SurveyQuestionMeta{
			8: {Num: 8, ProviderQuestionID: "108", ForcedOptionIndex: intPtr(1), ForcedOptionText: "200"},
		},
	}
	body := buildSubmitBody("demo", raw, []CredamoAnswerAction{
		{QuestionID: "108", QuestionType: "single", SelectedIndices: []int{1}},
	}, cfg, 1000, 9)

	items := body["answerQstList"].([]map[string]any)
	choice := items[0]["answerQstChoice"].(map[string]any)
	if choice["choiceId"] != 6789 {
		t.Fatalf("forced answer choice = %#v, want choiceId 6789", choice)
	}
}

func TestComputeSignatureUsesUppercaseDoubleSHA1(t *testing.T) {
	got := computeSignature("token", "NONCE1234567890", "1710000000000", "UNION12345")
	if got == "" || got != strings.ToUpper(got) {
		t.Fatalf("signature = %q, want uppercase hex", got)
	}
}

func TestSampleAnswerDurationSecondsAllowsFixedConfiguredRange(t *testing.T) {
	cfg := &models.ExecutionConfig{AnswerDurationRangeSeconds: [2]int{30, 30}}
	if got := providerutil.SampleAnswerDurationSeconds(cfg, 9, 16); got != 30 {
		t.Fatalf("sampleAnswerDurationSeconds fixed range = %d, want 30", got)
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

func intPtr(v int) *int {
	return &v
}
