package domain

import (
	"encoding/json"
	"strings"
)

const (
	ProviderWJX     = "wjx"
	ProviderQQ      = "qq"
	ProviderCredamo = "credamo"
)

const (
	LogicParseStatusComplete = "complete"
	LogicParseStatusNone     = "none"
	LogicParseStatusUnknown  = "unknown"
)

type FillOptions struct {
	ThreadName   string
	ProxyAddress string
	UserAgent    string
	StopChan     <-chan struct{}
}

type SurveyQuestionMeta struct {
	Num                      int              `json:"num"`
	Title                    string           `json:"title"`
	DisplayNum               *int             `json:"display_num,omitempty"`
	Description              string           `json:"description,omitempty"`
	TypeCode                 string           `json:"type_code"`
	Options                  int              `json:"options"`
	Rows                     int              `json:"rows,omitempty"`
	RowTexts                 []string         `json:"row_texts,omitempty"`
	Page                     int              `json:"page,omitempty"`
	OptionTexts              []string         `json:"option_texts,omitempty"`
	ForcedOptionIndex        *int             `json:"forced_option_index,omitempty"`
	ForcedOptionText         string           `json:"forced_option_text,omitempty"`
	ForcedTexts              []string         `json:"forced_texts,omitempty"`
	FillableOptions          []int            `json:"fillable_options,omitempty"`
	AttachedOptionSelects    []map[string]any `json:"attached_option_selects,omitempty"`
	HasAttachedOptionSelect  bool             `json:"has_attached_option_select,omitempty"`
	IsLocation               bool             `json:"is_location,omitempty"`
	IsRating                 bool             `json:"is_rating,omitempty"`
	IsDescription            bool             `json:"is_description,omitempty"`
	RatingMax                int              `json:"rating_max,omitempty"`
	TextInputCount           int              `json:"text_inputs,omitempty"`
	TextInputLabels          []string         `json:"text_input_labels,omitempty"`
	IsMultiText              bool             `json:"is_multi_text,omitempty"`
	IsTextLike               bool             `json:"is_text_like,omitempty"`
	IsSliderMatrix           bool             `json:"is_slider_matrix,omitempty"`
	HasJump                  bool             `json:"has_jump,omitempty"`
	JumpRules                []map[string]any `json:"jump_rules,omitempty"`
	HasDisplayCondition      bool             `json:"has_display_condition,omitempty"`
	DisplayConditions        []map[string]any `json:"display_conditions,omitempty"`
	HasDependentDisplayLogic bool             `json:"has_dependent_display_logic,omitempty"`
	ControlsDisplayTargets   []map[string]any `json:"controls_display_targets,omitempty"`
	LogicParseStatus         string           `json:"logic_parse_status,omitempty"`
	QuestionMedia            []map[string]any `json:"question_media,omitempty"`
	SliderMin                *float64         `json:"slider_min,omitempty"`
	SliderMax                *float64         `json:"slider_max,omitempty"`
	SliderStep               *float64         `json:"slider_step,omitempty"`
	MultiMinLimit            *int             `json:"multi_min_limit,omitempty"`
	MultiMaxLimit            *int             `json:"multi_max_limit,omitempty"`
	Provider                 string           `json:"provider"`
	ProviderQuestionID       string           `json:"provider_question_id,omitempty"`
	ProviderPageID           string           `json:"provider_page_id,omitempty"`
	ProviderType             string           `json:"provider_type,omitempty"`
	ProviderPageRaw          any              `json:"provider_page_raw,omitempty"`
	Unsupported              bool             `json:"unsupported,omitempty"`
	UnsupportedReason        string           `json:"unsupported_reason,omitempty"`
	Required                 bool             `json:"required,omitempty"`
}

func (q *SurveyQuestionMeta) Get(key string) any {
	switch key {
	case "num":
		return q.Num
	case "title":
		return q.Title
	case "type_code":
		return q.TypeCode
	case "options":
		return q.Options
	case "rows":
		return q.Rows
	case "provider":
		return q.Provider
	default:
		return nil
	}
}

func (q *SurveyQuestionMeta) ToDict() map[string]any {
	b, _ := json.Marshal(q)
	var m map[string]any
	json.Unmarshal(b, &m)
	return m
}

type SurveyDefinition struct {
	Provider  string               `json:"provider"`
	Title     string               `json:"title"`
	Questions []SurveyQuestionMeta `json:"questions"`
}

func CloneSurveyQuestionMetas(src []SurveyQuestionMeta) []SurveyQuestionMeta {
	if src == nil {
		return nil
	}
	dst := make([]SurveyQuestionMeta, len(src))
	copy(dst, src)
	return dst
}

func MakeProviderQuestionKey(provider, pageID, questionID string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	pageID = strings.TrimSpace(pageID)
	questionID = strings.TrimSpace(questionID)
	if provider == "" || pageID == "" || questionID == "" {
		return ""
	}
	return provider + ":" + pageID + ":" + questionID
}
