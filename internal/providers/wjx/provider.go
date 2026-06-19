package wjx

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/SurveyController/SurveyCore/internal/execution"
	runstate "github.com/SurveyController/SurveyCore/internal/runtime"

	"github.com/SurveyController/SurveyCore/internal/models"
	"github.com/SurveyController/SurveyCore/internal/network/httpclient"
)

const (
	ProviderName = "wjx"

	defaultUserAgent = "Mozilla/5.0 (Linux; Android 14; Pixel 8 Pro) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36 MicroMessenger/8.0.44"

	submitVerificationText = "需要安全校验，请重新提交"

	parseRetryAttempts = 3
	parseRetryDelay    = 350 * time.Millisecond
)

// WjxSubmitResult classification constants
const (
	SubmitSuccess      = "success"
	SubmitVerification = "verification"
	SubmitRejected     = "rejected"
)

var submitRejectRe = regexp.MustCompile(`^\s*(\d+)[〒](\d+)[〒](.+)$`)
var sceneIDPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bsceneId\s*[:=]\s*["']([^"']+)["']`),
	regexp.MustCompile(`(?i)\bscene_id\s*[:=]\s*["']([^"']+)["']`),
	regexp.MustCompile(`(?i)\bdata-scene-id\s*=\s*["']([^"']+)["']`),
}

// Provider implements the WJX survey provider.
type Provider struct{}

// NewProvider creates a new WJX provider.
func NewProvider() *Provider {
	return &Provider{}
}

func (p *Provider) ProviderName() string { return ProviderName }

// ParseSurvey fetches and parses a WJX survey page.
func (p *Provider) ParseSurvey(ctx context.Context, surveyURL string) (*models.SurveyDefinition, error) {
	surveyURL = normalizeSurveyURL(surveyURL)
	headers := map[string]string{
		"User-Agent": defaultUserAgent,
		"Accept":     "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
	}

	var lastHTML string
	var lastErr error
	for attempt := 1; attempt <= parseRetryAttempts; attempt++ {
		resp, err := httpclient.Get(ctx, surveyURL, headers, nil, 15*time.Second)
		if err != nil {
			lastErr = err
			continue
		}

		html := string(resp.Body)
		lastHTML = html
		if err := checkPageStateErrors(html); err != nil {
			return nil, err
		}

		questions, title, err := ParseHTML(html)
		if err != nil {
			lastErr = err
			continue
		}
		if len(questions) > 0 {
			return &models.SurveyDefinition{
				Provider:  ProviderName,
				Title:     strings.TrimSpace(title),
				Questions: questions,
			}, nil
		}
		if attempt < parseRetryAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(parseRetryDelay):
			}
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("无法获取问卷网页: %w", lastErr)
	}
	return nil, fmt.Errorf("无法打开问卷链接，HTTP 页面未返回可解析题目: %s", unparseablePageSummary(lastHTML))
}

// FillSurveyHTTP submits a single WJX survey response via HTTP.
func (p *Provider) FillSurveyHTTP(ctx context.Context, cfg *execution.ExecutionConfig, state *runstate.ExecutionState, opts models.FillOptions) (bool, error) {
	if state.IsStopped() {
		return false, nil
	}

	surveyURL := normalizeSurveyURL(cfg.URL)
	shortID, err := extractShortID(surveyURL)
	if err != nil {
		return false, err
	}

	ua := opts.UserAgent
	if ua == "" {
		ua = defaultUserAgent
	}
	channel := resolveChannelProfile(ua, opts.UserAgentCategory)

	headers := map[string]string{
		"User-Agent": ua,
		"Referer":    surveyURL,
		"Accept":     "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
	}

	// Load the survey page first to validate state
	resp, err := httpclient.Get(ctx, surveyURL, headers, nil, 15*time.Second)
	if err != nil {
		return false, fmt.Errorf("无法加载问卷页面: %w", err)
	}
	html := string(resp.Body)
	if err := checkPageStateErrors(html); err != nil {
		return false, err
	}

	// Build answer actions
	plan, err := buildAnswerPlan(cfg, state, opts.ThreadName)
	if err != nil {
		return false, fmt.Errorf("生成答案失败: %w", err)
	}
	if len(plan.Actions) == 0 {
		return false, fmt.Errorf("没有生成可提交答案")
	}

	// Build submitdata
	submitData := buildSubmitData(plan.Actions, cfg)
	if len(plan.SkippedNums) > 0 {
		submitData = buildSubmitDataWithSkipped(plan.Actions, cfg, plan.SkippedNums)
	}

	// Build submission parameters
	ktimes := sampleKtimes(cfg)
	currentMS := time.Now().UnixMilli()
	startSeconds := int(currentMS/1000) - ktimes
	jqnonce := generateUUID()
	domain := submitDomain(surveyURL)

	// Build query string from params
	params := url.Values{}
	params.Set("shortid", shortID)
	params.Set("starttime", formatWjxStarttime(startSeconds))
	params.Set("cst", fmt.Sprintf("%d", startSeconds*1000))
	params.Set("source", channel.Source)
	params.Set("submittype", "1")
	params.Set("ktimes", fmt.Sprintf("%d", ktimes))
	params.Set("rn", fmt.Sprintf("%.0f", 2000000000+rand.Float64()*100000000))
	params.Set("jcn", shortID)
	params.Set("nw", "1")
	params.Set("jwt", "4")
	params.Set("jpm", "62")
	params.Set("capt", "2")
	params.Set("t", fmt.Sprintf("%d", currentMS))
	params.Set("jqnonce", jqnonce)
	params.Set("jqsign", buildJqsign(jqnonce, ktimes))
	for key, value := range channel.ExtraParams {
		params.Set(key, value)
	}

	submitURL := fmt.Sprintf("https://%s/joinnew/processjq.ashx?%s", domain, params.Encode())

	submitHeaders := map[string]string{
		"User-Agent":       ua,
		"Referer":          surveyURL,
		"Accept":           "text/plain, */*; q=0.01",
		"Content-Type":     "application/x-www-form-urlencoded; charset=UTF-8",
		"Origin":           fmt.Sprintf("https://%s", domain),
		"X-Requested-With": "XMLHttpRequest",
	}

	// Submit
	submitBody := fmt.Sprintf("submitdata=%s&sceneId=%s", url.QueryEscape(submitData), url.QueryEscape(extractSceneID(html)))
	proxyAddr := parseProxyAddress(opts.ProxyAddress)
	submitResp, err := httpclient.Post(ctx, submitURL, submitBody, submitHeaders, proxyAddr, 20*time.Second)
	if err != nil {
		return false, fmt.Errorf("提交问卷失败: %w", err)
	}

	responseText := strings.TrimSpace(string(submitResp.Body))
	result := classifySubmitResponse(responseText)
	if result != SubmitSuccess {
		return false, classifySubmitError(cfg, responseText)
	}

	return true, nil
}

// Helper functions

func extractShortID(surveyURL string) (string, error) {
	u, err := url.Parse(normalizeSurveyURL(surveyURL))
	if err != nil {
		return "", fmt.Errorf("invalid survey URL: %w", err)
	}
	parts := strings.Split(strings.TrimRight(u.Path, "/"), "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("URL path is empty")
	}
	shortID := strings.TrimSuffix(parts[len(parts)-1], ".aspx")
	if shortID == "" {
		return "", fmt.Errorf("问卷星链接缺少 shortid")
	}
	return shortID, nil
}

func normalizeSurveyURL(surveyURL string) string {
	text := strings.TrimSpace(surveyURL)
	if text == "" || strings.Contains(text, "://") {
		return text
	}
	return "https://" + text
}

func submitDomain(surveyURL string) string {
	u, err := url.Parse(normalizeSurveyURL(surveyURL))
	if err != nil {
		return "v.wjx.cn"
	}
	host := strings.ToLower(u.Host)
	if strings.Contains(host, "ks.wjx.com") {
		return "ks.wjx.com"
	}
	return "v.wjx.cn"
}

func extractSceneID(pageHTML string) string {
	text := strings.TrimSpace(pageHTML)
	if text == "" {
		return "q0hcfsca"
	}
	for _, pattern := range sceneIDPatterns {
		match := pattern.FindStringSubmatch(text)
		if len(match) == 2 && strings.TrimSpace(match[1]) != "" {
			return strings.TrimSpace(match[1])
		}
	}
	return "q0hcfsca"
}

type channelProfile struct {
	Source      string
	ExtraParams map[string]string
}

func resolveChannelProfile(userAgent, category string) channelProfile {
	category = strings.ToLower(strings.TrimSpace(category))
	if category == "" {
		if strings.Contains(strings.ToLower(userAgent), "micromessenger") {
			category = "wechat"
		} else if strings.Contains(strings.ToLower(userAgent), "mobile") {
			category = "mobile"
		} else {
			category = "pc"
		}
	}
	switch category {
	case "wechat":
		return channelProfile{
			Source: "微信",
			ExtraParams: map[string]string{
				"wxfs":         "100",
				"access_token": "1",
				"openid":       fmt.Sprintf("%d", 100000000+rand.Intn(900000000)),
				"unionId":      fmt.Sprintf("%d", 100000000+rand.Intn(900000000)),
				"wxappid":      "wx8fe84c5d52db247a",
				"iwx":          "1",
			},
		}
	case "mobile":
		return channelProfile{Source: "手机访问"}
	default:
		return channelProfile{Source: "直链访问"}
	}
}

func formatWjxStarttime(timestampSeconds int) string {
	t := time.Unix(int64(timestampSeconds), 0)
	return fmt.Sprintf("%d/%d/%d %d:%d:%d", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
}

func buildJqsign(jqnonce string, ktimes int) string {
	tValue := ktimes % 10
	if tValue == 0 {
		tValue = 1
	}
	result := make([]rune, len(jqnonce))
	for i, ch := range jqnonce {
		result[i] = ch ^ rune(tValue)
	}
	return string(result)
}

func sampleKtimes(cfg *execution.ExecutionConfig) int {
	if cfg.AnswerDurationRangeSeconds[0] > 0 && cfg.AnswerDurationRangeSeconds[1] > 0 {
		min := cfg.AnswerDurationRangeSeconds[0]
		max := cfg.AnswerDurationRangeSeconds[1]
		if max > min {
			return min + rand.Intn(max-min+1)
		}
		return min
	}
	return 10 + rand.Intn(11) // 10-20
}

func generateUUID() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		rand.Int31(), rand.Int31n(0xffff), rand.Int31n(0xffff)|0x4000,
		rand.Int31n(0x3fff)|0x8000, rand.Int63n(0xffffffffffff))
}

func classifySubmitResponse(text string) string {
	if isSubmissionVerificationResponse(text) {
		return SubmitVerification
	}
	lowered := strings.ToLower(text)
	success := strings.Contains(lowered, "complete.aspx") ||
		strings.Contains(lowered, "success") ||
		strings.HasPrefix(lowered, "10") ||
		lowered == "1" || lowered == "ok"
	failure := strings.Contains(text, "抱歉") || strings.Contains(text, "不符合") ||
		strings.Contains(text, "错误") || strings.Contains(text, "重新提交")
	if success && !failure {
		return SubmitSuccess
	}
	return SubmitRejected
}

func isSubmissionVerificationResponse(text string) bool {
	return strings.Contains(text, submitVerificationText)
}

func classifySubmitError(cfg *execution.ExecutionConfig, text string) error {
	if isSubmissionVerificationResponse(text) {
		return fmt.Errorf("问卷星触发智能验证，当前链路已停止。请启用随机 IP 后再提交")
	}
	// Parse structured error: N〒N〒reason
	if match := submitRejectRe.FindStringSubmatch(text); match != nil {
		questionNum := match[2]
		reason := strings.TrimSpace(match[3])
		if reason == "" {
			reason = text
		}
		label := questionErrorLabel(cfg, questionNum)
		return fmt.Errorf("问卷星提交被拒绝: %s，%s", label, reason)
	}
	return fmt.Errorf("问卷星提交被拒绝: %s", truncate(text, 200))
}

func questionErrorLabel(cfg *execution.ExecutionConfig, questionNumStr string) string {
	var num int
	fmt.Sscanf(questionNumStr, "%d", &num)
	if num <= 0 {
		return fmt.Sprintf("第%s题", questionNumStr)
	}
	meta, ok := cfg.QuestionsMetadata[num]
	if !ok {
		return fmt.Sprintf("第%d题", num)
	}
	if meta.Title != "" {
		return fmt.Sprintf("第%d题（%s）", num, truncate(meta.Title, 30))
	}
	return fmt.Sprintf("第%d题", num)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func parseProxyAddress(addr string) *string {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	v := strings.TrimSpace(addr)
	if !strings.Contains(v, "://") {
		v = "http://" + v
	}
	return &v
}

func unparseablePageSummary(html string) string {
	text := normalizeHTMLText(html)
	if text == "" {
		return "空页面"
	}
	return truncate(text, 120)
}
