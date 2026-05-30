package tencent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/SurveyController/SurveyConsole/internal/models"
	"github.com/SurveyController/SurveyConsole/internal/network/httpclient"
	"github.com/SurveyController/SurveyConsole/internal/providers/providerutil"
)

const (
	ProviderName = "qq"
	defaultUA    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"
	apiBase      = "https://wj.qq.com/api/v2/respondent/surveys"
	localeOrder  = "zhs,zht,zh,en"
)

// Provider implements the Tencent survey provider.
type Provider struct{}

func NewProvider() *Provider { return &Provider{} }

func (p *Provider) ProviderName() string { return ProviderName }

// ParseSurvey fetches and parses a Tencent survey.
func (p *Provider) ParseSurvey(ctx context.Context, surveyURL string) (*models.SurveyDefinition, error) {
	surveyID, hashValue, err := extractQQIdentifiers(surveyURL)
	if err != nil {
		return nil, err
	}

	// Establish session, matching the Python runtime's initial handshake.
	_, cookieHeader, err := fetchSession(ctx, surveyID, hashValue)
	if err != nil {
		return nil, fmt.Errorf("建立会话失败: %w", err)
	}

	// Fetch questions without X-Answer-Session. Tencent rejects that header on
	// some parse-time requests with INVALIDARGUMENT.
	rawQuestions, title, err := fetchRawQuestionsWithSession(ctx, surveyID, hashValue, "", cookieHeader)
	if err != nil {
		return nil, fmt.Errorf("获取题目失败: %w", err)
	}
	questions := standardizeQuestions(rawQuestions)

	return &models.SurveyDefinition{
		Provider:  ProviderName,
		Title:     title,
		Questions: questions,
	}, nil
}

// FillSurveyHTTP submits a Tencent survey response via HTTP.
func (p *Provider) FillSurveyHTTP(ctx context.Context, cfg *models.ExecutionConfig, state *models.ExecutionState, opts models.FillOptions) (bool, error) {
	if state.IsStopped() {
		return false, nil
	}

	surveyID, hashValue, err := extractQQIdentifiers(cfg.URL)
	if err != nil {
		return false, err
	}

	// Fetch session and extract answer_session_id
	answerSessionID, cookieHeader, err := fetchSession(ctx, surveyID, hashValue)
	if err != nil {
		return false, fmt.Errorf("建立会话失败: %w", err)
	}

	// Fetch raw questions with session header
	rawQuestions, _, err := fetchRawQuestionsWithSession(ctx, surveyID, hashValue, answerSessionID, cookieHeader)
	if err != nil {
		return false, fmt.Errorf("获取题目失败: %w", err)
	}

	// Build answer actions
	actions, err := buildAnswerActions(cfg, state, rawQuestions, opts.ThreadName)
	if err != nil {
		return false, err
	}

	// Build submit body
	ua := opts.UserAgent
	if ua == "" {
		ua = defaultUA
	}
	duration := providerutil.SampleAnswerDurationSeconds(cfg, 60, 60)

	submitBody := buildSubmitBody(surveyID, hashValue, rawQuestions, actions, duration, ua)

	// Submit
	submitURL := fmt.Sprintf("%s/%s/answers", apiBase, surveyID)
	bodyBytes, _ := json.Marshal(submitBody)
	params := fmt.Sprintf("pv_uid=%s&hash=%s&_=%d", generateUUID(), hashValue, time.Now().UnixMilli())
	submitURL = submitURL + "?" + params

	proxyAddr := parseProxy(opts.ProxyAddress)
	submitHeaders := map[string]string{
		"User-Agent":   ua,
		"Content-Type": "application/json",
		"Referer":      fmt.Sprintf("https://wj.qq.com/s2/%s/%s/", surveyID, hashValue),
		"Origin":       "https://wj.qq.com",
	}
	if answerSessionID != "" {
		submitHeaders["X-Answer-Session"] = answerSessionID
	}
	if cookieHeader != "" {
		submitHeaders["Cookie"] = cookieHeader
	}
	resp, err := httpclient.Post(ctx, submitURL, string(bodyBytes), submitHeaders, proxyAddr, 20*time.Second)
	if err != nil {
		return false, fmt.Errorf("提交失败: %w", err)
	}

	// Check response
	var result map[string]any
	if err := json.Unmarshal(resp.Body, &result); err == nil {
		if classifyQQSubmitPayload(result) == SubmitSuccess {
			return true, nil
		}
		if msg, ok := result["message"].(string); ok {
			return false, fmt.Errorf("提交被拒绝: %s", msg)
		}
	}

	// Check if response contains success indicators
	text := string(resp.Body)
	if strings.Contains(text, `"code":0`) || strings.Contains(text, `"code":"OK"`) || strings.Contains(text, `"success"`) {
		return true, nil
	}

	return false, fmt.Errorf("提交失败: %s", truncate(text, 200))
}
