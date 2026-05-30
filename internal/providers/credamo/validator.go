package credamo

import (
	"fmt"
	"strings"

	"github.com/SurveyController/SurveyConsole/internal/models"
)

func validateCredamoActions(actions []CredamoAnswerAction, cfg *models.ExecutionConfig) error {
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
			if len(action.SelectedIndices) == 0 {
				return fmt.Errorf("Credamo 第%d题答案为空", meta.Num)
			}
			for _, idx := range action.SelectedIndices {
				if idx < 0 || (optionCount > 0 && idx >= optionCount) {
					return fmt.Errorf("Credamo 第%d题选项越界: %d", meta.Num, idx)
				}
			}
		case "1":
			if strings.TrimSpace(action.TextValue) == "" {
				return fmt.Errorf("Credamo 第%d题填空为空", meta.Num)
			}
		case "6":
			rows := meta.Rows
			if rows <= 0 {
				rows = 1
			}
			if len(action.MatrixAnswers) != rows {
				return fmt.Errorf("Credamo 第%d题矩阵行数不完整", meta.Num)
			}
			for row, cols := range action.MatrixAnswers {
				if len(cols) == 0 {
					return fmt.Errorf("Credamo 第%d题第%d行答案为空", meta.Num, row+1)
				}
				for _, idx := range cols {
					if idx < 0 || (optionCount > 0 && idx >= optionCount) {
						return fmt.Errorf("Credamo 第%d题第%d行选项越界: %d", meta.Num, row+1, idx)
					}
				}
			}
		case "11":
			if optionCount > 0 && len(action.OrderIndices) != optionCount {
				return fmt.Errorf("Credamo 第%d题排序答案不完整", meta.Num)
			}
		}
	}
	return nil
}
