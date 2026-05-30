package wjx

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/SurveyController/SurveyConsole/internal/execution"
	runstate "github.com/SurveyController/SurveyConsole/internal/runtime"

	"github.com/SurveyController/SurveyConsole/internal/models"
	"github.com/SurveyController/SurveyConsole/internal/providers/providerutil"
	"github.com/SurveyController/SurveyConsole/internal/questions"
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

type answerPlan struct {
	Actions         []AnswerAction
	SkippedNums     []int
	TerminatedEarly bool
}

// buildAnswerActions generates answer actions for all questions in the config.
func buildAnswerActions(cfg *execution.ExecutionConfig, state *runstate.ExecutionState, threadName string) ([]AnswerAction, error) {
	plan, err := buildAnswerPlan(cfg, state, threadName)
	if err != nil {
		return nil, err
	}
	if err := validateAnswerActions(plan.Actions, cfg); err != nil {
		return nil, err
	}
	return plan.Actions, nil
}

func buildAnswerPlan(cfg *execution.ExecutionConfig, state *runstate.ExecutionState, threadName string) (*answerPlan, error) {
	runtime := questions.NewRunContextForThread(cfg, state, threadName)
	ordered := sortedQuestions(cfg)
	maxQuestionNum := 0
	for _, meta := range ordered {
		if meta.Num > maxQuestionNum {
			maxQuestionNum = meta.Num
		}
	}

	plan := &answerPlan{}
	actionByNum := make(map[int]AnswerAction)
	jumpTarget := (*int)(nil)

	for _, meta := range ordered {
		if jumpTarget != nil {
			if meta.Num < *jumpTarget {
				plan.SkippedNums = append(plan.SkippedNums, meta.Num)
				continue
			}
			jumpTarget = nil
		}

		if !questionVisible(meta, actionByNum) {
			plan.SkippedNums = append(plan.SkippedNums, meta.Num)
			continue
		}

		if meta.Unsupported {
			return nil, fmt.Errorf("问卷星第%d题暂不支持: %s", meta.Num, meta.UnsupportedReason)
		}
		action, err := buildSingleAction(cfg, meta, runtime)
		if err != nil {
			return nil, err
		}
		if action != nil {
			actionByNum[meta.Num] = *action
			plan.Actions = append(plan.Actions, *action)
			if target, terminates := resolveJumpTarget(meta, *action); target != nil {
				if terminates || *target > maxQuestionNum {
					plan.TerminatedEarly = true
					return plan, nil
				}
				jumpTarget = target
			}
		}
	}
	return plan, nil
}

// sortedQuestions returns questions sorted by page and num.
func sortedQuestions(cfg *execution.ExecutionConfig) []models.SurveyQuestionMeta {
	questions := make([]models.SurveyQuestionMeta, 0, len(cfg.QuestionsMetadata))
	for _, q := range cfg.QuestionsMetadata {
		questions = append(questions, q)
	}
	// Simple sort by page then num
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

func questionVisible(meta models.SurveyQuestionMeta, actionByNum map[int]AnswerAction) bool {
	if len(meta.DisplayConditions) == 0 {
		return !meta.HasDisplayCondition
	}
	grouped := make(map[string][]map[string]any)
	for _, condition := range meta.DisplayConditions {
		sourceQuestionNum := intFromAny(condition["condition_question_num"])
		if sourceQuestionNum <= 0 {
			continue
		}
		mode := stringFromAny(condition["condition_mode"], "selected")
		key := fmt.Sprintf("%d:%s", sourceQuestionNum, mode)
		grouped[key] = append(grouped[key], condition)
	}
	if len(grouped) == 0 {
		return !meta.HasDisplayCondition
	}
	for _, group := range grouped {
		matched := false
		for _, condition := range group {
			if conditionMet(actionByNum, condition) {
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

func conditionMet(actionByNum map[int]AnswerAction, condition map[string]any) bool {
	sourceQuestionNum := intFromAny(condition["condition_question_num"])
	if sourceQuestionNum <= 0 {
		return false
	}
	sourceAction, ok := actionByNum[sourceQuestionNum]
	if !ok {
		return false
	}

	mode := stringFromAny(condition["condition_mode"], "selected")
	conditionIndices := intSetFromAny(condition["condition_option_indices"])
	selectedIndices := selectedIndexSet(sourceAction)
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

func resolveJumpTarget(meta models.SurveyQuestionMeta, action AnswerAction) (*int, bool) {
	if len(meta.JumpRules) == 0 {
		return nil, false
	}
	selectedIndices := selectedIndexSet(action)
	var unconditional *int
	unconditionalTerminates := false
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
				unconditionalTerminates = boolFromAny(rule["terminates_survey"])
			}
			continue
		}
		if selectedIndices[optionIndex] {
			targetCopy := target
			return &targetCopy, boolFromAny(rule["terminates_survey"])
		}
	}
	return unconditional, unconditionalTerminates
}

func selectedIndexSet(action AnswerAction) map[int]bool {
	result := make(map[int]bool)
	if action.Kind == "matrix" {
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

// buildSingleAction builds an answer action for a single question.
func buildSingleAction(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, runtime *questions.RunContext) (*AnswerAction, error) {
	typeCode := strings.TrimSpace(meta.TypeCode)
	questionNum := meta.Num

	// Check if we have a config entry for this question
	configIdx := -1
	if idx, ok := cfg.QuestionConfigIndexMap[questionNum]; ok {
		configIdx = providerutil.ParseConfigIndex(idx)
	}

	switch typeCode {
	case "1", "2": // Text / fill blank
		return buildTextAction(cfg, meta, configIdx, runtime)
	case "3", "33", "34": // Single choice
		return buildChoiceAction(cfg, meta, configIdx, false, runtime)
	case "4": // Multiple choice
		return buildMultipleChoiceAction(cfg, meta, configIdx, runtime)
	case "5": // Scale / rating
		return buildScaleAction(cfg, meta, configIdx, runtime)
	case "6": // Matrix
		return buildMatrixAction(cfg, meta, configIdx, runtime)
	case "7", "35": // Dropdown list
		return buildChoiceAction(cfg, meta, configIdx, true, runtime)
	case "8": // Slider
		return buildSliderAction(cfg, meta, configIdx)
	case "9":
		if meta.IsTextLike || meta.IsMultiText {
			return buildTextAction(cfg, meta, configIdx, runtime)
		}
		return buildMatrixAction(cfg, meta, configIdx, runtime)
	case "11", "12": // Order / ranking
		return buildOrderAction(cfg, meta, configIdx)
	case "0": // Description
		return nil, nil
	default:
		return nil, &providerutil.UnsupportedQuestionError{
			Provider:     ProviderName,
			QuestionNum:  meta.Num,
			TypeCode:     typeCode,
			ProviderType: meta.ProviderType,
			Reason:       meta.UnsupportedReason,
		}
	}
}

func buildChoiceAction(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int, isDropdown bool, runtime *questions.RunContext) (*AnswerAction, error) {
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
	selectedIdx := runtime.ChooseSingle(meta, configIdx, optionCount, probs, nil)

	// Resolve option fill text
	fillTexts := resolveChoiceOptionFillText(cfg, configIdx, selectedIdx, isDropdown)

	return &AnswerAction{
		QuestionNum:     meta.Num,
		Kind:            "choice",
		SelectedIndices: []int{selectedIdx},
		OptionFillTexts: fillTexts,
	}, nil
}

func buildMultipleChoiceAction(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int, runtime *questions.RunContext) (*AnswerAction, error) {
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

	selected := runtime.ChooseMultiple(meta, configIdx, optionCount, minLimit, maxLimit, probs)
	fillTexts := resolveSelectedOptionFillTexts(cfg.MultipleOptionFillTexts, configIdx, selected)

	return &AnswerAction{
		QuestionNum:     meta.Num,
		Kind:            "choice",
		SelectedIndices: selected,
		OptionFillTexts: fillTexts,
	}, nil
}

func buildMatrixAction(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int, runtime *questions.RunContext) (*AnswerAction, error) {
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
			copy(probs, providerutil.MatrixRowProbabilities(cfg.MatrixProb[configIdx], i, optionCount))
		}
		if len(probs) == 0 || providerutil.AllZero(probs) {
			for j := range probs {
				probs[j] = 1.0 / float64(optionCount)
			}
		}
		rowIndex := i
		indices[i] = runtime.ChooseSingle(meta, configIdx, optionCount, probs, &rowIndex)
	}

	return &AnswerAction{
		QuestionNum:   meta.Num,
		Kind:          "matrix",
		MatrixIndices: indices,
	}, nil
}

func buildScaleAction(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int, runtime *questions.RunContext) (*AnswerAction, error) {
	optionCount := meta.Options
	if optionCount <= 0 {
		optionCount = 5
	}

	probs := make([]float64, optionCount)
	if configIdx >= 0 && configIdx < len(cfg.ScaleProb) {
		if p, ok := providerutil.Float64Slice(cfg.ScaleProb[configIdx]); ok {
			copy(probs, p)
		}
	}
	if providerutil.AllZero(probs) {
		for i := range probs {
			probs[i] = 1.0 / float64(optionCount)
		}
	}

	idx := runtime.ChooseSingle(meta, configIdx, optionCount, probs, nil)
	return &AnswerAction{
		QuestionNum:     meta.Num,
		Kind:            "choice",
		SelectedIndices: []int{idx},
	}, nil
}

func buildTextAction(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int, runtime *questions.RunContext) (*AnswerAction, error) {
	textValues := []string{""}
	if text, ok := runtime.ConfiguredTextCandidate(configIdx); ok {
		textValues = []string{text}
	} else {
		textValues = []string{generateDefaultText(meta)}
	}
	blankCount := meta.TextInputCount
	if blankCount <= 0 {
		blankCount = len(textValues)
	}
	generated := runtime.GenerateText(meta, configIdx, textValues[0], blankCount)
	if blankCount > 1 {
		textValues = splitMultiTextAnswer(generated, blankCount)
	} else {
		textValues[0] = generated
	}

	return &AnswerAction{
		QuestionNum: meta.Num,
		Kind:        "text",
		TextValues:  textValues,
	}, nil
}

func splitMultiTextAnswer(answer string, blankCount int) []string {
	if blankCount <= 1 {
		return []string{answer}
	}
	parts := strings.FieldsFunc(answer, func(r rune) bool {
		return r == '|' || r == '^'
	})
	for len(parts) < blankCount {
		parts = append(parts, "")
	}
	return parts[:blankCount]
}

func buildSliderAction(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int) (*AnswerAction, error) {
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

func buildOrderAction(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int) (*AnswerAction, error) {
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

// resolveChoiceOptionFillText resolves fill text for selected single/dropdown options from config.
func resolveChoiceOptionFillText(cfg *execution.ExecutionConfig, configIdx, selectedIdx int, isDropdown bool) map[int]string {
	var fillTextsSource [][]*string
	if isDropdown {
		fillTextsSource = cfg.DroplistOptionFillTexts
	} else {
		fillTextsSource = cfg.SingleOptionFillTexts
	}
	return resolveSelectedOptionFillTexts(fillTextsSource, configIdx, []int{selectedIdx})
}

func resolveSelectedOptionFillTexts(fillTextsSource [][]*string, configIdx int, selected []int) map[int]string {
	if configIdx < 0 || configIdx >= len(fillTextsSource) {
		return nil
	}
	fillEntries := fillTextsSource[configIdx]
	if len(fillEntries) == 0 {
		return nil
	}
	result := make(map[int]string)
	for _, selectedIdx := range selected {
		if selectedIdx < 0 || selectedIdx >= len(fillEntries) {
			continue
		}
		fillValue := fillEntries[selectedIdx]
		if fillValue == nil || strings.TrimSpace(*fillValue) == "" {
			continue
		}
		result[selectedIdx] = strings.TrimSpace(*fillValue)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func getProbabilities(cfg *execution.ExecutionConfig, configIdx int, optionCount int, isDropdown bool) []float64 {
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
			probs[i] = 1.0 / float64(optionCount)
		}
	}
	return probs
}

func intFromAny(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			return parsed
		}
	}
	return 0
}

func stringFromAny(value any, fallback string) string {
	if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
		return strings.TrimSpace(text)
	}
	return fallback
}

func boolFromAny(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true") || strings.TrimSpace(v) == "1"
	case int:
		return v != 0
	case float64:
		return v != 0
	default:
		return false
	}
}

func intSetFromAny(value any) map[int]bool {
	result := make(map[int]bool)
	switch raw := value.(type) {
	case []int:
		for _, item := range raw {
			if item >= 0 {
				result[item] = true
			}
		}
	case []float64:
		for _, item := range raw {
			if item >= 0 {
				result[int(item)] = true
			}
		}
	case []any:
		for _, item := range raw {
			value := intFromAny(item)
			if value >= 0 {
				result[value] = true
			}
		}
	}
	return result
}

// buildSubmitData serializes answer actions into WJX submitdata format.
func buildSubmitData(actions []AnswerAction, cfg *execution.ExecutionConfig) string {
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
func buildSubmitDataWithSkipped(actions []AnswerAction, cfg *execution.ExecutionConfig, skippedNums []int) string {
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
func skippedAnswer(cfg *execution.ExecutionConfig, questionNum int) string {
	meta, ok := cfg.QuestionsMetadata[questionNum]
	if !ok {
		return "-3"
	}
	typeCode := strings.TrimSpace(meta.TypeCode)
	switch typeCode {
	case "3", "4", "5", "7", "35":
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
	case "11", "12":
		optionCount := meta.Options
		if optionCount <= 0 {
			optionCount = len(meta.OptionTexts)
		}
		if optionCount <= 0 {
			optionCount = 1
		}
		return strings.TrimRight(strings.Repeat("-3,", optionCount), ",")
	case "1", "2", "8", "9", "33", "34":
		return "(跳过)"
	default:
		return "-3"
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
			return strconv.FormatFloat(*action.SliderValue, 'f', -1, 64)
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

func validateAnswerActions(actions []AnswerAction, cfg *execution.ExecutionConfig) error {
	for _, action := range actions {
		meta, ok := cfg.QuestionsMetadata[action.QuestionNum]
		if !ok {
			continue
		}
		optionCount := meta.Options
		if optionCount <= 0 {
			optionCount = len(meta.OptionTexts)
		}
		switch action.Kind {
		case "choice", "select":
			if len(action.SelectedIndices) == 0 {
				return fmt.Errorf("问卷星第%d题答案为空", action.QuestionNum)
			}
			for _, idx := range action.SelectedIndices {
				if idx < 0 || (optionCount > 0 && idx >= optionCount) {
					return fmt.Errorf("问卷星第%d题选项越界: %d", action.QuestionNum, idx)
				}
			}
		case "text":
			if len(action.TextValues) == 0 {
				return fmt.Errorf("问卷星第%d题填空为空", action.QuestionNum)
			}
			for _, text := range action.TextValues {
				if strings.TrimSpace(text) == "" {
					return fmt.Errorf("问卷星第%d题填空包含空值", action.QuestionNum)
				}
			}
		case "matrix":
			rows := meta.Rows
			if rows <= 0 {
				rows = 1
			}
			if len(action.MatrixIndices) != rows {
				return fmt.Errorf("问卷星第%d题矩阵行数不完整: got=%d want=%d", action.QuestionNum, len(action.MatrixIndices), rows)
			}
			for row, idx := range action.MatrixIndices {
				if idx < 0 || (optionCount > 0 && idx >= optionCount) {
					return fmt.Errorf("问卷星第%d题第%d行选项越界: %d", action.QuestionNum, row+1, idx)
				}
			}
		case "slider":
			if action.SliderValue == nil {
				return fmt.Errorf("问卷星第%d题滑块答案为空", action.QuestionNum)
			}
		case "order":
			if optionCount > 0 && len(action.SelectedIndices) != optionCount {
				return fmt.Errorf("问卷星第%d题排序答案不完整", action.QuestionNum)
			}
		}
	}
	return nil
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
