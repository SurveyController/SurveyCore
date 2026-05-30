package credamo

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/SurveyController/SurveyConsole/internal/models"
)

func standardizeQuestions(raw []map[string]any) []models.SurveyQuestionMeta {
	var result []models.SurveyQuestionMeta
	num := 1

	for _, q := range raw {
		providerType := rawQuestionKind(q)
		typeCode := credamoTypeMap[providerType]
		if typeCode == "" {
			typeCode = "0"
		}

		questionNum := rawQuestionNum(q, num)
		title := firstString(q, "title", "qstTitle", "qstName", "questionTitle", "questionName", "display", "content", "name")
		if title == "" {
			title = fmt.Sprintf("Q%d", questionNum)
		}
		questionID := rawQuestionID(q)
		if questionID == "" {
			questionID = fmt.Sprintf("%d", questionNum)
		}

		optionTexts := extractOptionTexts(q)
		options := len(optionTexts)

		// Extract forced option
		var forcedIdx *int
		forceIdx, forcedText := extractForceSelect(title, optionTexts)
		if forceIdx >= 0 {
			forcedIdx = &forceIdx
		}

		// Multi-select limits
		minLimit, maxLimit := extractMultiSelectLimits(title, options)

		// Row texts for matrix
		rowTexts := extractRowTexts(q)
		rows := len(rowTexts)
		if typeCode == "6" && rows <= 0 {
			rows = 1
		}

		qm := models.SurveyQuestionMeta{
			Num:                questionNum,
			Title:              title,
			TypeCode:           typeCode,
			Options:            options,
			OptionTexts:        optionTexts,
			Rows:               rows,
			RowTexts:           rowTexts,
			Page:               1,
			Provider:           ProviderName,
			ProviderQuestionID: questionID,
			ProviderType:       providerType,
			ForcedOptionIndex:  forcedIdx,
			ForcedOptionText:   forcedText,
			Unsupported:        !credamoSupportedProviderTypes[providerType],
		}
		if qm.Unsupported {
			qm.UnsupportedReason = fmt.Sprintf("暂不支持 Credamo 题型：%s", providerType)
		}

		if minLimit != nil {
			qm.MultiMinLimit = minLimit
		}
		if maxLimit != nil {
			qm.MultiMaxLimit = maxLimit
		}
		if typeCode == "1" {
			qm.IsTextLike = true
		}
		if typeCode == "5" {
			qm.IsRating = true
			qm.RatingMax = options
		}

		result = append(result, qm)
		num++
	}
	return result
}

func extractOptionTexts(q map[string]any) []string {
	source := "options"
	if rawQuestionType(q) == 4 {
		source = "answers"
	}
	items := getArray(q, source)
	if len(items) == 0 && source != "choices" {
		items = getArray(q, "choices")
	}
	var texts []string
	for idx, opt := range items {
		text := choiceText(opt)
		if text == "" {
			text = fmt.Sprintf("选项 %d", idx+1)
		}
		texts = append(texts, text)
	}
	return texts
}

func extractRowTexts(q map[string]any) []string {
	var rows []string
	items := getArray(q, "sub_questions")
	if rawQuestionType(q) == 4 {
		items = getArray(q, "choices")
	}
	for idx, sub := range items {
		text := choiceText(sub)
		if text == "" && rawQuestionType(q) == 4 {
			text = fmt.Sprintf("第 %d 行", idx+1)
		}
		if text != "" {
			rows = append(rows, text)
		}
	}
	return rows
}

func iterRawQuestions(detail map[string]any) []map[string]any {
	result := append([]map[string]any{}, getArray(detail, "questions")...)
	for _, block := range getArray(detail, "blocks") {
		elements := getArray(block, "blockElements")
		if len(elements) == 0 {
			elements = getArray(block, "elements")
		}
		for _, element := range elements {
			candidates := []any{element["question"], element["qst"], element["surveyQuestion"], element}
			for _, candidate := range candidates {
				candidateMap, ok := candidate.(map[string]any)
				if !ok {
					continue
				}
				if rawQuestionID(candidateMap) != "" || rawQuestionType(candidateMap) > 0 {
					result = append(result, candidateMap)
					break
				}
			}
		}
	}
	return result
}

func extractForceSelect(title string, optionTexts []string) (int, string) {
	compactTitle := strings.ReplaceAll(title, " ", "")
	for i, opt := range optionTexts {
		text := strings.TrimSpace(opt)
		if text != "" && strings.Contains(compactTitle, strings.ReplaceAll(text, " ", "")) {
			return i, text
		}
	}
	re := regexp.MustCompile(`请(选择|勾选)\s*([A-Z])`)
	if match := re.FindStringSubmatch(title); match != nil {
		letter := match[2]
		for i, opt := range optionTexts {
			if strings.HasPrefix(strings.ToUpper(opt), letter) {
				return i, opt
			}
		}
	}
	return -1, ""
}

func extractMultiSelectLimits(title string, optionCount int) (*int, *int) {
	reMin := regexp.MustCompile(`至少\s*(\d+)\s*项|最少\s*(\d+)\s*项`)
	reMax := regexp.MustCompile(`至多\s*(\d+)\s*项|最多\s*(\d+)\s*项|不超过\s*(\d+)\s*项`)

	var minLimit, maxLimit *int

	if match := reMin.FindStringSubmatch(title); match != nil {
		s := match[1]
		if s == "" {
			s = match[2]
		}
		if v, err := strconv.Atoi(s); err == nil {
			minLimit = &v
		}
	}
	if match := reMax.FindStringSubmatch(title); match != nil {
		s := match[1]
		if s == "" {
			s = match[2]
		}
		if s == "" {
			s = match[3]
		}
		if v, err := strconv.Atoi(s); err == nil {
			maxLimit = &v
		}
	}
	return minLimit, maxLimit
}
