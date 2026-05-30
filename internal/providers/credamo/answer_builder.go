package credamo

import (
	"math/rand"

	"github.com/SurveyController/SurveyConsole/internal/execution"
	runstate "github.com/SurveyController/SurveyConsole/internal/runtime"

	"github.com/SurveyController/SurveyConsole/internal/models"
	"github.com/SurveyController/SurveyConsole/internal/providers/providerutil"
	"github.com/SurveyController/SurveyConsole/internal/questions"
)

func buildAnswerActions(cfg *execution.ExecutionConfig, state *runstate.ExecutionState, threadName string) ([]CredamoAnswerAction, error) {
	runtime := questions.NewRunContextForThread(cfg, state, threadName)
	var actions []CredamoAnswerAction
	for _, meta := range sortedQuestions(cfg) {
		action, err := buildSingleAction(cfg, meta, runtime)
		if err != nil {
			return nil, err
		}
		if action != nil {
			actions = append(actions, *action)
		}
	}
	if err := validateCredamoActions(actions, cfg); err != nil {
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
			if questions[i].Num > questions[j].Num {
				questions[i], questions[j] = questions[j], questions[i]
			}
		}
	}
	return questions
}

func buildSingleAction(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, runtime *questions.RunContext) (*CredamoAnswerAction, error) {
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
		return buildChoiceAction(cfg, meta, configIdx, optionCount, runtime), nil
	case "4": // multiple
		return buildMultipleAction(cfg, meta, configIdx, optionCount, runtime), nil
	case "5": // scale
		return buildScaleAction(cfg, meta, configIdx, optionCount, runtime), nil
	case "6": // matrix
		return buildMatrixAction(cfg, meta, configIdx, runtime), nil
	case "7": // dropdown
		return buildChoiceAction(cfg, meta, configIdx, optionCount, runtime), nil
	case "11": // order
		return buildOrderAction(meta, optionCount), nil
	case "1": // text
		return buildTextAction(cfg, meta, configIdx, runtime), nil
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

func buildChoiceAction(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int, runtime *questions.RunContext) *CredamoAnswerAction {
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

func buildMultipleAction(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int, runtime *questions.RunContext) *CredamoAnswerAction {
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

func buildScaleAction(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx, optionCount int, runtime *questions.RunContext) *CredamoAnswerAction {
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

func buildMatrixAction(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int, runtime *questions.RunContext) *CredamoAnswerAction {
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

func buildTextAction(cfg *execution.ExecutionConfig, meta models.SurveyQuestionMeta, configIdx int, runtime *questions.RunContext) *CredamoAnswerAction {
	text := "满意"
	if candidate, ok := runtime.ConfiguredTextCandidate(configIdx); ok {
		text = candidate
	}
	blankCount := meta.TextInputCount
	if blankCount <= 0 {
		blankCount = 1
	}
	text = runtime.GenerateText(meta, configIdx, text, blankCount)
	return &CredamoAnswerAction{
		QuestionID:   meta.ProviderQuestionID,
		QuestionType: meta.ProviderType,
		TextValue:    text,
	}
}
