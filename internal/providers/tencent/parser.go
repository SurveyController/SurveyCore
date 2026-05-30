package tencent

import (
	"encoding/json"
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
		providerType := getString(q, "type")
		typeCode := qqTypeMap[providerType]
		if typeCode == "" {
			typeCode = "0"
		}

		title := getString(q, "title")
		if title == "" {
			title = getString(q, "description")
		}

		questionID := getString(q, "id")
		pageID := getString(q, "page_id")
		if pageID == "" {
			pageID = "1"
		}
		page := intFromAny(q["page"])
		if page <= 0 {
			page = 1
		}

		optionTexts := extractOptionTexts(q, providerType)
		options := len(optionTexts)

		// Extract forced option
		var forcedIdx *int
		forceText := extractForceSelect(title, optionTexts)
		if forceText >= 0 {
			forcedIdx = &forceText
		}

		// Extract multi-select limits
		minLimit, maxLimit := extractMultiSelectLimits(title, options)
		if providerType == "checkbox" {
			if min := intFromAny(q["min_length"]); min > 0 {
				minLimit = &min
			}
			if max := intFromAny(q["max_length"]); max > 0 {
				maxLimit = &max
			}
		}

		// Row texts for matrix
		rowTexts := extractRowTexts(q)

		qm := models.SurveyQuestionMeta{
			Num:                num,
			Title:              title,
			TypeCode:           typeCode,
			Options:            options,
			OptionTexts:        optionTexts,
			Rows:               len(rowTexts),
			RowTexts:           rowTexts,
			Page:               page,
			Provider:           ProviderName,
			ProviderQuestionID: questionID,
			ProviderPageID:     pageID,
			ProviderType:       providerType,
			ForcedOptionIndex:  forcedIdx,
			LogicParseStatus:   models.LogicParseStatusNone,
			FillableOptions:    buildFillableOptionIndices(q, providerType),
			Unsupported:        !qqSupportedProviderTypes[providerType],
		}
		if qm.Unsupported {
			qm.UnsupportedReason = fmt.Sprintf("暂不支持腾讯题型：%s", providerType)
		}

		if minLimit != nil {
			qm.MultiMinLimit = minLimit
		}
		if maxLimit != nil {
			qm.MultiMaxLimit = maxLimit
		}

		// Detect text-like
		if typeCode == "1" {
			qm.IsTextLike = true
		}
		// Detect rating
		if providerType == "nps" || providerType == "star" {
			qm.IsRating = true
			qm.RatingMax = options
		}

		result = append(result, qm)
		num++
	}
	attachTencentLogicMetadata(raw, result)
	return result
}

func extractOptionTexts(q map[string]any, providerType string) []string {
	if providerType == "nps" {
		// NPS generates numeric labels
		beginNum := 0
		count := 10
		if b := intFromAny(q["star_begin_num"]); b > 0 {
			beginNum = b
		}
		if c := intFromAny(q["star_num"]); c > 0 {
			count = c
		}
		texts := make([]string, count)
		for i := range texts {
			texts[i] = strconv.Itoa(beginNum + i)
		}
		return texts
	}

	if providerType == "star" || providerType == "matrix_star" {
		count := 5
		if c := intFromAny(q["star_num"]); c > 0 {
			count = c
		}
		texts := make([]string, count)
		for i := range texts {
			texts[i] = strconv.Itoa(i + 1)
		}
		return texts
	}

	// Standard options
	if optsRaw, ok := q["options"].([]any); ok {
		var texts []string
		for _, opt := range optsRaw {
			if optMap, ok := opt.(map[string]any); ok {
				if text, ok := optMap["text"].(string); ok && text != "" {
					texts = append(texts, text)
				}
			}
		}
		return texts
	}
	return nil
}

func buildFillableOptionIndices(q map[string]any, providerType string) []int {
	if providerType != "radio" && providerType != "checkbox" && providerType != "select" {
		return nil
	}
	optsRaw, ok := q["options"].([]any)
	if !ok {
		return nil
	}
	var fillable []int
	for idx, item := range optsRaw {
		if payloadContainsFillblank(item, 0) {
			fillable = append(fillable, idx)
		}
	}
	return fillable
}

func payloadContainsFillblank(value any, depth int) bool {
	if depth > 4 || value == nil {
		return false
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if strings.Contains(strings.ToLower(key), "fillblank") || payloadContainsFillblank(item, depth+1) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if payloadContainsFillblank(item, depth+1) {
				return true
			}
		}
	case string:
		return strings.Contains(strings.ToLower(typed), "fillblank")
	}
	return false
}

func extractRowTexts(q map[string]any) []string {
	var rows []string
	if subsRaw, ok := q["sub_titles"].([]any); ok {
		for _, sub := range subsRaw {
			if subMap, ok := sub.(map[string]any); ok {
				if text, ok := subMap["text"].(string); ok && text != "" {
					rows = append(rows, text)
				}
			}
		}
	}
	return rows
}

func extractJumpRules(q map[string]any) []map[string]any {
	var rules []map[string]any
	if gotoRaw, ok := q["goto"].([]any); ok {
		for _, g := range gotoRaw {
			if gMap, ok := g.(map[string]any); ok {
				rules = append(rules, map[string]any{
					"source_option": gMap["option_id"],
					"target_page":   gMap["page_id"],
				})
			}
		}
	}
	return rules
}

type tencentLogicState struct {
	JumpRules             []map[string]any
	HasJump               bool
	HasSourceDisplayLogic bool
	ExactLogicParsed      bool
}

func attachTencentLogicMetadata(rawQuestions []map[string]any, questions []models.SurveyQuestionMeta) {
	if len(rawQuestions) == 0 || len(questions) == 0 {
		return
	}
	questionByProviderID := make(map[string]*models.SurveyQuestionMeta)
	questionNumByProviderID := make(map[string]int)
	firstQuestionNumByPageID := make(map[string]int)
	maxQuestionNum := 0
	for i := range questions {
		q := &questions[i]
		if q.ProviderQuestionID == "" || q.Num <= 0 {
			continue
		}
		questionByProviderID[q.ProviderQuestionID] = q
		questionNumByProviderID[q.ProviderQuestionID] = q.Num
		if q.Num > maxQuestionNum {
			maxQuestionNum = q.Num
		}
		if q.ProviderPageID != "" {
			if _, exists := firstQuestionNumByPageID[q.ProviderPageID]; !exists {
				firstQuestionNumByPageID[q.ProviderPageID] = q.Num
			}
		}
	}

	stateByProviderID := make(map[string]*tencentLogicState)
	sourceTargets := make(map[string][]map[string]any)
	inboundConditions := make(map[string][]map[string]any)
	for _, rawQuestion := range rawQuestions {
		providerQuestionID := strings.TrimSpace(getString(rawQuestion, "id"))
		if providerQuestionID == "" {
			continue
		}
		if _, ok := questionByProviderID[providerQuestionID]; !ok {
			continue
		}
		state := stateByProviderID[providerQuestionID]
		if state == nil {
			state = &tencentLogicState{}
			stateByProviderID[providerQuestionID] = state
		}

		if target := resolveTencentJumpTarget(rawQuestion["goto"], questionNumByProviderID, firstQuestionNumByPageID, maxQuestionNum); target != nil {
			state.JumpRules = append(state.JumpRules, map[string]any{
				"option_index": -1,
				"jumpto":       *target,
				"option_text":  nil,
			})
			state.HasJump = true
			state.ExactLogicParsed = true
		} else if valuePresent(rawQuestion["goto"]) {
			state.HasJump = true
		}

		options, _ := rawQuestion["options"].([]any)
		for optionIndex, optionRaw := range options {
			option, ok := optionRaw.(map[string]any)
			if !ok {
				continue
			}
			if target := resolveTencentJumpTarget(option["goto"], questionNumByProviderID, firstQuestionNumByPageID, maxQuestionNum); target != nil {
				state.JumpRules = append(state.JumpRules, map[string]any{
					"option_index": optionIndex,
					"jumpto":       *target,
					"option_text":  strings.TrimSpace(getString(option, "text")),
				})
				state.HasJump = true
				state.ExactLogicParsed = true
			} else if valuePresent(option["goto"]) {
				state.HasJump = true
			}

			displayPayload := option["display"]
			if !valuePresent(displayPayload) {
				continue
			}
			state.HasSourceDisplayLogic = true
			targetQuestionIDs := extractTencentQuestionRefs(displayPayload)
			if len(targetQuestionIDs) == 0 {
				continue
			}
			state.ExactLogicParsed = true
			for _, targetQuestionID := range targetQuestionIDs {
				targetQuestionNum := questionNumByProviderID[targetQuestionID]
				if targetQuestionNum <= 0 {
					continue
				}
				sourceQuestionNum := questionNumByProviderID[providerQuestionID]
				sourceTargets[providerQuestionID] = append(sourceTargets[providerQuestionID], map[string]any{
					"target_question_num":      targetQuestionNum,
					"condition_option_indices": []int{optionIndex},
					"condition_mode":           "selected",
				})
				inboundConditions[targetQuestionID] = append(inboundConditions[targetQuestionID], map[string]any{
					"condition_question_num":   sourceQuestionNum,
					"condition_mode":           "selected",
					"condition_option_indices": []int{optionIndex},
				})
			}
		}
	}

	for _, rawQuestion := range rawQuestions {
		providerQuestionID := strings.TrimSpace(getString(rawQuestion, "id"))
		if providerQuestionID == "" {
			continue
		}
		if _, hasInbound := inboundConditions[providerQuestionID]; hasInbound {
			continue
		}
		referQuestionIDs := extractTencentQuestionRefs(rawQuestion["refer"])
		if len(referQuestionIDs) == 0 {
			continue
		}
		targetQuestionNum := questionNumByProviderID[providerQuestionID]
		if targetQuestionNum <= 0 {
			continue
		}
		for _, referQuestionID := range referQuestionIDs {
			sourceQuestionNum := questionNumByProviderID[referQuestionID]
			if sourceQuestionNum <= 0 {
				continue
			}
			inboundConditions[providerQuestionID] = append(inboundConditions[providerQuestionID], map[string]any{
				"condition_question_num":   sourceQuestionNum,
				"condition_mode":           "selected",
				"condition_option_indices": []int{},
			})
			sourceTargets[referQuestionID] = append(sourceTargets[referQuestionID], map[string]any{
				"target_question_num":      targetQuestionNum,
				"condition_option_indices": []int{},
				"condition_mode":           "selected",
			})
		}
	}

	for _, rawQuestion := range rawQuestions {
		providerQuestionID := strings.TrimSpace(getString(rawQuestion, "id"))
		q := questionByProviderID[providerQuestionID]
		if q == nil {
			continue
		}
		state := stateByProviderID[providerQuestionID]
		if state == nil {
			state = &tencentLogicState{}
		}
		q.JumpRules = dedupeLogicMaps(state.JumpRules, "option_index", "jumpto")
		q.HasJump = state.HasJump || len(q.JumpRules) > 0

		controls := normalizeLogicMaps(sourceTargets[providerQuestionID])
		q.ControlsDisplayTargets = dedupeLogicMaps(controls, "target_question_num", "condition_option_indices", "condition_mode")
		if len(q.ControlsDisplayTargets) > 0 || state.HasSourceDisplayLogic {
			q.HasDependentDisplayLogic = true
		}

		referQuestionIDs := extractTencentQuestionRefs(rawQuestion["refer"])
		if valuePresent(rawQuestion["hidden"]) || len(referQuestionIDs) > 0 || len(inboundConditions[providerQuestionID]) > 0 {
			q.HasDisplayCondition = true
		}
		conditions := normalizeLogicMaps(inboundConditions[providerQuestionID])
		q.DisplayConditions = dedupeLogicMaps(conditions, "condition_question_num", "condition_option_indices", "condition_mode")
		if displayConditionsHaveOptionIndices(q.DisplayConditions) {
			state.ExactLogicParsed = true
		}

		hasAnyLogic := q.HasJump || q.HasDisplayCondition || q.HasDependentDisplayLogic
		if hasAnyLogic && state.ExactLogicParsed {
			q.LogicParseStatus = models.LogicParseStatusComplete
		} else if hasAnyLogic {
			q.LogicParseStatus = models.LogicParseStatusUnknown
		}
	}
}

func resolveTencentJumpTarget(rawTarget any, questionNumByProviderID map[string]int, firstQuestionNumByPageID map[string]int, maxQuestionNum int) *int {
	if !valuePresent(rawTarget) {
		return nil
	}
	if numeric := intFromAny(rawTarget); numeric > 0 {
		return &numeric
	}
	for _, questionID := range extractTencentQuestionRefs(rawTarget) {
		if target := questionNumByProviderID[questionID]; target > 0 {
			return &target
		}
	}
	for _, pageID := range extractTencentPageRefs(rawTarget) {
		if target := firstQuestionNumByPageID[pageID]; target > 0 {
			return &target
		}
	}
	lowered := strings.ToLower(strings.TrimSpace(fmt.Sprint(rawTarget)))
	if lowered != "" {
		for _, token := range qqLogicEndTokens {
			if strings.Contains(lowered, token) {
				target := maxQuestionNum + 1
				return &target
			}
		}
	}
	return nil
}

func extractTencentQuestionRefs(value any) []string {
	return uniqueStrings(collectTencentTokenRefs(value, qqQuestionIDRe, 0))
}

func extractTencentPageRefs(value any) []string {
	return uniqueStrings(collectTencentTokenRefs(value, qqPageIDRe, 0))
}

func collectTencentTokenRefs(value any, pattern *regexp.Regexp, depth int) []string {
	if depth > 5 || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		var result []string
		for key, item := range typed {
			result = append(result, collectTencentTokenRefs(key, pattern, depth+1)...)
			result = append(result, collectTencentTokenRefs(item, pattern, depth+1)...)
		}
		return result
	case []any:
		var result []string
		for _, item := range typed {
			result = append(result, collectTencentTokenRefs(item, pattern, depth+1)...)
		}
		return result
	case []map[string]any:
		var result []string
		for _, item := range typed {
			result = append(result, collectTencentTokenRefs(item, pattern, depth+1)...)
		}
		return result
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			return nil
		}
		return pattern.FindAllString(text, -1)
	}
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func normalizeLogicMaps(items []map[string]any) []map[string]any {
	normalized := make([]map[string]any, 0, len(items))
	for _, item := range items {
		normalizedItem := make(map[string]any, len(item))
		for key, value := range item {
			switch key {
			case "condition_question_num", "target_question_num", "jumpto", "option_index":
				normalizedItem[key] = intFromAny(value)
			case "condition_option_indices":
				normalizedItem[key] = intSliceFromAny(value)
			case "condition_mode":
				mode := strings.TrimSpace(fmt.Sprint(value))
				if mode == "" {
					mode = "selected"
				}
				normalizedItem[key] = mode
			default:
				normalizedItem[key] = value
			}
		}
		normalized = append(normalized, normalizedItem)
	}
	return normalized
}

func dedupeLogicMaps(items []map[string]any, keyFields ...string) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	seen := make(map[string]bool)
	for _, item := range items {
		if item == nil {
			continue
		}
		keyParts := make([]string, 0, len(keyFields))
		for _, field := range keyFields {
			keyParts = append(keyParts, fmt.Sprint(normalizedLogicKeyValue(item[field])))
		}
		key := strings.Join(keyParts, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}
	return result
}

func normalizedLogicKeyValue(value any) any {
	switch typed := value.(type) {
	case []int:
		values := append([]int{}, typed...)
		parts := make([]string, len(values))
		for i, value := range values {
			parts[i] = strconv.Itoa(value)
		}
		return strings.Join(parts, ",")
	case []any:
		values := intSliceFromAny(typed)
		parts := make([]string, len(values))
		for i, value := range values {
			parts[i] = strconv.Itoa(value)
		}
		return strings.Join(parts, ",")
	default:
		return typed
	}
}

func displayConditionsHaveOptionIndices(conditions []map[string]any) bool {
	for _, condition := range conditions {
		if len(intSliceFromAny(condition["condition_option_indices"])) > 0 {
			return true
		}
	}
	return false
}

func valuePresent(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case float32:
		return typed != 0
	case json.Number:
		return strings.TrimSpace(typed.String()) != "" && typed.String() != "0"
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		return len(typed) > 0
	case []map[string]any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return 0
}

func intSliceFromAny(value any) []int {
	switch typed := value.(type) {
	case []int:
		return append([]int{}, typed...)
	case []float64:
		result := make([]int, 0, len(typed))
		for _, item := range typed {
			if item >= 0 {
				result = append(result, int(item))
			}
		}
		return result
	case []any:
		result := make([]int, 0, len(typed))
		for _, item := range typed {
			value := intFromAny(item)
			if value >= 0 {
				result = append(result, value)
			}
		}
		return result
	default:
		return nil
	}
}

func extractForceSelect(title string, optionTexts []string) int {
	// Simple heuristic: look for "请选择 X" patterns
	for i, opt := range optionTexts {
		if strings.Contains(title, "请选择"+opt) || strings.Contains(title, "选择 "+opt) {
			return i
		}
	}
	return -1
}

func extractMultiSelectLimits(title string, optionCount int) (*int, *int) {
	// Look for "至少选 N 项" / "最多选 M 项"
	reMin := regexp.MustCompile(`至少选\s*(\d+)`)
	reMax := regexp.MustCompile(`最多选\s*(\d+)|至多选\s*(\d+)|不超过\s*(\d+)`)

	var minLimit, maxLimit *int

	if match := reMin.FindStringSubmatch(title); match != nil {
		if v, err := strconv.Atoi(match[1]); err == nil {
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
