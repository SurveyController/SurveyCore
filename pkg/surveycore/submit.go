package surveycore

import (
	"context"
	"fmt"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

// SubmissionRequest contains a run configuration and an optional parsed survey.
// Config target and concurrency do not change Submit's one-attempt behavior.
// This differs from model.SubmissionRequest, the experimental provider input.
type SubmissionRequest struct {
	Config *model.RunRequest       `json:"config"`
	Survey *model.SurveyDefinition `json:"survey,omitempty"`
}

// SubmissionResult reports one attempt's outcome, duration, and optional error.
type SubmissionResult = model.SubmissionResult

// Submit executes one submission. Missing configurations or incompatible providers
// return a nil result and a typed error before any network request. Failures return
// both an unsuccessful result and the typed cause, including context cancellation.
func (client *Client) Submit(ctx context.Context, request *SubmissionRequest) (*SubmissionResult, error) {
	if request == nil || request.Config == nil {
		return nil, fmt.Errorf("%w: 配置不能为空", ErrInvalidConfig)
	}
	cfg := *request.Config
	if request.Survey != nil {
		cfg.SurveyDefinition = model.CloneSurveyDefinition(*request.Survey)
	}
	return client.inner.Submit(ctx, &cfg)
}

// Submit uses a default Client to execute one submission.
func Submit(ctx context.Context, request *SubmissionRequest) (*SubmissionResult, error) {
	return New().Submit(ctx, request)
}
