package config

import (
	"strings"

	"github.com/SurveyController/SurveyConsole/internal/models"
)

const defaultFillText = "无"

// BuildDefaultQuestionEntries creates editable runtime entries from parsed survey metadata.
func BuildDefaultQuestionEntries(questions []models.SurveyQuestionMeta, existing []models.QuestionEntry) []models.QuestionEntry {
	existingByProvider := make(map[string]models.QuestionEntry)
	existingByNum := make(map[int]models.QuestionEntry)
	existingByTitle := make(map[string]models.QuestionEntry)
	for _, entry := range existing {
		if entry.ProviderQuestionID != nil && strings.TrimSpace(*entry.ProviderQuestionID) != "" {
			key := providerEntryKey(entry.SurveyProvider, *entry.ProviderQuestionID)
			if _, exists := existingByProvider[key]; !exists {
				existingByProvider[key] = entry
			}
		}
		if entry.QuestionNum != nil {
			if _, exists := existingByNum[*entry.QuestionNum]; !exists {
				existingByNum[*entry.QuestionNum] = entry
			}
		}
		if titleKey := normalizedTitle(ptrString(entry.QuestionTitle)); titleKey != "" {
			if _, exists := existingByTitle[titleKey]; !exists {
				existingByTitle[titleKey] = entry
			}
		}
	}

	entries := make([]models.QuestionEntry, 0, len(questions))
	for _, q := range questions {
		if q.IsDescription || q.Unsupported {
			continue
		}
		qType := inferQuestionEntryType(q)
		optionCount := defaultOptionCount(q, qType)
		rows := q.Rows
		if rows <= 0 {
			rows = 1
		}

		entry := defaultEntryForQuestion(q, qType, optionCount, rows)
		if existing, ok := matchingExistingEntry(q, qType, existingByProvider, existingByNum, existingByTitle); ok {
			entry = mergeExistingQuestionEntry(entry, existing, qType)
		}
		applyForcedDefaults(&entry, q, qType, optionCount)
		entries = append(entries, entry)
	}
	return entries
}

func defaultEntryForQuestion(q models.SurveyQuestionMeta, qType string, optionCount, rows int) models.QuestionEntry {
	entry := models.QuestionEntry{
		QuestionType:       qType,
		Rows:               rows,
		OptionCount:        optionCount,
		DistributionMode:   "random",
		QuestionNum:        intPtr(q.Num),
		QuestionTitle:      stringPtrIfNotEmpty(q.Title),
		SurveyProvider:     q.Provider,
		ProviderQuestionID: stringPtrIfNotEmpty(q.ProviderQuestionID),
		ProviderPageID:     stringPtrIfNotEmpty(q.ProviderPageID),
		IsLocation:         q.IsLocation,
	}

	switch qType {
	case "single", "dropdown", "scale", "matrix", "order":
		entry.Probabilities = -1
	case "score":
		if entry.OptionCount < 2 {
			entry.OptionCount = 2
		}
		weights := repeatFloat(1.0, entry.OptionCount)
		entry.Probabilities = append([]float64{}, weights...)
		entry.CustomWeights = append([]float64{}, weights...)
		entry.DistributionMode = "custom"
	case "multiple":
		entry.Probabilities = repeatFloat(50.0, optionCount)
	case "slider":
		target := sliderMidpoint(q)
		entry.OptionCount = 1
		entry.Probabilities = []float64{target}
		entry.CustomWeights = []float64{target}
		entry.DistributionMode = "custom"
	case "multi_text":
		entry.Probabilities = []float64{1.0}
		entry.Texts = []string{defaultFillText}
		entry.MultiTextBlankModes = inferMultiTextBlankModes(q, q.TextInputCount)
	case "text":
		entry.Probabilities = []float64{1.0}
		entry.Texts = []string{defaultFillText}
	default:
		entry.QuestionType = "single"
		entry.Probabilities = -1
	}

	if qType == "single" || qType == "multiple" || qType == "dropdown" {
		entry.FillableOptionIndices = normalizeOptionIndices(q.FillableOptions, optionCount)
		if qType == "single" {
			entry.AttachedOptionSelects = cloneMapSlice(q.AttachedOptionSelects)
		}
	}
	return entry
}

func mergeExistingQuestionEntry(base, existing models.QuestionEntry, qType string) models.QuestionEntry {
	base.Probabilities = cloneAny(existing.Probabilities)
	if existing.DistributionMode != "" {
		base.DistributionMode = existing.DistributionMode
	}
	base.CustomWeights = cloneAny(existing.CustomWeights)
	base.Texts = append([]string{}, existing.Texts...)
	if qType == "text" || qType == "multi_text" {
		base.AIEnabled = existing.AIEnabled
	}
	if qType == "text" {
		base.TextRandomMode = existing.TextRandomMode
		base.TextRandomIntRange = append([]int{}, existing.TextRandomIntRange...)
	}
	if qType == "multi_text" {
		base.MultiTextBlankModes = append([]string{}, existing.MultiTextBlankModes...)
		base.MultiTextBlankAIFlags = append([]bool{}, existing.MultiTextBlankAIFlags...)
		base.MultiTextBlankIntRanges = cloneIntRanges(existing.MultiTextBlankIntRanges)
	}
	if qType == "single" || qType == "multiple" || qType == "dropdown" {
		base.OptionFillTexts = cloneStringPointerSlice(existing.OptionFillTexts)
		base.FillableOptionIndices = append([]int{}, existing.FillableOptionIndices...)
	}
	if qType == "single" {
		base.AttachedOptionSelects = cloneMapSlice(existing.AttachedOptionSelects)
	}
	if existing.Dimension != nil {
		base.Dimension = stringPtrIfNotEmpty(*existing.Dimension)
	}
	base.LocationParts = append([]string{}, existing.LocationParts...)
	return base
}

func applyForcedDefaults(entry *models.QuestionEntry, q models.SurveyQuestionMeta, qType string, optionCount int) {
	if q.ForcedOptionIndex != nil && *q.ForcedOptionIndex >= 0 && *q.ForcedOptionIndex < optionCount {
		switch qType {
		case "single", "dropdown", "scale", "score":
			weights := make([]float64, optionCount)
			weights[*q.ForcedOptionIndex] = 1.0
			entry.Probabilities = weights
			entry.CustomWeights = append([]float64{}, weights...)
			entry.DistributionMode = "custom"
		}
	}
	if len(q.ForcedTexts) > 0 && (qType == "text" || qType == "multi_text") {
		texts := make([]string, 0, len(q.ForcedTexts))
		for _, text := range q.ForcedTexts {
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				texts = append(texts, trimmed)
			}
		}
		if len(texts) > 0 {
			entry.Texts = texts
		}
	}
}

func matchingExistingEntry(q models.SurveyQuestionMeta, qType string, byProvider map[string]models.QuestionEntry, byNum map[int]models.QuestionEntry, byTitle map[string]models.QuestionEntry) (models.QuestionEntry, bool) {
	if q.ProviderQuestionID != "" {
		if candidate, ok := byProvider[providerEntryKey(q.Provider, q.ProviderQuestionID)]; ok && candidate.QuestionType == qType {
			return candidate, true
		}
	}
	if candidate, ok := byNum[q.Num]; ok && candidate.QuestionType == qType {
		parsedTitle := normalizedTitle(q.Title)
		candidateTitle := normalizedTitle(ptrString(candidate.QuestionTitle))
		if parsedTitle == "" || candidateTitle == "" || parsedTitle == candidateTitle {
			return candidate, true
		}
	}
	if titleKey := normalizedTitle(q.Title); titleKey != "" {
		if candidate, ok := byTitle[titleKey]; ok && candidate.QuestionType == qType {
			return candidate, true
		}
	}
	return models.QuestionEntry{}, false
}

func inferQuestionEntryType(q models.SurveyQuestionMeta) string {
	typeCode := strings.TrimSpace(q.TypeCode)
	if q.IsSliderMatrix {
		return "matrix"
	}
	if q.IsMultiText || (q.IsTextLike && q.TextInputCount > 1) {
		return "multi_text"
	}
	if q.IsTextLike || typeCode == "1" || typeCode == "2" {
		return "text"
	}
	switch typeCode {
	case "3", "33", "34":
		return "single"
	case "4":
		return "multiple"
	case "5":
		if q.IsRating {
			return "score"
		}
		return "scale"
	case "6", "9":
		return "matrix"
	case "7", "35":
		return "dropdown"
	case "8":
		return "slider"
	case "11", "12":
		return "order"
	default:
		return "single"
	}
}

func defaultOptionCount(q models.SurveyQuestionMeta, qType string) int {
	optionCount := q.Options
	if q.RatingMax > optionCount {
		optionCount = q.RatingMax
	}
	if optionCount <= 0 {
		optionCount = len(q.OptionTexts)
	}
	if qType == "text" || qType == "multi_text" {
		if q.TextInputCount > optionCount {
			optionCount = q.TextInputCount
		}
	}
	if optionCount <= 0 {
		optionCount = 1
	}
	return optionCount
}

func sliderMidpoint(q models.SurveyQuestionMeta) float64 {
	minValue := 0.0
	maxValue := 100.0
	if q.SliderMin != nil {
		minValue = *q.SliderMin
	}
	if q.SliderMax != nil {
		maxValue = *q.SliderMax
	}
	if maxValue <= minValue {
		maxValue = minValue + 100.0
	}
	return minValue + (maxValue-minValue)/2.0
}

func inferMultiTextBlankModes(q models.SurveyQuestionMeta, blankCount int) []string {
	if blankCount <= 0 {
		blankCount = 1
	}
	modes := make([]string, blankCount)
	for i := 0; i < blankCount; i++ {
		label := ""
		if i < len(q.TextInputLabels) {
			label = q.TextInputLabels[i]
		}
		if label == "" && blankCount == 1 {
			label = q.Title
		}
		normalized := strings.ToLower(strings.Join(strings.Fields(label), ""))
		switch {
		case strings.Contains(normalized, "手机号") || strings.Contains(normalized, "手机号码") ||
			strings.Contains(normalized, "手机") || strings.Contains(normalized, "电话") ||
			strings.Contains(normalized, "联系电话") || strings.Contains(normalized, "联系方式"):
			modes[i] = models.TextRandomMobile
		case strings.Contains(normalized, "身份证") || strings.Contains(normalized, "证件号") || strings.Contains(normalized, "证件号码"):
			modes[i] = models.TextRandomIDCard
		case strings.Contains(normalized, "姓名") || strings.Contains(normalized, "名字") || strings.Contains(normalized, "联系人"):
			modes[i] = models.TextRandomName
		default:
			modes[i] = models.TextRandomNone
		}
	}
	return modes
}

func normalizeOptionIndices(indices []int, optionCount int) []int {
	result := make([]int, 0, len(indices))
	seen := make(map[int]bool)
	for _, idx := range indices {
		if idx < 0 || idx >= optionCount || seen[idx] {
			continue
		}
		seen[idx] = true
		result = append(result, idx)
	}
	return result
}

func repeatFloat(value float64, count int) []float64 {
	if count <= 0 {
		count = 1
	}
	result := make([]float64, count)
	for i := range result {
		result[i] = value
	}
	return result
}

func providerEntryKey(provider, questionID string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = models.ProviderWJX
	}
	return provider + ":" + strings.TrimSpace(questionID)
}

func normalizedTitle(value string) string {
	return strings.Join(strings.Fields(value), "")
}

func ptrString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intPtr(value int) *int {
	return &value
}

func stringPtrIfNotEmpty(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case []float64:
		return append([]float64{}, typed...)
	case []any:
		return append([]any{}, typed...)
	case [][]float64:
		out := make([][]float64, len(typed))
		for i := range typed {
			out[i] = append([]float64{}, typed[i]...)
		}
		return out
	default:
		return value
	}
}

func cloneStringPointerSlice(src []*string) []*string {
	if src == nil {
		return nil
	}
	dst := make([]*string, len(src))
	for i, item := range src {
		if item != nil {
			value := *item
			dst[i] = &value
		}
	}
	return dst
}

func cloneMapSlice(src []map[string]any) []map[string]any {
	if src == nil {
		return nil
	}
	dst := make([]map[string]any, len(src))
	for i, item := range src {
		if item == nil {
			continue
		}
		cloned := make(map[string]any, len(item))
		for key, value := range item {
			cloned[key] = cloneAny(value)
		}
		dst[i] = cloned
	}
	return dst
}
