package model

const (
	ProviderWJX     = "wjx"
	ProviderQQ      = "qq"
	ProviderCredamo = "credamo"

	LogicParseStatusComplete = "complete"
	LogicParseStatusNone     = "none"
	LogicParseStatusUnknown  = "unknown"
)

// QuestionKind identifies the normalized answer shape of a survey question.
type QuestionKind string

const (
	QuestionKindSingle    QuestionKind = "single"
	QuestionKindMultiple  QuestionKind = "multiple"
	QuestionKindDropdown  QuestionKind = "dropdown"
	QuestionKindScale     QuestionKind = "scale"
	QuestionKindScore     QuestionKind = "score"
	QuestionKindMatrix    QuestionKind = "matrix"
	QuestionKindOrder     QuestionKind = "order"
	QuestionKindSlider    QuestionKind = "slider"
	QuestionKindText      QuestionKind = "text"
	QuestionKindMultiText QuestionKind = "multi_text"
)

// SurveySource identifies the provider and canonical survey URL used by a run.
type SurveySource struct {
	URL      string `json:"url"`
	Provider string `json:"provider,omitempty"`
}

// SurveyDefinition is the provider-neutral survey snapshot returned by Parse.
// SurveyURL is the normalized parse URL, named to preserve RunRequest.URL access.
type SurveyDefinition struct {
	Provider  string         `json:"provider"`
	Title     string         `json:"title"`
	SurveyURL string         `json:"url,omitempty"`
	Questions []QuestionMeta `json:"questions"`
}
