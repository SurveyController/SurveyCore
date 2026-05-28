package models

// Text random mode constants
const (
	TextRandomNone    = "none"
	TextRandomName    = "name"
	TextRandomMobile  = "mobile"
	TextRandomIDCard  = "id_card"
	TextRandomInteger = "integer"

	TextRandomNameToken   = "__RANDOM_NAME__"
	TextRandomMobileToken = "__RANDOM_MOBILE__"
	TextRandomIDCardToken = "__RANDOM_ID_CARD__"

	GlobalReliabilityDimension = "__global_reliability__"
)

// QuestionEntry represents how a single question should be answered.
type QuestionEntry struct {
	QuestionType           string           `json:"question_type"`
	Probabilities          any              `json:"probabilities"` // []float64, [][]float64, int, or nil
	Texts                  []string         `json:"texts,omitempty"`
	Rows                   int              `json:"rows,omitempty"`
	OptionCount            int              `json:"option_count,omitempty"`
	DistributionMode       string           `json:"distribution_mode,omitempty"`
	CustomWeights          any              `json:"custom_weights,omitempty"` // []float64 or [][]float64 or nil
	QuestionNum            *int             `json:"question_num,omitempty"`
	QuestionTitle          *string          `json:"question_title,omitempty"`
	SurveyProvider         string           `json:"survey_provider,omitempty"`
	ProviderQuestionID     *string          `json:"provider_question_id,omitempty"`
	ProviderPageID         *string          `json:"provider_page_id,omitempty"`
	AIEnabled              bool             `json:"ai_enabled,omitempty"`
	MultiTextBlankModes    []string         `json:"multi_text_blank_modes,omitempty"`
	MultiTextBlankAIFlags  []bool           `json:"multi_text_blank_ai_flags,omitempty"`
	MultiTextBlankIntRanges [][]int         `json:"multi_text_blank_int_ranges,omitempty"`
	TextRandomMode         string           `json:"text_random_mode,omitempty"`
	TextRandomIntRange     []int            `json:"text_random_int_range,omitempty"`
	OptionFillTexts        []*string        `json:"option_fill_texts,omitempty"`
	FillableOptionIndices  []int            `json:"fillable_option_indices,omitempty"`
	AttachedOptionSelects  []map[string]any `json:"attached_option_selects,omitempty"`
	IsLocation             bool             `json:"is_location,omitempty"`
	LocationParts          []string         `json:"location_parts,omitempty"`
	Dimension              *string          `json:"dimension,omitempty"`
	PsychoBias             string           `json:"psycho_bias,omitempty"`
}

// InferOptionCount tries to determine the option count from saved weights/texts.
func InferOptionCount(entry *QuestionEntry) int {
	if entry == nil {
		return 0
	}

	if entry.QuestionType == "matrix" {
		if nested := nestedLength(entry.CustomWeights); nested > 0 {
			return nested
		}
		if nested := nestedLength(entry.Probabilities); nested > 0 {
			return nested
		}
	}

	if entry.OptionCount > 0 {
		return entry.OptionCount
	}
	if entry.CustomWeights != nil {
		if sl, ok := toFloat64Slice(entry.CustomWeights); ok && len(sl) > 0 {
			return len(sl)
		}
	}
	if entry.Probabilities != nil {
		if sl, ok := toFloat64Slice(entry.Probabilities); ok && len(sl) > 0 {
			return len(sl)
		}
	}
	if len(entry.Texts) > 0 {
		return len(entry.Texts)
	}
	if entry.QuestionType == "scale" || entry.QuestionType == "score" {
		return 5
	}
	return 0
}

// nestedLength checks for the max inner slice length in a nested structure.
func nestedLength(raw any) int {
	if raw == nil {
		return 0
	}
	if sl, ok := raw.([]any); ok {
		maxLen := 0
		for _, item := range sl {
			if inner, ok := item.([]any); ok && len(inner) > maxLen {
				maxLen = len(inner)
			}
		}
		return maxLen
	}
	return 0
}

// toFloat64Slice attempts to convert an interface{} to []float64.
func toFloat64Slice(v any) ([]float64, bool) {
	if v == nil {
		return nil, false
	}
	switch sl := v.(type) {
	case []float64:
		return sl, true
	case []any:
		result := make([]float64, 0, len(sl))
		for _, item := range sl {
			if f, ok := toFloat(item); ok {
				result = append(result, f)
			}
		}
		return result, true
	}
	return nil, false
}

// toFloat converts various numeric types to float64.
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}
