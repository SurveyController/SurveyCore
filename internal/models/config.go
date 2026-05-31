package models

import (
	"encoding/json"

	"github.com/SurveyController/SurveyCore/internal/domain"
)

// Default UA keys for random user agent
var DefaultRandomUAKeys = []string{"wechat", "mobile", "pc"}

// RuntimeConfig is the top-level user-facing configuration object.
type RuntimeConfig struct {
	URL                    string               `json:"url"`
	SurveyTitle            string               `json:"survey_title,omitempty"`
	SurveyProvider         string               `json:"survey_provider,omitempty"`
	Target                 int                  `json:"target,omitempty"`
	Threads                int                  `json:"threads,omitempty"`
	SubmitInterval         [2]int               `json:"submit_interval,omitempty"`
	AnswerDuration         [2]int               `json:"answer_duration,omitempty"`
	AnswerDatetimeWindow   [2]string            `json:"answer_datetime_window,omitempty"`
	RandomUAEnabled        bool                 `json:"random_ua_enabled,omitempty"`
	RandomUAKeys           []string             `json:"random_ua_keys,omitempty"`
	RandomUARatios         map[string]int       `json:"random_ua_ratios,omitempty"`
	ReliabilityModeEnabled bool                 `json:"reliability_mode_enabled,omitempty"`
	PsychoTargetAlpha      float64              `json:"psycho_target_alpha,omitempty"`
	ReverseFillEnabled     bool                 `json:"reverse_fill_enabled,omitempty"`
	ReverseFillSourcePath  string               `json:"reverse_fill_source_path,omitempty"`
	ReverseFillFormat      string               `json:"reverse_fill_format,omitempty"`
	ReverseFillStartRow    int                  `json:"reverse_fill_start_row,omitempty"`
	ReverseFillThreads     int                  `json:"reverse_fill_threads,omitempty"`
	AnswerRules            []map[string]any     `json:"answer_rules,omitempty"`
	DimensionGroups        []string             `json:"dimension_groups,omitempty"`
	QuestionEntries        []QuestionEntry      `json:"question_entries,omitempty"`
	QuestionsInfo          []SurveyQuestionMeta `json:"questions_info,omitempty"`
}

// NewDefaultRuntimeConfig returns a RuntimeConfig with sensible defaults.
func NewDefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		SurveyProvider:         ProviderWJX,
		Target:                 1,
		Threads:                1,
		AnswerDuration:         [2]int{60, 120},
		ReliabilityModeEnabled: true,
		PsychoTargetAlpha:      0.85,
		ReverseFillFormat:      "auto",
		ReverseFillStartRow:    1,
		ReverseFillThreads:     1,
		RandomUAKeys:           append([]string{}, DefaultRandomUAKeys...),
		RandomUARatios:         map[string]int{"wechat": 33, "mobile": 33, "pc": 34},
	}
}

// SerializeRuntimeConfig converts config to JSON bytes.
func SerializeRuntimeConfig(cfg *RuntimeConfig) ([]byte, error) {
	return json.MarshalIndent(cfg, "", "  ")
}

// DeserializeRuntimeConfig parses JSON bytes into RuntimeConfig.
func DeserializeRuntimeConfig(data []byte) (*RuntimeConfig, error) {
	var cfg RuntimeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Provider constants
const (
	ProviderWJX     = domain.ProviderWJX
	ProviderQQ      = domain.ProviderQQ
	ProviderCredamo = domain.ProviderCredamo
)

// MakeProviderQuestionKey creates a unique key for a provider question.
func MakeProviderQuestionKey(provider, pageID, questionID string) string {
	return domain.MakeProviderQuestionKey(provider, pageID, questionID)
}
