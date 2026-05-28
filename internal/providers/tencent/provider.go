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

	"github.com/SurveyController/SurveyController-Go/internal/models"
	"github.com/SurveyController/SurveyController-Go/internal/network/httpclient"
	"github.com/SurveyController/SurveyController-Go/internal/questions"
)

const (
	ProviderName    = "qq"
	defaultUA       = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	apiBase         = "https://wj.qq.com/api/v2/respondent/surveys"
	localeOrder     = "zhs,zht,zh,en"
)

var urlRe = regexp.MustCompile(`/s\d+/(\d+)/([A-Za-z0-9_-]+)/?$`)

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

	// Establish session
	if _, err := fetchSession(ctx, surveyID, hashValue); err != nil {
		return nil, fmt.Errorf("建立会话失败: %w", err)
	}

	// Fetch questions
	questions, title, err := fetchQuestions(ctx, surveyID, hashValue)
	if err != nil {
		return nil, fmt.Errorf("获取题目失败: %w", err)
	}

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
	answerSessionID, err := fetchSession(ctx, surveyID, hashValue)
	if err != nil {
		return false, fmt.Errorf("建立会话失败: %w", err)
	}

	// Fetch raw questions with session header
	rawQuestions, _, err := fetchRawQuestionsWithSession(ctx, surveyID, hashValue, answerSessionID)
	if err != nil {
		return false, fmt.Errorf("获取题目失败: %w", err)
	}

	// Build answer actions
	actions := buildAnswerActions(cfg, state, rawQuestions)

	// Build submit body
	ua := opts.UserAgent
	if ua == "" {
		ua = defaultUA
	}
	duration := 10 + rand.Intn(20)
	if cfg.AnswerDurationRangeSeconds[0] > 0 && cfg.AnswerDurationRangeSeconds[1] > 0 {
		duration = cfg.AnswerDurationRangeSeconds[0] + rand.Intn(cfg.AnswerDurationRangeSeconds[1]-cfg.AnswerDurationRangeSeconds[0])
	}

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
	resp, err := httpclient.Post(ctx, submitURL, string(bodyBytes), submitHeaders, proxyAddr, 20*time.Second)
	if err != nil {
		return false, fmt.Errorf("提交失败: %w", err)
	}

	// Check response
	var result map[string]any
	if err := json.Unmarshal(resp.Body, &result); err == nil {
		if code, ok := result["code"].(float64); ok && code == 0 {
			return true, nil
		}
		if msg, ok := result["message"].(string); ok {
			return false, fmt.Errorf("提交被拒绝: %s", msg)
		}
	}

	// Check if response contains success indicators
	text := string(resp.Body)
	if strings.Contains(text, `"code":0`) || strings.Contains(text, `"success"`) {
		return true, nil
	}

	return false, fmt.Errorf("提交失败: %s", truncate(text, 200))
}

func extractQQIdentifiers(surveyURL string) (string, string, error) {
	matches := urlRe.FindStringSubmatch(surveyURL)
	if len(matches) < 3 {
		return "", "", fmt.Errorf("无法从 URL 提取腾讯问卷 ID: %s", surveyURL)
	}
	return matches[1], matches[2], nil
}

func fetchSession(ctx context.Context, surveyID, hashValue string) (string, error) {
	url := fmt.Sprintf("%s/%s/session?hash=%s&_=%d", apiBase, surveyID, hashValue, time.Now().UnixMilli())
	resp, err := httpclient.Get(ctx, url, map[string]string{
		"User-Agent": defaultUA,
		"Referer":    fmt.Sprintf("https://wj.qq.com/s2/%s/%s/", surveyID, hashValue),
	}, nil, 10*time.Second)
	if err != nil {
		return "", err
	}

	// Extract answer_session_id from response
	var result map[string]any
	if err := json.Unmarshal(resp.Body, &result); err == nil {
		if data, ok := result["data"].(map[string]any); ok {
			if sessionID, ok := data["answer_session_id"].(string); ok {
				return sessionID, nil
			}
		}
	}
	// Session ID not found - continue without it
	return "", nil
}

func fetchRawQuestionsWithSession(ctx context.Context, surveyID, hashValue, sessionID string) ([]map[string]any, string, error) {
	for _, locale := range strings.Split(localeOrder, ",") {
		questionsURL := fmt.Sprintf("%s/%s/questions?hash=%s&locale=%s&_=%d",
			apiBase, surveyID, hashValue, locale, time.Now().UnixMilli())

		headers := map[string]string{
			"User-Agent": defaultUA,
			"Referer":    fmt.Sprintf("https://wj.qq.com/s2/%s/%s/", surveyID, hashValue),
		}
		if sessionID != "" {
			headers["X-Answer-Session"] = sessionID
		}

		resp, err := httpclient.Get(ctx, questionsURL, headers, nil, 10*time.Second)
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

		// Get title from meta
		title := ""
		metaURL := fmt.Sprintf("%s/%s/meta?hash=%s&locale=%s&_=%d",
			apiBase, surveyID, hashValue, locale, time.Now().UnixMilli())
		metaResp, err := httpclient.Get(ctx, metaURL, headers, nil, 10*time.Second)
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

func fetchQuestions(ctx context.Context, surveyID, hashValue string) ([]models.SurveyQuestionMeta, string, error) {
	rawQuestions, title, err := fetchRawQuestions(ctx, surveyID, hashValue)
	if err != nil {
		return nil, "", err
	}
	normalized := standardizeQuestions(rawQuestions)
	return normalized, title, nil
}

func fetchRawQuestions(ctx context.Context, surveyID, hashValue string) ([]map[string]any, string, error) {
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

		// Extract jump rules
		jumpRules := extractJumpRules(q)

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
			Page:               1,
			Provider:           ProviderName,
			ProviderQuestionID: questionID,
			ProviderPageID:     pageID,
			ProviderType:       providerType,
			ForcedOptionIndex:  forcedIdx,
			JumpRules:          jumpRules,
			HasJump:            len(jumpRules) > 0,
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

func buildAnswerActions(cfg *models.ExecutionConfig, state *models.ExecutionState, rawQuestions []map[string]any) []TencentAnswerAction {
	// Build a map from question ID to raw question for option ID lookup
	rawByQID := make(map[string]map[string]any)
	for _, rq := range rawQuestions {
		rawByQID[getString(rq, "id")] = rq
	}

	var actions []TencentAnswerAction
	for _, meta := range sortedQuestions(cfg) {
		rawQ := rawByQID[meta.ProviderQuestionID]
		action := buildSingleAction(cfg, meta, rawQ)
		if action != nil {
			actions = append(actions, *action)
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
			if questions[i].Num > questions[j].Num {
				questions[i], questions[j] = questions[j], questions[i]
			}
		}
	}
	return questions
}

func buildSingleAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, rawQ map[string]any) *TencentAnswerAction {
	typeCode := meta.TypeCode
	optionCount := meta.Options
	if optionCount <= 0 {
		optionCount = 1
	}

	configIdx := -1
	if idx, ok := cfg.QuestionConfigIndexMap[meta.Num]; ok {
		configIdx = parseConfigIdx(idx)
	}

	switch typeCode {
	case "3": // single
		return buildChoiceAnswer(cfg, meta, configIdx, optionCount, false, rawQ)
	case "4": // multiple
		return buildMultipleAnswer(cfg, meta, configIdx, optionCount, rawQ)
	case "5": // scale/nps
		return buildScaleAnswer(cfg, meta, configIdx, optionCount, rawQ)
	case "6": // matrix
		return buildMatrixAnswer(cfg, meta, configIdx, rawQ)
	case "7": // dropdown
		return buildChoiceAnswer(cfg, meta, configIdx, optionCount, true, rawQ)
	case "1": // text
		return buildTextAnswer(cfg, meta, configIdx)
	default:
		return nil
	}
}

// TencentAnswerAction represents a Tencent survey answer.
type TencentAnswerAction struct {
	QuestionID    string
	QuestionType  string
	SelectedIDs   []string
	SelectedTexts []string
	TextValue     string
	MatrixAnswers []TencentMatrixRow
}

type TencentMatrixRow struct {
	RowID      string
	OptionIDs  []string
}

func buildChoiceAnswer(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int, isDropdown bool, rawQ map[string]any) *TencentAnswerAction {
	probs := getProbs(cfg, configIdx, optionCount, isDropdown)
	idx := questions.WeightedIndex(probs, optionCount)

	// Get actual option ID from raw data
	optID := getOptionIDFromRaw(rawQ, idx)
	return &TencentAnswerAction{
		QuestionID:   meta.ProviderQuestionID,
		QuestionType: meta.ProviderType,
		SelectedIDs:  []string{optID},
	}
}

func buildMultipleAnswer(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int, rawQ map[string]any) *TencentAnswerAction {
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
	if allZero(probs) {
		for i := range probs {
			probs[i] = 1.0
		}
	}

	numSelect := minLimit + rand.Intn(maxLimit-minLimit+1)
	selected := questions.WeightedSampleWithoutReplacement(probs, optionCount, numSelect)
	ids := make([]string, len(selected))
	for i, idx := range selected {
		ids[i] = getOptionIDFromRaw(rawQ, idx)
	}

	return &TencentAnswerAction{
		QuestionID:   meta.ProviderQuestionID,
		QuestionType: meta.ProviderType,
		SelectedIDs:  ids,
	}
}

func buildScaleAnswer(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int, rawQ map[string]any) *TencentAnswerAction {
	probs := make([]float64, optionCount)
	if configIdx >= 0 && configIdx < len(cfg.ScaleProb) {
		if p, ok := toFloat64Slice(cfg.ScaleProb[configIdx]); ok {
			copy(probs, p)
		}
	}
	if allZero(probs) {
		for i := range probs {
			probs[i] = 1.0
		}
	}

	idx := questions.WeightedIndex(probs, optionCount)
	optID := getOptionIDFromRaw(rawQ, idx)
	return &TencentAnswerAction{
		QuestionID:   meta.ProviderQuestionID,
		QuestionType: meta.ProviderType,
		SelectedIDs:  []string{optID},
	}
}

func buildMatrixAnswer(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int, rawQ map[string]any) *TencentAnswerAction {
	rows := meta.Rows
	if rows <= 0 {
		rows = 1
	}
	optionCount := meta.Options
	if optionCount <= 0 {
		optionCount = 1
	}

	// Get raw sub_titles for option ID lookup
	subsRaw, _ := rawQ["sub_titles"].([]any)

	var matrixAnswers []TencentMatrixRow
	for i := 0; i < rows; i++ {
		probs := make([]float64, optionCount)
		if configIdx >= 0 && configIdx < len(cfg.MatrixProb) {
			if p, ok := toFloat64Slice(cfg.MatrixProb[configIdx]); ok {
				copy(probs, p)
			}
		}
		if allZero(probs) {
			for j := range probs {
				probs[j] = 1.0
			}
		}
		idx := questions.WeightedIndex(probs, optionCount)

		// Get actual option ID from raw sub_titles
		optID := getSubOptionIDFromRaw(subsRaw, i, idx)
		matrixAnswers = append(matrixAnswers, TencentMatrixRow{
			RowID:     fmt.Sprintf("%d", i+1),
			OptionIDs: []string{optID},
		})
	}

	return &TencentAnswerAction{
		QuestionID:    meta.ProviderQuestionID,
		QuestionType:  meta.ProviderType,
		MatrixAnswers: matrixAnswers,
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

// getSubOptionIDFromRaw extracts option ID from matrix sub_titles.
func getSubOptionIDFromRaw(subsRaw []any, rowIndex, optIndex int) string {
	if rowIndex >= len(subsRaw) {
		return fmt.Sprintf("%d", optIndex+1)
	}
	subMap, ok := subsRaw[rowIndex].(map[string]any)
	if !ok {
		return fmt.Sprintf("%d", optIndex+1)
	}
	optsRaw, ok := subMap["options"].([]any)
	if !ok || optIndex >= len(optsRaw) {
		return fmt.Sprintf("%d", optIndex+1)
	}
	if optMap, ok := optsRaw[optIndex].(map[string]any); ok {
		if id := getString(optMap, "id"); id != "" {
			return id
		}
	}
	return fmt.Sprintf("%d", optIndex+1)
}

func buildTextAnswer(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int) *TencentAnswerAction {
	text := "满意"
	if configIdx >= 0 && configIdx < len(cfg.Texts) && len(cfg.Texts[configIdx]) > 0 {
		text = cfg.Texts[configIdx][rand.Intn(len(cfg.Texts[configIdx]))]
	}
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
			for i, sub := range subsRaw {
				if subMap, ok := sub.(map[string]any); ok {
					// Build ALL options for this row (matching Python behavior)
					subOptsRaw, _ := subMap["options"].([]any)
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

func parseConfigIdx(s string) int {
	var idx int
	fmt.Sscanf(s, "%d", &idx)
	return idx
}

func allZero(probs []float64) bool {
	for _, p := range probs {
		if p != 0 {
			return false
		}
	}
	return true
}

func toFloat64Slice(v any) ([]float64, bool) {
	if v == nil {
		return nil, false
	}
	switch sl := v.(type) {
	case []float64:
		return sl, true
	case []any:
		result := make([]float64, 0, len(sl))
		for _, item := range sl {
			if f, ok := item.(float64); ok {
				result = append(result, f)
			}
		}
		return result, true
	}
	return nil, false
}

func getProbs(cfg *models.ExecutionConfig, configIdx, optionCount int, isDropdown bool) []float64 {
	probs := make([]float64, optionCount)
	source := cfg.SingleProb
	if isDropdown {
		source = cfg.DroplistProb
	}
	if configIdx >= 0 && configIdx < len(source) {
		if p, ok := toFloat64Slice(source[configIdx]); ok {
			copy(probs, p)
		}
	}
	if allZero(probs) {
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
