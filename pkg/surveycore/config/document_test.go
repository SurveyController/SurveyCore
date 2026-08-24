package config

import (
	"strings"
	"testing"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

func TestRunRequestFromConfigDocumentRejectsAllZeroWeights(t *testing.T) {
	questionNum := 1
	_, err := RunRequestFromConfigDocument(ConfigDocument{
		SchemaVersion: ConfigSchemaVersion,
		Answers: model.AnswerPlan{Strategies: []model.QuestionStrategy{{
			QuestionType:  model.QuestionKindSingle,
			QuestionNum:   &questionNum,
			Probabilities: model.OptionWeights(0, 0),
		}}},
	})
	if err == nil || !strings.Contains(err.Error(), "不能全部为 0") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunRequestFromConfigDocumentUsesCustomWeightsForCustomMode(t *testing.T) {
	questionNum := 1
	request, err := RunRequestFromConfigDocument(ConfigDocument{
		SchemaVersion: ConfigSchemaVersion,
		Answers: model.AnswerPlan{Strategies: []model.QuestionStrategy{{
			QuestionType:     model.QuestionKindSingle,
			QuestionNum:      &questionNum,
			DistributionMode: "custom",
			Probabilities:    model.OptionWeights(1, 0),
			CustomWeights:    model.OptionWeights(0, 1),
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := request.AnswerPlan.Strategies[0].CustomWeights.Options[1]; got != 1 {
		t.Fatalf("custom weights = %#v", request.AnswerPlan.Strategies[0].CustomWeights.Options)
	}
}
