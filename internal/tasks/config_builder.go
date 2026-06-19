package tasks

import (
	"context"

	"github.com/SurveyController/SurveyCore/internal/models"
	"github.com/SurveyController/SurveyCore/internal/surveyparse"
)

func (m *TaskManager) BuildDefaultConfig(ctx context.Context, surveyURL string) (*models.RuntimeConfig, error) {
	return surveyparse.New(m.registry).DefaultConfig(ctx, surveyURL)
}
