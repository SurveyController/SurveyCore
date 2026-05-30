package tencent

import (
	"fmt"
	"math/rand"
	"strings"
)

func classifyQQSubmitPayload(payload map[string]any) string {
	code := strings.ToUpper(strings.TrimSpace(fmt.Sprint(payload["code"])))
	if code == "OK" || code == "0" {
		return SubmitSuccess
	}
	return SubmitRejected
}

func buildSubmitBody(surveyID, hashValue string, rawQuestions []map[string]any, actions []TencentAnswerAction, duration int, ua string) map[string]any {
	actionMap := make(map[string]TencentAnswerAction)
	for _, a := range actions {
		actionMap[a.QuestionID] = a
	}

	// Group questions by page_id
	pageQuestions := make(map[string][]map[string]any)
	pageOrder := make([]string, 0)

	for _, rq := range rawQuestions {
		qID := getString(rq, "id")
		qType := getString(rq, "type")
		pageID := getString(rq, "page_id")
		if pageID == "" {
			pageID = "1"
		}

		action, ok := actionMap[qID]
		if !ok {
			continue
		}

		answer := map[string]any{
			"id":     qID,
			"type":   qType,
			"blanks": []any{},
		}

		switch qType {
		case "radio", "checkbox", "nps", "star", "select", "dropdown":
			optsRaw, _ := rq["options"].([]any)
			var options []map[string]any
			for _, opt := range optsRaw {
				if optMap, ok := opt.(map[string]any); ok {
					optID := getString(optMap, "id")
					checked := 0
					for _, selID := range action.SelectedIDs {
						if selID == optID {
							checked = 1
						}
					}
					options = append(options, map[string]any{
						"id":      optID,
						"text":    getString(optMap, "text"),
						"checked": checked,
					})
				}
			}
			answer["options"] = options

		case "text", "textarea":
			answer["text"] = action.TextValue

		case "matrix_radio", "matrix_check":
			var subTitles []map[string]any
			subsRaw, _ := rq["sub_titles"].([]any)
			topLevelOptions, _ := rq["options"].([]any)
			for i, sub := range subsRaw {
				if subMap, ok := sub.(map[string]any); ok {
					// Build ALL options for this row (matching Python behavior)
					subOptsRaw, _ := subMap["options"].([]any)
					if len(subOptsRaw) == 0 {
						subOptsRaw = topLevelOptions
					}
					var subOpts []map[string]any
					for _, subOpt := range subOptsRaw {
						if subOptMap, ok := subOpt.(map[string]any); ok {
							subOptID := getString(subOptMap, "id")
							checked := 0
							if i < len(action.MatrixAnswers) {
								for _, selID := range action.MatrixAnswers[i].OptionIDs {
									if selID == subOptID {
										checked = 1
									}
								}
							}
							subOpts = append(subOpts, map[string]any{
								"id":      subOptID,
								"text":    getString(subOptMap, "text"),
								"checked": checked,
							})
						}
					}
					subTitles = append(subTitles, map[string]any{
						"id":      getString(subMap, "id"),
						"text":    getString(subMap, "text"),
						"options": subOpts,
					})
				}
			}
			answer["sub_titles"] = subTitles
		}

		if _, exists := pageQuestions[pageID]; !exists {
			pageOrder = append(pageOrder, pageID)
		}
		pageQuestions[pageID] = append(pageQuestions[pageID], answer)
	}

	// Build pages array preserving page order
	var pages []map[string]any
	for _, pageID := range pageOrder {
		pages = append(pages, map[string]any{
			"id":        pageID,
			"questions": pageQuestions[pageID],
		})
	}

	// Convert survey_id to int (API expects integer)
	surveyIDInt := 0
	fmt.Sscanf(surveyID, "%d", &surveyIDInt)

	return map[string]any{
		"survey_id": surveyIDInt,
		"hash":      hashValue,
		"answer_survey": map[string]any{
			"duration":  duration,
			"ua":        ua,
			"referrer":  "",
			"uid":       generateUUID(),
			"sid":       generateUUID(),
			"openid":    "",
			"latitude":  nil,
			"longitude": nil,
			"is_update": false,
			"locale":    "zhs",
			"pages":     pages,
		},
	}
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func generateUUID() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		rand.Int31(), rand.Int31n(0xffff), rand.Int31n(0xffff)|0x4000,
		rand.Int31n(0x3fff)|0x8000, rand.Int63n(0xffffffffffff))
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
