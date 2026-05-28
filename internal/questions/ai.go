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

// AIConfig holds AI generation configuration.
type AIConfig struct {
	Mode         string // "free", "api"
	Provider     string // "deepseek", etc.
	APIKey       string
	BaseURL      string
	Model        string
	SystemPrompt string
}

// AIClient generates text answers using AI.
type AIClient struct {
	config AIConfig
	client *http.Client
}

// NewAIClient creates a new AI client.
func NewAIClient(config AIConfig) *AIClient {
	return &AIClient{
		config: config,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// GenerateAnswer generates a text answer for a question.
func (a *AIClient) GenerateAnswer(questionTitle, questionType string, blankCount int) (string, error) {
	if a.config.Mode == "" || a.config.Mode == "free" {
		return a.generateFree(questionTitle, questionType, blankCount)
	}
	return a.generateAPI(questionTitle, questionType, blankCount)
}

func (a *AIClient) generateFree(questionTitle, questionType string, blankCount int) (string, error) {
	// Free mode uses a simple heuristic response
	if blankCount > 1 {
		answers := make([]string, blankCount)
		defaults := []string{"非常满意", "服务态度好", "环境优美", "效率高", "质量好"}
		for i := 0; i < blankCount && i < len(defaults); i++ {
			answers[i] = defaults[i]
		}
		return strings.Join(answers, "|"), nil
	}
	return "非常满意", nil
}

func (a *AIClient) generateAPI(questionTitle, questionType string, blankCount int) (string, error) {
	if a.config.APIKey == "" {
		return "", fmt.Errorf("AI API key 未配置")
	}

	prompt := a.buildPrompt(questionTitle, questionType, blankCount)
	systemPrompt := a.config.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = "你是一个问卷答题助手，请根据题目生成合理的答案。只输出答案内容，不要解释。"
	}

	baseURL := a.config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.deepseek.com/v1"
	}
	model := a.config.Model
	if model == "" {
		model = "deepseek-chat"
	}

	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
		"max_tokens":  200,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.config.APIKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("AI 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("AI HTTP %d: %s", resp.StatusCode, truncateString(string(respBody), 200))
	}
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("解析 AI 响应失败: %w", err)
	}

	// Extract content from response
	if choices, ok := result["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if message, ok := choice["message"].(map[string]any); ok {
				if content, ok := message["content"].(string); ok {
					return strings.TrimSpace(content), nil
				}
			}
		}
	}

	return "", fmt.Errorf("AI 响应格式错误")
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
		return fmt.Sprintf("题目：%s\n这是一个包含 %d 个空格的填空题，请为每个空格生成一个答案，用 | 分隔。", cleaned, blankCount)
	}
	return fmt.Sprintf("题目：%s\n请生成一个简短的回答。", cleaned)
}

func cleanQuestionTitle(title string) string {
	// Remove numbering
	cleaned := title
	// Remove common prefixes
	for _, prefix := range []string{"Q:", "q:", "问:", "问题:"} {
		cleaned = strings.TrimPrefix(cleaned, prefix)
	}
	return strings.TrimSpace(cleaned)
}
