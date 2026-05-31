package models

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/SurveyController/SurveyCore/internal/domain"
)

// Default UA keys for random user agent, matching Python's USER_AGENT_PRESETS.
var DefaultRandomUAKeys = []string{"wechat_android", "mobile_android", "pc_web"}

const (
	CurrentConfigSchemaVersion = 6
	maxAnswerDurationSeconds   = 30 * 60
)

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
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	normalized := normalizeRuntimeConfigJSON(raw)

	type runtimeConfigAlias RuntimeConfig
	var alias runtimeConfigAlias
	if err := json.Unmarshal(configMustJSON(normalized), &alias); err != nil {
		return err
	}
	*cfg = RuntimeConfig(alias)

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
	if _, ok := raw["config_schema_version"]; !ok {
		raw["config_schema_version"] = configMustJSON(CurrentConfigSchemaVersion)
	}
	if _, ok := raw["_ai_config_present"]; !ok {
		raw["_ai_config_present"] = configMustJSON(cfg.hasAIConfig())
	}
	return json.Marshal(raw)
}

func (cfg RuntimeConfig) hasAIConfig() bool {
	return cfg.AIMode != "" ||
		cfg.AIProvider != "" ||
		cfg.AIAPIKey != "" ||
		cfg.AIBaseURL != "" ||
		cfg.AIAPIProtocol != "" ||
		cfg.AIModel != "" ||
		cfg.AISystemPrompt != ""
}

func normalizeRuntimeConfigJSON(raw map[string]json.RawMessage) map[string]json.RawMessage {
	normalized := make(map[string]json.RawMessage, len(raw))
	for key, value := range raw {
		normalized[key] = value
	}
	for _, key := range []string{"target", "threads", "random_ip_user_id", "random_ip_lease_minute"} {
		if value, ok := raw[key]; ok {
			normalized[key] = rawConfigInt(value, 0)
		}
	}
	if value, ok := raw["reverse_fill_start_row"]; ok {
		normalized["reverse_fill_start_row"] = rawMinInt(value, 1, 1)
	}
	if value, ok := raw["reverse_fill_threads"]; ok {
		normalized["reverse_fill_threads"] = rawMinInt(value, 1, 1)
	}
	for _, key := range []string{"random_ip_enabled", "random_ua_enabled", "fail_stop_enabled", "pause_on_aliyun_captcha", "reliability_mode_enabled", "reverse_fill_enabled"} {
		if value, ok := raw[key]; ok {
			normalized[key] = rawConfigBool(value, false)
		}
	}
	if value, ok := raw["psycho_target_alpha"]; ok {
		normalized["psycho_target_alpha"] = rawConfigFloat(value, 0)
	}
	for _, key := range []string{"url", "survey_title", "survey_provider", "proxy_source", "custom_proxy_api", "random_ip_device_id", "ip_extract_endpoint", "ai_mode", "ai_provider", "ai_api_key", "ai_base_url", "ai_api_protocol", "ai_model", "ai_system_prompt", "reverse_fill_source_path", "reverse_fill_format"} {
		if value, ok := raw[key]; ok {
			normalized[key] = rawString(value)
		}
	}
	if value, ok := raw["proxy_area_code"]; ok {
		normalized["proxy_area_code"] = rawNullableString(value)
	}
	if value, ok := raw["submit_interval"]; ok {
		normalized["submit_interval"] = rawIntPair(value, [2]int{})
	}
	if value, ok := raw["answer_duration"]; ok {
		normalized["answer_duration"] = rawAnswerDuration(value)
	}
	if value, ok := raw["answer_datetime_window"]; ok {
		normalized["answer_datetime_window"] = rawAnswerDatetimeWindow(value)
	}
	if value, ok := raw["random_ua_ratios"]; ok {
		normalized["random_ua_ratios"] = rawRandomUARatios(value)
	}
	if value, ok := raw["random_ua_keys"]; ok {
		normalized["random_ua_keys"] = rawRandomUAKeys(value)
	}
	if value, ok := raw["proxy_source"]; ok {
		normalized["proxy_source"] = rawProxySource(value)
	}
	if value, ok := raw["ai_mode"]; ok {
		normalized["ai_mode"] = rawAIMode(value)
	}
	if value, ok := raw["reverse_fill_format"]; ok {
		normalized["reverse_fill_format"] = rawReverseFillFormat(value)
	}
	return normalized
}

func rawConfigInt(raw json.RawMessage, fallback int) json.RawMessage {
	return configMustJSON(configToInt(configAnyFromRaw(raw), fallback))
}

func rawConfigFloat(raw json.RawMessage, fallback float64) json.RawMessage {
	return configMustJSON(configToFloat(configAnyFromRaw(raw), fallback))
}

func rawConfigBool(raw json.RawMessage, fallback bool) json.RawMessage {
	return configMustJSON(configToBool(configAnyFromRaw(raw), fallback))
}

func rawMinInt(raw json.RawMessage, fallback, minValue int) json.RawMessage {
	value := configToInt(configAnyFromRaw(raw), fallback)
	if value < minValue {
		value = minValue
	}
	return configMustJSON(value)
}

func rawString(raw json.RawMessage) json.RawMessage {
	value := configAnyFromRaw(raw)
	if value == nil {
		return configMustJSON("")
	}
	return configMustJSON(strings.TrimSpace(fmt.Sprint(value)))
}

func rawNullableString(raw json.RawMessage) json.RawMessage {
	value := configAnyFromRaw(raw)
	if value == nil {
		return configMustJSON(nil)
	}
	return rawString(raw)
}

func rawIntPair(raw json.RawMessage, fallback [2]int) json.RawMessage {
	value := configAnyFromRaw(raw)
	items, ok := value.([]any)
	if !ok || len(items) < 2 {
		return configMustJSON(fallback)
	}
	first := configToInt(items[0], fallback[0])
	second := configToInt(items[1], fallback[1])
	return configMustJSON([2]int{first, second})
}

func rawAnswerDuration(raw json.RawMessage) json.RawMessage {
	value := configAnyFromRaw(raw)
	defaultRange := [2]int{60, 120}
	if value == nil {
		return configMustJSON(defaultRange)
	}
	if items, ok := value.([]any); ok {
		if len(items) >= 2 {
			low := configClampInt(configToInt(items[0], 0), 0, maxAnswerDurationSeconds)
			high := configClampInt(configToInt(items[1], low), low, maxAnswerDurationSeconds)
			if low == 0 && high == 0 {
				return configMustJSON(defaultRange)
			}
			if low == high {
				return configMustJSON(configLegacyAnswerDurationRange(low))
			}
			return configMustJSON([2]int{low, high})
		}
		if len(items) == 1 {
			return configMustJSON(configLegacyAnswerDurationRange(configToInt(items[0], 0)))
		}
		return configMustJSON(defaultRange)
	}
	return configMustJSON(configLegacyAnswerDurationRange(configToInt(value, 0)))
}

func rawAnswerDatetimeWindow(raw json.RawMessage) json.RawMessage {
	value := configAnyFromRaw(raw)
	items, ok := value.([]any)
	if !ok {
		return configMustJSON([2]string{})
	}
	var result [2]string
	if len(items) >= 1 {
		result[0] = configNormalizeAnswerDatetimeString(items[0])
	}
	if len(items) >= 2 {
		result[1] = configNormalizeAnswerDatetimeString(items[1])
	}
	return configMustJSON(result)
}

func rawRandomUAKeys(raw json.RawMessage) json.RawMessage {
	value := configAnyFromRaw(raw)
	items, ok := value.([]any)
	if !ok {
		return configMustJSON([]string{})
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		key := strings.TrimSpace(fmt.Sprint(item))
		if isValidRandomUAKey(key) {
			result = append(result, key)
		}
	}
	return configMustJSON(result)
}

func rawRandomUARatios(raw json.RawMessage) json.RawMessage {
	var source map[string]any
	if err := json.Unmarshal(raw, &source); err != nil {
		return configMustJSON(defaultRandomUARatios())
	}
	total := 0
	for _, value := range source {
		total += configToInt(value, 0)
	}
	if total != 100 {
		return configMustJSON(defaultRandomUARatios())
	}
	result := map[string]int{
		"wechat": configToInt(source["wechat"], 33),
		"mobile": configToInt(source["mobile"], 33),
		"pc":     configToInt(source["pc"], 34),
	}
	return configMustJSON(result)
}

func rawProxySource(raw json.RawMessage) json.RawMessage {
	value := strings.ToLower(strings.TrimSpace(fmt.Sprint(configAnyFromRaw(raw))))
	switch value {
	case "default", "benefit", "custom":
		return configMustJSON(value)
	default:
		return configMustJSON("default")
	}
}

func rawAIMode(raw json.RawMessage) json.RawMessage {
	value := strings.ToLower(strings.TrimSpace(fmt.Sprint(configAnyFromRaw(raw))))
	switch value {
	case "free", "provider":
		return configMustJSON(value)
	default:
		return configMustJSON("free")
	}
}

func rawReverseFillFormat(raw json.RawMessage) json.RawMessage {
	value := strings.ToLower(strings.TrimSpace(fmt.Sprint(configAnyFromRaw(raw))))
	switch value {
	case domain.ReverseFillFormatAuto, domain.ReverseFillFormatWJXSequence, domain.ReverseFillFormatWJXScore, domain.ReverseFillFormatWJXText:
		return configMustJSON(value)
	default:
		return configMustJSON(domain.ReverseFillFormatAuto)
	}
}

func defaultRandomUARatios() map[string]int {
	return map[string]int{"wechat": 33, "mobile": 33, "pc": 34}
}

func isValidRandomUAKey(key string) bool {
	switch key {
	case "wechat_android", "mobile_android", "pc_web", "wechat", "mobile", "pc":
		return true
	default:
		return false
	}
}

func configAnyFromRaw(raw json.RawMessage) any {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return value
}

func configToInt(value any, fallback int) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	}
	return fallback
}

func configToFloat(value any, fallback float64) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return parsed
		}
	case json.Number:
		parsed, err := typed.Float64()
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func configToBool(value any, fallback bool) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case float64:
		return typed != 0
	case string:
		text := strings.ToLower(strings.TrimSpace(typed))
		switch text {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off", "":
			return false
		}
	}
	return fallback
}

func configLegacyAnswerDurationRange(value int) [2]int {
	normalized := configClampInt(value, 0, maxAnswerDurationSeconds)
	if normalized <= 0 {
		return [2]int{60, 120}
	}
	low := int(float64(normalized)*0.9 + 0.5)
	high := int(float64(normalized)*1.1 + 0.5)
	return [2]int{configClampInt(low, 0, maxAnswerDurationSeconds), configClampInt(high, low, maxAnswerDurationSeconds)}
}

func configClampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func configNormalizeAnswerDatetimeString(value any) string {
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return ""
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", text, time.Local)
	if err != nil {
		return ""
	}
	return parsed.Format("2006-01-02 15:04:05")
}

func configMustJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("null")
	}
	return data
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
