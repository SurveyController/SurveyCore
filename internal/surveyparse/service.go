package surveyparse

import (
	"context"
	"fmt"
	"strings"

	"github.com/SurveyController/SurveyCore/internal/config"
	"github.com/SurveyController/SurveyCore/internal/engine"
	"github.com/SurveyController/SurveyCore/internal/models"
)

type Service struct {
	registry engine.ProviderRegistry
}

func New(registry engine.ProviderRegistry) *Service {
	return &Service{registry: registry}
}

func (s *Service) Parse(ctx context.Context, surveyURL string) (*models.SurveyDefinition, error) {
	surveyURL = strings.TrimSpace(surveyURL)
	if surveyURL == "" {
		return nil, fmt.Errorf("url 不能为空")
	}
	return engine.NewEngine(s.registry, nil).ParseSurvey(ctx, surveyURL)
}

func (s *Service) DefaultConfig(ctx context.Context, surveyURL string) (*models.RuntimeConfig, error) {
	cfg := models.NewDefaultRuntimeConfig()
	cfg.URL = strings.TrimSpace(surveyURL)
	if cfg.URL == "" {
		return &cfg, nil
	}
	def, err := s.Parse(ctx, cfg.URL)
	if err != nil {
		return nil, err
	}
	cfg.SurveyTitle = def.Title
	cfg.SurveyProvider = def.Provider
	cfg.QuestionsInfo = models.CloneSurveyQuestionMetas(def.Questions)
	cfg.QuestionEntries = config.BuildDefaultQuestionEntries(def.Questions, nil)
	return &cfg, nil
}
