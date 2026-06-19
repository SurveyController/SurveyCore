package surveycore

import (
	"context"

	"github.com/SurveyController/SurveyCore/internal/engine"
	"github.com/SurveyController/SurveyCore/internal/providers"
)

type Client struct {
	registry  engine.ProviderRegistry
	aiAPIKey  string
	aiBaseURL string
	aiModel   string
}

func New(opts ...Option) *Client {
	c := &Client{
		registry: providers.Default(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

func Parse(ctx context.Context, surveyURL string) (*SurveyDefinition, error) {
	return New().Parse(ctx, surveyURL)
}

func DefaultConfig(ctx context.Context, surveyURL string) (*RuntimeConfig, error) {
	return New().DefaultConfig(ctx, surveyURL)
}

func Run(ctx context.Context, cfg *RuntimeConfig) (*RunResult, error) {
	return New().Run(ctx, cfg)
}
