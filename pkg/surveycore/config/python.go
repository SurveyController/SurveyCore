package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

// pythonConfig mirrors the serializable RunConfig contract in surveycore-py.
// Callable answer generators and ai_enabled are intentionally not serialized there.
type pythonConfig struct {
	SchemaVersion        int                     `json:"schema_version"`
	URL                  string                  `json:"url"`
	Questions            []json.RawMessage       `json:"questions"`
	Target               int                     `json:"target"`
	Concurrency          int                     `json:"concurrency"`
	Provider             *string                 `json:"provider"`
	AnswerDuration       []int                   `json:"answer_duration"`
	SubmitInterval       []int                   `json:"submit_interval"`
	AnswerDatetimeWindow []string                `json:"answer_datetime_window"`
	RandomUserAgent      bool                    `json:"random_user_agent"`
	UserAgentRatios      map[string]int          `json:"user_agent_ratios"`
	ReliabilityMode      bool                    `json:"reliability_mode"`
	TargetAlpha          float64                 `json:"target_alpha"`
	AnswerRules          []model.ConsistencyRule `json:"answer_rules"`
}

// DeserializePythonConfig imports a surveycore-py schema_version 1 object into
// the local document format, retaining user-agent settings for subsequent runs.
func DeserializePythonConfig(payload map[string]any) (ConfigDocument, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return ConfigDocument{}, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil || raw == nil {
		return ConfigDocument{}, fmt.Errorf("Python 配置必须是对象")
	}
	for field, value := range raw {
		if field != "provider" && bytes.Equal(value, []byte("null")) {
			return ConfigDocument{}, fmt.Errorf("Python 配置 %s 不能为 null", field)
		}
	}
	config := pythonConfig{
		Target: 1, Concurrency: 1,
		AnswerDuration: []int{60, 120}, SubmitInterval: []int{0, 0},
		AnswerDatetimeWindow: []string{"", ""},
		UserAgentRatios:      map[string]int{"wechat": 33, "mobile": 33, "pc": 34},
		ReliabilityMode:      true, TargetAlpha: 0.85,
	}
	if _, provided := raw["user_agent_ratios"]; provided {
		config.UserAgentRatios = nil
	}
	if err := decodePythonObject(data, &config); err != nil {
		return ConfigDocument{}, fmt.Errorf("Python 配置字段无效：%w", err)
	}
	if config.SchemaVersion != 1 {
		return ConfigDocument{}, fmt.Errorf("不支持的 Python 配置版本：%d", config.SchemaVersion)
	}
	config.URL = strings.TrimSpace(config.URL)
	if config.URL == "" || config.Target < 1 || config.Concurrency < 1 {
		return ConfigDocument{}, fmt.Errorf("Python 配置 url 不能为空，target 和 concurrency 必须大于 0")
	}
	provider := ""
	if config.Provider != nil {
		provider = *config.Provider
		if provider != model.ProviderWJX && provider != model.ProviderQQ && provider != model.ProviderCredamo {
			return ConfigDocument{}, fmt.Errorf("Python 配置 provider 必须是 wjx、qq 或 credamo")
		}
	}
	for field, values := range map[string][]int{"answer_duration": config.AnswerDuration, "submit_interval": config.SubmitInterval} {
		if len(values) != 2 || values[0] < 0 || values[1] < values[0] || containsPythonNull(raw[field]) {
			return ConfigDocument{}, fmt.Errorf("Python 配置 %s 必须是递增的两个非负整数", field)
		}
	}
	if len(config.AnswerDatetimeWindow) != 2 || containsPythonNull(raw["answer_datetime_window"]) {
		return ConfigDocument{}, fmt.Errorf("Python 配置 answer_datetime_window 必须包含两个字符串")
	}
	if containsPythonNull(raw["user_agent_ratios"]) || containsPythonNull(raw["answer_rules"]) {
		return ConfigDocument{}, fmt.Errorf("Python 配置 user_agent_ratios 和 answer_rules 不能包含 null")
	}
	strategies := make([]model.QuestionStrategy, 0, len(config.Questions))
	for index, question := range config.Questions {
		strategy, err := decodePythonQuestion(question, index+1)
		if err != nil {
			return ConfigDocument{}, fmt.Errorf("Python 配置 questions[%d]：%w", index+1, err)
		}
		strategies = append(strategies, strategy)
	}
	document := ConfigDocument{
		SchemaVersion: ConfigSchemaVersion,
		Survey:        SurveyDocument{URL: config.URL, Provider: provider, Definition: model.SurveyDefinition{Provider: provider}},
		Execution: model.ExecutionPlan{
			Target: config.Target, Threads: config.Concurrency,
			AnswerDuration:       [2]int{config.AnswerDuration[0], config.AnswerDuration[1]},
			SubmitInterval:       [2]int{config.SubmitInterval[0], config.SubmitInterval[1]},
			AnswerDatetimeWindow: [2]string{config.AnswerDatetimeWindow[0], config.AnswerDatetimeWindow[1]},
		},
		Network:       NetworkSettings{ProxySource: "default", RandomUAEnabled: config.RandomUserAgent, RandomUARatios: config.UserAgentRatios},
		Answers:       model.AnswerPlan{Rules: config.AnswerRules, Strategies: strategies},
		ReverseFill:   model.ReverseFillPlan{Format: ReverseFillFormatAuto, StartRow: 1, Threads: 1},
		Psychometrics: model.PsychometricPolicy{Enabled: config.ReliabilityMode, TargetAlpha: config.TargetAlpha},
	}
	if _, err := RunRequestFromConfigDocument(document); err != nil {
		return ConfigDocument{}, err
	}
	return document, nil
}

func decodePythonQuestion(data []byte, index int) (model.QuestionStrategy, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil || raw == nil {
		return model.QuestionStrategy{}, fmt.Errorf("题目必须是对象")
	}
	if _, ok := raw["ai_enabled"]; ok {
		return model.QuestionStrategy{}, fmt.Errorf("ai_enabled 不属于 Python 序列化配置")
	}
	strategy := model.QuestionStrategy{
		Rows: 1, DistributionMode: "random", TextRandomMode: "none",
		PsychoBias: "custom", SurveyProvider: model.ProviderWJX,
	}
	weights := raw["probabilities"]
	custom := raw["custom_weights"]
	delete(raw, "probabilities")
	delete(raw, "custom_weights")
	remaining, err := json.Marshal(raw)
	if err != nil {
		return strategy, err
	}
	if err := decodePythonObject(remaining, &strategy); err != nil {
		return strategy, err
	}
	if strings.TrimSpace(string(strategy.QuestionType)) == "" {
		return strategy, fmt.Errorf("question_type 必须是非空字符串")
	}
	if strategy.QuestionNum == nil {
		strategy.QuestionNum = &index
	}
	if strategy.Probabilities, err = decodePythonWeights(weights, strategy.QuestionType); err != nil {
		return strategy, fmt.Errorf("probabilities：%w", err)
	}
	if strategy.CustomWeights, err = decodePythonWeights(custom, strategy.QuestionType); err != nil {
		return strategy, fmt.Errorf("custom_weights：%w", err)
	}
	return strategy, nil
}

func decodePythonWeights(data []byte, kind model.QuestionKind) (model.WeightTable, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return model.WeightTable{}, nil
	}
	var value float64
	if json.Unmarshal(data, &value) == nil {
		if value <= 0 && kind != model.QuestionKindSlider {
			return model.WeightTable{}, nil
		}
		return model.OptionWeights(value), nil
	}
	var values []float64
	if json.Unmarshal(data, &values) == nil {
		return model.OptionWeights(values...), nil
	}
	var rows [][]float64
	if err := json.Unmarshal(data, &rows); err != nil {
		return model.WeightTable{}, fmt.Errorf("必须是数字、数组或矩阵权重")
	}
	return model.RowWeights(rows...), nil
}

func decodePythonObject(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func containsPythonNull(data []byte) bool {
	var value any
	if len(data) == 0 || json.Unmarshal(data, &value) != nil {
		return false
	}
	switch values := value.(type) {
	case []any:
		for _, item := range values {
			if item == nil {
				return true
			}
		}
	case map[string]any:
		for _, item := range values {
			if item == nil {
				return true
			}
		}
	}
	return false
}
