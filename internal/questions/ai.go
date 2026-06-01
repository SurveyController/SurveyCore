package questions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	aiRequestTimeout = 12 * time.Second
	aiMaxAttempts    = 4
	aiRetryBackoff   = 400 * time.Millisecond

	aiChatCompletionsSuffix = "/chat/completions"
	aiResponsesSuffix       = "/responses"
	aiLegacyCompletions     = "/completions"
)

// AIConfig holds server-side AI generation configuration.
type AIConfig struct {
	Provider     string
	APIKey       string
	BaseURL      string
	Protocol     string
	Model        string
	SystemPrompt string
}

// AIClient generates text answers using a configured AI provider.
type AIClient struct {
	config AIConfig
	client *http.Client
}

// AIError classifies an AI generation failure for callers and tests.
type AIError struct {
	Kind string
	Err  error
}

func (e *AIError) Error() string {
	if e == nil || e.Err == nil {
		return "AI 调用失败"
	}
	return fmt.Sprintf("AI %s: %v", e.Kind, e.Err)
}

func (e *AIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

const (
	AIErrorConfig   = "config"
	AIErrorTimeout  = "timeout"
	AIErrorHTTP     = "http"
	AIErrorResponse = "response"
	AIErrorNetwork  = "network"
)

// NewAIClient creates a new AI client.
func NewAIClient(config AIConfig) *AIClient {
	return &AIClient{
		config: config,
		client: &http.Client{Timeout: aiRequestTimeout},
	}
}

// GenerateAnswer generates a text answer for a question.
func (a *AIClient) GenerateAnswer(questionTitle, questionType string, blankCount int) (string, error) {
	return a.generateAPI(questionTitle, questionType, blankCount)
}

func (a *AIClient) TestConnection() (string, error) {
	return a.GenerateAnswer("这是一个测试问题，请回复'连接成功'", "fill_blank", 1)
}

func (a *AIClient) generateAPI(questionTitle, questionType string, blankCount int) (string, error) {
	if strings.TrimSpace(a.config.APIKey) == "" {
		return "", classifyAIError(AIErrorConfig, fmt.Errorf("API key 未配置"))
	}

	prompt := a.buildPrompt(questionTitle, questionType, blankCount)
	systemPrompt := strings.TrimSpace(a.config.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = "你是一个问卷答题助手，请根据题目生成合理的答案。只输出答案内容，不要解释。"
	}

	endpoint, err := a.resolveEndpoint()
	if err != nil {
		return "", err
	}

	var lastErr error
	for attempt := 1; attempt <= aiMaxAttempts; attempt++ {
		answer, err := a.doGenerateAPI(endpoint.Protocol, endpoint.URL, endpoint.Model, prompt, systemPrompt)
		if err != nil && endpoint.Protocol == "chat_completions" && endpoint.AutoFallbackToResponses && isEndpointMismatchAIError(err) {
			fallbackURL := strings.TrimRight(endpoint.BaseURL, "/") + aiResponsesSuffix
			answer, err = a.doGenerateAPI("responses", fallbackURL, endpoint.Model, prompt, systemPrompt)
		}
		if err == nil {
			return answer, nil
		}
		lastErr = err
		if attempt >= aiMaxAttempts || !isRetryableAIError(err) {
			break
		}
		time.Sleep(aiRetryBackoff)
	}
	return "", lastErr
}

type aiEndpoint struct {
	Protocol                string
	URL                     string
	BaseURL                 string
	Model                   string
	AutoFallbackToResponses bool
}

func (a *AIClient) resolveEndpoint() (aiEndpoint, error) {
	provider := strings.ToLower(strings.TrimSpace(a.config.Provider))
	model := strings.TrimSpace(a.config.Model)
	baseURL := normalizeAIEndpointURL(a.config.BaseURL)
	protocol := normalizeAIProtocol(a.config.Protocol)

	if provider == "custom" {
		if baseURL == "" {
			return aiEndpoint{}, classifyAIError(AIErrorConfig, fmt.Errorf("自定义模式需要配置 Base URL"))
		}
		if model == "" {
			return aiEndpoint{}, classifyAIError(AIErrorConfig, fmt.Errorf("自定义模式需要配置模型名称"))
		}
		return resolveCustomAIEndpoint(baseURL, protocol, model)
	}

	if baseURL == "" {
		baseURL = "https://api.deepseek.com/v1"
	}
	if model == "" {
		model = "deepseek-chat"
	}
	if endpoint, err := resolveExplicitAIEndpoint(baseURL, protocol, model); err == nil {
		return endpoint, nil
	} else if strings.HasSuffix(strings.ToLower(strings.TrimRight(baseURL, "/")), aiLegacyCompletions) {
		return aiEndpoint{}, err
	}
	return aiEndpoint{
		Protocol: "chat_completions",
		URL:      strings.TrimRight(baseURL, "/") + aiChatCompletionsSuffix,
		BaseURL:  baseURL,
		Model:    model,
	}, nil
}

func resolveCustomAIEndpoint(baseURL string, protocol string, model string) (aiEndpoint, error) {
	if endpoint, err := resolveExplicitAIEndpoint(baseURL, protocol, model); err == nil {
		return endpoint, nil
	} else if strings.HasSuffix(strings.ToLower(strings.TrimRight(baseURL, "/")), aiLegacyCompletions) {
		return aiEndpoint{}, err
	}

	if protocol == "responses" {
		return aiEndpoint{
			Protocol: "responses",
			URL:      strings.TrimRight(baseURL, "/") + aiResponsesSuffix,
			BaseURL:  baseURL,
			Model:    model,
		}, nil
	}
	return aiEndpoint{
		Protocol:                "chat_completions",
		URL:                     strings.TrimRight(baseURL, "/") + aiChatCompletionsSuffix,
		BaseURL:                 baseURL,
		Model:                   model,
		AutoFallbackToResponses: protocol == "auto",
	}, nil
}

func resolveExplicitAIEndpoint(baseURL string, protocol string, model string) (aiEndpoint, error) {
	path := strings.ToLower(strings.TrimRight(baseURL, "/"))
	switch {
	case strings.HasSuffix(path, aiChatCompletionsSuffix):
		return aiEndpoint{Protocol: "chat_completions", URL: baseURL, BaseURL: trimAISuffix(baseURL, aiChatCompletionsSuffix), Model: model}, nil
	case strings.HasSuffix(path, aiResponsesSuffix):
		return aiEndpoint{Protocol: "responses", URL: baseURL, BaseURL: trimAISuffix(baseURL, aiResponsesSuffix), Model: model}, nil
	case strings.HasSuffix(path, aiLegacyCompletions):
		return aiEndpoint{}, classifyAIError(AIErrorConfig, fmt.Errorf("暂不支持旧版 /completions 协议，请改用 /chat/completions 或 /responses"))
	}
	return aiEndpoint{}, classifyAIError(AIErrorConfig, fmt.Errorf("未配置完整 AI endpoint"))
}

func normalizeAIEndpointURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func normalizeAIProtocol(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "chat_completions", "responses":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "auto"
	}
}

func trimAISuffix(rawURL string, suffix string) string {
	trimmed := strings.TrimRight(rawURL, "/")
	if len(trimmed) < len(suffix) {
		return trimmed
	}
	return trimmed[:len(trimmed)-len(suffix)]
}

func (a *AIClient) doGenerateAPI(protocol string, url string, model string, prompt string, systemPrompt string) (string, error) {
	reqBody := buildAIRequestBody(protocol, model, prompt, systemPrompt)
	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", classifyAIError(AIErrorConfig, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(a.config.APIKey))

	resp, err := a.client.Do(req)
	if err != nil {
		if isTimeoutError(err) {
			return "", classifyAIError(AIErrorTimeout, err)
		}
		return "", classifyAIError(AIErrorNetwork, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		kind := AIErrorHTTP
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			kind = AIErrorConfig
		}
		return "", classifyAIError(kind, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateString(string(respBody), 200)))
	}
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", classifyAIError(AIErrorResponse, err)
	}

	answer, err := extractAIResponseText(protocol, result)
	if err != nil {
		return "", classifyAIError(AIErrorResponse, err)
	}
	return answer, nil
}

func buildAIRequestBody(protocol string, model string, prompt string, systemPrompt string) map[string]any {
	if protocol == "responses" {
		return map[string]any{
			"model":             model,
			"instructions":      systemPrompt,
			"input":             prompt,
			"temperature":       0.7,
			"max_output_tokens": 200,
		}
	}
	return map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
		"max_tokens":  200,
	}
}

func extractAIResponseText(protocol string, result map[string]any) (string, error) {
	if protocol == "responses" {
		if text, ok := result["output_text"].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text), nil
		}
		if output, ok := result["output"].([]any); ok {
			for _, item := range output {
				itemMap, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if text := joinAITextParts(itemMap["content"]); text != "" {
					return text, nil
				}
			}
		}
		return "", fmt.Errorf("Responses API 返回内容为空")
	}

	if choices, ok := result["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if message, ok := choice["message"].(map[string]any); ok {
				if text := joinAITextParts(message["content"]); text != "" {
					return text, nil
				}
			}
		}
	}
	return "", fmt.Errorf("响应格式错误")
}

func joinAITextParts(content any) string {
	switch value := content.(type) {
	case string:
		return strings.TrimSpace(value)
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			switch part := item.(type) {
			case string:
				if text := strings.TrimSpace(part); text != "" {
					parts = append(parts, text)
				}
			case map[string]any:
				itemType := strings.ToLower(strings.TrimSpace(fmt.Sprint(part["type"])))
				text := strings.TrimSpace(fmt.Sprint(firstNonEmpty(part["text"], part["content"])))
				if text != "" && (itemType == "text" || itemType == "output_text" || itemType == "input_text") {
					parts = append(parts, text)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return ""
	}
}

func firstNonEmpty(values ...any) any {
	for _, value := range values {
		if strings.TrimSpace(fmt.Sprint(value)) != "" && fmt.Sprint(value) != "<nil>" {
			return value
		}
	}
	return ""
}

func classifyAIError(kind string, err error) error {
	return &AIError{Kind: kind, Err: err}
}

func isRetryableAIError(err error) bool {
	if err == nil {
		return false
	}
	aiErr, ok := err.(*AIError)
	if !ok {
		return true
	}
	switch aiErr.Kind {
	case AIErrorConfig, AIErrorResponse:
		return false
	default:
		return true
	}
}

func isEndpointMismatchAIError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, marker := range []string{"404", "405", "410", "not found", "no route", "no handler", "unsupported path", "invalid url", "method not allowed"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "timeout") || strings.Contains(text, "timed out") || strings.Contains(text, "超时")
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (a *AIClient) buildPrompt(questionTitle, questionType string, blankCount int) string {
	cleaned := cleanQuestionTitle(questionTitle)
	if blankCount > 1 {
		return fmt.Sprintf("题目：%s\n题型：%s\n这是一个包含 %d 个空格的填空题，请为每个空格生成一个答案，用 | 分隔。", cleaned, questionType, blankCount)
	}
	return fmt.Sprintf("题目：%s\n题型：%s\n请生成一个简短的回答。", cleaned, questionType)
}

func cleanQuestionTitle(title string) string {
	cleaned := title
	for _, prefix := range []string{"Q:", "q:", "问:", "问题:"} {
		cleaned = strings.TrimPrefix(cleaned, prefix)
	}
	return strings.TrimSpace(cleaned)
}
