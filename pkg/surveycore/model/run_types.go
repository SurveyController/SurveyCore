package model

import "time"

// ExecutionPlan controls target count, concurrency, timing, and stop behavior.
type ExecutionPlan struct {
	Target               int       `json:"target"`
	Threads              int       `json:"threads"`
	SubmitInterval       [2]int    `json:"submitInterval"`
	AnswerDuration       [2]int    `json:"answerDuration"`
	AnswerDatetimeWindow [2]string `json:"answerDatetimeWindow,omitempty"`
	FailStop             bool      `json:"failStop"`
	PauseOnAliyunCaptcha bool      `json:"pauseOnAliyunCaptcha"`
}

// ReverseFillPlan controls experimental Excel-backed answer generation.
type ReverseFillPlan struct {
	Enabled    bool   `json:"enabled"`
	SourcePath string `json:"sourcePath,omitempty"`
	Format     string `json:"format"`
	StartRow   int    `json:"startRow"`
	Threads    int    `json:"threads"`
}

// PsychometricPolicy controls psychometric answer shaping.
type PsychometricPolicy struct {
	Enabled     bool    `json:"enabled"`
	TargetAlpha float64 `json:"targetAlpha"`
}

// UserAgentSettings controls randomized runtime user-agent selection.
type UserAgentSettings struct {
	Enabled bool
	Ratios  map[string]int
}

// RunRequest is the complete configuration accepted by Run and the V1 task API.
type RunRequest struct {
	SurveySource       `json:"source"`
	SurveyDefinition   `json:"definition"`
	ExecutionPlan      `json:"execution"`
	AnswerPlan         `json:"answers"`
	ReverseFillPlan    `json:"reverseFill"`
	PsychometricPolicy `json:"psychometrics"`
}

// AnswerAction is a prepared provider-neutral answer for one question.
type AnswerAction struct {
	QuestionNum     int            `json:"questionNum"`
	QuestionID      string         `json:"questionId,omitempty"`
	Kind            QuestionKind   `json:"kind"`
	SelectedIndices []int          `json:"selectedIndices,omitempty"`
	MatrixIndices   []int          `json:"matrixIndices,omitempty"`
	TextValues      []string       `json:"textValues,omitempty"`
	SliderValue     string         `json:"sliderValue,omitempty"`
	OptionFillTexts map[int]string `json:"optionFillTexts,omitempty"`
}

// SubmissionContext carries per-submission runtime state to a provider runner.
type SubmissionContext struct {
	ProxyAddress string
	UserAgent    string
	Runtime      AnswerRuntime
	RuntimeOwner string
	Persona      *Persona
	AIProfile    AIProfile
	Actions      []AnswerAction
}

// SubmissionRequest is the experimental low-level provider submission input.
type SubmissionRequest struct {
	Source               SurveySource
	Definition           SurveyDefinition
	AnswerDuration       [2]int
	AnswerDatetimeWindow [2]string
	Context              SubmissionContext
}

// AIProfile configures text answer generation without serializing the API key.
type AIProfile struct {
	Mode         string `json:"mode"`
	Provider     string `json:"provider"`
	BaseURL      string `json:"baseURL,omitempty"`
	APIProtocol  string `json:"apiProtocol"`
	Model        string `json:"model,omitempty"`
	SystemPrompt string `json:"systemPrompt,omitempty"`
	APIKey       string `json:"-"`
}

// RunResult reports final submission counts and worker progress.
type RunResult struct {
	Success        int              `json:"success"`
	Fail           int              `json:"fail"`
	Stopped        bool             `json:"stopped"`
	ThreadProgress []ThreadProgress `json:"thread_progress,omitempty"`
}

// ThreadProgress reports the latest state of one execution worker.
type ThreadProgress struct {
	ThreadName   string    `json:"thread_name"`
	ThreadIndex  int       `json:"thread_index"`
	SuccessCount int       `json:"success_count"`
	FailCount    int       `json:"fail_count"`
	StepCurrent  int       `json:"step_current"`
	StepTotal    int       `json:"step_total"`
	StatusText   string    `json:"status_text"`
	Running      bool      `json:"running"`
	LastUpdate   time.Time `json:"last_update,omitempty"`
}

// Event reports one execution progress update.
type Event struct {
	Worker  string    `json:"worker"`
	Message string    `json:"message"`
	Success bool      `json:"success"`
	Fail    bool      `json:"fail"`
	Current int       `json:"current"`
	Total   int       `json:"total"`
	Time    time.Time `json:"time"`
}
