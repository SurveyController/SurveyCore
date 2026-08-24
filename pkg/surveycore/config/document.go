package config

import (
	"fmt"
	"strings"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

const (
	ConfigSchemaVersion = 2

	ReverseFillFormatAuto        = "auto"
	ReverseFillFormatWJXSequence = "wjx_sequence"
	ReverseFillFormatWJXScore    = "wjx_score"
	ReverseFillFormatWJXText     = "wjx_text"
)

// SurveyDocument stores survey identity and the parsed definition snapshot.
type SurveyDocument struct {
	URL        string                 `json:"url"`
	Provider   string                 `json:"provider"`
	Title      string                 `json:"title"`
	Definition model.SurveyDefinition `json:"definition"`
}

// NetworkSettings stores local proxy and user-agent settings outside RunRequest.
type NetworkSettings struct {
	RandomProxyEnabled bool           `json:"randomProxyEnabled"`
	ProxyMode          string         `json:"proxyMode,omitempty"`
	FixedProxyAddress  string         `json:"fixedProxyAddress,omitempty"`
	ProxySource        string         `json:"proxySource"`
	CustomProxyAPI     string         `json:"customProxyApi,omitempty"`
	ProxyAreaCode      string         `json:"proxyAreaCode,omitempty"`
	RandomUAEnabled    bool           `json:"randomUaEnabled"`
	RandomUARatios     map[string]int `json:"randomUaRatios,omitempty"`
}

// ConfigDocument is the schemaVersion 2 local configuration format.
type ConfigDocument struct {
	SchemaVersion int                      `json:"schemaVersion"`
	Survey        SurveyDocument           `json:"survey"`
	Execution     model.ExecutionPlan      `json:"execution"`
	Network       NetworkSettings          `json:"network"`
	Answers       model.AnswerPlan         `json:"answers"`
	ReverseFill   model.ReverseFillPlan    `json:"reverseFill"`
	Psychometrics model.PsychometricPolicy `json:"psychometrics"`
}

// ConfigDocumentFromRunRequest converts a public run request to a local document.
func ConfigDocumentFromRunRequest(config model.RunRequest) ConfigDocument {
	return ConfigDocument{
		SchemaVersion: ConfigSchemaVersion,
		Survey: SurveyDocument{
			URL:        strings.TrimSpace(config.SurveySource.URL),
			Provider:   strings.TrimSpace(config.SurveySource.Provider),
			Title:      strings.TrimSpace(config.SurveyDefinition.Title),
			Definition: model.CloneSurveyDefinition(config.SurveyDefinition),
		},
		Execution:     config.ExecutionPlan,
		Answers:       model.CloneAnswerPlan(config.AnswerPlan),
		ReverseFill:   config.ReverseFillPlan,
		Psychometrics: config.PsychometricPolicy,
	}
}

// RunRequestFromConfigDocument validates and converts a local document.
func RunRequestFromConfigDocument(document ConfigDocument) (model.RunRequest, error) {
	if document.SchemaVersion != ConfigSchemaVersion {
		return model.RunRequest{}, fmt.Errorf("不支持的配置版本：%d", document.SchemaVersion)
	}
	if err := validateAnswerWeights(document.Answers); err != nil {
		return model.RunRequest{}, err
	}
	definition := model.CloneSurveyDefinition(document.Survey.Definition)
	definition.Provider = firstNonEmpty(document.Survey.Provider, definition.Provider)
	definition.Title = firstNonEmpty(document.Survey.Title, definition.Title)
	return model.RunRequest{
		SurveySource:       model.SurveySource{URL: strings.TrimSpace(document.Survey.URL), Provider: firstNonEmpty(document.Survey.Provider, definition.Provider)},
		SurveyDefinition:   definition,
		ExecutionPlan:      document.Execution,
		AnswerPlan:         model.CloneAnswerPlan(document.Answers),
		ReverseFillPlan:    document.ReverseFill,
		PsychometricPolicy: document.Psychometrics,
	}, nil
}

func validateAnswerWeights(plan model.AnswerPlan) error {
	for _, strategy := range plan.Strategies {
		if err := strategy.Probabilities.Validate(); err != nil {
			return err
		}
		if err := strategy.CustomWeights.Validate(); err != nil {
			return err
		}
		weights := strategy.Probabilities
		if strings.EqualFold(strings.TrimSpace(strategy.DistributionMode), "custom") && hasWeightValues(strategy.CustomWeights) {
			weights = strategy.CustomWeights
		}
		if err := validatePositiveWeights(strategy.QuestionType, weights); err != nil {
			return err
		}
	}
	return nil
}

func hasWeightValues(weights model.WeightTable) bool {
	return len(weights.Options) > 0 || len(weights.Rows) > 0
}

func positiveWeightCount(values []float64) int {
	count := 0
	for _, value := range values {
		if value > 0 {
			count++
		}
	}
	return count
}

func validatePositiveWeights(kind model.QuestionKind, weights model.WeightTable) error {
	switch kind {
	case model.QuestionKindSingle, model.QuestionKindDropdown, model.QuestionKindScale, model.QuestionKindScore, model.QuestionKindMultiple:
		if len(weights.Options) > 0 && positiveWeightCount(weights.Options) == 0 {
			return fmt.Errorf("%s题配比不能全部为 0", kind)
		}
	case model.QuestionKindMatrix:
		for index, row := range weights.Rows {
			if len(row) > 0 && positiveWeightCount(row) == 0 {
				return fmt.Errorf("矩阵第%d行配比不能全部为 0", index+1)
			}
		}
	}
	return nil
}

func cloneIntValues(values map[string]int) map[string]int {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]int, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
