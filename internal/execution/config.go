package execution

import (
	"github.com/SurveyController/SurveyCore/internal/domain"
)

// ExecutionConfig is the static, thread-safe snapshot used at runtime.
type ExecutionConfig struct {
	URL            string `json:"url"`
	SurveyTitle    string `json:"survey_title"`
	SurveyProvider string `json:"survey_provider"`

	SingleProb    []any       `json:"single_prob,omitempty"`
	DroplistProb  []any       `json:"droplist_prob,omitempty"`
	MultipleProb  [][]float64 `json:"multiple_prob,omitempty"`
	MatrixProb    []any       `json:"matrix_prob,omitempty"`
	ScaleProb     []any       `json:"scale_prob,omitempty"`
	SliderTargets []float64   `json:"slider_targets,omitempty"`

	Texts               [][]string       `json:"texts,omitempty"`
	TextsProb           [][]float64      `json:"texts_prob,omitempty"`
	TextEntryTypes      []string         `json:"text_entry_types,omitempty"`
	TextRandomModes     []string         `json:"text_random_modes,omitempty"`
	TextRandomIntRanges [][]int          `json:"text_random_int_ranges,omitempty"`
	TextAIFlags         []bool           `json:"text_ai_flags,omitempty"`
	TextTitles          []string         `json:"text_titles,omitempty"`
	LocationParts       map[int][]string `json:"location_parts,omitempty"`
	DistributionModes   []string         `json:"distribution_modes,omitempty"`

	MultiTextBlankModes     [][]string `json:"multi_text_blank_modes,omitempty"`
	MultiTextBlankAIFlags   [][]bool   `json:"multi_text_blank_ai_flags,omitempty"`
	MultiTextBlankIntRanges [][][]int  `json:"multi_text_blank_int_ranges,omitempty"`

	SingleOptionFillTexts       [][]*string        `json:"single_option_fill_texts,omitempty"`
	SingleAttachedOptionSelects [][]map[string]any `json:"single_attached_option_selects,omitempty"`
	DroplistOptionFillTexts     [][]*string        `json:"droplist_option_fill_texts,omitempty"`
	MultipleOptionFillTexts     [][]*string        `json:"multiple_option_fill_texts,omitempty"`

	AnswerRules     []map[string]any        `json:"answer_rules,omitempty"`
	ReverseFillSpec *domain.ReverseFillSpec `json:"reverse_fill_spec,omitempty"`

	QuestionConfigIndexMap         map[int]string                       `json:"question_config_index_map,omitempty"`
	ProviderQuestionConfigIndexMap map[string]string                    `json:"provider_question_config_index_map,omitempty"`
	QuestionDimensionMap           map[int]*string                      `json:"question_dimension_map,omitempty"`
	QuestionOrdinalScoreMap        map[int][]int                        `json:"question_ordinal_score_map,omitempty"`
	QuestionStrictRatioMap         map[int]bool                         `json:"question_strict_ratio_map,omitempty"`
	QuestionPsychoBiasMap          map[int]string                       `json:"question_psycho_bias_map,omitempty"`
	QuestionsMetadata              map[int]domain.SurveyQuestionMeta    `json:"questions_metadata,omitempty"`
	ProviderQuestionMetadataMap    map[string]domain.SurveyQuestionMeta `json:"provider_question_metadata_map,omitempty"`
	JointPsychometricAnswerPlan    any                                  `json:"joint_psychometric_answer_plan,omitempty"`

	PsychoTargetAlpha float64 `json:"psycho_target_alpha"`
	AIAPIKey          string  `json:"-"`
	AIBaseURL         string  `json:"-"`
	AIModel           string  `json:"-"`

	NumThreads    int `json:"num_threads"`
	TargetNum     int `json:"target_num"`
	FailThreshold int `json:"fail_threshold"`

	SubmitIntervalRangeSeconds [2]int   `json:"submit_interval_range_seconds"`
	AnswerDurationRangeSeconds [2]int   `json:"answer_duration_range_seconds"`
	AnswerDatetimeWindowMS     [2]int64 `json:"answer_datetime_window_ms,omitempty"`

	RandomUserAgentEnabled bool           `json:"random_user_agent_enabled"`
	RandomUserAgentKeys    []string       `json:"random_user_agent_keys,omitempty"`
	UserAgentRatios        map[string]int `json:"user_agent_ratios"`
}
