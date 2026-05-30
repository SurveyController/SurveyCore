package tencent

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/SurveyController/SurveyConsole/internal/models"
	"github.com/SurveyController/SurveyConsole/internal/network/httpclient"
	"github.com/SurveyController/SurveyConsole/internal/providers/providerutil"
	"github.com/SurveyController/SurveyConsole/internal/questions"
)

const (
	ProviderName = "qq"
	defaultUA    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"
	apiBase      = "https://wj.qq.com/api/v2/respondent/surveys"
	localeOrder  = "zhs,zht,zh,en"
)

const (
	SubmitSuccess  = "success"
	SubmitRejected = "rejected"
)

var (
	urlRe            = regexp.MustCompile(`/s\d+/(\d+)/([A-Za-z0-9_-]+)/?$`)
	qqQuestionIDRe   = regexp.MustCompile(`\bq-[A-Za-z0-9_-]+\b`)
	qqPageIDRe       = regexp.MustCompile(`\bp-[A-Za-z0-9_-]+\b`)
	qqLogicEndTokens = []string{"submit", "finish", "complete", "end", "结束", "提交", "完成"}
)

// Provider implements the Tencent survey provider.
type Provider struct{}

func NewProvider() *Provider             { return &Provider{} }
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
	actions := buildAnswerActions(cfg, state, rawQuestions, opts.ThreadName)

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

func classifyQQSubmitPayload(payload map[string]any) string {
	code := strings.ToUpper(strings.TrimSpace(fmt.Sprint(payload["code"])))
	if code == "OK" || code == "0" {
		return SubmitSuccess
	}
	return SubmitRejected
}

func extractQQIdentifiers(surveyURL string) (string, string, error) {
	matches := urlRe.FindStringSubmatch(surveyURL)
	if len(matches) < 3 {
		return "", "", fmt.Errorf("无法从 URL 提取腾讯问卷 ID: %s", surveyURL)
	}
	return matches[1], matches[2], nil
}

func fetchSession(ctx context.Context, surveyID, hashValue string) (string, string, error) {
	url := fmt.Sprintf("%s/%s/session?_=%d&hash=%s", apiBase, surveyID, time.Now().UnixMilli(), hashValue)
	resp, err := httpclient.Get(ctx, url, qqAPIHeaders(surveyID, hashValue), nil, 10*time.Second)
	if err != nil {
		return "", "", err
	}
	cookieHeader := responseCookieHeader(resp)

	// Extract answer_session_id from response
	var result map[string]any
	if err := json.Unmarshal(resp.Body, &result); err == nil {
		if data, ok := result["data"].(map[string]any); ok {
			if sessionID, ok := data["answer_session_id"].(string); ok {
				return sessionID, cookieHeader, nil
			}
		}
	}
	// Session ID not found - continue without it
	return "", cookieHeader, nil
}

func fetchRawQuestionsWithSession(ctx context.Context, surveyID, hashValue, sessionID, cookieHeader string) ([]map[string]any, string, error) {
	var lastErr error
	for _, locale := range strings.Split(localeOrder, ",") {
		questionsURL := fmt.Sprintf("%s/%s/questions?_=%d&hash=%s&locale=%s",
			apiBase, surveyID, time.Now().UnixMilli(), hashValue, locale)

		headers := qqAPIHeaders(surveyID, hashValue)
		if sessionID != "" {
			headers["X-Answer-Session"] = sessionID
		}
		if cookieHeader != "" {
			headers["Cookie"] = cookieHeader
		}

		resp, err := httpclient.Get(ctx, questionsURL, headers, nil, 10*time.Second)
		if err != nil {
			lastErr = err
			continue
		}

		var result map[string]any
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			lastErr = err
			continue
		}
		if err := ensureQQAPIOK(result, "questions"); err != nil {
			lastErr = err
			continue
		}

		data, ok := result["data"].(map[string]any)
		if !ok {
			lastErr = fmt.Errorf("题目接口缺少 data 对象")
			continue
		}

		// Get title from meta
		title := ""
		metaURL := fmt.Sprintf("%s/%s/meta?_=%d&hash=%s&locale=%s",
			apiBase, surveyID, time.Now().UnixMilli(), hashValue, locale)
		metaResp, err := httpclient.Get(ctx, metaURL, headers, nil, 10*time.Second)
		if err == nil {
			var metaResult map[string]any
			if json.Unmarshal(metaResp.Body, &metaResult) == nil {
				_ = ensureQQAPIOK(metaResult, "meta")
				if metaData, ok := metaResult["data"].(map[string]any); ok {
					if t, ok := metaData["title"].(string); ok {
						title = t
					}
				}
			}
		}

		if questionsRaw, ok := data["questions"].([]any); ok {
			questions := make([]map[string]any, 0, len(questionsRaw))
			for _, q := range questionsRaw {
				if qm, ok := q.(map[string]any); ok {
					questions = append(questions, qm)
				}
			}
			return questions, title, nil
		}
		lastErr = fmt.Errorf("题目接口未返回 questions 数组")
	}

	if lastErr != nil {
		return nil, "", fmt.Errorf("无法获取腾讯问卷题目: %w", lastErr)
	}
	return nil, "", fmt.Errorf("无法获取腾讯问卷题目")
}

func qqAPIHeaders(surveyID, hashValue string) map[string]string {
	return map[string]string{
		"User-Agent":      defaultUA,
		"Accept":          "application/json, text/plain, */*",
		"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
		"Connection":      "close",
		"Origin":          "https://wj.qq.com",
		"Referer":         fmt.Sprintf("https://wj.qq.com/s2/%s/%s/", surveyID, hashValue),
	}
}

func responseCookieHeader(resp *httpclient.Response) string {
	if resp == nil {
		return ""
	}
	var cookies []string
	for _, raw := range resp.Headers.Values("Set-Cookie") {
		part := strings.TrimSpace(strings.Split(raw, ";")[0])
		if part != "" {
			cookies = append(cookies, part)
		}
	}
	return strings.Join(cookies, "; ")
}

func ensureQQAPIOK(payload map[string]any, endpoint string) error {
	code := strings.ToUpper(strings.TrimSpace(fmt.Sprint(payload["code"])))
	if code == "" || code == "OK" || code == "0" {
		return nil
	}
	return fmt.Errorf("%s 接口返回异常: %s", endpoint, code)
}

func fetchQuestions(ctx context.Context, surveyID, hashValue string) ([]models.SurveyQuestionMeta, string, error) {
	rawQuestions, title, err := fetchRawQuestions(ctx, surveyID, hashValue)
	if err != nil {
		return nil, "", err
	}
	normalized := standardizeQuestions(rawQuestions)
	return normalized, title, nil
}

func fetchRawQuestions(ctx context.Context, surveyID, hashValue string) ([]map[string]any, string, error) {
	return fetchRawQuestionsWithSession(ctx, surveyID, hashValue, "", "")
}

func fetchRawQuestionsLegacy(ctx context.Context, surveyID, hashValue string) ([]map[string]any, string, error) {
	// Try different locales
	for _, locale := range strings.Split(localeOrder, ",") {
		questionsURL := fmt.Sprintf("%s/%s/questions?hash=%s&locale=%s&_=%d",
			apiBase, surveyID, hashValue, locale, time.Now().UnixMilli())

		resp, err := httpclient.Get(ctx, questionsURL, map[string]string{
			"User-Agent": defaultUA,
			"Referer":    fmt.Sprintf("https://wj.qq.com/s2/%s/%s/", surveyID, hashValue),
		}, nil, 10*time.Second)
		if err != nil {
			continue
		}

		var result map[string]any
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			continue
		}

		data, ok := result["data"].(map[string]any)
		if !ok {
			continue
		}

		// Get title
		title := ""
		if metaURL := fmt.Sprintf("%s/%s/meta?hash=%s&locale=%s&_=%d",
			apiBase, surveyID, hashValue, locale, time.Now().UnixMilli()); true {
			metaResp, err := httpclient.Get(ctx, metaURL, map[string]string{
				"User-Agent": defaultUA,
				"Referer":    fmt.Sprintf("https://wj.qq.com/s2/%s/%s/", surveyID, hashValue),
			}, nil, 10*time.Second)
			if err == nil {
				var metaResult map[string]any
				if json.Unmarshal(metaResp.Body, &metaResult) == nil {
					if metaData, ok := metaResult["data"].(map[string]any); ok {
						if t, ok := metaData["title"].(string); ok {
							title = t
						}
					}
				}
			}
		}

		// Extract questions array
		if questionsRaw, ok := data["questions"].([]any); ok {
			questions := make([]map[string]any, 0, len(questionsRaw))
			for _, q := range questionsRaw {
				if qm, ok := q.(map[string]any); ok {
					questions = append(questions, qm)
				}
			}
			return questions, title, nil
		}
	}

	return nil, "", fmt.Errorf("无法获取腾讯问卷题目")
}

// Type code mapping from QQ types
var qqTypeMap = map[string]string{
	"radio":        "3",
	"checkbox":     "4",
	"text":         "1",
	"textarea":     "1",
	"nps":          "5",
	"star":         "5",
	"matrix_radio": "6",
	"matrix_check": "6",
	"select":       "7",
	"dropdown":     "7",
	"description":  "0",
}

func standardizeQuestions(raw []map[string]any) []models.SurveyQuestionMeta {
	var result []models.SurveyQuestionMeta
	num := 1

	for _, q := range raw {
		providerType := getString(q, "type")
		typeCode := qqTypeMap[providerType]
		if typeCode == "" {
			typeCode = "1"
		}

		title := getString(q, "title")
		if title == "" {
			title = getString(q, "description")
		}

		questionID := getString(q, "id")
		pageID := getString(q, "page_id")
		if pageID == "" {
			pageID = "1"
		}
		page := intFromAny(q["page"])
		if page <= 0 {
			page = 1
		}

		optionTexts := extractOptionTexts(q, providerType)
		options := len(optionTexts)

		// Extract forced option
		var forcedIdx *int
		forceText := extractForceSelect(title, optionTexts)
		if forceText >= 0 {
			forcedIdx = &forceText
		}

		// Extract multi-select limits
		minLimit, maxLimit := extractMultiSelectLimits(title, options)

		// Row texts for matrix
		rowTexts := extractRowTexts(q)

		qm := models.SurveyQuestionMeta{
			Num:                num,
			Title:              title,
			TypeCode:           typeCode,
			Options:            options,
			OptionTexts:        optionTexts,
			Rows:               len(rowTexts),
			RowTexts:           rowTexts,
			Page:               page,
			Provider:           ProviderName,
			ProviderQuestionID: questionID,
			ProviderPageID:     pageID,
			ProviderType:       providerType,
			ForcedOptionIndex:  forcedIdx,
			LogicParseStatus:   models.LogicParseStatusNone,
		}

		if minLimit != nil {
			qm.MultiMinLimit = minLimit
		}
		if maxLimit != nil {
			qm.MultiMaxLimit = maxLimit
		}

		// Detect text-like
		if typeCode == "1" {
			qm.IsTextLike = true
		}
		// Detect rating
		if providerType == "nps" || providerType == "star" {
			qm.IsRating = true
			qm.RatingMax = options
		}

		result = append(result, qm)
		num++
	}
	attachTencentLogicMetadata(raw, result)
	return result
}

func extractOptionTexts(q map[string]any, providerType string) []string {
	if providerType == "nps" {
		// NPS generates numeric labels
		beginNum := 0
		count := 10
		if opts, ok := q["options"].(map[string]any); ok {
			if b, ok := opts["star_begin_num"].(float64); ok {
				beginNum = int(b)
			}
			if c, ok := opts["star_num"].(float64); ok {
				count = int(c)
			}
		}
		texts := make([]string, count)
		for i := range texts {
			texts[i] = strconv.Itoa(beginNum + i)
		}
		return texts
	}

	if providerType == "star" {
		count := 5
		if opts, ok := q["options"].(map[string]any); ok {
			if c, ok := opts["star_num"].(float64); ok {
				count = int(c)
			}
		}
		texts := make([]string, count)
		for i := range texts {
			texts[i] = strconv.Itoa(i + 1)
		}
		return texts
	}

	// Standard options
	if optsRaw, ok := q["options"].([]any); ok {
		var texts []string
		for _, opt := range optsRaw {
			if optMap, ok := opt.(map[string]any); ok {
				if text, ok := optMap["text"].(string); ok && text != "" {
					texts = append(texts, text)
				}
			}
		}
		return texts
	}
	return nil
}

func extractRowTexts(q map[string]any) []string {
	var rows []string
	if subsRaw, ok := q["sub_titles"].([]any); ok {
		for _, sub := range subsRaw {
			if subMap, ok := sub.(map[string]any); ok {
				if text, ok := subMap["text"].(string); ok && text != "" {
					rows = append(rows, text)
				}
			}
		}
	}
	return rows
}

func extractJumpRules(q map[string]any) []map[string]any {
	var rules []map[string]any
	if gotoRaw, ok := q["goto"].([]any); ok {
		for _, g := range gotoRaw {
			if gMap, ok := g.(map[string]any); ok {
				rules = append(rules, map[string]any{
					"source_option": gMap["option_id"],
					"target_page":   gMap["page_id"],
				})
			}
		}
	}
	return rules
}

type tencentLogicState struct {
	JumpRules             []map[string]any
	HasJump               bool
	HasSourceDisplayLogic bool
	ExactLogicParsed      bool
}

func attachTencentLogicMetadata(rawQuestions []map[string]any, questions []models.SurveyQuestionMeta) {
	if len(rawQuestions) == 0 || len(questions) == 0 {
		return
	}
	questionByProviderID := make(map[string]*models.SurveyQuestionMeta)
	questionNumByProviderID := make(map[string]int)
	firstQuestionNumByPageID := make(map[string]int)
	maxQuestionNum := 0
	for i := range questions {
		q := &questions[i]
		if q.ProviderQuestionID == "" || q.Num <= 0 {
			continue
		}
		questionByProviderID[q.ProviderQuestionID] = q
		questionNumByProviderID[q.ProviderQuestionID] = q.Num
		if q.Num > maxQuestionNum {
			maxQuestionNum = q.Num
		}
		if q.ProviderPageID != "" {
			if _, exists := firstQuestionNumByPageID[q.ProviderPageID]; !exists {
				firstQuestionNumByPageID[q.ProviderPageID] = q.Num
			}
		}
	}

	stateByProviderID := make(map[string]*tencentLogicState)
	sourceTargets := make(map[string][]map[string]any)
	inboundConditions := make(map[string][]map[string]any)
	for _, rawQuestion := range rawQuestions {
		providerQuestionID := strings.TrimSpace(getString(rawQuestion, "id"))
		if providerQuestionID == "" {
			continue
		}
		if _, ok := questionByProviderID[providerQuestionID]; !ok {
			continue
		}
		state := stateByProviderID[providerQuestionID]
		if state == nil {
			state = &tencentLogicState{}
			stateByProviderID[providerQuestionID] = state
		}

		if target := resolveTencentJumpTarget(rawQuestion["goto"], questionNumByProviderID, firstQuestionNumByPageID, maxQuestionNum); target != nil {
			state.JumpRules = append(state.JumpRules, map[string]any{
				"option_index": -1,
				"jumpto":       *target,
				"option_text":  nil,
			})
			state.HasJump = true
			state.ExactLogicParsed = true
		} else if valuePresent(rawQuestion["goto"]) {
			state.HasJump = true
		}

		options, _ := rawQuestion["options"].([]any)
		for optionIndex, optionRaw := range options {
			option, ok := optionRaw.(map[string]any)
			if !ok {
				continue
			}
			if target := resolveTencentJumpTarget(option["goto"], questionNumByProviderID, firstQuestionNumByPageID, maxQuestionNum); target != nil {
				state.JumpRules = append(state.JumpRules, map[string]any{
					"option_index": optionIndex,
					"jumpto":       *target,
					"option_text":  strings.TrimSpace(getString(option, "text")),
				})
				state.HasJump = true
				state.ExactLogicParsed = true
			} else if valuePresent(option["goto"]) {
				state.HasJump = true
			}

			displayPayload := option["display"]
			if !valuePresent(displayPayload) {
				continue
			}
			state.HasSourceDisplayLogic = true
			targetQuestionIDs := extractTencentQuestionRefs(displayPayload)
			if len(targetQuestionIDs) == 0 {
				continue
			}
			state.ExactLogicParsed = true
			for _, targetQuestionID := range targetQuestionIDs {
				targetQuestionNum := questionNumByProviderID[targetQuestionID]
				if targetQuestionNum <= 0 {
					continue
				}
				sourceQuestionNum := questionNumByProviderID[providerQuestionID]
				sourceTargets[providerQuestionID] = append(sourceTargets[providerQuestionID], map[string]any{
					"target_question_num":      targetQuestionNum,
					"condition_option_indices": []int{optionIndex},
					"condition_mode":           "selected",
				})
				inboundConditions[targetQuestionID] = append(inboundConditions[targetQuestionID], map[string]any{
					"condition_question_num":   sourceQuestionNum,
					"condition_mode":           "selected",
					"condition_option_indices": []int{optionIndex},
				})
			}
		}
	}

	for _, rawQuestion := range rawQuestions {
		providerQuestionID := strings.TrimSpace(getString(rawQuestion, "id"))
		if providerQuestionID == "" {
			continue
		}
		if _, hasInbound := inboundConditions[providerQuestionID]; hasInbound {
			continue
		}
		referQuestionIDs := extractTencentQuestionRefs(rawQuestion["refer"])
		if len(referQuestionIDs) == 0 {
			continue
		}
		targetQuestionNum := questionNumByProviderID[providerQuestionID]
		if targetQuestionNum <= 0 {
			continue
		}
		for _, referQuestionID := range referQuestionIDs {
			sourceQuestionNum := questionNumByProviderID[referQuestionID]
			if sourceQuestionNum <= 0 {
				continue
			}
			inboundConditions[providerQuestionID] = append(inboundConditions[providerQuestionID], map[string]any{
				"condition_question_num":   sourceQuestionNum,
				"condition_mode":           "selected",
				"condition_option_indices": []int{},
			})
			sourceTargets[referQuestionID] = append(sourceTargets[referQuestionID], map[string]any{
				"target_question_num":      targetQuestionNum,
				"condition_option_indices": []int{},
				"condition_mode":           "selected",
			})
		}
	}

	for _, rawQuestion := range rawQuestions {
		providerQuestionID := strings.TrimSpace(getString(rawQuestion, "id"))
		q := questionByProviderID[providerQuestionID]
		if q == nil {
			continue
		}
		state := stateByProviderID[providerQuestionID]
		if state == nil {
			state = &tencentLogicState{}
		}
		q.JumpRules = dedupeLogicMaps(state.JumpRules, "option_index", "jumpto")
		q.HasJump = state.HasJump || len(q.JumpRules) > 0

		controls := normalizeLogicMaps(sourceTargets[providerQuestionID])
		q.ControlsDisplayTargets = dedupeLogicMaps(controls, "target_question_num", "condition_option_indices", "condition_mode")
		if len(q.ControlsDisplayTargets) > 0 || state.HasSourceDisplayLogic {
			q.HasDependentDisplayLogic = true
		}

		referQuestionIDs := extractTencentQuestionRefs(rawQuestion["refer"])
		if valuePresent(rawQuestion["hidden"]) || len(referQuestionIDs) > 0 || len(inboundConditions[providerQuestionID]) > 0 {
			q.HasDisplayCondition = true
		}
		conditions := normalizeLogicMaps(inboundConditions[providerQuestionID])
		q.DisplayConditions = dedupeLogicMaps(conditions, "condition_question_num", "condition_option_indices", "condition_mode")
		if displayConditionsHaveOptionIndices(q.DisplayConditions) {
			state.ExactLogicParsed = true
		}

		hasAnyLogic := q.HasJump || q.HasDisplayCondition || q.HasDependentDisplayLogic
		if hasAnyLogic && state.ExactLogicParsed {
			q.LogicParseStatus = models.LogicParseStatusComplete
		} else if hasAnyLogic {
			q.LogicParseStatus = models.LogicParseStatusUnknown
		}
	}
}

func resolveTencentJumpTarget(rawTarget any, questionNumByProviderID map[string]int, firstQuestionNumByPageID map[string]int, maxQuestionNum int) *int {
	if !valuePresent(rawTarget) {
		return nil
	}
	if numeric := intFromAny(rawTarget); numeric > 0 {
		return &numeric
	}
	for _, questionID := range extractTencentQuestionRefs(rawTarget) {
		if target := questionNumByProviderID[questionID]; target > 0 {
			return &target
		}
	}
	for _, pageID := range extractTencentPageRefs(rawTarget) {
		if target := firstQuestionNumByPageID[pageID]; target > 0 {
			return &target
		}
	}
	lowered := strings.ToLower(strings.TrimSpace(fmt.Sprint(rawTarget)))
	if lowered != "" {
		for _, token := range qqLogicEndTokens {
			if strings.Contains(lowered, token) {
				target := maxQuestionNum + 1
				return &target
			}
		}
	}
	return nil
}

func extractTencentQuestionRefs(value any) []string {
	return uniqueStrings(collectTencentTokenRefs(value, qqQuestionIDRe, 0))
}

func extractTencentPageRefs(value any) []string {
	return uniqueStrings(collectTencentTokenRefs(value, qqPageIDRe, 0))
}

func collectTencentTokenRefs(value any, pattern *regexp.Regexp, depth int) []string {
	if depth > 5 || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		var result []string
		for key, item := range typed {
			result = append(result, collectTencentTokenRefs(key, pattern, depth+1)...)
			result = append(result, collectTencentTokenRefs(item, pattern, depth+1)...)
		}
		return result
	case []any:
		var result []string
		for _, item := range typed {
			result = append(result, collectTencentTokenRefs(item, pattern, depth+1)...)
		}
		return result
	case []map[string]any:
		var result []string
		for _, item := range typed {
			result = append(result, collectTencentTokenRefs(item, pattern, depth+1)...)
		}
		return result
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			return nil
		}
		return pattern.FindAllString(text, -1)
	}
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func normalizeLogicMaps(items []map[string]any) []map[string]any {
	normalized := make([]map[string]any, 0, len(items))
	for _, item := range items {
		normalizedItem := make(map[string]any, len(item))
		for key, value := range item {
			switch key {
			case "condition_question_num", "target_question_num", "jumpto", "option_index":
				normalizedItem[key] = intFromAny(value)
			case "condition_option_indices":
				normalizedItem[key] = intSliceFromAny(value)
			case "condition_mode":
				mode := strings.TrimSpace(fmt.Sprint(value))
				if mode == "" {
					mode = "selected"
				}
				normalizedItem[key] = mode
			default:
				normalizedItem[key] = value
			}
		}
		normalized = append(normalized, normalizedItem)
	}
	return normalized
}

func dedupeLogicMaps(items []map[string]any, keyFields ...string) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	seen := make(map[string]bool)
	for _, item := range items {
		if item == nil {
			continue
		}
		keyParts := make([]string, 0, len(keyFields))
		for _, field := range keyFields {
			keyParts = append(keyParts, fmt.Sprint(normalizedLogicKeyValue(item[field])))
		}
		key := strings.Join(keyParts, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}
	return result
}

func normalizedLogicKeyValue(value any) any {
	switch typed := value.(type) {
	case []int:
		values := append([]int{}, typed...)
		parts := make([]string, len(values))
		for i, value := range values {
			parts[i] = strconv.Itoa(value)
		}
		return strings.Join(parts, ",")
	case []any:
		values := intSliceFromAny(typed)
		parts := make([]string, len(values))
		for i, value := range values {
			parts[i] = strconv.Itoa(value)
		}
		return strings.Join(parts, ",")
	default:
		return typed
	}
}

func displayConditionsHaveOptionIndices(conditions []map[string]any) bool {
	for _, condition := range conditions {
		if len(intSliceFromAny(condition["condition_option_indices"])) > 0 {
			return true
		}
	}
	return false
}

func valuePresent(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case float32:
		return typed != 0
	case json.Number:
		return strings.TrimSpace(typed.String()) != "" && typed.String() != "0"
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		return len(typed) > 0
	case []map[string]any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return 0
}

func intSliceFromAny(value any) []int {
	switch typed := value.(type) {
	case []int:
		return append([]int{}, typed...)
	case []float64:
		result := make([]int, 0, len(typed))
		for _, item := range typed {
			if item >= 0 {
				result = append(result, int(item))
			}
		}
		return result
	case []any:
		result := make([]int, 0, len(typed))
		for _, item := range typed {
			value := intFromAny(item)
			if value >= 0 {
				result = append(result, value)
			}
		}
		return result
	default:
		return nil
	}
}

func extractForceSelect(title string, optionTexts []string) int {
	// Simple heuristic: look for "请选择 X" patterns
	for i, opt := range optionTexts {
		if strings.Contains(title, "请选择"+opt) || strings.Contains(title, "选择 "+opt) {
			return i
		}
	}
	return -1
}

func extractMultiSelectLimits(title string, optionCount int) (*int, *int) {
	// Look for "至少选 N 项" / "最多选 M 项"
	reMin := regexp.MustCompile(`至少选\s*(\d+)`)
	reMax := regexp.MustCompile(`最多选\s*(\d+)|至多选\s*(\d+)|不超过\s*(\d+)`)

	var minLimit, maxLimit *int

	if match := reMin.FindStringSubmatch(title); match != nil {
		if v, err := strconv.Atoi(match[1]); err == nil {
			minLimit = &v
		}
	}
	if match := reMax.FindStringSubmatch(title); match != nil {
		s := match[1]
		if s == "" {
			s = match[2]
		}
		if s == "" {
			s = match[3]
		}
		if v, err := strconv.Atoi(s); err == nil {
			maxLimit = &v
		}
	}

	return minLimit, maxLimit
}

func buildAnswerActions(cfg *models.ExecutionConfig, state *models.ExecutionState, rawQuestions []map[string]any, threadName string) []TencentAnswerAction {
	runtime := questions.NewRunContextForThread(cfg, state, threadName)
	// Build a map from question ID to raw question for option ID lookup
	rawByQID := make(map[string]map[string]any)
	for _, rq := range rawQuestions {
		rawByQID[getString(rq, "id")] = rq
	}

	var actions []TencentAnswerAction
	actionByNum := make(map[int]TencentAnswerAction)
	jumpTarget := (*int)(nil)
	maxQuestionNum := 0
	ordered := sortedQuestions(cfg)
	for _, meta := range ordered {
		if meta.Num > maxQuestionNum {
			maxQuestionNum = meta.Num
		}
	}
	for _, meta := range ordered {
		if jumpTarget != nil {
			if meta.Num < *jumpTarget {
				continue
			}
			jumpTarget = nil
		}
		if !tencentQuestionVisible(meta, actionByNum) {
			continue
		}
		rawQ := rawByQID[meta.ProviderQuestionID]
		action := buildSingleAction(cfg, meta, rawQ, runtime)
		if action != nil {
			actionByNum[meta.Num] = *action
			actions = append(actions, *action)
			if target := tencentJumpTarget(meta, *action); target != nil {
				if *target > maxQuestionNum {
					break
				}
				jumpTarget = target
			}
		}
	}
	return actions
}

func sortedQuestions(cfg *models.ExecutionConfig) []models.SurveyQuestionMeta {
	questions := make([]models.SurveyQuestionMeta, 0, len(cfg.QuestionsMetadata))
	for _, q := range cfg.QuestionsMetadata {
		questions = append(questions, q)
	}
	for i := 0; i < len(questions); i++ {
		for j := i + 1; j < len(questions); j++ {
			pi, pj := questions[i].Page, questions[j].Page
			if pi == 0 {
				pi = 1
			}
			if pj == 0 {
				pj = 1
			}
			if pi > pj || (pi == pj && questions[i].Num > questions[j].Num) {
				questions[i], questions[j] = questions[j], questions[i]
			}
		}
	}
	return questions
}

func buildSingleAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, rawQ map[string]any, runtime *questions.RunContext) *TencentAnswerAction {
	typeCode := meta.TypeCode
	optionCount := meta.Options
	if optionCount <= 0 {
		optionCount = 1
	}

	configIdx := -1
	if idx, ok := cfg.QuestionConfigIndexMap[meta.Num]; ok {
		configIdx = providerutil.ParseConfigIndex(idx)
	} else if idx, ok := providerutil.ProviderConfigIndex(cfg, meta); ok {
		configIdx = providerutil.ParseConfigIndex(idx)
	}

	switch typeCode {
	case "3": // single
		return buildChoiceAnswer(cfg, meta, configIdx, optionCount, false, rawQ, runtime)
	case "4": // multiple
		return buildMultipleAnswer(cfg, meta, configIdx, optionCount, rawQ, runtime)
	case "5": // scale/nps
		return buildScaleAnswer(cfg, meta, configIdx, optionCount, rawQ, runtime)
	case "6": // matrix
		return buildMatrixAnswer(cfg, meta, configIdx, rawQ, runtime)
	case "7": // dropdown
		return buildChoiceAnswer(cfg, meta, configIdx, optionCount, true, rawQ, runtime)
	case "1": // text
		return buildTextAnswer(cfg, meta, configIdx, runtime)
	default:
		return nil
	}
}

func tencentQuestionVisible(meta models.SurveyQuestionMeta, actionByNum map[int]TencentAnswerAction) bool {
	if len(meta.DisplayConditions) == 0 {
		return !meta.HasDisplayCondition
	}
	grouped := make(map[string][]map[string]any)
	for _, condition := range meta.DisplayConditions {
		sourceQuestionNum := intFromAny(condition["condition_question_num"])
		if sourceQuestionNum <= 0 {
			continue
		}
		mode := strings.TrimSpace(fmt.Sprint(condition["condition_mode"]))
		if mode == "" {
			mode = "selected"
		}
		key := fmt.Sprintf("%d:%s", sourceQuestionNum, mode)
		grouped[key] = append(grouped[key], condition)
	}
	if len(grouped) == 0 {
		return !meta.HasDisplayCondition
	}
	for _, group := range grouped {
		matched := false
		for _, condition := range group {
			if tencentConditionMet(actionByNum, condition) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func tencentConditionMet(actionByNum map[int]TencentAnswerAction, condition map[string]any) bool {
	sourceQuestionNum := intFromAny(condition["condition_question_num"])
	if sourceQuestionNum <= 0 {
		return false
	}
	sourceAction, ok := actionByNum[sourceQuestionNum]
	if !ok {
		return false
	}

	mode := strings.TrimSpace(fmt.Sprint(condition["condition_mode"]))
	if mode == "" {
		mode = "selected"
	}
	conditionIndices := tencentIntSet(condition["condition_option_indices"])
	selectedIndices := tencentSelectedIndexSet(sourceAction)
	if len(conditionIndices) == 0 {
		return mode == "selected"
	}
	if mode == "not_selected" {
		for idx := range conditionIndices {
			if selectedIndices[idx] {
				return false
			}
		}
		return true
	}
	for idx := range conditionIndices {
		if selectedIndices[idx] {
			return true
		}
	}
	return false
}

func tencentJumpTarget(meta models.SurveyQuestionMeta, action TencentAnswerAction) *int {
	if len(meta.JumpRules) == 0 {
		return nil
	}
	selectedIndices := tencentSelectedIndexSet(action)
	var unconditional *int
	for _, rule := range meta.JumpRules {
		target := intFromAny(rule["jumpto"])
		if target <= 0 {
			continue
		}
		optionIndex := intFromAny(rule["option_index"])
		if raw, ok := rule["option_index"]; !ok || raw == nil {
			optionIndex = -1
		}
		if optionIndex < 0 {
			if unconditional == nil {
				targetCopy := target
				unconditional = &targetCopy
			}
			continue
		}
		if selectedIndices[optionIndex] {
			targetCopy := target
			return &targetCopy
		}
	}
	return unconditional
}

func tencentSelectedIndexSet(action TencentAnswerAction) map[int]bool {
	result := make(map[int]bool)
	if len(action.MatrixIndices) > 0 {
		for _, idx := range action.MatrixIndices {
			if idx >= 0 {
				result[idx] = true
			}
		}
		return result
	}
	for _, idx := range action.SelectedIndices {
		if idx >= 0 {
			result[idx] = true
		}
	}
	return result
}

func tencentIntSet(value any) map[int]bool {
	result := make(map[int]bool)
	for _, idx := range intSliceFromAny(value) {
		if idx >= 0 {
			result[idx] = true
		}
	}
	return result
}

// TencentAnswerAction represents a Tencent survey answer.
type TencentAnswerAction struct {
	QuestionID      string
	QuestionType    string
	SelectedIDs     []string
	SelectedTexts   []string
	SelectedIndices []int
	TextValue       string
	MatrixAnswers   []TencentMatrixRow
	MatrixIndices   []int
}

type TencentMatrixRow struct {
	RowID     string
	OptionIDs []string
}

func buildChoiceAnswer(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int, isDropdown bool, rawQ map[string]any, runtime *questions.RunContext) *TencentAnswerAction {
	if meta.ForcedOptionIndex != nil && *meta.ForcedOptionIndex >= 0 {
		idx := *meta.ForcedOptionIndex
		if idx >= optionCount {
			idx = 0
		}
		return &TencentAnswerAction{
			QuestionID:      meta.ProviderQuestionID,
			QuestionType:    meta.ProviderType,
			SelectedIDs:     []string{getOptionIDFromRaw(rawQ, idx)},
			SelectedIndices: []int{idx},
		}
	}

	probs := getProbs(cfg, configIdx, optionCount, isDropdown)
	idx := runtime.ChooseSingle(meta, configIdx, optionCount, probs, nil)

	// Get actual option ID from raw data
	optID := getOptionIDFromRaw(rawQ, idx)
	return &TencentAnswerAction{
		QuestionID:      meta.ProviderQuestionID,
		QuestionType:    meta.ProviderType,
		SelectedIDs:     []string{optID},
		SelectedIndices: []int{idx},
	}
}

func buildMultipleAnswer(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int, rawQ map[string]any, runtime *questions.RunContext) *TencentAnswerAction {
	minLimit := 1
	maxLimit := optionCount
	if meta.MultiMinLimit != nil {
		minLimit = *meta.MultiMinLimit
	}
	if meta.MultiMaxLimit != nil {
		maxLimit = *meta.MultiMaxLimit
	}

	probs := make([]float64, optionCount)
	if configIdx >= 0 && configIdx < len(cfg.MultipleProb) {
		copy(probs, cfg.MultipleProb[configIdx])
	}
	if providerutil.AllZero(probs) {
		for i := range probs {
			probs[i] = 1.0
		}
	}

	selected := runtime.ChooseMultiple(meta, configIdx, optionCount, minLimit, maxLimit, probs)
	ids := make([]string, len(selected))
	for i, idx := range selected {
		ids[i] = getOptionIDFromRaw(rawQ, idx)
	}

	return &TencentAnswerAction{
		QuestionID:      meta.ProviderQuestionID,
		QuestionType:    meta.ProviderType,
		SelectedIDs:     ids,
		SelectedIndices: append([]int{}, selected...),
	}
}

func buildScaleAnswer(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int, rawQ map[string]any, runtime *questions.RunContext) *TencentAnswerAction {
	probs := make([]float64, optionCount)
	if configIdx >= 0 && configIdx < len(cfg.ScaleProb) {
		if p, ok := providerutil.Float64Slice(cfg.ScaleProb[configIdx]); ok {
			copy(probs, p)
		}
	}
	if providerutil.AllZero(probs) {
		for i := range probs {
			probs[i] = 1.0
		}
	}

	idx := runtime.ChooseSingle(meta, configIdx, optionCount, probs, nil)
	optID := getOptionIDFromRaw(rawQ, idx)
	return &TencentAnswerAction{
		QuestionID:      meta.ProviderQuestionID,
		QuestionType:    meta.ProviderType,
		SelectedIDs:     []string{optID},
		SelectedIndices: []int{idx},
	}
}

func buildMatrixAnswer(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int, rawQ map[string]any, runtime *questions.RunContext) *TencentAnswerAction {
	rows := meta.Rows
	if rows <= 0 {
		rows = 1
	}
	optionCount := meta.Options
	if optionCount <= 0 {
		optionCount = 1
	}

	var matrixAnswers []TencentMatrixRow
	var matrixIndices []int
	for i := 0; i < rows; i++ {
		probs := make([]float64, optionCount)
		if configIdx >= 0 && configIdx < len(cfg.MatrixProb) {
			copy(probs, providerutil.MatrixRowProbabilities(cfg.MatrixProb[configIdx], i, optionCount))
		}
		if providerutil.AllZero(probs) {
			for j := range probs {
				probs[j] = 1.0
			}
		}
		rowIndex := i
		idx := runtime.ChooseSingle(meta, configIdx, optionCount, probs, &rowIndex)

		optID := getOptionIDFromRaw(rawQ, idx)
		rowID := fmt.Sprintf("%d", i+1)
		if sub := getMatrixRowRaw(rawQ, i); sub != nil {
			if id := getString(sub, "id"); id != "" {
				rowID = id
			}
		}
		matrixAnswers = append(matrixAnswers, TencentMatrixRow{
			RowID:     rowID,
			OptionIDs: []string{optID},
		})
		matrixIndices = append(matrixIndices, idx)
	}

	return &TencentAnswerAction{
		QuestionID:    meta.ProviderQuestionID,
		QuestionType:  meta.ProviderType,
		MatrixAnswers: matrixAnswers,
		MatrixIndices: matrixIndices,
	}
}

// getOptionIDFromRaw extracts the actual option ID from raw question data by index.
func getOptionIDFromRaw(rawQ map[string]any, idx int) string {
	if rawQ == nil {
		return fmt.Sprintf("%d", idx+1)
	}
	optsRaw, ok := rawQ["options"].([]any)
	if !ok || idx >= len(optsRaw) {
		return fmt.Sprintf("%d", idx+1)
	}
	if optMap, ok := optsRaw[idx].(map[string]any); ok {
		if id := getString(optMap, "id"); id != "" {
			return id
		}
	}
	return fmt.Sprintf("%d", idx+1)
}

func getMatrixRowRaw(rawQ map[string]any, rowIndex int) map[string]any {
	if rawQ == nil {
		return nil
	}
	subsRaw, ok := rawQ["sub_titles"].([]any)
	if !ok || rowIndex < 0 || rowIndex >= len(subsRaw) {
		return nil
	}
	subMap, _ := subsRaw[rowIndex].(map[string]any)
	return subMap
}

func buildTextAnswer(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int, runtime *questions.RunContext) *TencentAnswerAction {
	text := "满意"
	if candidate, ok := questions.ChooseConfiguredTextCandidate(cfg, configIdx); ok {
		text = candidate
	}
	text = runtime.GenerateText(meta, configIdx, text, 1)
	return &TencentAnswerAction{
		QuestionID:   meta.ProviderQuestionID,
		QuestionType: meta.ProviderType,
		TextValue:    text,
	}
}

func buildSubmitBody(surveyID, hashValue string, rawQuestions []map[string]any, actions []TencentAnswerAction, duration int, ua string) map[string]any {
	actionMap := make(map[string]TencentAnswerAction)
	for _, a := range actions {
		actionMap[a.QuestionID] = a
	}

	// Group questions by page_id
	pageQuestions := make(map[string][]map[string]any)
	pageOrder := make([]string, 0)

	for _, rq := range rawQuestions {
		qID := getString(rq, "id")
		qType := getString(rq, "type")
		pageID := getString(rq, "page_id")
		if pageID == "" {
			pageID = "1"
		}

		action, ok := actionMap[qID]
		if !ok {
			continue
		}

		answer := map[string]any{
			"id":     qID,
			"type":   qType,
			"blanks": []any{},
		}

		switch qType {
		case "radio", "checkbox", "nps", "star", "select", "dropdown":
			optsRaw, _ := rq["options"].([]any)
			var options []map[string]any
			for _, opt := range optsRaw {
				if optMap, ok := opt.(map[string]any); ok {
					optID := getString(optMap, "id")
					checked := 0
					for _, selID := range action.SelectedIDs {
						if selID == optID {
							checked = 1
						}
					}
					options = append(options, map[string]any{
						"id":      optID,
						"text":    getString(optMap, "text"),
						"checked": checked,
					})
				}
			}
			answer["options"] = options

		case "text", "textarea":
			answer["text"] = action.TextValue

		case "matrix_radio", "matrix_check":
			var subTitles []map[string]any
			subsRaw, _ := rq["sub_titles"].([]any)
			topLevelOptions, _ := rq["options"].([]any)
			for i, sub := range subsRaw {
				if subMap, ok := sub.(map[string]any); ok {
					// Build ALL options for this row (matching Python behavior)
					subOptsRaw, _ := subMap["options"].([]any)
					if len(subOptsRaw) == 0 {
						subOptsRaw = topLevelOptions
					}
					var subOpts []map[string]any
					for _, subOpt := range subOptsRaw {
						if subOptMap, ok := subOpt.(map[string]any); ok {
							subOptID := getString(subOptMap, "id")
							checked := 0
							if i < len(action.MatrixAnswers) {
								for _, selID := range action.MatrixAnswers[i].OptionIDs {
									if selID == subOptID {
										checked = 1
									}
								}
							}
							subOpts = append(subOpts, map[string]any{
								"id":      subOptID,
								"text":    getString(subOptMap, "text"),
								"checked": checked,
							})
						}
					}
					subTitles = append(subTitles, map[string]any{
						"id":      getString(subMap, "id"),
						"text":    getString(subMap, "text"),
						"options": subOpts,
					})
				}
			}
			answer["sub_titles"] = subTitles
		}

		if _, exists := pageQuestions[pageID]; !exists {
			pageOrder = append(pageOrder, pageID)
		}
		pageQuestions[pageID] = append(pageQuestions[pageID], answer)
	}

	// Build pages array preserving page order
	var pages []map[string]any
	for _, pageID := range pageOrder {
		pages = append(pages, map[string]any{
			"id":        pageID,
			"questions": pageQuestions[pageID],
		})
	}

	// Convert survey_id to int (API expects integer)
	surveyIDInt := 0
	fmt.Sscanf(surveyID, "%d", &surveyIDInt)

	return map[string]any{
		"survey_id": surveyIDInt,
		"hash":      hashValue,
		"answer_survey": map[string]any{
			"duration":  duration,
			"ua":        ua,
			"referrer":  "",
			"uid":       generateUUID(),
			"sid":       generateUUID(),
			"openid":    "",
			"latitude":  nil,
			"longitude": nil,
			"is_update": false,
			"locale":    "zhs",
			"pages":     pages,
		},
	}
}

// Helpers

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func generateUUID() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		rand.Int31(), rand.Int31n(0xffff), rand.Int31n(0xffff)|0x4000,
		rand.Int31n(0x3fff)|0x8000, rand.Int63n(0xffffffffffff))
}

func getProbs(cfg *models.ExecutionConfig, configIdx, optionCount int, isDropdown bool) []float64 {
	probs := make([]float64, optionCount)
	source := cfg.SingleProb
	if isDropdown {
		source = cfg.DroplistProb
	}
	if configIdx >= 0 && configIdx < len(source) {
		if p, ok := providerutil.Float64Slice(source[configIdx]); ok {
			copy(probs, p)
		}
	}
	if providerutil.AllZero(probs) {
		for i := range probs {
			probs[i] = 1.0
		}
	}
	return probs
}

func parseProxy(addr string) *string {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	v := strings.TrimSpace(addr)
	return &v
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
