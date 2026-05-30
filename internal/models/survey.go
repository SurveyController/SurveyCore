package models

import (
	"context"

	"github.com/SurveyController/SurveyConsole/internal/domain"
)

type FillOptions = domain.FillOptions
type SurveyQuestionMeta = domain.SurveyQuestionMeta
type SurveyDefinition = domain.SurveyDefinition

const (
	LogicParseStatusComplete = domain.LogicParseStatusComplete
	LogicParseStatusNone     = domain.LogicParseStatusNone
	LogicParseStatusUnknown  = domain.LogicParseStatusUnknown
)

// ProviderAdapter defines the interface that all survey providers must implement.
type ProviderAdapter interface {
	ProviderName() string
	ParseSurvey(ctx context.Context, url string) (*SurveyDefinition, error)
	FillSurveyHTTP(ctx context.Context, cfg *ExecutionConfig, state *ExecutionState, opts FillOptions) (bool, error)
}

func CloneSurveyQuestionMetas(src []SurveyQuestionMeta) []SurveyQuestionMeta {
	return domain.CloneSurveyQuestionMetas(src)
}
