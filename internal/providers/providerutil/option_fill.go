package providerutil

import (
	"strings"

	"github.com/SurveyController/SurveyCore/internal/models"
)

const DefaultOptionFillText = "其他"

// ResolveOptionFillText returns the normalized fill text for a selected option.
// It falls back to a default placeholder when the option requires fill text
// but the config entry is empty.
func ResolveOptionFillText(fillEntries []*string, optionIndex int, meta *models.SurveyQuestionMeta) string {
	if optionIndex < 0 {
		return ""
	}
	if optionIndex < len(fillEntries) && fillEntries[optionIndex] != nil {
		if value := strings.TrimSpace(*fillEntries[optionIndex]); value != "" {
			return value
		}
	}
	if optionRequiresFill(meta, optionIndex) {
		return DefaultOptionFillText
	}
	return ""
}

func optionRequiresFill(meta *models.SurveyQuestionMeta, optionIndex int) bool {
	if meta == nil {
		return false
	}
	for _, idx := range meta.FillableOptions {
		if idx == optionIndex {
			return true
		}
	}
	return false
}
