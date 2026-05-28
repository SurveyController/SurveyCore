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

	"github.com/SurveyController/SurveyController-Go/internal/models"
	"github.com/SurveyController/SurveyController-Go/internal/network/httpclient"
	"github.com/SurveyController/SurveyController-Go/internal/questions"
)

const (
	ProviderName  = "credamo"
	defaultUA     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	signCipher    = "P96D0A7D0M8C3R2D0M1"
	apiOrigin     = "https://www.credamo.com"
)

var shortURLRe = regexp.MustCompile(`/([A-Za-z0-9_-]+)\s*$`)

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
	rawQuestions := getArray(detail, "questions")

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
	detail, err := fetchDetail(ctx, shortURL)
	if err != nil {
		return false, fmt.Errorf("获取详情失败: %w", err)
	}

	rawQuestions := getArray(detail, "questions")

	// Init answer session
	initData, err := initAnswer(ctx, shortURL)
	if err != nil {
		return false, fmt.Errorf("初始化答题会话失败: %w", err)
	}

	// Build actions
	actions := buildAnswerActions(cfg, state)

	// Build submit body
	startTimeMS := time.Now().UnixMilli() - int64(10+rand.Intn(20))*1000
	duration := 10 + rand.Intn(20)
	ua := opts.UserAgent
	if ua == "" {
		ua = defaultUA
	}

	body := buildSubmitBody(shortURL, rawQuestions, actions, cfg, startTimeMS, duration)

	// Sign and submit
	answerToken := getString(initData, "answerToken")
	timeCode := getString(initData, "timeCode")

	proxyAddr := parseProxy(opts.ProxyAddress)
	return saveAnswers(ctx, shortURL, answerToken, timeCode, body, ua, proxyAddr)
}

func extractShortURL(surveyURL string) string {
	matches := shortURLRe.FindStringSubmatch(surveyURL)
	if len(matches) >= 2 {
		return matches[1]
	}
	// Try parsing as URL
	u, err := url.Parse(surveyURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.TrimRight(u.Path, "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

func fetchDetail(ctx context.Context, shortURL string) (map[string]any, error) {
	fetchURL := fmt.Sprintf("%s/v1/survey/noauth/detail/get/%s", apiOrigin, shortURL)
	resp, err := httpclient.Get(ctx, fetchURL, map[string]string{
		"User-Agent": defaultUA,
		"Origin":     apiOrigin,
		"Referer":    fmt.Sprintf("%s/s/%s", apiOrigin, shortURL),
	}, nil, 15*time.Second)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("响应格式错误")
	}
	return data, nil
}

func initAnswer(ctx context.Context, shortURL string) (map[string]any, error) {
	initURL := fmt.Sprintf("%s/v1/survey/answer/noauth/init/%s?accountCode=CDM&resolution=1920px*1080px", apiOrigin, shortURL)
	resp, err := httpclient.Get(ctx, initURL, map[string]string{
		"User-Agent": defaultUA,
		"Origin":     apiOrigin,
		"Referer":    fmt.Sprintf("%s/s/%s", apiOrigin, shortURL),
	}, nil, 10*time.Second)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, err
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("初始化响应格式错误")
	}
	return data, nil
}

func saveAnswers(ctx context.Context, shortURL, answerToken, timeCode string, body map[string]any, ua string, proxyAddr *string) (bool, error) {
	nonce := fmt.Sprintf("%016x", rand.Int63())
	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	unionID := fmt.Sprintf("%016x", rand.Int63())

	signature := computeSignature(answerToken, nonce, timestamp, unionID)

	saveURL := fmt.Sprintf("%s/v1/survey/answer/noauth/save?timeCode=%s&answerToken=%s",
		apiOrigin, timeCode, answerToken)

	bodyBytes, _ := json.Marshal(body)

	resp, err := httpclient.Post(ctx, saveURL, string(bodyBytes), map[string]string{
		"User-Agent":  ua,
		"Content-Type": "application/json",
		"Origin":      apiOrigin,
		"Referer":     fmt.Sprintf("%s/s/%s", apiOrigin, shortURL),
		"unionId":     unionID,
		"nonce":       nonce,
		"timestamp":   timestamp,
		"signature":   signature,
	}, proxyAddr, 20*time.Second)
	if err != nil {
		return false, fmt.Errorf("提交失败: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body, &result); err == nil {
		if code, ok := result["code"].(float64); ok && code == 0 {
			return true, nil
		}
		if msg, ok := result["message"].(string); ok {
			return false, fmt.Errorf("提交被拒绝: %s", msg)
		}
	}

	text := string(resp.Body)
	if strings.Contains(text, `"code":0`) {
		return true, nil
	}
	return false, fmt.Errorf("提交失败: %s", truncate(text, 200))
}

// computeSignature implements Credamo's double SHA1 signature scheme.
func computeSignature(token, nonce, timestamp, unionID string) string {
	// Inner hash: SHA1(token + nonce + timestamp + unionID + cipher)
	innerInput := token + nonce + timestamp + unionID + signCipher
	innerHash := sha1.Sum([]byte(innerInput))
	innerHex := hex.EncodeToString(innerHash[:])

	// Outer hash: SHA1(token + nonce + timestamp + innerHex + unionID + cipher)
	outerInput := token + nonce + timestamp + innerHex + unionID + signCipher
	outerHash := sha1.Sum([]byte(outerInput))
	return hex.EncodeToString(outerHash[:])
}

// Type code mapping
var credamoTypeMap = map[string]string{
	"single_choice":   "3",
	"multiple_choice": "4",
	"scale":           "5",
	"matrix":          "6",
	"dropdown":        "7",
	"ordering":        "11",
	"text":            "1",
	"textarea":        "1",
	"description":     "0",
}

func standardizeQuestions(raw []map[string]any) []models.SurveyQuestionMeta {
	var result []models.SurveyQuestionMeta
	num := 1

	for _, q := range raw {
		providerType := getString(q, "question_kind")
		if providerType == "" {
			providerType = getString(q, "type")
		}
		typeCode := credamoTypeMap[providerType]
		if typeCode == "" {
			typeCode = "1"
		}

		title := getString(q, "title")
		questionID := getString(q, "id")
		if questionID == "" {
			questionID = fmt.Sprintf("%d", num)
		}

		optionTexts := extractOptionTexts(q)
		options := len(optionTexts)

		// Extract forced option
		var forcedIdx *int
		forceIdx := extractForceSelect(title, optionTexts)
		if forceIdx >= 0 {
			forcedIdx = &forceIdx
		}

		// Multi-select limits
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
			Page:               1,
			Provider:           ProviderName,
			ProviderQuestionID: questionID,
			ProviderType:       providerType,
			ForcedOptionIndex:  forcedIdx,
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
	if optsRaw, ok := q["options"].([]any); ok {
		var texts []string
		for _, opt := range optsRaw {
			if optMap, ok := opt.(map[string]any); ok {
				text := getString(optMap, "text")
				if text == "" {
					text = getString(optMap, "label")
				}
				if text != "" {
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
	if subsRaw, ok := q["sub_questions"].([]any); ok {
		for _, sub := range subsRaw {
			if subMap, ok := sub.(map[string]any); ok {
				if text := getString(subMap, "text"); text != "" {
					rows = append(rows, text)
				}
			}
		}
	}
	return rows
}

func extractForceSelect(title string, optionTexts []string) int {
	re := regexp.MustCompile(`请(选择|勾选)\s*([A-Z])`)
	if match := re.FindStringSubmatch(title); match != nil {
		letter := match[2]
		for i, opt := range optionTexts {
			if strings.HasPrefix(strings.ToUpper(opt), letter) {
				return i
			}
		}
	}
	return -1
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

func buildAnswerActions(cfg *models.ExecutionConfig, state *models.ExecutionState) []CredamoAnswerAction {
	var actions []CredamoAnswerAction
	for _, meta := range sortedQuestions(cfg) {
		action := buildSingleAction(cfg, meta)
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
	QuestionID     string
	QuestionType   string
	SelectedIndices []int
	TextValue      string
	MatrixAnswers  [][]int
	OrderIndices   []int
}

func buildSingleAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta) *CredamoAnswerAction {
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
		return buildChoiceAction(cfg, meta, configIdx, optionCount)
	case "4": // multiple
		return buildMultipleAction(cfg, meta, configIdx, optionCount)
	case "5": // scale
		return buildScaleAction(cfg, meta, configIdx, optionCount)
	case "6": // matrix
		return buildMatrixAction(cfg, meta, configIdx)
	case "7": // dropdown
		return buildChoiceAction(cfg, meta, configIdx, optionCount)
	case "11": // order
		return buildOrderAction(meta, optionCount)
	case "1": // text
		return buildTextAction(cfg, meta, configIdx)
	default:
		return nil
	}
}

func buildChoiceAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int) *CredamoAnswerAction {
	probs := make([]float64, optionCount)
	if configIdx >= 0 && configIdx < len(cfg.SingleProb) {
		if p, ok := toFloat64Slice(cfg.SingleProb[configIdx]); ok {
			copy(probs, p)
		}
	}
	if allZero(probs) {
		for i := range probs {
			probs[i] = 1.0
		}
	}

	idx := questions.WeightedIndex(probs, optionCount)
	return &CredamoAnswerAction{
		QuestionID:      meta.ProviderQuestionID,
		QuestionType:    meta.ProviderType,
		SelectedIndices: []int{idx},
	}
}

func buildMultipleAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int) *CredamoAnswerAction {
	probs := make([]float64, optionCount)
	if configIdx >= 0 && configIdx < len(cfg.MultipleProb) {
		copy(probs, cfg.MultipleProb[configIdx])
	}
	if allZero(probs) {
		for i := range probs {
			probs[i] = 1.0
		}
	}

	// Select options with positive weight
	var selected []int
	for i, p := range probs[:optionCount] {
		if p > 0 {
			selected = append(selected, i)
		}
	}
	if len(selected) == 0 {
		// Random single selection
		selected = []int{rand.Intn(optionCount)}
	}

	return &CredamoAnswerAction{
		QuestionID:      meta.ProviderQuestionID,
		QuestionType:    meta.ProviderType,
		SelectedIndices: selected,
	}
}

func buildScaleAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int) *CredamoAnswerAction {
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
	return &CredamoAnswerAction{
		QuestionID:      meta.ProviderQuestionID,
		QuestionType:    meta.ProviderType,
		SelectedIndices: []int{idx},
	}
}

func buildMatrixAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int) *CredamoAnswerAction {
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

func buildTextAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int) *CredamoAnswerAction {
	text := "满意"
	if configIdx >= 0 && configIdx < len(cfg.Texts) && len(cfg.Texts[configIdx]) > 0 {
		text = cfg.Texts[configIdx][rand.Intn(len(cfg.Texts[configIdx]))]
	}
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
	if len(rawQuestions) > 0 {
		perQuestionMS = int64(duration) * 1000 / int64(len(rawQuestions))
	}

	var qstList []map[string]any
	for _, rq := range rawQuestions {
		qID := getString(rq, "id")
		qType := getString(rq, "question_kind")
		if qType == "" {
			qType = getString(rq, "type")
		}

		action, ok := actionMap[qID]
		if !ok {
			continue
		}

		qst := map[string]any{
			"questionId": qID,
			"type":       qType,
			"answerTime": perQuestionMS,
		}

		// Get raw options for building proper answer format
		rawOptions := getArray(rq, "options")
		rawSubs := getArray(rq, "sub_questions")

		switch typeCodeForKind(qType) {
		case "3", "5", "7": // single/scale/dropdown
			if len(action.SelectedIndices) > 0 {
				idx := action.SelectedIndices[0]
				if idx < len(rawOptions) {
					opt := rawOptions[idx]
					qst["answerQstChoice"] = map[string]any{
						"choiceId":      getString(opt, "id"),
						"choiceContent": getString(opt, "text"),
					}
				}
			}
		case "4": // multiple
			var choiceList []map[string]any
			for _, idx := range action.SelectedIndices {
				if idx < len(rawOptions) {
					opt := rawOptions[idx]
					choiceList = append(choiceList, map[string]any{
						"choiceId":      getString(opt, "id"),
						"choiceContent": getString(opt, "text"),
					})
				}
			}
			qst["answerQstChoiceList"] = choiceList
		case "6": // matrix
			var choiceList []map[string]any
			for rowIdx, colIndices := range action.MatrixAnswers {
				if rowIdx < len(rawSubs) {
					sub := rawSubs[rowIdx]
					subOptions := getArray(sub, "options")
					for _, colIdx := range colIndices {
						if colIdx < len(subOptions) {
							opt := subOptions[colIdx]
							choiceList = append(choiceList, map[string]any{
								"choiceId":      getString(opt, "id"),
								"choiceContent": getString(opt, "text"),
							})
						}
					}
				}
			}
			qst["answerQstChoiceList"] = choiceList
		case "11": // order
			var choiceList []map[string]any
			for _, idx := range action.OrderIndices {
				if idx < len(rawOptions) {
					opt := rawOptions[idx]
					choiceList = append(choiceList, map[string]any{
						"choiceId":      getString(opt, "id"),
						"choiceContent": getString(opt, "text"),
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
	return credamoTypeMap[kind]
}

// Helpers

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
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
	return nil
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
