package surveycore

import (
	"time"

	"github.com/SurveyController/SurveyCore/internal/models"
)

type RuntimeConfig = models.RuntimeConfig
type SurveyDefinition = models.SurveyDefinition
type SurveyQuestionMeta = models.SurveyQuestionMeta
type QuestionEntry = models.QuestionEntry

const (
	ProviderWJX     = models.ProviderWJX
	ProviderQQ      = models.ProviderQQ
	ProviderCredamo = models.ProviderCredamo
)

type RunResult struct {
	Success               int              `json:"success"`
	Fail                  int              `json:"fail"`
	Stopped               bool             `json:"stopped"`
	TerminalStopCategory  string           `json:"terminal_stop_category,omitempty"`
	TerminalFailureReason string           `json:"terminal_failure_reason,omitempty"`
	TerminalStopMessage   string           `json:"terminal_stop_message,omitempty"`
	ThreadProgress        []ThreadProgress `json:"thread_progress,omitempty"`
}

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
