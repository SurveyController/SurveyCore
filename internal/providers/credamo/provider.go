package credamo

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
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
	ProviderName = "credamo"
	defaultUA    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	signCipher   = "P96D0A7D0M8C3R2D0M1"
	apiOrigin    = "https://www.credamo.com"
	resolution   = "1920px*1080px"
)

var shortURLRe = regexp.MustCompile(`(?i)(?:^|/)([A-Za-z0-9_-]+)/?$`)

// Provider implements the Credamo survey provider.
type Provider struct{}

func NewProvider() *Provider             { return &Provider{} }
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
	actions := buildAnswerActions(cfg, state, opts.ThreadName)

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

func sampleAnswerStartTimeMS(cfg *models.ExecutionConfig, initStartedAtMS int64, durationSeconds int) int64 {
	if cfg == nil {
		return initStartedAtMS
	}
	windowStartMS, windowEndMS := cfg.AnswerDatetimeWindowMS[0], cfg.AnswerDatetimeWindowMS[1]
	if windowStartMS <= 0 || windowEndMS <= windowStartMS {
		return initStartedAtMS
	}
	durationMS := int64(durationSeconds) * 1000
	if durationMS <= 0 {
		durationMS = 1
	}
	latestStartMS := windowEndMS - durationMS
	if latestStartMS <= windowStartMS {
		return windowStartMS
	}
	return windowStartMS + rand.Int63n(latestStartMS-windowStartMS+1)
}

func extractShortURL(surveyURL string) string {
	text := strings.TrimSpace(surveyURL)
	if text == "" {
		return ""
	}
	parseText := text
	if !strings.Contains(parseText, "://") {
		parseText = "https://" + parseText
	}
	u, err := url.Parse(parseText)
	if err != nil {
		return ""
	}

	fragmentPath := strings.TrimRight(u.Fragment, "/")
	if strings.HasPrefix(strings.ToLower(fragmentPath), "/s/") {
		if id := lastPathSegment(fragmentPath); id != "" {
			return id
		}
	}

	path := strings.TrimRight(u.Path, "/")
	if strings.HasPrefix(strings.ToLower(path), "/s/") {
		return lastPathSegment(path)
	}
	if strings.EqualFold(path, "/answer.html") {
		return ""
	}

	if matches := shortURLRe.FindStringSubmatch(path); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func lastPathSegment(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

func fetchDetail(ctx context.Context, shortURL string) (map[string]any, error) {
	data, _, err := fetchDetailSession(ctx, shortURL, "")
	return data, err
}

func fetchDetailSession(ctx context.Context, shortURL, cookieHeader string) (map[string]any, string, error) {
	fetchURL := fmt.Sprintf("%s/v1/survey/noauth/detail/get/%s", apiOrigin, shortURL)
	headers := credamoHeaders(shortURL, defaultUA, "", false)
	if cookieHeader != "" {
		headers["Cookie"] = cookieHeader
	}
	resp, err := httpclient.Get(ctx, fetchURL, headers, nil, 15*time.Second)
	if err != nil {
		return nil, "", err
	}
	nextCookieHeader := responseCookieHeader(resp)

	var result map[string]any
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, nextCookieHeader, fmt.Errorf("解析响应失败: %w", err)
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		return nil, nextCookieHeader, fmt.Errorf("响应格式错误: %s", truncate(string(resp.Body), 200))
	}
	return data, nextCookieHeader, nil
}

func initAnswer(ctx context.Context, shortURL, cookieHeader string) (map[string]any, string, error) {
	timeCode := newTimeCode()
	initURL := fmt.Sprintf("%s/v1/survey/answer/noauth/init/%s?timeCode=%s&accountCode=CDM&resolution=%s",
		apiOrigin, shortURL, url.QueryEscape(timeCode), url.QueryEscape(resolution))
	headers := credamoHeaders(shortURL, defaultUA, "", false)
	if cookieHeader != "" {
		headers["Cookie"] = cookieHeader
	}
	resp, err := httpclient.Get(ctx, initURL, headers, nil, 10*time.Second)
	if err != nil {
		return nil, "", err
	}
	nextCookieHeader := responseCookieHeader(resp)

	var result map[string]any
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, nextCookieHeader, err
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		return nil, nextCookieHeader, fmt.Errorf("初始化响应格式错误: %s", truncate(string(resp.Body), 200))
	}
	if getString(data, "timeCode") == "" {
		data["timeCode"] = timeCode
	}
	return data, nextCookieHeader, nil
}

func saveAnswers(ctx context.Context, shortURL, answerToken, timeCode string, body map[string]any, ua, cookieHeader string, proxyAddr *string) (bool, error) {
	nonce := fmt.Sprintf("%016x", rand.Int63())
	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	unionID := fmt.Sprintf("%016x", rand.Int63())

	signature := computeSignature(answerToken, nonce, timestamp, unionID)

	saveURL := fmt.Sprintf("%s/v1/survey/answer/noauth/save?timeCode=%s&answerToken=%s",
		apiOrigin, timeCode, answerToken)

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return false, fmt.Errorf("提交 JSON 构造失败: %w", err)
	}

	headers := credamoHeaders(shortURL, ua, answerToken, true)
	headers["unionId"] = unionID
	headers["nonce"] = nonce
	headers["timestamp"] = timestamp
	headers["signature"] = signature
	if cookieHeader != "" {
		headers["Cookie"] = cookieHeader
	}
	resp, err := httpclient.Post(ctx, saveURL, string(bodyBytes), headers, proxyAddr, 20*time.Second)
	if err != nil {
		return false, fmt.Errorf("提交失败: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body, &result); err == nil {
		if classifyCredamoPayload(result) == SubmitSuccess {
			return true, nil
		}
		if msg, ok := result["message"].(string); ok {
			return false, fmt.Errorf("提交被拒绝: %s", msg)
		}
	}

	text := string(resp.Body)
	if strings.Contains(text, `"code":0`) || strings.Contains(text, `"success":true`) {
		return true, nil
	}
	return false, fmt.Errorf("提交失败: %s", truncate(text, 200))
}

func credamoHeaders(shortURL, ua, answerToken string, jsonBody bool) map[string]string {
	headers := map[string]string{
		"User-Agent":      ua,
		"Accept":          "application/json, text/plain, */*",
		"Accept-Language": "zh-CN,zh;q=0.9",
		"Referer":         fmt.Sprintf("%s/answer.html#/s/%s", apiOrigin, shortURL),
	}
	for key, value := range buildSignatureHeaders(answerToken) {
		headers[key] = value
	}
	if jsonBody {
		headers["Origin"] = apiOrigin
		headers["Content-Type"] = "application/json"
	}
	return headers
}

func buildSignatureHeaders(answerToken string) map[string]string {
	nonce := randomCredamoToken(16)
	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	unionID := randomCredamoToken(10)
	return map[string]string{
		"unionId":   unionID,
		"nonce":     nonce,
		"timestamp": timestamp,
		"signature": computeSignature(answerToken, nonce, timestamp, unionID),
	}
}

func randomCredamoToken(length int) string {
	const chars = "ABCDEFGHJKMNPQRSTWXYZabcdefhijkmnprstwxyz2345678"
	if length <= 0 {
		length = 1
	}
	var b strings.Builder
	for i := 0; i < length; i++ {
		b.WriteByte(chars[rand.Intn(len(chars))])
	}
	return b.String()
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

func mergeCookieHeaders(values ...string) string {
	seen := make(map[string]string)
	var order []string
	for _, value := range values {
		for _, part := range strings.Split(value, ";") {
			cookie := strings.TrimSpace(part)
			if cookie == "" {
				continue
			}
			name := cookie
			if idx := strings.Index(cookie, "="); idx >= 0 {
				name = cookie[:idx]
			}
			if _, ok := seen[name]; !ok {
				order = append(order, name)
			}
			seen[name] = cookie
		}
	}
	var cookies []string
	for _, name := range order {
		cookies = append(cookies, seen[name])
	}
	return strings.Join(cookies, "; ")
}

func newTimeCode() string {
	return fmt.Sprintf("%08x%04x%04x%04x%012x",
		rand.Int31(), rand.Int31n(0xffff), rand.Int31n(0xffff),
		rand.Int31n(0xffff), rand.Int63n(0xffffffffffff))
}

const (
	SubmitSuccess = "success"
	SubmitFailed  = "failed"
)

func classifyCredamoPayload(payload map[string]any) string {
	if success, ok := payload["success"].(bool); ok && !success {
		return SubmitFailed
	}
	return SubmitSuccess
}

// computeSignature implements Credamo's double SHA1 signature scheme.
func computeSignature(token, nonce, timestamp, unionID string) string {
	// Inner hash: SHA1(token + nonce + timestamp + unionID + cipher)
	innerInput := token + nonce + timestamp + unionID + signCipher
	innerHash := sha1.Sum([]byte(innerInput))
	innerHex := strings.ToUpper(hex.EncodeToString(innerHash[:]))

	// Outer hash: SHA1(token + nonce + timestamp + innerHex + unionID + cipher)
	outerInput := token + nonce + timestamp + innerHex + unionID + signCipher
	outerHash := sha1.Sum([]byte(outerInput))
	return strings.ToUpper(hex.EncodeToString(outerHash[:]))
}

// Type code mapping
var credamoTypeMap = map[string]string{
	"single_choice":   "3",
	"single":          "3",
	"multiple_choice": "4",
	"multiple":        "4",
	"scale":           "5",
	"matrix":          "6",
	"dropdown":        "7",
	"ordering":        "11",
	"order":           "11",
	"text":            "1",
	"textarea":        "1",
	"description":     "0",
}

func standardizeQuestions(raw []map[string]any) []models.SurveyQuestionMeta {
	var result []models.SurveyQuestionMeta
	num := 1

	for _, q := range raw {
		providerType := rawQuestionKind(q)
		typeCode := credamoTypeMap[providerType]
		if typeCode == "" {
			typeCode = "1"
		}

		questionNum := rawQuestionNum(q, num)
		title := firstString(q, "title", "qstTitle", "qstName", "questionTitle", "questionName", "display", "content", "name")
		if title == "" {
			title = fmt.Sprintf("Q%d", questionNum)
		}
		questionID := rawQuestionID(q)
		if questionID == "" {
			questionID = fmt.Sprintf("%d", questionNum)
		}

		optionTexts := extractOptionTexts(q)
		options := len(optionTexts)

		// Extract forced option
		var forcedIdx *int
		forceIdx, forcedText := extractForceSelect(title, optionTexts)
		if forceIdx >= 0 {
			forcedIdx = &forceIdx
		}

		// Multi-select limits
		minLimit, maxLimit := extractMultiSelectLimits(title, options)

		// Row texts for matrix
		rowTexts := extractRowTexts(q)
		rows := len(rowTexts)
		if typeCode == "6" && rows <= 0 {
			rows = 1
		}

		qm := models.SurveyQuestionMeta{
			Num:                questionNum,
			Title:              title,
			TypeCode:           typeCode,
			Options:            options,
			OptionTexts:        optionTexts,
			Rows:               rows,
			RowTexts:           rowTexts,
			Page:               1,
			Provider:           ProviderName,
			ProviderQuestionID: questionID,
			ProviderType:       providerType,
			ForcedOptionIndex:  forcedIdx,
			ForcedOptionText:   forcedText,
		}

		if minLimit != nil {
			qm.MultiMinLimit = minLimit
		}
		if maxLimit != nil {
			qm.MultiMaxLimit = maxLimit
		}
		if typeCode == "1" {
			qm.IsTextLike = true
		}
		if typeCode == "5" {
			qm.IsRating = true
			qm.RatingMax = options
		}

		result = append(result, qm)
		num++
	}
	return result
}

func extractOptionTexts(q map[string]any) []string {
	source := "options"
	if rawQuestionType(q) == 4 {
		source = "answers"
	}
	items := getArray(q, source)
	if len(items) == 0 && source != "choices" {
		items = getArray(q, "choices")
	}
	var texts []string
	for idx, opt := range items {
		text := choiceText(opt)
		if text == "" {
			text = fmt.Sprintf("选项 %d", idx+1)
		}
		texts = append(texts, text)
	}
	return texts
}

func extractRowTexts(q map[string]any) []string {
	var rows []string
	items := getArray(q, "sub_questions")
	if rawQuestionType(q) == 4 {
		items = getArray(q, "choices")
	}
	for idx, sub := range items {
		text := choiceText(sub)
		if text == "" && rawQuestionType(q) == 4 {
			text = fmt.Sprintf("第 %d 行", idx+1)
		}
		if text != "" {
			rows = append(rows, text)
		}
	}
	return rows
}

func iterRawQuestions(detail map[string]any) []map[string]any {
	result := append([]map[string]any{}, getArray(detail, "questions")...)
	for _, block := range getArray(detail, "blocks") {
		elements := getArray(block, "blockElements")
		if len(elements) == 0 {
			elements = getArray(block, "elements")
		}
		for _, element := range elements {
			candidates := []any{element["question"], element["qst"], element["surveyQuestion"], element}
			for _, candidate := range candidates {
				candidateMap, ok := candidate.(map[string]any)
				if !ok {
					continue
				}
				if rawQuestionID(candidateMap) != "" || rawQuestionType(candidateMap) > 0 {
					result = append(result, candidateMap)
					break
				}
			}
		}
	}
	return result
}

func extractForceSelect(title string, optionTexts []string) (int, string) {
	compactTitle := strings.ReplaceAll(title, " ", "")
	for i, opt := range optionTexts {
		text := strings.TrimSpace(opt)
		if text != "" && strings.Contains(compactTitle, strings.ReplaceAll(text, " ", "")) {
			return i, text
		}
	}
	re := regexp.MustCompile(`请(选择|勾选)\s*([A-Z])`)
	if match := re.FindStringSubmatch(title); match != nil {
		letter := match[2]
		for i, opt := range optionTexts {
			if strings.HasPrefix(strings.ToUpper(opt), letter) {
				return i, opt
			}
		}
	}
	return -1, ""
}

func extractMultiSelectLimits(title string, optionCount int) (*int, *int) {
	reMin := regexp.MustCompile(`至少\s*(\d+)\s*项|最少\s*(\d+)\s*项`)
	reMax := regexp.MustCompile(`至多\s*(\d+)\s*项|最多\s*(\d+)\s*项|不超过\s*(\d+)\s*项`)

	var minLimit, maxLimit *int

	if match := reMin.FindStringSubmatch(title); match != nil {
		s := match[1]
		if s == "" {
			s = match[2]
		}
		if v, err := strconv.Atoi(s); err == nil {
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

func buildAnswerActions(cfg *models.ExecutionConfig, state *models.ExecutionState, threadName string) []CredamoAnswerAction {
	runtime := questions.NewRunContextForThread(cfg, state, threadName)
	var actions []CredamoAnswerAction
	for _, meta := range sortedQuestions(cfg) {
		action := buildSingleAction(cfg, meta, runtime)
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

// CredamoAnswerAction represents a Credamo survey answer.
type CredamoAnswerAction struct {
	QuestionID      string
	QuestionType    string
	SelectedIndices []int
	TextValue       string
	MatrixAnswers   [][]int
	OrderIndices    []int
}

func buildSingleAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, runtime *questions.RunContext) *CredamoAnswerAction {
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
		return buildChoiceAction(cfg, meta, configIdx, optionCount, runtime)
	case "4": // multiple
		return buildMultipleAction(cfg, meta, configIdx, optionCount, runtime)
	case "5": // scale
		return buildScaleAction(cfg, meta, configIdx, optionCount, runtime)
	case "6": // matrix
		return buildMatrixAction(cfg, meta, configIdx, runtime)
	case "7": // dropdown
		return buildChoiceAction(cfg, meta, configIdx, optionCount, runtime)
	case "11": // order
		return buildOrderAction(meta, optionCount)
	case "1": // text
		return buildTextAction(cfg, meta, configIdx, runtime)
	default:
		return nil
	}
}

func buildChoiceAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int, runtime *questions.RunContext) *CredamoAnswerAction {
	if meta.ForcedOptionIndex != nil && *meta.ForcedOptionIndex >= 0 {
		idx := *meta.ForcedOptionIndex
		if idx >= optionCount {
			idx = 0
		}
		return &CredamoAnswerAction{
			QuestionID:      meta.ProviderQuestionID,
			QuestionType:    meta.ProviderType,
			SelectedIndices: []int{idx},
		}
	}

	probs := make([]float64, optionCount)
	if configIdx >= 0 && configIdx < len(cfg.SingleProb) {
		if p, ok := providerutil.Float64Slice(cfg.SingleProb[configIdx]); ok {
			copy(probs, p)
		}
	}
	if providerutil.AllZero(probs) {
		for i := range probs {
			probs[i] = 1.0
		}
	}

	idx := runtime.ChooseSingle(meta, configIdx, optionCount, probs, nil)
	return &CredamoAnswerAction{
		QuestionID:      meta.ProviderQuestionID,
		QuestionType:    meta.ProviderType,
		SelectedIndices: []int{idx},
	}
}

func buildMultipleAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int, runtime *questions.RunContext) *CredamoAnswerAction {
	probs := make([]float64, optionCount)
	if configIdx >= 0 && configIdx < len(cfg.MultipleProb) {
		copy(probs, cfg.MultipleProb[configIdx])
	}
	if providerutil.AllZero(probs) {
		for i := range probs {
			probs[i] = 1.0
		}
	}

	minLimit := 1
	maxLimit := optionCount
	if meta.MultiMinLimit != nil {
		minLimit = *meta.MultiMinLimit
	}
	if meta.MultiMaxLimit != nil {
		maxLimit = *meta.MultiMaxLimit
	}
	selected := runtime.ChooseMultiple(meta, configIdx, optionCount, minLimit, maxLimit, probs)

	return &CredamoAnswerAction{
		QuestionID:      meta.ProviderQuestionID,
		QuestionType:    meta.ProviderType,
		SelectedIndices: selected,
	}
}

func buildScaleAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int, runtime *questions.RunContext) *CredamoAnswerAction {
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
	return &CredamoAnswerAction{
		QuestionID:      meta.ProviderQuestionID,
		QuestionType:    meta.ProviderType,
		SelectedIndices: []int{idx},
	}
}

func buildMatrixAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int, runtime *questions.RunContext) *CredamoAnswerAction {
	rows := meta.Rows
	if rows <= 0 {
		rows = 1
	}
	optionCount := meta.Options
	if optionCount <= 0 {
		optionCount = 1
	}

	var matrixAnswers [][]int
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
		matrixAnswers = append(matrixAnswers, []int{idx})
	}

	return &CredamoAnswerAction{
		QuestionID:    meta.ProviderQuestionID,
		QuestionType:  meta.ProviderType,
		MatrixAnswers: matrixAnswers,
	}
}

func buildOrderAction(meta models.SurveyQuestionMeta, optionCount int) *CredamoAnswerAction {
	indices := make([]int, optionCount)
	for i := range indices {
		indices[i] = i
	}
	// Shuffle
	for i := len(indices) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		indices[i], indices[j] = indices[j], indices[i]
	}
	return &CredamoAnswerAction{
		QuestionID:   meta.ProviderQuestionID,
		QuestionType: meta.ProviderType,
		OrderIndices: indices,
	}
}

func buildTextAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int, runtime *questions.RunContext) *CredamoAnswerAction {
	text := "满意"
	if candidate, ok := questions.ChooseConfiguredTextCandidate(cfg, configIdx); ok {
		text = candidate
	}
	text = runtime.GenerateText(meta, configIdx, text, 1)
	return &CredamoAnswerAction{
		QuestionID:   meta.ProviderQuestionID,
		QuestionType: meta.ProviderType,
		TextValue:    text,
	}
}

func buildSubmitBody(shortURL string, rawQuestions []map[string]any, actions []CredamoAnswerAction, cfg *models.ExecutionConfig, startMS int64, duration int) map[string]any {
	actionMap := make(map[string]CredamoAnswerAction)
	for _, a := range actions {
		actionMap[a.QuestionID] = a
	}

	// Per-question timing: distribute duration evenly
	perQuestionMS := int64(0)
	if len(actions) > 0 {
		perQuestionMS = int64(duration) * 1000 / int64(len(actions))
	}

	var qstList []map[string]any
	for _, rq := range rawQuestions {
		qID := rawQuestionID(rq)
		qType := rawQuestionKind(rq)

		action, ok := actionMap[qID]
		if !ok {
			continue
		}

		qst := map[string]any{
			"qstId":         normalizeID(qID),
			"answerTime":    perQuestionMS,
			"answerContent": "",
		}
		if rawQuestionType(rq) == 2 && rawSelector(rq) > 0 {
			qst["questionType"] = 2
			qst["subSelector"] = rawSelector(rq)
		}

		// Get raw options for building proper answer format
		rawOptions := rawChoiceOptions(rq)
		rawRows := rawMatrixRows(rq)
		rawColumns := rawMatrixColumns(rq)

		switch typeCodeForKind(qType) {
		case "3", "5", "7": // single/scale/dropdown
			if len(action.SelectedIndices) > 0 {
				idx := action.SelectedIndices[0]
				if forced := forcedChoiceIndex(cfg, qID, rawOptions); forced >= 0 {
					idx = forced
				}
				if idx < len(rawOptions) {
					opt := rawOptions[idx]
					qst["answerQstChoice"] = choicePayload(opt)
				}
			}
		case "4": // multiple
			var choiceList []map[string]any
			for _, idx := range action.SelectedIndices {
				if idx < len(rawOptions) {
					opt := rawOptions[idx]
					choiceList = append(choiceList, choicePayload(opt))
				}
			}
			qst["answerQstChoiceList"] = choiceList
		case "6": // matrix
			var choiceList []map[string]any
			for rowIdx, colIndices := range action.MatrixAnswers {
				if rowIdx < len(rawRows) {
					row := rawRows[rowIdx]
					subOptions := getArray(row, "options")
					if len(subOptions) == 0 {
						subOptions = rawColumns
					}
					var answerList []map[string]any
					for _, colIdx := range colIndices {
						if colIdx < len(subOptions) {
							opt := subOptions[colIdx]
							answerList = append(answerList, map[string]any{
								"answerId": choiceID(opt, "answerId", "id", "choiceId"),
							})
						}
					}
					if len(answerList) > 0 {
						choiceList = append(choiceList, map[string]any{
							"choiceId":         choiceID(row, "choiceId", "id"),
							"choiceAnswerList": answerList,
						})
					}
				}
			}
			qst["answerQstChoiceList"] = choiceList
		case "11": // order
			var choiceList []map[string]any
			for rank, idx := range action.OrderIndices {
				if idx < len(rawOptions) {
					opt := rawOptions[idx]
					choiceList = append(choiceList, map[string]any{
						"choiceId":      choiceID(opt, "choiceId", "id"),
						"choiceContent": rank + 1,
					})
				}
			}
			qst["answerChoiceContent"] = choiceList
		case "1": // text
			qst["answerContent"] = action.TextValue
		}

		qstList = append(qstList, qst)
	}

	return map[string]any{
		"answerStartTime": startMS,
		"answerEndTime":   startMS + int64(duration)*1000,
		"status":          1,
		"shortUrl":        shortURL,
		"resolution":      "1920px*1080px",
		"sourceDetail":    1,
		"answerQstList":   qstList,
	}
}

func typeCodeForKind(kind string) string {
	if code := credamoTypeMap[strings.ToLower(strings.TrimSpace(kind))]; code != "" {
		return code
	}
	return credamoTypeMap[rawQuestionKind(map[string]any{"questionType": kind})]
}

// Helpers

func int64FromAny(value any, fallback int64) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return n
		}
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func rawQuestionID(m map[string]any) string {
	return firstString(m, "qstId", "id", "questionId")
}

func rawQuestionNum(m map[string]any, fallback int) int {
	for _, key := range []string{"qstNo", "questionNo", "qstNum", "sortNo"} {
		match := regexp.MustCompile(`\d+`).FindString(valueString(m[key]))
		if match != "" {
			if num, err := strconv.Atoi(match); err == nil && num > 0 {
				return num
			}
		}
	}
	return fallback
}

func rawQuestionType(m map[string]any) int {
	return toInt(m["questionType"])
}

func rawSelector(m map[string]any) int {
	return toInt(m["selector"])
}

func rawQuestionKind(m map[string]any) string {
	if kind := strings.ToLower(strings.TrimSpace(firstString(m, "question_kind", "provider_type", "type"))); kind != "" {
		return kind
	}
	questionType := rawQuestionType(m)
	selector := rawSelector(m)
	switch {
	case questionType == 2 && selector == 2:
		return "multiple"
	case questionType == 2 && selector == 3:
		return "dropdown"
	case questionType == 2:
		return "single"
	case questionType == 4:
		return "matrix"
	case questionType == 6:
		return "order"
	case questionType == 11:
		return "scale"
	case questionType == 1:
		return "text"
	}
	return ""
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s := strings.TrimSpace(valueString(m[key])); s != "" {
			return s
		}
	}
	return ""
}

func valueString(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case float64:
		if value == float64(int64(value)) {
			return strconv.FormatInt(int64(value), 10)
		}
		return strconv.FormatFloat(value, 'f', -1, 64)
	case float32:
		f := float64(value)
		if f == float64(int64(f)) {
			return strconv.FormatInt(int64(f), 10)
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func toInt(v any) int {
	switch value := v.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case float32:
		return int(value)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(value))
		return i
	default:
		return 0
	}
}

func rawChoiceOptions(q map[string]any) []map[string]any {
	if options := getArray(q, "options"); len(options) > 0 {
		return options
	}
	return getArray(q, "choices")
}

func rawMatrixRows(q map[string]any) []map[string]any {
	if rows := getArray(q, "sub_questions"); len(rows) > 0 {
		return rows
	}
	return getArray(q, "choices")
}

func rawMatrixColumns(q map[string]any) []map[string]any {
	if columns := getArray(q, "answers"); len(columns) > 0 {
		return columns
	}
	return rawChoiceOptions(q)
}

func choiceText(m map[string]any) string {
	return firstString(m, "text", "label", "display", "answerContent", "choiceContent", "choiceTitle", "answerTitle", "content", "title", "name")
}

func choiceID(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := m[key]; ok && strings.TrimSpace(valueString(value)) != "" {
			return normalizeID(value)
		}
	}
	return ""
}

func normalizeID(value any) any {
	text := strings.TrimSpace(valueString(value))
	if text == "" {
		return ""
	}
	if id, err := strconv.Atoi(text); err == nil {
		return id
	}
	return text
}

func choicePayload(choice map[string]any) map[string]any {
	return map[string]any{
		"choiceId":      choiceID(choice, "choiceId", "id"),
		"choiceContent": choiceText(choice),
	}
}

func forcedChoiceIndex(cfg *models.ExecutionConfig, questionID string, choices []map[string]any) int {
	if cfg == nil || questionID == "" {
		return -1
	}
	meta, ok := cfg.ProviderQuestionMetadataMap[questionID]
	if !ok {
		for _, candidate := range cfg.QuestionsMetadata {
			if candidate.ProviderQuestionID == questionID {
				meta = candidate
				ok = true
				break
			}
		}
	}
	if !ok {
		return -1
	}
	if meta.ForcedOptionText != "" {
		target := strings.TrimSpace(meta.ForcedOptionText)
		for idx, choice := range choices {
			if choiceText(choice) == target {
				return idx
			}
		}
	}
	if meta.ForcedOptionIndex != nil && *meta.ForcedOptionIndex >= 0 && *meta.ForcedOptionIndex < len(choices) {
		return *meta.ForcedOptionIndex
	}
	return -1
}

func getString(m map[string]any, key string) string {
	return strings.TrimSpace(valueString(m[key]))
}

func getArray(m map[string]any, key string) []map[string]any {
	if arr, ok := m[key].([]any); ok {
		result := make([]map[string]any, 0, len(arr))
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				result = append(result, m)
			}
		}
		return result
	}
	if arr, ok := m[key].([]map[string]any); ok {
		return arr
	}
	return nil
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
