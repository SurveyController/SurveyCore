package credamo

import (
	"context"
	"fmt"
	"time"

	"github.com/SurveyController/SurveyConsole/internal/models"
	"github.com/SurveyController/SurveyConsole/internal/providers/providerutil"
)

const (
	ProviderName = "credamo"
	defaultUA    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	signCipher   = "P96D0A7D0M8C3R2D0M1"
	apiOrigin    = "https://www.credamo.com"
	resolution   = "1920px*1080px"
)

// Provider implements the Credamo survey provider.
type Provider struct{}

func NewProvider() *Provider { return &Provider{} }

func (p *Provider) ProviderName() string { return ProviderName }

// ParseSurvey fetches and parses a Credamo survey.
func (p *Provider) ParseSurvey(ctx context.Context, surveyURL string) (*models.SurveyDefinition, error) {
	shortURL := extractShortURL(surveyURL)
	if shortURL == "" {
		return nil, fmt.Errorf("无法从 URL 提取 Credamo short URL: %s", surveyURL)
	}

	detail, err := fetchDetail(ctx, shortURL)
	if err != nil {
		return nil, fmt.Errorf("获取问卷详情失败: %w", err)
	}

	title := getString(detail, "title")
	rawQuestions := iterRawQuestions(detail)

	normalized := standardizeQuestions(rawQuestions)

	return &models.SurveyDefinition{
		Provider:  ProviderName,
		Title:     title,
		Questions: normalized,
	}, nil
}

// FillSurveyHTTP submits a Credamo survey response via HTTP.
func (p *Provider) FillSurveyHTTP(ctx context.Context, cfg *models.ExecutionConfig, state *models.ExecutionState, opts models.FillOptions) (bool, error) {
	if state.IsStopped() {
		return false, nil
	}

	shortURL := extractShortURL(cfg.URL)
	if shortURL == "" {
		return false, fmt.Errorf("无法提取 short URL")
	}

	// Fetch detail
	detail, cookieHeader, err := fetchDetailSession(ctx, shortURL, "")
	if err != nil {
		return false, fmt.Errorf("获取详情失败: %w", err)
	}

	rawQuestions := iterRawQuestions(detail)

	// Init answer session
	initData, initCookieHeader, err := initAnswer(ctx, shortURL, cookieHeader)
	if err != nil {
		return false, fmt.Errorf("初始化答题会话失败: %w", err)
	}
	cookieHeader = mergeCookieHeaders(cookieHeader, initCookieHeader)

	// Build actions
	actions, err := buildAnswerActions(cfg, state, opts.ThreadName)
	if err != nil {
		return false, err
	}

	// Build submit body
	duration := providerutil.SampleAnswerDurationSeconds(cfg, 9, 16)
	startTimeMS := sampleAnswerStartTimeMS(cfg, int64FromAny(initData["timestamp"], time.Now().UnixMilli()), duration)
	ua := opts.UserAgent
	if ua == "" {
		ua = defaultUA
	}

	body := buildSubmitBody(shortURL, rawQuestions, actions, cfg, startTimeMS, duration)

	// Sign and submit
	answerToken := getString(initData, "answerToken")
	timeCode := getString(initData, "timeCode")

	proxyAddr := parseProxy(opts.ProxyAddress)
	return saveAnswers(ctx, shortURL, answerToken, timeCode, body, ua, cookieHeader, proxyAddr)
}
