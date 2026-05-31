package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/SurveyController/SurveyCore/internal/questions"
)

type aiTestRequest struct {
	AIMode         string `json:"ai_mode,omitempty"`
	AIProvider     string `json:"ai_provider,omitempty"`
	AIAPIKey       string `json:"ai_api_key,omitempty"`
	AIBaseURL      string `json:"ai_base_url,omitempty"`
	AIAPIProtocol  string `json:"ai_api_protocol,omitempty"`
	AIModel        string `json:"ai_model,omitempty"`
	AISystemPrompt string `json:"ai_system_prompt,omitempty"`
	AIFreeEndpoint string `json:"ai_free_endpoint,omitempty"`
	RandomIPUserID int    `json:"random_ip_user_id,omitempty"`
	RandomIPDevice string `json:"random_ip_device_id,omitempty"`
	Question       string `json:"question,omitempty"`
}

func (s *Server) handleTestAI(w http.ResponseWriter, r *http.Request) {
	var req aiTestRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "JSON 请求体无效", err)
		return
	}

	question := strings.TrimSpace(req.Question)
	if question == "" {
		question = "这是一个测试问题，请回复'连接成功'"
	}
	client := questions.NewAIClient(questions.AIConfig{
		Mode:         req.AIMode,
		Provider:     req.AIProvider,
		APIKey:       req.AIAPIKey,
		BaseURL:      req.AIBaseURL,
		Protocol:     req.AIAPIProtocol,
		Model:        req.AIModel,
		SystemPrompt: req.AISystemPrompt,
		FreeEndpoint: req.AIFreeEndpoint,
		FreeUserID:   req.RandomIPUserID,
		FreeDeviceID: req.RandomIPDevice,
		StrictFree:   isFreeAIMode(req.AIMode),
	})
	preview, err := client.GenerateAnswer(question, "fill_blank", 1)
	if err != nil {
		status := http.StatusBadGateway
		code := "ai_connection_failed"
		var aiErr *questions.AIError
		if errors.As(err, &aiErr) && aiErr.Kind == questions.AIErrorConfig {
			status = http.StatusBadRequest
			code = "ai_config_error"
		}
		writeError(w, status, code, "AI 连接测试失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "AI 连接测试成功",
		"preview": preview,
	})
}

func isFreeAIMode(mode string) bool {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	return normalized == "" || normalized == "free"
}
