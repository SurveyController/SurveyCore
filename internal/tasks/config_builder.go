package tasks

import (
	"context"

	"github.com/SurveyController/SurveyCore/internal/config"
	"github.com/SurveyController/SurveyCore/internal/models"
)

func (m *TaskManager) BuildDefaultConfig(ctx context.Context, surveyURL string) (*models.RuntimeConfig, error) {
	cfg := models.NewDefaultRuntimeConfig()
	cfg.URL = surveyURL
	if cfg.URL == "" {
		return &cfg, nil
	}
	def, err := m.ParseSurvey(ctx, cfg.URL)
	if err != nil {
		return nil, err
	}
	cfg.SurveyTitle = def.Title
	cfg.SurveyProvider = def.Provider
	cfg.QuestionsInfo = models.CloneSurveyQuestionMetas(def.Questions)
	cfg.QuestionEntries = config.BuildDefaultQuestionEntries(def.Questions, nil)
	return &cfg, nil
}
