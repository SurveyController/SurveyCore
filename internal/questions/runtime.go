package questions

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SurveyController/SurveyCore/internal/execution"
	runstate "github.com/SurveyController/SurveyCore/internal/runtime"

	"github.com/SurveyController/SurveyCore/internal/models"
)

var stateDistributionTrackers sync.Map // map[*runstate.ExecutionState]*DistributionTracker

// RunContext holds per-submission answer-generation state.
type RunContext struct {
	cfg          *execution.ExecutionConfig
	state        *runstate.ExecutionState
	threadName   string
	distribution *DistributionTracker
	consistency  *ConsistencyContext
	psychometric *DimensionPsychometricPlan
	ai           *AIClient
}

// NewRunContext builds the answer-generation context for one submitted sample.
func NewRunContext(cfg *execution.ExecutionConfig, state *runstate.ExecutionState) *RunContext {
	return NewRunContextForThread(cfg, state, "")
}

// NewRunContextForThread builds the answer-generation context bound to a worker thread.
func NewRunContextForThread(cfg *execution.ExecutionConfig, state *runstate.ExecutionState, threadName string) *RunContext {
	if cfg == nil {
		cfg = &execution.ExecutionConfig{}
	}
	rt := &RunContext{
		cfg:          cfg,
		state:        state,
		threadName:   threadName,
		distribution: distributionTrackerForState(state),
		consistency:  NewConsistencyContext(ParseAnswerRules(cfg.AnswerRules)),
		psychometric: buildPsychometricPlanFromConfig(cfg),
	}
	if hasAIText(cfg) {
		rt.ai = NewAIClient(AIConfig{
			Mode:         cfg.AIMode,
			Provider:     cfg.AIProvider,
			APIKey:       cfg.AIAPIKey,
			BaseURL:      cfg.AIBaseURL,
			Protocol:     cfg.AIAPIProtocol,
			Model:        cfg.AIModel,
			SystemPrompt: cfg.AISystemPrompt,
			FreeUserID:   cfg.RandomIPUserID,
			FreeDeviceID: cfg.RandomIPDeviceID,
		})
	}
	return rt
}

func distributionTrackerForState(state *runstate.ExecutionState) *DistributionTracker {
	if state == nil {
		return NewDistributionTracker()
	}
	if tracker, ok := stateDistributionTrackers.Load(state); ok {
		return tracker.(*DistributionTracker)
	}
	tracker := NewDistributionTracker()
	actual, _ := stateDistributionTrackers.LoadOrStore(state, tracker)
	return actual.(*DistributionTracker)
}

// ParseAnswerRules converts JSON-compatible rule maps into typed rules.
func ParseAnswerRules(raw []map[string]any) []AnswerRule {
	rules := make([]AnswerRule, 0, len(raw))
	for _, item := range raw {
		rule := AnswerRule{
			ConditionQuestionNum:   intFromAny(item["condition_question_num"]),
			ConditionMode:          stringFromAny(item["condition_mode"], "selected"),
			ConditionOptionIndices: intSliceFromAny(item["condition_option_indices"]),
			TargetQuestionNum:      intFromAny(item["target_question_num"]),
			ActionMode:             stringFromAny(item["action_mode"], "must_not_select"),
			TargetOptionIndices:    intSliceFromAny(item["target_option_indices"]),
			ConditionRowIndex:      optionalIntFromAny(item["condition_row_index"]),
			TargetRowIndex:         optionalIntFromAny(item["target_row_index"]),
		}
		if rule.ConditionQuestionNum <= 0 || rule.TargetQuestionNum <= 0 || len(rule.TargetOptionIndices) == 0 {
			continue
		}
		rules = append(rules, rule)
	}
	return rules
}

// ChooseSingle selects one option, applying consistency, psychometric, and distribution rules.
func (r *RunContext) ChooseSingle(meta models.SurveyQuestionMeta, configIdx, optionCount int, probs []float64, rowIndex *int) int {
	weights := normalizedWeights(probs, optionCount)
	if selected, ok := r.reverseFillChoice(meta, optionCount, rowIndex); ok {
		r.recordSingleChoice(meta.Num, selected, optionCount, rowIndex)
		return selected
	}
	if r != nil && r.consistency != nil {
		if rowIndex != nil {
			weights = r.consistency.ApplyMatrixRowConsistency(weights, meta.Num, *rowIndex)
		} else {
			weights = r.consistency.ApplySingleConsistency(weights, meta.Num)
		}
	}

	selected := -1
	if r != nil && r.psychometric != nil {
		if choice := r.psychometric.GetChoice(meta.Num, rowIndex); choice != nil && validWeightedChoice(weights, *choice) {
			selected = *choice
		}
	}
	if selected < 0 {
		if r != nil && r.usesDistribution(configIdx) {
			weights = ResolveDistributionProbabilities(weights, optionCount, r.distribution, meta.Num, rowIndex)
		}
		selected = WeightedIndex(weights, optionCount)
	}

	if r != nil {
		r.recordSingleChoice(meta.Num, selected, optionCount, rowIndex)
	}
	return selected
}

// ChooseMultiple selects multiple options while honoring consistency constraints.
func (r *RunContext) ChooseMultiple(meta models.SurveyQuestionMeta, configIdx, optionCount, minLimit, maxLimit int, probs []float64) []int {
	if optionCount <= 0 {
		return nil
	}
	if selected, ok := r.reverseFillChoice(meta, optionCount, nil); ok {
		result := []int{selected}
		if r != nil {
			r.consistency.RecordAnswers(meta.Num, result)
			if r.distribution != nil {
				r.distribution.RecordChoice(meta.Num, selected, optionCount, nil)
			}
		}
		return result
	}
	weights := normalizedWeights(probs, optionCount)
	if r != nil && r.usesDistribution(configIdx) {
		weights = ResolveDistributionProbabilities(weights, optionCount, r.distribution, meta.Num, nil)
	}

	var mustSelect, mustNotSelect []int
	if r != nil && r.consistency != nil {
		mustSelect, mustNotSelect = r.consistency.GetMultipleConstraint(meta.Num, optionCount)
	}
	blocked := make(map[int]bool)
	for _, idx := range mustNotSelect {
		if idx >= 0 && idx < optionCount {
			blocked[idx] = true
			weights[idx] = 0
		}
	}

	selectedSet := make(map[int]bool)
	for _, idx := range mustSelect {
		if idx >= 0 && idx < optionCount && !blocked[idx] {
			selectedSet[idx] = true
			weights[idx] = 0
		}
	}

	minLimit, maxLimit = clampSelectLimits(minLimit, maxLimit, optionCount)
	selectedCount := minLimit
	if maxLimit > minLimit {
		selectedCount = minLimit + rand.Intn(maxLimit-minLimit+1)
	}
	if selectedCount < len(selectedSet) {
		selectedCount = len(selectedSet)
	}
	if selectedCount > maxLimit {
		selectedCount = maxLimit
	}

	need := selectedCount - len(selectedSet)
	if need > 0 {
		for _, idx := range WeightedSampleWithoutReplacement(weights, optionCount, need) {
			if !blocked[idx] {
				selectedSet[idx] = true
			}
		}
	}
	if len(selectedSet) == 0 {
		for i := 0; i < optionCount; i++ {
			if !blocked[i] {
				selectedSet[i] = true
				break
			}
		}
	}

	selected := make([]int, 0, len(selectedSet))
	for idx := range selectedSet {
		selected = append(selected, idx)
	}
	sort.Ints(selected)

	if r != nil {
		r.consistency.RecordAnswers(meta.Num, selected)
		for _, idx := range selected {
			r.distribution.RecordChoice(meta.Num, idx, optionCount, nil)
		}
	}
	return selected
}

// GenerateText returns a configured, random, AI-generated, or fallback text answer.
func (r *RunContext) GenerateText(meta models.SurveyQuestionMeta, configIdx int, fallback string, blankCount int) string {
	if answer := r.reverseFillAnswer(meta.Num); answer != nil {
		switch answer.Kind {
		case models.ReverseFillKindText:
			if strings.TrimSpace(answer.TextValue) != "" {
				return strings.TrimSpace(answer.TextValue)
			}
		case models.ReverseFillKindMultiText:
			if len(answer.TextValues) > 0 {
				return strings.Join(answer.TextValues, "|")
			}
		}
	}
	if r == nil || r.cfg == nil {
		return resolveDynamicTextValue(fallback)
	}
	if location, ok := r.configuredLocationAnswer(meta, blankCount); ok {
		return location
	}
	if generated, ok := r.generateConfiguredRandomText(configIdx, fallback, blankCount); ok {
		return generated
	}
	if r.ai == nil || !r.textAIEnabled(configIdx, blankCount) {
		return resolveDynamicTextValue(fallback)
	}
	title := meta.Title
	if configIdx < len(r.cfg.TextTitles) && strings.TrimSpace(r.cfg.TextTitles[configIdx]) != "" {
		title = r.cfg.TextTitles[configIdx]
	}
	answer, err := r.ai.GenerateAnswer(title, meta.TypeCode, blankCount)
	if err != nil || strings.TrimSpace(answer) == "" {
		return fallback
	}
	return strings.TrimSpace(answer)
}

func (r *RunContext) ConfiguredTextCandidate(configIdx int) (string, bool) {
	if r == nil {
		return "", false
	}
	return ChooseConfiguredTextCandidate(r.cfg, configIdx)
}

func (r *RunContext) configuredLocationAnswer(meta models.SurveyQuestionMeta, blankCount int) (string, bool) {
	if r == nil || r.cfg == nil || len(r.cfg.LocationParts) == 0 {
		return "", false
	}
	parts, ok := r.cfg.LocationParts[meta.Num]
	if !ok || len(parts) == 0 {
		return "", false
	}
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		return "", false
	}
	if blankCount > 1 {
		return strings.Join(cleaned, "|"), true
	}
	return strings.Join(cleaned, " "), true
}

func (r *RunContext) generateConfiguredRandomText(configIdx int, fallback string, blankCount int) (string, bool) {
	if r == nil || r.cfg == nil || configIdx < 0 {
		return "", false
	}
	if blankCount > 1 {
		return r.generateConfiguredMultiText(configIdx, fallback, blankCount)
	}

	mode := ""
	if configIdx < len(r.cfg.TextRandomModes) {
		mode = strings.ToLower(strings.TrimSpace(r.cfg.TextRandomModes[configIdx]))
	}
	if mode != "" && mode != models.TextRandomNone {
		var intRange []int
		if configIdx < len(r.cfg.TextRandomIntRanges) {
			intRange = r.cfg.TextRandomIntRanges[configIdx]
		}
		return randomTextForMode(mode, intRange), true
	}
	if resolved, ok := resolveDynamicTextToken(fallback); ok {
		return resolved, true
	}
	return "", false
}

func (r *RunContext) generateConfiguredMultiText(configIdx int, fallback string, blankCount int) (string, bool) {
	parts := splitConfiguredTextParts(fallback, blankCount)
	applied := false
	for i := range parts {
		if resolved, ok := resolveDynamicTextToken(parts[i]); ok {
			parts[i] = resolved
			applied = true
		}
	}

	var modes []string
	if configIdx < len(r.cfg.MultiTextBlankModes) {
		modes = r.cfg.MultiTextBlankModes[configIdx]
	}
	var intRanges [][]int
	if configIdx < len(r.cfg.MultiTextBlankIntRanges) {
		intRanges = r.cfg.MultiTextBlankIntRanges[configIdx]
	}
	for i := 0; i < blankCount && i < len(modes); i++ {
		mode := strings.ToLower(strings.TrimSpace(modes[i]))
		if mode == "" || mode == models.TextRandomNone {
			continue
		}
		var intRange []int
		if i < len(intRanges) {
			intRange = intRanges[i]
		}
		parts[i] = randomTextForMode(mode, intRange)
		applied = true
	}
	if !applied {
		return "", false
	}
	return strings.Join(parts, "|"), true
}

func splitConfiguredTextParts(value string, blankCount int) []string {
	if blankCount <= 0 {
		blankCount = 1
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '|' || r == '^'
	})
	if len(parts) == 0 {
		parts = []string{value}
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
		if parts[i] == "" {
			parts[i] = defaultTextFallback()
		}
	}
	for len(parts) < blankCount {
		parts = append(parts, parts[len(parts)-1])
	}
	return parts[:blankCount]
}

func (r *RunContext) textAIEnabled(configIdx, blankCount int) bool {
	if r == nil || r.cfg == nil || configIdx < 0 {
		return false
	}
	if configIdx < len(r.cfg.TextAIFlags) && r.cfg.TextAIFlags[configIdx] {
		return true
	}
	if blankCount <= 1 || configIdx >= len(r.cfg.MultiTextBlankAIFlags) {
		return false
	}
	flags := r.cfg.MultiTextBlankAIFlags[configIdx]
	if len(flags) == 0 {
		return false
	}
	for i := 0; i < blankCount; i++ {
		if i >= len(flags) || !flags[i] {
			return false
		}
	}
	return true
}

func randomTextForMode(mode string, intRange []int) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case models.TextRandomName:
		return generateRandomChineseName()
	case models.TextRandomMobile:
		return generateRandomMobile()
	case models.TextRandomIDCard:
		return generateRandomIDCard()
	case models.TextRandomInteger:
		minValue, maxValue := normalizeIntRange(intRange)
		return strconv.Itoa(minValue + rand.Intn(maxValue-minValue+1))
	default:
		return defaultTextFallback()
	}
}

func resolveDynamicTextValue(value string) string {
	if resolved, ok := resolveDynamicTextToken(value); ok {
		return resolved
	}
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return defaultTextFallback()
}

func resolveDynamicTextToken(value string) (string, bool) {
	text := strings.TrimSpace(value)
	switch text {
	case models.TextRandomNameToken:
		return generateRandomChineseName(), true
	case models.TextRandomMobileToken:
		return generateRandomMobile(), true
	case models.TextRandomIDCardToken:
		return generateRandomIDCard(), true
	}
	const randomIntTokenPrefix = "__RANDOM_INT__:"
	if strings.HasPrefix(text, randomIntTokenPrefix) {
		payload := strings.TrimPrefix(text, randomIntTokenPrefix)
		parts := strings.SplitN(payload, ":", 2)
		if len(parts) != 2 {
			return "", false
		}
		minValue, errMin := strconv.Atoi(strings.TrimSpace(parts[0]))
		maxValue, errMax := strconv.Atoi(strings.TrimSpace(parts[1]))
		if errMin != nil || errMax != nil {
			return "", false
		}
		minValue, maxValue = normalizeIntRange([]int{minValue, maxValue})
		return strconv.Itoa(minValue + rand.Intn(maxValue-minValue+1)), true
	}
	return "", false
}

func normalizeIntRange(values []int) (int, int) {
	minValue, maxValue := 0, 100
	if len(values) >= 2 {
		minValue, maxValue = values[0], values[1]
	}
	if minValue > maxValue {
		minValue, maxValue = maxValue, minValue
	}
	return minValue, maxValue
}

func generateRandomChineseName() string {
	surnames := []string{"赵", "钱", "孙", "李", "周", "吴", "郑", "王", "陈", "刘", "杨", "黄", "张", "林", "何", "郭"}
	givenPool := []rune("伟俊涛强磊凯鹏鑫宇浩瑞博杰宁豪轩婷雅静怡欣萱琳玲芳颖慧敏雪晶佳媛嘉明华安晨泽文洋")
	givenLen := 1
	if rand.Intn(100) >= 65 {
		givenLen = 2
	}
	given := make([]rune, givenLen)
	for i := range given {
		given[i] = givenPool[rand.Intn(len(givenPool))]
	}
	return surnames[rand.Intn(len(surnames))] + string(given)
}

func generateRandomMobile() string {
	prefixes := []string{
		"130", "131", "132", "133", "134", "135", "136", "137", "138", "139",
		"147", "150", "151", "152", "153", "155", "156", "157", "158", "159",
		"166", "171", "172", "173", "175", "176", "177", "178", "180", "181",
		"182", "183", "184", "185", "186", "187", "188", "189", "198", "199",
	}
	tail := make([]byte, 8)
	for i := range tail {
		tail[i] = byte('0' + rand.Intn(10))
	}
	return prefixes[rand.Intn(len(prefixes))] + string(tail)
}

func generateRandomIDCard() string {
	areaCodes := []string{"110100", "310100", "440100", "330100", "510100"}
	age := 18 + rand.Intn(43)
	year := time.Now().Year() - age
	month := time.Month(1 + rand.Intn(12))
	maxDay := time.Date(year, month+1, 0, 0, 0, 0, 0, time.Local).Day()
	day := 1 + rand.Intn(maxDay)
	firstSeventeen := fmt.Sprintf("%s%04d%02d%02d%03d", areaCodes[rand.Intn(len(areaCodes))], year, month, day, rand.Intn(1000))
	return firstSeventeen + idCardChecksum(firstSeventeen)
}

func idCardChecksum(firstSeventeen string) string {
	weights := []int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	chars := "10X98765432"
	total := 0
	for i := 0; i < len(firstSeventeen) && i < len(weights); i++ {
		total += int(firstSeventeen[i]-'0') * weights[i]
	}
	return string(chars[total%11])
}

func defaultTextFallback() string {
	return "无"
}

func (r *RunContext) usesDistribution(configIdx int) bool {
	if r == nil || r.cfg == nil || configIdx < 0 || configIdx >= len(r.cfg.DistributionModes) {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(r.cfg.DistributionModes[configIdx]))
	return mode != "" && mode != "random"
}

func (r *RunContext) reverseFillAnswer(questionNum int) *models.ReverseFillAnswer {
	if r == nil || r.state == nil {
		return nil
	}
	return r.state.GetReverseFillAnswer(questionNum, r.threadName)
}

func (r *RunContext) reverseFillChoice(meta models.SurveyQuestionMeta, optionCount int, rowIndex *int) (int, bool) {
	answer := r.reverseFillAnswer(meta.Num)
	if answer == nil {
		return 0, false
	}
	if rowIndex != nil && answer.Kind == models.ReverseFillKindMatrix {
		row := *rowIndex
		if row >= 0 && row < len(answer.MatrixChoiceIndexes) {
			selected := answer.MatrixChoiceIndexes[row]
			if selected >= 0 && selected < optionCount {
				return selected, true
			}
		}
		return 0, false
	}
	if rowIndex == nil && answer.Kind == models.ReverseFillKindChoice && answer.ChoiceIndex != nil {
		selected := *answer.ChoiceIndex
		if selected >= 0 && selected < optionCount {
			return selected, true
		}
	}
	return 0, false
}

func (r *RunContext) recordSingleChoice(questionNum, selected, optionCount int, rowIndex *int) {
	if r == nil {
		return
	}
	if r.consistency != nil {
		if rowIndex != nil {
			r.consistency.RecordMatrixAnswer(questionNum, *rowIndex, selected)
		} else {
			r.consistency.RecordAnswer(questionNum, selected)
		}
	}
	if r.distribution != nil {
		r.distribution.RecordChoice(questionNum, selected, optionCount, rowIndex)
	}
}

func hasAIText(cfg *execution.ExecutionConfig) bool {
	if cfg == nil {
		return false
	}
	for _, enabled := range cfg.TextAIFlags {
		if enabled {
			return true
		}
	}
	for _, flags := range cfg.MultiTextBlankAIFlags {
		for _, enabled := range flags {
			if enabled {
				return true
			}
		}
	}
	return false
}

func buildPsychometricPlanFromConfig(cfg *execution.ExecutionConfig) *DimensionPsychometricPlan {
	if cfg == nil || len(cfg.QuestionDimensionMap) == 0 {
		return nil
	}

	grouped := make(map[string][]PsychometricItem)
	metas := make([]models.SurveyQuestionMeta, 0, len(cfg.QuestionsMetadata))
	for _, meta := range cfg.QuestionsMetadata {
		metas = append(metas, meta)
	}
	sort.Slice(metas, func(i, j int) bool {
		if metas[i].Page == metas[j].Page {
			return metas[i].Num < metas[j].Num
		}
		return metas[i].Page < metas[j].Page
	})

	for _, meta := range metas {
		dimensionPtr := cfg.QuestionDimensionMap[meta.Num]
		if dimensionPtr == nil || strings.TrimSpace(*dimensionPtr) == "" {
			continue
		}
		optionCount := meta.Options
		if optionCount <= 0 {
			optionCount = len(meta.OptionTexts)
		}
		if optionCount <= 1 || meta.IsTextLike || meta.Unsupported {
			continue
		}
		scoreByChoice := scoreMapToFloat64(cfg.QuestionOrdinalScoreMap[meta.Num])
		dimension := strings.TrimSpace(*dimensionPtr)
		bias := psychometricBiasFromConfig(cfg.QuestionPsychoBiasMap[meta.Num])
		targetProb := psychometricTargetProbabilities(cfg, meta, cfg.QuestionConfigIndexMap[meta.Num], optionCount, nil)
		if meta.TypeCode == "6" && meta.Rows > 0 {
			for row := 0; row < meta.Rows; row++ {
				rowIndex := row
				grouped[dimension] = append(grouped[dimension], PsychometricItem{
					Kind:          "matrix",
					QuestionIndex: meta.Num,
					RowIndex:      &rowIndex,
					OptionCount:   optionCount,
					Bias:          bias,
					ScoreByChoice: scoreByChoice,
					TargetProb:    psychometricTargetProbabilities(cfg, meta, cfg.QuestionConfigIndexMap[meta.Num], optionCount, &rowIndex),
				})
			}
			continue
		}
		grouped[dimension] = append(grouped[dimension], PsychometricItem{
			Kind:          "single",
			QuestionIndex: meta.Num,
			OptionCount:   optionCount,
			Bias:          bias,
			ScoreByChoice: scoreByChoice,
			TargetProb:    targetProb,
		})
	}
	return BuildDimensionPsychometricPlan(grouped, cfg.PsychoTargetAlpha)
}

func psychometricTargetProbabilities(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, rawConfigIdx string, optionCount int, rowIndex *int) []float64 {
	if cfg == nil || optionCount <= 0 {
		return nil
	}
	configIdx := -1
	if strings.TrimSpace(rawConfigIdx) != "" {
		if parsed, err := strconv.Atoi(strings.TrimSpace(rawConfigIdx)); err == nil {
			configIdx = parsed
		}
	}
	if configIdx < 0 {
		return nil
	}
	switch meta.TypeCode {
	case "3", "33", "34":
		if configIdx < len(cfg.SingleProb) {
			return float64SliceForOptionCount(cfg.SingleProb[configIdx], optionCount)
		}
	case "5":
		if configIdx < len(cfg.ScaleProb) {
			return float64SliceForOptionCount(cfg.ScaleProb[configIdx], optionCount)
		}
	case "7", "35":
		if configIdx < len(cfg.DroplistProb) {
			return float64SliceForOptionCount(cfg.DroplistProb[configIdx], optionCount)
		}
	case "6":
		if configIdx < len(cfg.MatrixProb) {
			row := 0
			if rowIndex != nil {
				row = *rowIndex
			}
			return matrixProbabilitiesForOptionCount(cfg.MatrixProb[configIdx], row, optionCount)
		}
	}
	return nil
}

func float64SliceForOptionCount(raw any, optionCount int) []float64 {
	values := make([]float64, optionCount)
	switch typed := raw.(type) {
	case []float64:
		copy(values, typed)
	case []any:
		for i := 0; i < optionCount && i < len(typed); i++ {
			values[i] = floatFromAny(typed[i])
		}
	default:
		return nil
	}
	if allZeroFloat64(values) {
		return nil
	}
	return values
}

func matrixProbabilitiesForOptionCount(raw any, rowIndex, optionCount int) []float64 {
	switch typed := raw.(type) {
	case [][]float64:
		if rowIndex < len(typed) {
			return float64SliceForOptionCount(typed[rowIndex], optionCount)
		}
	case []any:
		if rowIndex < len(typed) {
			if row := float64SliceForOptionCount(typed[rowIndex], optionCount); len(row) > 0 {
				return row
			}
		}
		return float64SliceForOptionCount(typed, optionCount)
	default:
		return float64SliceForOptionCount(raw, optionCount)
	}
	return nil
}

func floatFromAny(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

func allZeroFloat64(values []float64) bool {
	for _, value := range values {
		if value != 0 {
			return false
		}
	}
	return true
}

func psychometricBiasFromConfig(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "left", "right", "center":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "center"
	}
}

func normalizedWeights(probs []float64, optionCount int) []float64 {
	if optionCount <= 0 {
		return nil
	}
	result := make([]float64, optionCount)
	total := 0.0
	for i := 0; i < optionCount && i < len(probs); i++ {
		result[i] = math.Max(0, probs[i])
		total += result[i]
	}
	if total <= 0 {
		for i := range result {
			result[i] = 1
		}
	}
	return result
}

func validWeightedChoice(weights []float64, choice int) bool {
	return choice >= 0 && choice < len(weights) && weights[choice] > 0
}

func clampSelectLimits(minLimit, maxLimit, optionCount int) (int, int) {
	if minLimit <= 0 {
		minLimit = 1
	}
	if maxLimit <= 0 || maxLimit > optionCount {
		maxLimit = optionCount
	}
	if minLimit > optionCount {
		minLimit = optionCount
	}
	if minLimit > maxLimit {
		minLimit = maxLimit
	}
	return minLimit, maxLimit
}

func scoreMapToFloat64(scores []int) []float64 {
	if len(scores) == 0 {
		return nil
	}
	result := make([]float64, len(scores))
	for i, value := range scores {
		result[i] = float64(value)
	}
	return result
}

func intSliceFromAny(v any) []int {
	switch raw := v.(type) {
	case []int:
		return append([]int{}, raw...)
	case []float64:
		result := make([]int, 0, len(raw))
		for _, item := range raw {
			result = append(result, int(item))
		}
		return result
	case []any:
		result := make([]int, 0, len(raw))
		for _, item := range raw {
			result = append(result, intFromAny(item))
		}
		return result
	default:
		return nil
	}
}

func optionalIntFromAny(v any) *int {
	if v == nil {
		return nil
	}
	value := intFromAny(v)
	return &value
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	default:
		return 0
	}
}

func stringFromAny(v any, fallback string) string {
	if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	return fallback
}
