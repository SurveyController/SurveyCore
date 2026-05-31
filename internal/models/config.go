package models

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/SurveyController/SurveyCore/internal/domain"
)

// Default UA keys for random user agent
var DefaultRandomUAKeys = []string{"wechat", "mobile", "pc"}

// RuntimeConfig is the top-level user-facing configuration object.
type RuntimeConfig struct {
	URL                    string                     `json:"url"`
	SurveyTitle            string                     `json:"survey_title,omitempty"`
	SurveyProvider         string                     `json:"survey_provider,omitempty"`
	Target                 int                        `json:"target,omitempty"`
	Threads                int                        `json:"threads,omitempty"`
	SubmitInterval         [2]int                     `json:"submit_interval,omitempty"`
	AnswerDuration         [2]int                     `json:"answer_duration,omitempty"`
	AnswerDatetimeWindow   [2]string                  `json:"answer_datetime_window,omitempty"`
	RandomIPEnabled        bool                       `json:"random_ip_enabled,omitempty"`
	ProxySource            string                     `json:"proxy_source,omitempty"`
	CustomProxyAPI         string                     `json:"custom_proxy_api,omitempty"`
	ProxyAreaCode          *string                    `json:"proxy_area_code,omitempty"`
	RandomIPUserID         int                        `json:"random_ip_user_id,omitempty"`
	RandomIPDeviceID       string                     `json:"random_ip_device_id,omitempty"`
	IPExtractEndpoint      string                     `json:"ip_extract_endpoint,omitempty"`
	RandomIPLeaseMinute    int                        `json:"random_ip_lease_minute,omitempty"`
	RandomUAEnabled        bool                       `json:"random_ua_enabled,omitempty"`
	RandomUAKeys           []string                   `json:"random_ua_keys,omitempty"`
	RandomUARatios         map[string]int             `json:"random_ua_ratios,omitempty"`
	FailStopEnabled        bool                       `json:"fail_stop_enabled,omitempty"`
	PauseOnAliyunCaptcha   bool                       `json:"pause_on_aliyun_captcha,omitempty"`
	ReliabilityModeEnabled bool                       `json:"reliability_mode_enabled,omitempty"`
	PsychoTargetAlpha      float64                    `json:"psycho_target_alpha,omitempty"`
	AIMode                 string                     `json:"ai_mode,omitempty"`
	AIProvider             string                     `json:"ai_provider,omitempty"`
	AIAPIKey               string                     `json:"ai_api_key,omitempty"`
	AIBaseURL              string                     `json:"ai_base_url,omitempty"`
	AIAPIProtocol          string                     `json:"ai_api_protocol,omitempty"`
	AIModel                string                     `json:"ai_model,omitempty"`
	AISystemPrompt         string                     `json:"ai_system_prompt,omitempty"`
	ReverseFillEnabled     bool                       `json:"reverse_fill_enabled,omitempty"`
	ReverseFillSourcePath  string                     `json:"reverse_fill_source_path,omitempty"`
	ReverseFillFormat      string                     `json:"reverse_fill_format,omitempty"`
	ReverseFillStartRow    int                        `json:"reverse_fill_start_row,omitempty"`
	ReverseFillThreads     int                        `json:"reverse_fill_threads,omitempty"`
	AnswerRules            []map[string]any           `json:"answer_rules,omitempty"`
	DimensionGroups        []string                   `json:"dimension_groups,omitempty"`
	QuestionEntries        []QuestionEntry            `json:"question_entries,omitempty"`
	QuestionsInfo          []SurveyQuestionMeta       `json:"questions_info,omitempty"`
	ExtraFields            map[string]json.RawMessage `json:"-"`
}

// NewDefaultRuntimeConfig returns a RuntimeConfig with sensible defaults.
func NewDefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		SurveyProvider:         ProviderWJX,
		Target:                 1,
		Threads:                1,
		AnswerDuration:         [2]int{60, 120},
		ProxySource:            "default",
		FailStopEnabled:        true,
		PauseOnAliyunCaptcha:   true,
		ReliabilityModeEnabled: true,
		PsychoTargetAlpha:      0.85,
		AIMode:                 "free",
		AIProvider:             "deepseek",
		AIAPIProtocol:          "auto",
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

// UnmarshalJSON keeps Python-only or future config fields for lossless round trips.
func (cfg *RuntimeConfig) UnmarshalJSON(data []byte) error {
	type runtimeConfigAlias RuntimeConfig
	var alias runtimeConfigAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*cfg = RuntimeConfig(alias)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for key := range runtimeConfigJSONKeys() {
		delete(raw, key)
	}
	if len(raw) == 0 {
		cfg.ExtraFields = nil
		return nil
	}
	cfg.ExtraFields = raw
	return nil
}

// MarshalJSON writes preserved Python-only fields back alongside Go-supported fields.
func (cfg RuntimeConfig) MarshalJSON() ([]byte, error) {
	type runtimeConfigAlias RuntimeConfig
	data, err := json.Marshal(runtimeConfigAlias(cfg))
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	known := runtimeConfigJSONKeys()
	for key, value := range cfg.ExtraFields {
		if len(value) == 0 {
			continue
		}
		if _, ok := known[key]; ok {
			continue
		}
		raw[key] = value
	}
	return json.Marshal(raw)
}

func runtimeConfigJSONKeys() map[string]struct{} {
	result := make(map[string]struct{})
	t := reflect.TypeOf(RuntimeConfig{})
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		result[name] = struct{}{}
	}
	return result
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
