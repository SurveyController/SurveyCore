package tasks

import (
	"context"
	"time"

	runstate "github.com/SurveyController/SurveyCore/internal/runtime"

	"github.com/SurveyController/SurveyCore/internal/engine"
	"github.com/SurveyController/SurveyCore/internal/models"
)

const (
	TaskPending     = "pending"
	TaskRunning     = "running"
	TaskSucceeded   = "succeeded"
	TaskFailed      = "failed"
	TaskStopped     = "stopped"
	TaskInterrupted = "interrupted"
)

// TaskRecord is the persisted task state.
type TaskRecord struct {
	ID                   string                   `json:"id"`
	Status               string                   `json:"status"`
	Config               *models.RuntimeConfig    `json:"config"`
	State                *runstate.ExecutionState `json:"state,omitempty"`
	Progress             *TaskProgress            `json:"progress,omitempty"`
	CreatedAt            time.Time                `json:"created_at"`
	StartedAt            *time.Time               `json:"started_at,omitempty"`
	FinishedAt           *time.Time               `json:"finished_at,omitempty"`
	Error                string                   `json:"error,omitempty"`
	ErrorCode            string                   `json:"error_code,omitempty"`
	FailureReason        string                   `json:"failure_reason,omitempty"`
	TerminalStopCategory string                   `json:"terminal_stop_category,omitempty"`
	StopMessage          string                   `json:"stop_message,omitempty"`
}

// TaskProgress is a stable summary of task progress for API consumers.
type TaskProgress struct {
	Current int     `json:"current"`
	Target  int     `json:"target"`
	Success int     `json:"success"`
	Fail    int     `json:"fail"`
	Percent float64 `json:"percent"`
}

// TaskLog is one persisted task log entry.
type TaskLog struct {
	ID        int64               `json:"id"`
	Timestamp time.Time           `json:"timestamp"`
	Level     string              `json:"level"`
	Message   string              `json:"message"`
	Fields    map[string]any      `json:"fields,omitempty"`
	Event     *engine.StatusEvent `json:"event,omitempty"`
}

type TaskLogPage struct {
	Logs       []TaskLog `json:"logs"`
	NextCursor int64     `json:"next_cursor"`
	HasMore    bool      `json:"has_more"`
}

type taskRuntime struct {
	cancel context.CancelFunc
}

type createConfigRequest struct {
	URL string `json:"url"`
}

type parseSurveyRequest struct {
	URL string `json:"url"`
}
