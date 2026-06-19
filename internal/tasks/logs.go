package tasks

import (
	"time"

	"github.com/SurveyController/SurveyCore/internal/engine"
	"github.com/SurveyController/SurveyCore/internal/logging"
)

func (m *TaskManager) logTask(id string, level logging.Level, message string, fields ...logging.Field) {
	m.consoleLog(level, message, id, fields...)
	entry := TaskLog{Timestamp: time.Now(), Level: logLevelName(level), Message: message, Fields: map[string]any{"task_id": id}}
	for _, field := range fields {
		entry.Fields[field.Key] = field.Value
	}
	m.appendLog(id, entry)
}

func (m *TaskManager) logTaskEvent(id string, level logging.Level, message string, event engine.StatusEvent) {
	m.consoleLog(level, message, id,
		logging.F("worker", event.ThreadName),
		logging.F("current", event.Current),
		logging.F("total", event.Total),
	)
	entry := TaskLog{
		Timestamp: time.Now(),
		Level:     logLevelName(level),
		Message:   message,
		Fields: map[string]any{
			"task_id": id,
			"worker":  event.ThreadName,
			"current": event.Current,
			"total":   event.Total,
		},
		Event: &event,
	}
	m.appendLog(id, entry)
}

func (m *TaskManager) appendLog(id string, entry TaskLog) {
	if err := m.store.AppendLog(id, entry); err != nil {
		logging.ErrorFields("写入任务日志失败", logging.F("task_id", id), logging.F("error", err))
	}
}

func (m *TaskManager) saveTask(task *TaskRecord) {
	if err := m.store.SaveTask(task); err != nil {
		logging.ErrorFields("保存任务状态失败", logging.F("task_id", task.ID), logging.F("error", err))
	}
}

func (m *TaskManager) consoleLog(level logging.Level, message, id string, fields ...logging.Field) {
	allFields := append([]logging.Field{logging.F("task_id", id)}, fields...)
	logging.Log(level, message, allFields...)
}

func logLevelName(level logging.Level) string {
	switch level {
	case logging.LevelDebug:
		return "DEBUG"
	case logging.LevelWarn:
		return "WARN"
	case logging.LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}
