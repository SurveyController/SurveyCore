package service

import (
	"context"
	"errors"
	"time"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

const (
	TaskPending     = "pending"
	TaskRunning     = "running"
	TaskSucceeded   = "succeeded"
	TaskFailed      = "failed"
	TaskStopped     = "stopped"
	TaskInterrupted = "interrupted"
)

var ErrTaskNotFound = errors.New("task not found")

type Task struct {
	ID          string            `json:"id"`
	Status      string            `json:"status"`
	Config      *model.RunRequest `json:"config"`
	Result      *model.RunResult  `json:"result,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	FinishedAt  *time.Time        `json:"finished_at,omitempty"`
	Error       string            `json:"error,omitempty"`
	StopMessage string            `json:"stop_message,omitempty"`
}

type TaskLog struct {
	ID        int64          `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields,omitempty"`
	Event     *model.Event   `json:"event,omitempty"`
}

type TaskLogPage struct {
	Logs       []TaskLog `json:"logs"`
	NextCursor int64     `json:"next_cursor"`
	HasMore    bool      `json:"has_more"`
}

type taskRuntime struct {
	cancel context.CancelFunc
	result *model.RunResult
}
