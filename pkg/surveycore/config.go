package surveycore

import (
	"context"

	"github.com/SurveyController/SurveyCore/internal/surveyparse"
)

func (c *Client) DefaultConfig(ctx context.Context, surveyURL string) (*RuntimeConfig, error) {
	return surveyparse.New(c.registry).DefaultConfig(ctx, surveyURL)
}
