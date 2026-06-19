package surveycore

import (
	"context"
	"fmt"

	"github.com/SurveyController/SurveyCore/internal/surveyparse"
)

func (c *Client) Parse(ctx context.Context, surveyURL string) (*SurveyDefinition, error) {
	def, err := surveyparse.New(c.registry).Parse(ctx, surveyURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParseFailed, err)
	}
	return def, nil
}
