package tasks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/SurveyController/SurveyCore/internal/engine"
	"github.com/SurveyController/SurveyCore/internal/execution"
	"github.com/SurveyController/SurveyCore/internal/logging"
	"github.com/SurveyController/SurveyCore/internal/models"
)

type TaskManager struct {
	store             *Store
	registry          engine.ProviderRegistry
	executionDefaults func(*execution.ExecutionConfig)
	mu                sync.RWMutex
	tasks             map[string]*TaskRecord
	runtimes          map[string]*taskRuntime
	wg                sync.WaitGroup
}

func NewTaskManager(store *Store, registry engine.ProviderRegistry) *TaskManager {
	return NewTaskManagerWithExecutionDefaults(store, registry, nil)
}

func NewTaskManagerWithExecutionDefaults(store *Store, registry engine.ProviderRegistry, executionDefaults func(*execution.ExecutionConfig)) *TaskManager {
	return &TaskManager{
		store:             store,
		registry:          registry,
		executionDefaults: executionDefaults,
		tasks:             make(map[string]*TaskRecord),
		runtimes:          make(map[string]*taskRuntime),
	}
}

func (m *TaskManager) Load() []error {
	tasks, errs := m.store.LoadTasks()
	for _, task := range tasks {
		if task.Status == TaskPending || task.Status == TaskRunning {
			now := time.Now()
			task.Status = TaskInterrupted
			task.FinishedAt = &now
			task.Error = "服务重启，任务已中断"
			errs = appendSaveErr(errs, m.store.SaveTask(task))
		}
		m.tasks[task.ID] = task
	}
	return errs
}

func appendSaveErr(errs []error, err error) []error {
	if err != nil {
		return append(errs, err)
	}
	return errs
}

func (m *TaskManager) Create(ctx context.Context, cfg *models.RuntimeConfig) (*TaskRecord, error) {
	if cfg == nil {
		return nil, errors.New("请求配置为空")
	}
	id, err := newTaskID()
	if err != nil {
		return nil, err
	}
	runtimeCfg := cloneRuntimeConfig(cfg)
	now := time.Now()
	task := &TaskRecord{
		ID:        id,
		Status:    TaskPending,
		Config:    runtimeCfg,
		CreatedAt: now,
	}
	if err := m.store.SaveTask(task); err != nil {
		return nil, err
	}
	m.appendLog(id, TaskLog{Timestamp: now, Level: "INFO", Message: "任务已创建", Fields: map[string]any{"task_id": id}})

	runCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.tasks[id] = task
	m.runtimes[id] = &taskRuntime{cancel: cancel}
	m.wg.Add(1)
	m.mu.Unlock()

	go func() {
		defer m.wg.Done()
		m.run(runCtx, id)
	}()
	return task, nil
}

func (m *TaskManager) List() []*TaskRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tasks := make([]*TaskRecord, 0, len(m.tasks))
	for _, task := range m.tasks {
		tasks = append(tasks, cloneTask(task))
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
	})
	return tasks
}

func (m *TaskManager) Get(id string) (*TaskRecord, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, ok := m.tasks[id]
	if !ok {
		return nil, false
	}
	return cloneTask(task), true
}

func (m *TaskManager) Stop(id string) (*TaskRecord, error) {
	m.mu.Lock()
	task, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return nil, errors.New("任务不存在")
	}
	runtime := m.runtimes[id]
	if runtime == nil || (task.Status != TaskPending && task.Status != TaskRunning) {
		snapshot := cloneTask(task)
		m.mu.Unlock()
		return snapshot, nil
	}
	task.Status = TaskStopped
	task.StopMessage = "用户请求停止"
	if task.State != nil {
		task.State.SignalStop()
	}
	runtime.cancel()
	snapshot := cloneTask(task)
	m.mu.Unlock()

	m.consoleLog(logging.LevelWarn, "任务停止请求", id, logging.F("status", TaskStopped))
	m.appendLog(id, TaskLog{Timestamp: time.Now(), Level: "WARN", Message: "任务停止请求", Fields: map[string]any{"task_id": id}})
	m.saveTask(snapshot)
	return snapshot, nil
}

func (m *TaskManager) StopAll() {
	m.mu.RLock()
	ids := make([]string, 0, len(m.runtimes))
	for id := range m.runtimes {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	for _, id := range ids {
		_, _ = m.Stop(id)
	}
	m.wg.Wait()
}

func (m *TaskManager) Close() error {
	return m.store.Close()
}

func (m *TaskManager) Logs(id string, afterID int64, limit int) (*TaskLogPage, error) {
	if _, ok := m.Get(id); !ok {
		return nil, errors.New("任务不存在")
	}
	return m.store.LoadLogs(id, afterID, limit)
}

func (m *TaskManager) ParseSurvey(ctx context.Context, surveyURL string) (*models.SurveyDefinition, error) {
	return engine.NewEngine(m.registry, nil).ParseSurvey(ctx, surveyURL)
}

func (m *TaskManager) updateTask(id string, mutate func(*TaskRecord)) {
	m.mu.Lock()
	task := m.tasks[id]
	if task == nil {
		m.mu.Unlock()
		return
	}
	mutate(task)
	snapshot := cloneTask(task)
	m.mu.Unlock()
	m.saveTask(snapshot)
}

func (m *TaskManager) getInternal(id string) (*TaskRecord, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, ok := m.tasks[id]
	if !ok {
		return nil, false
	}
	return cloneTask(task), true
}

func newTaskID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
