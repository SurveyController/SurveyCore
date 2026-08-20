package surveycore

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/credamo"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/internal/httpjson"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/internal/model"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/tencent"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/wjx"
)

type Client struct {
	httpClient             HTTPClient
	aiAPIKey               string
	aiBaseURL              string
	aiModel                string
	aiTextResolver         AITextResolver
	freeAIIdentityProvider FreeAIIdentityProvider
}

type Option func(*Client)

func New(opts ...Option) *Client {
	c := &Client{}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = HTTPClient{Client: client}
	}
}

func WithAI(apiKey string, baseURL string, modelName string) Option {
	return func(c *Client) {
		c.aiAPIKey = strings.TrimSpace(apiKey)
		c.aiBaseURL = strings.TrimSpace(baseURL)
		c.aiModel = strings.TrimSpace(modelName)
	}
}

func WithAITextResolver(resolver AITextResolver) Option {
	return func(c *Client) {
		c.aiTextResolver = resolver
	}
}

func WithFreeAIIdentityProvider(provider FreeAIIdentityProvider) Option {
	return func(c *Client) {
		c.freeAIIdentityProvider = provider
	}
}

func Parse(ctx context.Context, surveyURL string) (*SurveyDefinition, error) {
	return New().Parse(ctx, surveyURL)
}

func DefaultConfig(ctx context.Context, surveyURL string) (*RunRequest, error) {
	return New().DefaultConfig(ctx, surveyURL)
}

func Run(ctx context.Context, cfg *RunRequest) (*RunResult, error) {
	return New().Run(ctx, cfg)
}

func IsSupportedURL(rawURL string) bool {
	host, path := surveyHostPath(rawURL)
	if host == "" {
		return false
	}
	if isHost(host, "wjx.cn", "wjx.com", "wjx.top") {
		return true
	}
	if host == "wj.qq.com" {
		return regexp.MustCompile(`(?i)^/s\d+/\d+/[A-Za-z0-9_-]+/?$`).MatchString(path)
	}
	if isHost(host, "credamo.com", "credamo.cn") {
		return strings.HasPrefix(strings.ToLower(path), "/answer.html") || regexp.MustCompile(`(?i)^/s/[A-Za-z0-9_-]+/?$`).MatchString(path)
	}
	return false
}

func surveyHostPath(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}
	return strings.ToLower(u.Hostname()), u.Path
}
func isHost(host string, domains ...string) bool {
	for _, d := range domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func (c *Client) parserFor(url string) (Parser, error) {
	switch detectProvider(url) {
	case model.ProviderCredamo:
		return credamo.Parser{HTTP: httpClientOrDefault(c.httpClient)}, nil
	case model.ProviderQQ:
		return tencent.Parser{HTTP: httpClientOrDefault(c.httpClient)}, nil
	case model.ProviderWJX:
		return wjx.Parser{Client: c.httpClient.Client}, nil
	default:
		return nil, ErrUnsupportedOperation
	}
}

func httpClientOrDefault(client HTTPClient) httpjson.Client {
	return httpjson.Client{Client: client.Client}
}

func detectProvider(rawURL string) string {
	lowered := strings.ToLower(strings.TrimSpace(rawURL))
	switch {
	case strings.Contains(lowered, "credamo.com") || strings.Contains(lowered, "credamo.cn"):
		return model.ProviderCredamo
	case strings.Contains(lowered, "127.0.0.1") || strings.Contains(lowered, "localhost"):
		if strings.Contains(lowered, "/s/") || strings.Contains(lowered, "#/s/") {
			return model.ProviderCredamo
		}
		return ""
	case strings.Contains(lowered, "wj.qq.com"):
		return model.ProviderQQ
	case strings.Contains(lowered, "wjx.cn") || strings.Contains(lowered, "wjx.com") || strings.Contains(lowered, "wjx.top"):
		return model.ProviderWJX
	default:
		return ""
	}
}
