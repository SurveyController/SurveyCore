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
)

type AIConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

type AIClient struct {
	config AIConfig
	client *http.Client
}

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

func NewAIClient(config AIConfig) *AIClient {
	return &AIClient{
		config: config,
		client: &http.Client{Timeout: aiRequestTimeout},
	}
}

func (a *AIClient) GenerateAnswer(questionTitle, questionType string, blankCount int) (string, error) {
	if strings.TrimSpace(a.config.APIKey) == "" {
		return "", classifyAIError(AIErrorConfig, fmt.Errorf("API key 未配置"))
	}
	if strings.TrimSpace(a.config.BaseURL) == "" {
		return "", classifyAIError(AIErrorConfig, fmt.Errorf("base_url 未配置"))
	}
	if strings.TrimSpace(a.config.Model) == "" {
		return "", classifyAIError(AIErrorConfig, fmt.Errorf("model 未配置"))
	}

	reqBody := map[string]any{
		"model": strings.TrimSpace(a.config.Model),
		"messages": []map[string]string{
			{"role": "system", "content": "你是一个问卷答题助手，请根据题目生成合理的答案。只输出答案内容，不要解释。"},
			{"role": "user", "content": buildAIPrompt(questionTitle, questionType, blankCount)},
		},
		"temperature": 0.7,
		"max_tokens":  200,
	}

	var lastErr error
	for attempt := 1; attempt <= aiMaxAttempts; attempt++ {
		answer, err := a.doGenerateAPI(reqBody)
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

func (a *AIClient) doGenerateAPI(reqBody map[string]any) (string, error) {
	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", strings.TrimRight(a.config.BaseURL, "/")+"/chat/completions", bytes.NewReader(bodyBytes))
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
	if choices, ok := result["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if message, ok := choice["message"].(map[string]any); ok {
				if content, ok := message["content"].(string); ok {
					return strings.TrimSpace(content), nil
				}
			}
		}
	}

	return "", classifyAIError(AIErrorResponse, fmt.Errorf("响应格式错误"))
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

func buildAIPrompt(questionTitle, questionType string, blankCount int) string {
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
