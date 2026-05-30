package credamo

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/SurveyController/SurveyConsole/internal/models"
	"github.com/SurveyController/SurveyConsole/internal/network/httpclient"
)

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

func classifyCredamoPayload(payload map[string]any) string {
	if success, ok := payload["success"].(bool); ok && !success {
		return SubmitFailed
	}
	return SubmitSuccess
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
