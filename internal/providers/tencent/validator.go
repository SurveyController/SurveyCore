package tencent

import (
	"fmt"
	"strings"

	"github.com/SurveyController/SurveyConsole/internal/models"
)

func validateTencentActions(actions []TencentAnswerAction, cfg *models.ExecutionConfig) error {
	for _, action := range actions {
		meta, ok := cfg.ProviderQuestionMetadataMap[action.QuestionID]
		if !ok {
			continue
		}
		optionCount := meta.Options
		if optionCount <= 0 {
			optionCount = len(meta.OptionTexts)
		}
		switch meta.TypeCode {
		case "3", "4", "5", "7":
			if len(action.SelectedIndices) == 0 || len(action.SelectedIDs) == 0 {
				return fmt.Errorf("腾讯问卷第%d题答案为空", meta.Num)
			}
			for _, idx := range action.SelectedIndices {
				if idx < 0 || (optionCount > 0 && idx >= optionCount) {
					return fmt.Errorf("腾讯问卷第%d题选项越界: %d", meta.Num, idx)
				}
			}
		case "1":
			if strings.TrimSpace(action.TextValue) == "" {
				return fmt.Errorf("腾讯问卷第%d题填空为空", meta.Num)
			}
		case "6":
			rows := meta.Rows
			if rows <= 0 {
				rows = 1
			}
			if len(action.MatrixIndices) != rows || len(action.MatrixAnswers) != rows {
				return fmt.Errorf("腾讯问卷第%d题矩阵行数不完整", meta.Num)
			}
			for row, idx := range action.MatrixIndices {
				if idx < 0 || (optionCount > 0 && idx >= optionCount) {
					return fmt.Errorf("腾讯问卷第%d题第%d行选项越界: %d", meta.Num, row+1, idx)
				}
			}
		}
	}
	return nil
}
