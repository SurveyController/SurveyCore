package runtime

import "testing"

func TestDefaultQuestionEntriesAlignParserTypes(t *testing.T) {
	questions := []QuestionMeta{
		{Num: 1, Title: "单选", Provider: ProviderCredamo, ProviderID: "101", ProviderType: "single", TypeCode: "3", Options: 2},
		{Num: 2, Title: "多选", Provider: ProviderCredamo, ProviderID: "102", ProviderType: "multiple", TypeCode: "4", Options: 3},
		{Num: 3, Title: "矩阵", Provider: ProviderCredamo, ProviderID: "103", ProviderType: "matrix", TypeCode: "6", Options: 2, Rows: 2},
		{Num: 4, Title: "文本", Provider: ProviderCredamo, ProviderID: "104", ProviderType: "text", TypeCode: "1", TextInputs: 1, ForcedTexts: []string{"hello"}},
	}

	entries := buildDefaultQuestionStrategies(questions)
	if len(entries) != 4 {
		t.Fatalf("entry count = %d", len(entries))
	}

	assertEntry(t, entries[0], "single", 2, "101")
	assertEntry(t, entries[1], "multiple", 3, "102")
	assertEntry(t, entries[2], "matrix", 2, "103")
	assertEntry(t, entries[3], "text", 1, "104")
	if len(entries[3].Texts) != 1 || entries[3].Texts[0] != "hello" {
		t.Fatalf("text entry = %#v", entries[3])
	}
}

func TestDefaultScoreStrategyUsesCustomMiddleWeights(t *testing.T) {
	entries := buildDefaultQuestionStrategies([]QuestionMeta{{
		Num: 1, Title: "评分", Provider: ProviderWJX, ProviderType: "score", TypeCode: "5", IsRating: true, Options: 5,
	}})
	if len(entries) != 1 {
		t.Fatalf("entries = %#v", entries)
	}
	entry := entries[0]
	if entry.QuestionType != "score" || entry.DistributionMode != "custom" {
		t.Fatalf("entry = %#v", entry)
	}
	if len(entry.CustomWeights.Options) != 5 || entry.CustomWeights.Options[2] <= entry.CustomWeights.Options[0] {
		t.Fatalf("score weights = %#v", entry.CustomWeights.Options)
	}
}

func assertEntry(t *testing.T, entry QuestionStrategy, questionType string, probabilities int, providerID string) {
	t.Helper()
	if string(entry.QuestionType) != questionType {
		t.Fatalf("type = %s, want %s", entry.QuestionType, questionType)
	}
	values := entry.Probabilities.Values()
	if len(values) != probabilities {
		t.Fatalf("probabilities = %#v, want %d values", entry.Probabilities, probabilities)
	}
	if entry.ProviderQuestionID == nil || *entry.ProviderQuestionID != providerID {
		t.Fatalf("provider id = %#v, want %s", entry.ProviderQuestionID, providerID)
	}
}
