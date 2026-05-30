package tencent

import (
	"fmt"
	"strings"

	"github.com/SurveyController/SurveyConsole/internal/execution"
	runstate "github.com/SurveyController/SurveyConsole/internal/runtime"

	"github.com/SurveyController/SurveyConsole/internal/models"
	"github.com/SurveyController/SurveyConsole/internal/providers/providerutil"
	"github.com/SurveyController/SurveyConsole/internal/questions"
)

func buildAnswerActions(cfg *execution.ExecutionConfig, state *runstate.ExecutionState, rawQuestions []map[string]any, threadName string) ([]TencentAnswerAction, error) {
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
		action, err := buildSingleAction(cfg, meta, rawQ, runtime)
		if err != nil {
			return nil, err
		}
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
	if err := validateTencentActions(actions, cfg); err != nil {
		return nil, err
	}
	return actions, nil
}

func sortedQuestions(cfg *execution.ExecutionConfig) []models.SurveyQuestionMeta {
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

func buildSingleAction(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, rawQ map[string]any, runtime *questions.RunContext) (*TencentAnswerAction, error) {
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

	if meta.Unsupported {
		return nil, &providerutil.UnsupportedQuestionError{
			Provider:     ProviderName,
			QuestionNum:  meta.Num,
			TypeCode:     typeCode,
			ProviderType: meta.ProviderType,
			Reason:       meta.UnsupportedReason,
		}
	}

	switch typeCode {
	case "3": // single
		return buildChoiceAnswer(cfg, meta, configIdx, optionCount, false, rawQ, runtime), nil
	case "4": // multiple
		return buildMultipleAnswer(cfg, meta, configIdx, optionCount, rawQ, runtime), nil
	case "5": // scale/nps
		return buildScaleAnswer(cfg, meta, configIdx, optionCount, rawQ, runtime), nil
	case "6": // matrix
		return buildMatrixAnswer(cfg, meta, configIdx, rawQ, runtime), nil
	case "7": // dropdown
		return buildChoiceAnswer(cfg, meta, configIdx, optionCount, true, rawQ, runtime), nil
	case "1": // text
		return buildTextAnswer(cfg, meta, configIdx, runtime), nil
	case "0":
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

func buildChoiceAnswer(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int, isDropdown bool, rawQ map[string]any, runtime *questions.RunContext) *TencentAnswerAction {
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

func buildMultipleAnswer(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int, rawQ map[string]any, runtime *questions.RunContext) *TencentAnswerAction {
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

func buildScaleAnswer(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int, rawQ map[string]any, runtime *questions.RunContext) *TencentAnswerAction {
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

func buildMatrixAnswer(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int, rawQ map[string]any, runtime *questions.RunContext) *TencentAnswerAction {
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

func buildTextAnswer(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int, runtime *questions.RunContext) *TencentAnswerAction {
	text := "满意"
	if candidate, ok := runtime.ConfiguredTextCandidate(configIdx); ok {
		text = candidate
	}
	blankCount := meta.TextInputCount
	if blankCount <= 0 {
		blankCount = 1
	}
	text = runtime.GenerateText(meta, configIdx, text, blankCount)
	return &TencentAnswerAction{
		QuestionID:   meta.ProviderQuestionID,
		QuestionType: meta.ProviderType,
		TextValue:    text,
	}
}

func getProbs(cfg *execution.ExecutionConfig, configIdx, optionCount int, isDropdown bool) []float64 {
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
