package surveycore

import (
	"strings"

	"github.com/SurveyController/SurveyCore/internal/execution"
)

type Option func(*Client)

func WithAI(apiKey, baseURL, model string) Option {
	return func(c *Client) {
		c.aiAPIKey = strings.TrimSpace(apiKey)
		c.aiBaseURL = strings.TrimSpace(baseURL)
		c.aiModel = strings.TrimSpace(model)
	}
}

func (c *Client) applyExecutionDefaults(cfg *execution.ExecutionConfig) {
	if cfg == nil {
		return
	}
	if cfg.AIAPIKey == "" {
		cfg.AIAPIKey = c.aiAPIKey
	}
	if cfg.AIBaseURL == "" {
		cfg.AIBaseURL = c.aiBaseURL
	}
	if cfg.AIModel == "" {
		cfg.AIModel = c.aiModel
	}
}
