package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SurveyController/SurveyController-Go/internal/models"
)

// LoadFile loads a RuntimeConfig from a JSON file.
func LoadFile(path string) (*models.RuntimeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	cfg, err := models.DeserializeRuntimeConfig(data)
	if err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	return cfg, nil
}

// SaveFile saves a RuntimeConfig to a JSON file.
func SaveFile(cfg *models.RuntimeConfig, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	data, err := models.SerializeRuntimeConfig(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	return nil
}

// MergeDefaults fills in missing fields with sensible defaults.
func MergeDefaults(cfg *models.RuntimeConfig) {
	defaults := models.NewDefaultRuntimeConfig()
	if cfg.SurveyProvider == "" {
		cfg.SurveyProvider = defaults.SurveyProvider
	}
	if cfg.Target <= 0 {
		cfg.Target = defaults.Target
	}
	if cfg.Threads <= 0 {
		cfg.Threads = defaults.Threads
	}
	if cfg.ProxySource == "" {
		cfg.ProxySource = defaults.ProxySource
	}
	if cfg.AIMode == "" {
		cfg.AIMode = defaults.AIMode
	}
	if cfg.AIProvider == "" {
		cfg.AIProvider = defaults.AIProvider
	}
	if cfg.AIAPIProtocol == "" {
		cfg.AIAPIProtocol = defaults.AIAPIProtocol
	}
	if cfg.PsychoTargetAlpha == 0 {
		cfg.PsychoTargetAlpha = defaults.PsychoTargetAlpha
	}
	if cfg.RandomUAKeys == nil {
		cfg.RandomUAKeys = defaults.RandomUAKeys
	}
	if cfg.RandomUARatios == nil {
		cfg.RandomUARatios = defaults.RandomUARatios
	}
}

// BuildExecutionConfig creates an ExecutionConfig from a RuntimeConfig.
func BuildExecutionConfig(cfg *models.RuntimeConfig, questions []models.SurveyQuestionMeta) *models.ExecutionConfig {
	ec := &models.ExecutionConfig{
		URL:            cfg.URL,
		SurveyTitle:    cfg.SurveyTitle,
		SurveyProvider: cfg.SurveyProvider,
		NumThreads:     cfg.Threads,
		TargetNum:      cfg.Target,
		FailThreshold:  5,
		StopOnFailEnabled: cfg.FailStopEnabled,
		SubmitIntervalRangeSeconds: cfg.SubmitInterval,
		AnswerDurationRangeSeconds: cfg.AnswerDuration,
		RandomProxyIPEnabled:       cfg.RandomIPEnabled,
		ProxySource:                cfg.ProxySource,
		RandomUserAgentEnabled:     cfg.RandomUAEnabled,
		UserAgentRatios:            cfg.RandomUARatios,
		PauseOnAliyunCaptcha:       cfg.PauseOnAliyunCaptcha,
		PsychoTargetAlpha:          cfg.PsychoTargetAlpha,
		AnswerRules:                cfg.AnswerRules,
	}

	// Build question metadata map
	ec.QuestionsMetadata = make(map[int]models.SurveyQuestionMeta)
	ec.ProviderQuestionMetadataMap = make(map[string]models.SurveyQuestionMeta)
	for _, q := range questions {
		ec.QuestionsMetadata[q.Num] = q
		key := models.MakeProviderQuestionKey(q.Provider, q.ProviderPageID, q.ProviderQuestionID)
		if key != "" {
			ec.ProviderQuestionMetadataMap[key] = q
		}
	}

	// Build config index maps
	ec.QuestionConfigIndexMap = make(map[int]string)
	ec.ProviderQuestionConfigIndexMap = make(map[string]string)
	for i, entry := range cfg.QuestionEntries {
		if entry.QuestionNum != nil {
			ec.QuestionConfigIndexMap[*entry.QuestionNum] = fmt.Sprintf("%d", i)
		}
		if entry.ProviderQuestionID != nil {
			key := models.MakeProviderQuestionKey(entry.SurveyProvider, safeStr(entry.ProviderPageID), *entry.ProviderQuestionID)
			ec.ProviderQuestionConfigIndexMap[key] = fmt.Sprintf("%d", i)
		}
	}

	// Build probability arrays
	for _, entry := range cfg.QuestionEntries {
		switch entry.QuestionType {
		case "single", "scale", "score", "droplist":
			ec.SingleProb = append(ec.SingleProb, entry.Probabilities)
		case "multiple":
			if probs, ok := toFloat64Slice2D(entry.Probabilities); ok {
				ec.MultipleProb = append(ec.MultipleProb, probs)
			}
		case "matrix":
			ec.MatrixProb = append(ec.MatrixProb, entry.Probabilities)
		}
		if len(entry.Texts) > 0 {
			ec.Texts = append(ec.Texts, entry.Texts)
		}
	}

	return ec
}

func safeStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func toFloat64Slice2D(v any) ([]float64, bool) {
	if v == nil {
		return nil, false
	}
	switch sl := v.(type) {
	case []float64:
		return sl, true
	case []any:
		result := make([]float64, 0, len(sl))
		for _, item := range sl {
			if f, ok := item.(float64); ok {
				result = append(result, f)
			}
		}
		return result, true
	}
	return nil, false
}

// PrettyPrint prints a config as formatted JSON.
func PrettyPrint(cfg *models.RuntimeConfig) string {
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return string(data)
}
