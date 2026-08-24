package runtime

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/internal/httpjson"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/internal/providers/credamo"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/internal/providers/tencent"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/internal/providers/wjx"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
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
	parsed, ok := parseSurveyURL(rawURL)
	if !ok {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	path := parsed.Path
	if isHost(host, "wjx.cn", "wjx.com", "wjx.top") {
		return wjxSurveyPathRE.MatchString(path)
	}
	if host == "wj.qq.com" {
		return regexp.MustCompile(`(?i)^/s\d+/\d+/[A-Za-z0-9_-]+/?$`).MatchString(path)
	}
	if isHost(host, "credamo.com", "credamo.cn") {
		if regexp.MustCompile(`(?i)^/s/[A-Za-z0-9_-]+/?$`).MatchString(path) {
			return true
		}
		if !strings.EqualFold(strings.TrimRight(path, "/"), "/answer.html") {
			return false
		}
		fragment := strings.Trim(strings.SplitN(strings.TrimSpace(parsed.Fragment), "?", 2)[0], "/")
		parts := strings.Split(fragment, "/")
		return len(parts) == 2 && strings.EqualFold(parts[0], "s") && regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(parts[1])
	}
	return false
}

func surveyHostPath(raw string) (string, string) {
	parsed, ok := parseSurveyURL(raw)
	if !ok {
		return "", ""
	}
	return strings.ToLower(parsed.Hostname()), parsed.Path
}

var wjxSurveyPathRE = regexp.MustCompile(`(?i)^/(?:vm|m|vj|v|s)/[A-Za-z0-9_-]+(?:\.aspx)?/?$`)

func parseSurveyURL(raw string) (*url.URL, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, false
	}
	// Keep accepting the historical scheme-less QR form, but validate every
	// explicit scheme instead of dispatching from arbitrary string fragments.
	if !strings.Contains(text, "://") {
		text = "https://" + text
	}
	parsed, err := url.Parse(text)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil {
		return nil, false
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return nil, false
	}
	return parsed, true
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
	parsed, ok := parseSurveyURL(rawURL)
	if !ok {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	path := parsed.Path
	switch {
	case isHost(host, "credamo.com", "credamo.cn") && credamoSurveyPath(parsed):
		return model.ProviderCredamo
	case (host == "127.0.0.1" || host == "localhost") && credamoSurveyPath(parsed):
		return model.ProviderCredamo
	case host == "wj.qq.com" && regexp.MustCompile(`(?i)^/s\d+/\d+/[A-Za-z0-9_-]+/?$`).MatchString(path):
		return model.ProviderQQ
	case isHost(host, "wjx.cn", "wjx.com", "wjx.top") && wjxSurveyPathRE.MatchString(path):
		return model.ProviderWJX
	default:
		return ""
	}
}

func credamoSurveyPath(parsed *url.URL) bool {
	if parsed == nil {
		return false
	}
	path := parsed.Path
	if regexp.MustCompile(`(?i)^/s/[A-Za-z0-9_-]+/?$`).MatchString(path) {
		return true
	}
	if !strings.EqualFold(strings.TrimRight(path, "/"), "/answer.html") {
		return false
	}
	fragment := strings.Trim(strings.SplitN(strings.TrimSpace(parsed.Fragment), "?", 2)[0], "/")
	parts := strings.Split(fragment, "/")
	return len(parts) == 2 && strings.EqualFold(parts[0], "s") && regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(parts[1])
}
