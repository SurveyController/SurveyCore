package tasks

import (
	"context"
	"time"

	"github.com/SurveyController/SurveyCore/internal/engine"
	"github.com/SurveyController/SurveyCore/internal/execution"
	"github.com/SurveyController/SurveyCore/internal/logging"
	"github.com/SurveyController/SurveyCore/internal/models"
	runstate "github.com/SurveyController/SurveyCore/internal/runtime"
	"github.com/SurveyController/SurveyCore/internal/surveyrun"
)

func (m *TaskManager) run(ctx context.Context, id string) {
	task, ok := m.getInternal(id)
	if !ok {
		return
	}
	start := time.Now()
	m.updateTask(id, func(t *TaskRecord) {
		t.Status = TaskRunning
		t.StartedAt = &start
	})
	m.logTask(id, logging.LevelInfo, "开始执行", logging.F("target", task.Config.Target), logging.F("threads", task.Config.Threads))

	state := runstate.NewExecutionState()
	err := m.execute(ctx, task.Config, state, id)

	finished := time.Now()
	m.mu.Lock()
	current := m.tasks[id]
	if current == nil {
		m.mu.Unlock()
		return
	}
	current.State = state
	current.FinishedAt = &finished
	delete(m.runtimes, id)
	if current.Status == TaskStopped || ctx.Err() != nil {
		current.Status = TaskStopped
		if current.StopMessage == "" {
			current.StopMessage = "任务已停止"
		}
	} else if err != nil {
		current.Status = TaskFailed
		current.Error = err.Error()
	} else {
		current.Status = TaskSucceeded
	}
	snapshot := cloneTask(current)
	m.mu.Unlock()

	if err != nil {
		m.logTask(id, logging.LevelError, "执行失败", logging.F("error", err))
	} else {
		m.logTask(id, logging.LevelInfo, "执行完成", logging.F("success", state.GetCurNum()), logging.F("fail", state.GetCurFail()))
	}
	m.saveTask(snapshot)
}

func (m *TaskManager) execute(ctx context.Context, cfg *models.RuntimeConfig, state *runstate.ExecutionState, taskID string) error {
	m.logTask(taskID, logging.LevelInfo, "解析问卷", logging.F("url", cfg.URL))
	hooks := surveyrun.Hooks{
		OnParsed: func(def *models.SurveyDefinition) {
			m.logTask(taskID, logging.LevelInfo, "解析成功", logging.F("title", def.Title), logging.F("questions", len(def.Questions)))
		},
		OnPrepared: func() {
			m.updateTask(taskID, func(t *TaskRecord) {
				t.Config = cloneRuntimeConfig(cfg)
				t.State = state
			})
		},
		OnEvent: func(event engine.StatusEvent) {
			level := logging.LevelInfo
			message := event.StatusText
			if event.Fail {
				level = logging.LevelWarn
			}
			m.logTaskEvent(taskID, level, message, event)
			m.updateTask(taskID, func(t *TaskRecord) {
				t.State = state
			})
		},
	}
	_, err := surveyrun.New(m.registry, m.applyExecutionDefaults).RunWithHooks(ctx, cfg, state, hooks)
	if err != nil {
		return err
	}
	return nil
}

func (m *TaskManager) applyExecutionDefaults(cfg *execution.ExecutionConfig) {
	if m.executionDefaults != nil {
		m.executionDefaults(cfg)
	}
}
