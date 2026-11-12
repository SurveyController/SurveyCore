package tencent

import (
	"fmt"
	"strings"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

func ratingAnswer(raw map[string]any, action model.AnswerAction) (map[string]any, error) {
	if len(action.SelectedIndices) != 1 {
		return nil, fmt.Errorf("腾讯问卷第%d题没有生成唯一评分答案", action.QuestionNum)
	}
	providerType := strings.TrimSpace(stringValue(raw["type"]))
	score, err := ratingValue(raw, providerType, action.SelectedIndices[0], action.QuestionNum)
	if err != nil {
		return nil, err
	}
	id := firstString(raw["id"], action.QuestionID)
	if id == "" {
		return nil, fmt.Errorf("腾讯问卷第%d题缺少评分题 id", action.QuestionNum)
	}
	return map[string]any{"id": id + "-" + score, "type": providerType, "answer": score}, nil
}

func matrixRatingAnswer(raw map[string]any, action model.AnswerAction) ([]map[string]any, error) {
	rows := asMapList(raw["sub_titles"])
	if len(rows) == 0 || len(action.MatrixIndices) != len(rows) {
		return nil, fmt.Errorf("腾讯问卷第%d题没有生成完整的矩阵评分答案", action.QuestionNum)
	}
	questionID := firstString(raw["id"], action.QuestionID)
	if questionID == "" {
		return nil, fmt.Errorf("腾讯问卷第%d题缺少矩阵评分题 id", action.QuestionNum)
	}
	answers := make([]map[string]any, 0, len(rows))
	for index, row := range rows {
		score, err := ratingValue(raw, "matrix_star", action.MatrixIndices[index], action.QuestionNum)
		if err != nil {
			return nil, err
		}
		id := questionID
		if rowID := strings.TrimSpace(stringValue(row["id"])); rowID != "" {
			id += "-" + rowID
		}
		answers = append(answers, map[string]any{"id": id + "-" + score, "type": "matrix_star", "answer": "on"})
	}
	return answers, nil
}

func ratingValue(raw map[string]any, providerType string, index, questionNum int) (string, error) {
	values := ratingOptionTexts(raw, providerType)
	if index < 0 || index >= len(values) {
		return "", fmt.Errorf("腾讯问卷第%d题评分选项超出范围", questionNum)
	}
	return values[index], nil
}
