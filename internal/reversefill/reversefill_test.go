package reversefill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SurveyController/SurveyConsole/internal/models"
)

func TestBuildSpecParsesWjxCSVChoicesAndText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "samples.csv")
	content := "1、Color,2、Comment\nB,hello\n1,world\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	q1, q2 := 1, 2
	cfg := &models.RuntimeConfig{
		SurveyProvider:        models.ProviderWJX,
		ReverseFillEnabled:    true,
		ReverseFillSourcePath: path,
		ReverseFillFormat:     models.ReverseFillFormatWJXText,
		ReverseFillStartRow:   1,
		Target:                2,
		QuestionEntries: []models.QuestionEntry{
			{QuestionNum: &q1, QuestionType: "single"},
			{QuestionNum: &q2, QuestionType: "text"},
		},
	}
	questions := []models.SurveyQuestionMeta{
		{Num: q1, Title: "Color", TypeCode: "3", Options: 2, OptionTexts: []string{"A", "B"}, Provider: models.ProviderWJX},
		{Num: q2, Title: "Comment", TypeCode: "8", IsTextLike: true, Provider: models.ProviderWJX},
	}

	spec, err := BuildSpec(cfg, questions)
	if err != nil {
		t.Fatalf("BuildSpec returned error: %v", err)
	}
	if spec.AvailableSamples != 2 || len(spec.Samples) != 2 {
		t.Fatalf("samples = available %d len %d, want 2 and 2", spec.AvailableSamples, len(spec.Samples))
	}
	answer := spec.Samples[0].Answers[q1]
	if answer.ChoiceIndex == nil || *answer.ChoiceIndex != 1 {
		t.Fatalf("choice index = %v, want 1", answer.ChoiceIndex)
	}
	text := spec.Samples[0].Answers[q2].TextValue
	if text != "hello" {
		t.Fatalf("text answer = %q, want hello", text)
	}
}
