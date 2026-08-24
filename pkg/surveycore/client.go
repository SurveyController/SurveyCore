package surveycore

import (
	"context"
	"net/http"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
	surveyRuntime "github.com/SurveyController/SurveyCore/pkg/surveycore/runtime"
)

// Client parses surveys, builds default configurations, and submits responses.
// A Client must not be reconfigured after first use. Event callbacks may run
// concurrently when a request uses more than one worker.
type Client struct {
	inner *surveyRuntime.Client
}

type clientOptions struct {
	runtime []surveyRuntime.Option
}

// Option configures a Client.
type Option func(*clientOptions)

// New creates a high-level SurveyCore client.
func New(opts ...Option) *Client {
	options := clientOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return &Client{inner: surveyRuntime.New(options.runtime...)}
}

// WithHTTPClient injects the HTTP client used for provider requests.
func WithHTTPClient(client *http.Client) Option {
	return func(options *clientOptions) {
		options.runtime = append(options.runtime, surveyRuntime.WithHTTPClient(client))
	}
}

// WithAI configures the default AI provider used for generated text answers.
func WithAI(apiKey string, baseURL string, modelName string) Option {
	return func(options *clientOptions) {
		options.runtime = append(options.runtime, surveyRuntime.WithAI(apiKey, baseURL, modelName))
	}
}

// AITextRequest describes one text-answer generation request.
type AITextRequest struct {
	QuestionNum int
	Title       string
	Description string
	BlankCount  int
}

// AITextResolver generates text answers for a question.
type AITextResolver interface {
	ResolveText(ctx context.Context, profile model.AIProfile, persona *model.Persona, request AITextRequest) ([]string, error)
}

// AITextResolverFunc adapts a function to AITextResolver.
type AITextResolverFunc func(ctx context.Context, profile model.AIProfile, persona *model.Persona, request AITextRequest) ([]string, error)

// ResolveText implements AITextResolver.
func (fn AITextResolverFunc) ResolveText(ctx context.Context, profile model.AIProfile, persona *model.Persona, request AITextRequest) ([]string, error) {
	return fn(ctx, profile, persona, request)
}

type aiTextResolverAdapter struct {
	resolver AITextResolver
}

func (adapter aiTextResolverAdapter) ResolveText(ctx context.Context, profile model.AIProfile, persona *model.Persona, request surveyRuntime.AITextRequest) ([]string, error) {
	return adapter.resolver.ResolveText(ctx, profile, persona, AITextRequest(request))
}

// WithAITextResolver replaces the default AI text resolver.
func WithAITextResolver(resolver AITextResolver) Option {
	return func(options *clientOptions) {
		if resolver != nil {
			options.runtime = append(options.runtime, surveyRuntime.WithAITextResolver(aiTextResolverAdapter{resolver: resolver}))
		}
	}
}

// Parse reads a survey definition without submitting a response.
func (client *Client) Parse(ctx context.Context, surveyURL string) (*model.SurveyDefinition, error) {
	return client.inner.Parse(ctx, surveyURL)
}

// DefaultConfig parses a survey and builds a runnable default configuration.
func (client *Client) DefaultConfig(ctx context.Context, surveyURL string) (*model.RunRequest, error) {
	return client.inner.DefaultConfig(ctx, surveyURL)
}

// Run submits responses according to cfg.
func (client *Client) Run(ctx context.Context, cfg *model.RunRequest) (*RunResult, error) {
	return client.inner.Run(ctx, cfg)
}

// EventHandler receives execution progress events. Calls may come from worker goroutines.
type EventHandler func(Event)

// RunWithEvents submits responses and reports progress through handler.
func (client *Client) RunWithEvents(ctx context.Context, cfg *model.RunRequest, handler EventHandler) (*RunResult, error) {
	if handler == nil {
		return client.inner.RunWithEvents(ctx, cfg, nil)
	}
	return client.inner.RunWithEvents(ctx, cfg, func(event surveyRuntime.Event) {
		handler(Event(event))
	})
}

// Parse uses a default Client to read a survey definition.
func Parse(ctx context.Context, surveyURL string) (*model.SurveyDefinition, error) {
	return New().Parse(ctx, surveyURL)
}

// DefaultConfig uses a default Client to build a runnable configuration.
func DefaultConfig(ctx context.Context, surveyURL string) (*model.RunRequest, error) {
	return New().DefaultConfig(ctx, surveyURL)
}

// Run uses a default Client to submit responses.
func Run(ctx context.Context, cfg *model.RunRequest) (*RunResult, error) {
	return New().Run(ctx, cfg)
}

// RunWithEvents uses a default Client to submit responses and report progress.
func RunWithEvents(ctx context.Context, cfg *model.RunRequest, handler EventHandler) (*RunResult, error) {
	return New().RunWithEvents(ctx, cfg, handler)
}

// IsSupportedURL reports whether a URL matches a supported survey provider.
func IsSupportedURL(rawURL string) bool {
	return surveyRuntime.IsSupportedURL(rawURL)
}
