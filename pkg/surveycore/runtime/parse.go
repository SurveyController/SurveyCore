package runtime

import (
	"context"
	"fmt"
	"strings"
)

func (c *Client) Parse(ctx context.Context, surveyURL string) (*SurveyDefinition, error) {
	surveyURL = normalizeSurveyURL(surveyURL)
	if surveyURL == "" {
		return nil, fmt.Errorf("%w: url 不能为空", ErrInvalidConfig)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	parser, err := c.parserFor(surveyURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, detectProvider(surveyURL))
	}
	definition, err := parser.Parse(ctx, surveyURL)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", ErrParseFailed, err)
	}
	definition.SurveyURL = surveyURL
	for index := range definition.Questions {
		question := &definition.Questions[index]
		if question.RowTexts == nil {
			question.RowTexts = []string{}
		}
		if question.OptionTexts == nil {
			question.OptionTexts = []string{}
		}
		if question.TextInputLabels == nil {
			question.TextInputLabels = []string{}
		}
		if question.ForcedTexts == nil {
			question.ForcedTexts = []string{}
		}
		if question.FillableOptions == nil {
			question.FillableOptions = []int{}
		}
	}
	return &definition, nil
}

// normalizeSurveyURL follows the Python parse URL convention. Provider
// dispatch still validates protocols, hosts and supported survey paths.
func normalizeSurveyURL(raw string) string {
	parsed, ok := parseSurveyURL(raw)
	if !ok {
		return strings.TrimSpace(raw)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	if detectProvider(parsed.String()) == ProviderCredamo && strings.HasPrefix(strings.ToLower(parsed.Path), "/s/") && parsed.Fragment == "" {
		parsed.Fragment = parsed.Path
		parsed.Path = "/answer.html"
	}
	return parsed.String()
}
