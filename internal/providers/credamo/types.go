package credamo

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var shortURLRe = regexp.MustCompile(`(?i)(?:^|/)([A-Za-z0-9_-]+)/?$`)

const (
	SubmitSuccess = "success"
	SubmitFailed  = "failed"
)

// Type code mapping
var credamoTypeMap = map[string]string{
	"single_choice":   "3",
	"single":          "3",
	"multiple_choice": "4",
	"multiple":        "4",
	"scale":           "5",
	"matrix":          "6",
	"dropdown":        "7",
	"ordering":        "11",
	"order":           "11",
	"text":            "1",
	"textarea":        "1",
	"description":     "0",
}

var credamoSupportedProviderTypes = map[string]bool{
	"single_choice":   true,
	"single":          true,
	"multiple_choice": true,
	"multiple":        true,
	"scale":           true,
	"matrix":          true,
	"dropdown":        true,
	"ordering":        true,
	"order":           true,
	"text":            true,
	"textarea":        true,
	"description":     true,
}

// CredamoAnswerAction represents a Credamo survey answer.
type CredamoAnswerAction struct {
	QuestionID      string
	QuestionType    string
	SelectedIndices []int
	TextValue       string
	MatrixAnswers   [][]int
	OrderIndices    []int
}

func int64FromAny(value any, fallback int64) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return n
		}
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func rawQuestionID(m map[string]any) string {
	return firstString(m, "qstId", "id", "questionId")
}

func rawQuestionNum(m map[string]any, fallback int) int {
	for _, key := range []string{"qstNo", "questionNo", "qstNum", "sortNo"} {
		match := regexp.MustCompile(`\d+`).FindString(valueString(m[key]))
		if match != "" {
			if num, err := strconv.Atoi(match); err == nil && num > 0 {
				return num
			}
		}
	}
	return fallback
}

func rawQuestionType(m map[string]any) int {
	return toInt(m["questionType"])
}

func rawSelector(m map[string]any) int {
	return toInt(m["selector"])
}

func rawQuestionKind(m map[string]any) string {
	if kind := strings.ToLower(strings.TrimSpace(firstString(m, "question_kind", "provider_type", "type"))); kind != "" {
		return kind
	}
	questionType := rawQuestionType(m)
	selector := rawSelector(m)
	switch {
	case questionType == 2 && selector == 2:
		return "multiple"
	case questionType == 2 && selector == 3:
		return "dropdown"
	case questionType == 2:
		return "single"
	case questionType == 4:
		return "matrix"
	case questionType == 6:
		return "order"
	case questionType == 11:
		return "scale"
	case questionType == 1:
		return "text"
	}
	return ""
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s := strings.TrimSpace(valueString(m[key])); s != "" {
			return s
		}
	}
	return ""
}

func valueString(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case float64:
		if value == float64(int64(value)) {
			return strconv.FormatInt(int64(value), 10)
		}
		return strconv.FormatFloat(value, 'f', -1, 64)
	case float32:
		f := float64(value)
		if f == float64(int64(f)) {
			return strconv.FormatInt(int64(f), 10)
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func toInt(v any) int {
	switch value := v.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case float32:
		return int(value)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(value))
		return i
	default:
		return 0
	}
}

func rawChoiceOptions(q map[string]any) []map[string]any {
	if options := getArray(q, "options"); len(options) > 0 {
		return options
	}
	return getArray(q, "choices")
}

func rawMatrixRows(q map[string]any) []map[string]any {
	if rows := getArray(q, "sub_questions"); len(rows) > 0 {
		return rows
	}
	return getArray(q, "choices")
}

func rawMatrixColumns(q map[string]any) []map[string]any {
	if columns := getArray(q, "answers"); len(columns) > 0 {
		return columns
	}
	return rawChoiceOptions(q)
}

func choiceText(m map[string]any) string {
	return firstString(m, "text", "label", "display", "answerContent", "choiceContent", "choiceTitle", "answerTitle", "content", "title", "name")
}

func choiceID(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := m[key]; ok && strings.TrimSpace(valueString(value)) != "" {
			return normalizeID(value)
		}
	}
	return ""
}

func normalizeID(value any) any {
	text := strings.TrimSpace(valueString(value))
	if text == "" {
		return ""
	}
	if id, err := strconv.Atoi(text); err == nil {
		return id
	}
	return text
}

func choicePayload(choice map[string]any) map[string]any {
	return map[string]any{
		"choiceId":      choiceID(choice, "choiceId", "id"),
		"choiceContent": choiceText(choice),
	}
}

func getString(m map[string]any, key string) string {
	return strings.TrimSpace(valueString(m[key]))
}

func getArray(m map[string]any, key string) []map[string]any {
	if arr, ok := m[key].([]any); ok {
		result := make([]map[string]any, 0, len(arr))
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				result = append(result, m)
			}
		}
		return result
	}
	if arr, ok := m[key].([]map[string]any); ok {
		return arr
	}
	return nil
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
