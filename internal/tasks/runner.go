package tasks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/SurveyController/SurveyCore/internal/config"
	"github.com/SurveyController/SurveyCore/internal/engine"
	"github.com/SurveyController/SurveyCore/internal/execution"
	"github.com/SurveyController/SurveyCore/internal/logging"
	"github.com/SurveyController/SurveyCore/internal/models"
	runstate "github.com/SurveyController/SurveyCore/internal/runtime"
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
	config.MergeDefaults(cfg)
	if cfg.URL == "" {
		return errors.New("必须提供问卷链接")
	}

	e := engine.NewEngine(m.registry, nil)
	m.logTask(taskID, logging.LevelInfo, "解析问卷", logging.F("url", cfg.URL))
	def, err := e.ParseSurvey(ctx, cfg.URL)
	if err != nil {
		return fmt.Errorf("解析问卷失败: %w", err)
	}

	cfg.SurveyTitle = def.Title
	cfg.SurveyProvider = def.Provider
	m.logTask(taskID, logging.LevelInfo, "解析成功", logging.F("title", def.Title), logging.F("questions", len(def.Questions)))

	execCfg, err := config.BuildExecutionConfigWithError(cfg, def.Questions)
	if err != nil {
		return fmt.Errorf("准备执行配置失败: %w", err)
	}
	m.applyExecutionDefaults(execCfg)
	state.Config = execCfg
	m.updateTask(taskID, func(t *TaskRecord) {
		t.Config = cloneRuntimeConfig(cfg)
		t.State = state
	})

	handler := func(event engine.StatusEvent) {
		level := logging.LevelInfo
		message := event.StatusText
		if event.Fail {
			level = logging.LevelWarn
		}
		m.logTaskEvent(taskID, level, message, event)
		m.updateTask(taskID, func(t *TaskRecord) {
			t.State = state
		})
	}
	runner := engine.NewEngine(m.registry, handler)
	return runner.Run(ctx, execCfg, state)
}

func (m *TaskManager) applyExecutionDefaults(cfg *execution.ExecutionConfig) {
	if m.executionDefaults != nil {
		m.executionDefaults(cfg)
	}
}
