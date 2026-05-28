package wjx

import (
	"fmt"
	"math"
	"math/rand"
	"strings"

	"github.com/SurveyController/SurveyController-Go/internal/models"
)

// AnswerAction represents a single answer to be submitted.
type AnswerAction struct {
	QuestionNum     int
	Kind            string // "choice", "text", "matrix", "slider", "order"
	SelectedIndices []int
	TextValues      []string
	MatrixIndices   []int
	SliderValue     *float64
	OptionFillTexts map[int]string // index -> fill text
}

// buildAnswerActions generates answer actions for all questions in the config.
func buildAnswerActions(cfg *models.ExecutionConfig, state *models.ExecutionState) ([]AnswerAction, error) {
	var actions []AnswerAction
	for _, meta := range sortedQuestions(cfg) {
		if meta.Unsupported {
			return nil, fmt.Errorf("问卷星第%d题暂不支持: %s", meta.Num, meta.UnsupportedReason)
		}
		action, err := buildSingleAction(cfg, meta)
		if err != nil {
			return nil, err
		}
		if action != nil {
			actions = append(actions, *action)
		}
	}
	return actions, nil
}

// sortedQuestions returns questions sorted by page and num.
func sortedQuestions(cfg *models.ExecutionConfig) []models.SurveyQuestionMeta {
	questions := make([]models.SurveyQuestionMeta, 0, len(cfg.QuestionsMetadata))
	for _, q := range cfg.QuestionsMetadata {
		questions = append(questions, q)
	}
	// Simple sort by page then num
	for i := 0; i < len(questions); i++ {
		for j := i + 1; j < len(questions); j++ {
			pi, pj := questions[i].Page, questions[j].Page
			if pi == 0 { pi = 1 }
			if pj == 0 { pj = 1 }
			if pi > pj || (pi == pj && questions[i].Num > questions[j].Num) {
				questions[i], questions[j] = questions[j], questions[i]
			}
		}
	}
	return questions
}

// buildSingleAction builds an answer action for a single question.
func buildSingleAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta) (*AnswerAction, error) {
	typeCode := strings.TrimSpace(meta.TypeCode)
	questionNum := meta.Num

	// Check if we have a config entry for this question
	configIdx := -1
	if idx, ok := cfg.QuestionConfigIndexMap[questionNum]; ok {
		configIdx = parseConfigIndex(idx)
	}

	switch typeCode {
	case "1", "2", "33", "34": // Single choice
		return buildChoiceAction(cfg, meta, configIdx, false)
	case "3", "4", "5": // Multiple choice
		return buildMultipleChoiceAction(cfg, meta, configIdx)
	case "6": // Matrix
		return buildMatrixAction(cfg, meta, configIdx)
	case "7": // Scale / rating
		return buildScaleAction(cfg, meta, configIdx)
	case "8", "9": // Text
		return buildTextAction(cfg, meta, configIdx)
	case "11": // Slider
		return buildSliderAction(cfg, meta, configIdx)
	case "35": // Dropdown list
		return buildChoiceAction(cfg, meta, configIdx, true)
	case "12": // Order / ranking
		return buildOrderAction(cfg, meta, configIdx)
	default:
		// Skip unsupported types
		return nil, nil
	}
}

func buildChoiceAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int, isDropdown bool) (*AnswerAction, error) {
	optionCount := meta.Options
	if optionCount <= 0 {
		optionCount = len(meta.OptionTexts)
	}
	if optionCount <= 0 {
		optionCount = 1
	}

	// Check for forced option
	if meta.ForcedOptionIndex != nil && *meta.ForcedOptionIndex >= 0 {
		idx := *meta.ForcedOptionIndex
		if idx >= optionCount {
			idx = 0
		}
		return &AnswerAction{
			QuestionNum:     meta.Num,
			Kind:            "choice",
			SelectedIndices: []int{idx},
		}, nil
	}

	// Get probabilities from config
	probs := getProbabilities(cfg, configIdx, optionCount, isDropdown)
	selectedIdx := weightedIndex(probs, optionCount)

	// Resolve option fill text
	fillTexts := resolveOptionFillText(cfg, configIdx, selectedIdx, isDropdown)

	return &AnswerAction{
		QuestionNum:     meta.Num,
		Kind:            "choice",
		SelectedIndices: []int{selectedIdx},
		OptionFillTexts: fillTexts,
	}, nil
}

func buildMultipleChoiceAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int) (*AnswerAction, error) {
	optionCount := meta.Options
	if optionCount <= 0 {
		optionCount = len(meta.OptionTexts)
	}
	if optionCount <= 0 {
		optionCount = 1
	}

	minLimit := 1
	maxLimit := optionCount
	if meta.MultiMinLimit != nil && *meta.MultiMinLimit > 0 {
		minLimit = *meta.MultiMinLimit
	}
	if meta.MultiMaxLimit != nil && *meta.MultiMaxLimit > 0 {
		maxLimit = *meta.MultiMaxLimit
	}

	// Get probabilities
	probs := make([]float64, optionCount)
	if configIdx >= 0 && configIdx < len(cfg.MultipleProb) {
		copy(probs, cfg.MultipleProb[configIdx])
	} else {
		// Uniform distribution
		for i := range probs {
			probs[i] = 1.0 / float64(optionCount)
		}
	}

	// Select based on probabilities
	numSelect := minLimit + rand.Intn(maxLimit-minLimit+1)
	selected := weightedSampleWithoutReplacement(probs, optionCount, numSelect)

	return &AnswerAction{
		QuestionNum:     meta.Num,
		Kind:            "choice",
		SelectedIndices: selected,
	}, nil
}

func buildMatrixAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int) (*AnswerAction, error) {
	rows := meta.Rows
	if rows <= 0 {
		rows = 1
	}
	optionCount := meta.Options
	if optionCount <= 0 {
		optionCount = 1
	}

	indices := make([]int, rows)
	for i := 0; i < rows; i++ {
		// Get probabilities for this row
		probs := make([]float64, optionCount)
		if configIdx >= 0 && configIdx < len(cfg.MatrixProb) {
			if nested, ok := cfg.MatrixProb[configIdx].([]any); ok && i < len(nested) {
				if rowProbs, ok := nested[i].([]float64); ok {
					copy(probs, rowProbs)
				}
			}
		}
		if len(probs) == 0 || allZero(probs) {
			for j := range probs {
				probs[j] = 1.0 / float64(optionCount)
			}
		}
		indices[i] = weightedIndex(probs, optionCount)
	}

	return &AnswerAction{
		QuestionNum:   meta.Num,
		Kind:          "matrix",
		MatrixIndices: indices,
	}, nil
}

func buildScaleAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int) (*AnswerAction, error) {
	optionCount := meta.Options
	if optionCount <= 0 {
		optionCount = 5
	}

	probs := make([]float64, optionCount)
	if configIdx >= 0 && configIdx < len(cfg.ScaleProb) {
		if p, ok := toFloat64Slice(cfg.ScaleProb[configIdx]); ok {
			copy(probs, p)
		}
	}
	if allZero(probs) {
		for i := range probs {
			probs[i] = 1.0 / float64(optionCount)
		}
	}

	idx := weightedIndex(probs, optionCount)
	return &AnswerAction{
		QuestionNum:     meta.Num,
		Kind:            "choice",
		SelectedIndices: []int{idx},
	}, nil
}

func buildTextAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int) (*AnswerAction, error) {
	textValues := []string{""}
	if configIdx >= 0 && configIdx < len(cfg.Texts) && len(cfg.Texts[configIdx]) > 0 {
		texts := cfg.Texts[configIdx]
		textValues = []string{texts[rand.Intn(len(texts))]}
	} else {
		textValues = []string{generateDefaultText(meta)}
	}

	return &AnswerAction{
		QuestionNum: meta.Num,
		Kind:        "text",
		TextValues:  textValues,
	}, nil
}

func buildSliderAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int) (*AnswerAction, error) {
	var target float64
	if configIdx >= 0 && configIdx < len(cfg.SliderTargets) {
		target = cfg.SliderTargets[configIdx]
	} else {
		min := 0.0
		max := 100.0
		if meta.SliderMin != nil {
			min = *meta.SliderMin
		}
		if meta.SliderMax != nil {
			max = *meta.SliderMax
		}
		target = min + rand.Float64()*(max-min)
	}

	return &AnswerAction{
		QuestionNum: meta.Num,
		Kind:        "slider",
		SliderValue: &target,
	}, nil
}

func generateDefaultText(meta models.SurveyQuestionMeta) string {
	defaultTexts := []string{
		"非常好", "满意", "一般", "可以接受", "不错",
	}
	return defaultTexts[rand.Intn(len(defaultTexts))]
}

func buildOrderAction(cfg *models.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int) (*AnswerAction, error) {
	optionCount := meta.Options
	if optionCount <= 0 {
		optionCount = len(meta.OptionTexts)
	}
	if optionCount <= 0 {
		optionCount = 1
	}

	// Random shuffle of all indices
	indices := make([]int, optionCount)
	for i := range indices {
		indices[i] = i
	}
	for i := len(indices) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		indices[i], indices[j] = indices[j], indices[i]
	}

	return &AnswerAction{
		QuestionNum:     meta.Num,
		Kind:            "order",
		SelectedIndices: indices,
	}, nil
}

// resolveOptionFillText resolves fill text for selected options from config.
func resolveOptionFillText(cfg *models.ExecutionConfig, configIdx, selectedIdx int, isDropdown bool) map[int]string {
	var fillTextsSource [][]*string
	if isDropdown {
		fillTextsSource = cfg.DroplistOptionFillTexts
	} else {
		fillTextsSource = cfg.SingleOptionFillTexts
	}

	if configIdx < 0 || configIdx >= len(fillTextsSource) {
		return nil
	}

	fillEntries := fillTextsSource[configIdx]
	if fillEntries == nil || selectedIdx >= len(fillEntries) {
		return nil
	}

	fillValue := fillEntries[selectedIdx]
	if fillValue == nil || *fillValue == "" {
		return nil
	}

	return map[int]string{selectedIdx: *fillValue}
}

func getProbabilities(cfg *models.ExecutionConfig, configIdx int, optionCount int, isDropdown bool) []float64 {
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
			probs[i] = 1.0 / float64(optionCount)
		}
	}
	return probs
}

func weightedIndex(probs []float64, optionCount int) int {
	if optionCount <= 0 {
		return 0
	}
	total := 0.0
	for i := 0; i < optionCount && i < len(probs); i++ {
		total += math.Max(0, probs[i])
	}
	if total <= 0 {
		return rand.Intn(optionCount)
	}
	r := rand.Float64() * total
	cumulative := 0.0
	for i := 0; i < optionCount && i < len(probs); i++ {
		cumulative += math.Max(0, probs[i])
		if r <= cumulative {
			return i
		}
	}
	return optionCount - 1
}

func weightedSampleWithoutReplacement(probs []float64, optionCount, numSelect int) []int {
	if numSelect >= optionCount {
		result := make([]int, optionCount)
		for i := range result {
			result[i] = i
		}
		return result
	}

	selected := make([]int, 0, numSelect)
	remaining := make([]float64, optionCount)
	copy(remaining, probs)

	for len(selected) < numSelect {
		idx := weightedIndex(remaining, optionCount)
		selected = append(selected, idx)
		remaining[idx] = 0
	}
	return selected
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

func parseConfigIndex(s string) int {
	var idx int
	fmt.Sscanf(s, "%d", &idx)
	return idx
}

// buildSubmitData serializes answer actions into WJX submitdata format.
func buildSubmitData(actions []AnswerAction, cfg *models.ExecutionConfig) string {
	var parts []string
	for _, action := range actions {
		answer := formatAnswer(action)
		if answer == "" {
			continue
		}
		answer = strings.ReplaceAll(answer, "，", ",")
		parts = append(parts, fmt.Sprintf("%d$%s", action.QuestionNum, answer))
	}
	return strings.Join(parts, "}")
}

// buildSubmitDataWithSkipped serializes answer actions including skipped questions.
func buildSubmitDataWithSkipped(actions []AnswerAction, cfg *models.ExecutionConfig, skippedNums []int) string {
	actionByNum := make(map[int]AnswerAction)
	for _, a := range actions {
		actionByNum[a.QuestionNum] = a
	}

	// Collect all question numbers
	allNums := make(map[int]bool)
	for _, a := range actions {
		allNums[a.QuestionNum] = true
	}
	for _, n := range skippedNums {
		allNums[n] = true
	}

	// Sort question numbers
	sortedNums := make([]int, 0, len(allNums))
	for n := range allNums {
		sortedNums = append(sortedNums, n)
	}
	for i := 0; i < len(sortedNums); i++ {
		for j := i + 1; j < len(sortedNums); j++ {
			if sortedNums[i] > sortedNums[j] {
				sortedNums[i], sortedNums[j] = sortedNums[j], sortedNums[i]
			}
		}
	}

	var parts []string
	for _, num := range sortedNums {
		if num <= 0 {
			continue
		}
		action, ok := actionByNum[num]
		var answer string
		if ok {
			answer = formatAnswer(action)
		} else {
			// Skipped question - generate placeholder
			answer = skippedAnswer(cfg, num)
		}
		if answer == "" {
			continue
		}
		answer = strings.ReplaceAll(answer, "，", ",")
		parts = append(parts, fmt.Sprintf("%d$%s", num, answer))
	}
	return strings.Join(parts, "}")
}

// skippedAnswer generates a placeholder answer for skipped questions.
func skippedAnswer(cfg *models.ExecutionConfig, questionNum int) string {
	meta, ok := cfg.QuestionsMetadata[questionNum]
	if !ok {
		return "-3"
	}
	typeCode := strings.TrimSpace(meta.TypeCode)
	switch typeCode {
	case "3", "4", "5": // Multiple choice
		return "-3"
	case "6": // Matrix
		rows := meta.Rows
		if rows <= 0 {
			rows = 1
		}
		var parts []string
		for i := 0; i < rows; i++ {
			parts = append(parts, fmt.Sprintf("%d!-3", i+1))
		}
		return strings.Join(parts, ",")
	case "11": // Slider/order
		return "-3"
	default: // Single, text, etc.
		return "(跳过)"
	}
}

func formatAnswer(action AnswerAction) string {
	switch action.Kind {
	case "choice", "select":
		return formatSelectedIndicesWithFill(action.SelectedIndices, action.OptionFillTexts)
	case "text":
		if len(action.TextValues) > 1 {
			return strings.Join(action.TextValues, "^")
		}
		if len(action.TextValues) > 0 {
			return action.TextValues[0]
		}
		return ""
	case "matrix":
		var parts []string
		for rowIdx, colIdx := range action.MatrixIndices {
			parts = append(parts, fmt.Sprintf("%d!%d", rowIdx+1, colIdx+1))
		}
		return strings.Join(parts, ",")
	case "slider":
		if action.SliderValue != nil {
			return fmt.Sprintf("%.0f", *action.SliderValue)
		}
		return ""
	case "order":
		var parts []string
		for _, idx := range action.SelectedIndices {
			parts = append(parts, fmt.Sprintf("%d", idx+1))
		}
		return strings.Join(parts, ",")
	}
	return ""
}

// formatSelectedIndicesWithFill formats indices with optional fill text (e.g., "1!custom text|2").
func formatSelectedIndicesWithFill(indices []int, fillTexts map[int]string) string {
	var parts []string
	for _, idx := range indices {
		val := fmt.Sprintf("%d", idx+1)
		if fillTexts != nil {
			if fill, ok := fillTexts[idx]; ok && fill != "" {
				val = fmt.Sprintf("%d!%s", idx+1, fill)
			}
		}
		parts = append(parts, val)
	}
	return strings.Join(parts, "|")
}

func formatSelectedIndices(indices []int) string {
	return formatSelectedIndicesWithFill(indices, nil)
}
