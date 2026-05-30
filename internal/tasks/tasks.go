package tasks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/SurveyController/SurveyConsole/internal/config"
	"github.com/SurveyController/SurveyConsole/internal/engine"
	"github.com/SurveyController/SurveyConsole/internal/logging"
	"github.com/SurveyController/SurveyConsole/internal/models"
	"github.com/SurveyController/SurveyConsole/internal/network/proxy"
	"github.com/SurveyController/SurveyConsole/internal/providers"
)

type TaskManager struct {
	store    *Store
	registry engine.ProviderRegistry
	mu       sync.RWMutex
	tasks    map[string]*TaskRecord
	runtimes map[string]*taskRuntime
	wg       sync.WaitGroup
}

func NewTaskManager(store *Store, registry engine.ProviderRegistry) *TaskManager {
	return &TaskManager{
		store:    store,
		registry: registry,
		tasks:    make(map[string]*TaskRecord),
		runtimes: make(map[string]*taskRuntime),
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
	now := time.Now()
	task := &TaskRecord{
		ID:        id,
		Status:    TaskPending,
		Config:    cloneRuntimeConfig(cfg),
		CreatedAt: now,
	}
	if err := m.store.SaveTask(task); err != nil {
		return nil, err
	}
	_ = m.store.AppendLog(id, TaskLog{Timestamp: now, Level: "INFO", Message: "任务已创建", Fields: map[string]any{"task_id": id}})

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
	_ = m.store.AppendLog(id, TaskLog{Timestamp: time.Now(), Level: "WARN", Message: "任务停止请求", Fields: map[string]any{"task_id": id}})
	_ = m.store.SaveTask(snapshot)
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

func (m *TaskManager) Logs(id string) ([]TaskLog, error) {
	if _, ok := m.Get(id); !ok {
		return nil, errors.New("任务不存在")
	}
	return m.store.LoadLogs(id)
}

func (m *TaskManager) ParseSurvey(ctx context.Context, surveyURL string) (*models.SurveyDefinition, error) {
	return engine.NewEngine(m.registry, nil, nil).ParseSurvey(ctx, surveyURL)
}

func (m *TaskManager) BuildDefaultConfig(ctx context.Context, surveyURL string) (*models.RuntimeConfig, error) {
	cfg := models.NewDefaultRuntimeConfig()
	cfg.URL = surveyURL
	if cfg.URL == "" {
		return &cfg, nil
	}
	def, err := m.ParseSurvey(ctx, cfg.URL)
	if err != nil {
		return nil, err
	}
	cfg.SurveyTitle = def.Title
	cfg.SurveyProvider = def.Provider
	cfg.QuestionsInfo = models.CloneSurveyQuestionMetas(def.Questions)
	cfg.QuestionEntries = config.BuildDefaultQuestionEntries(def.Questions, nil)
	return &cfg, nil
}

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

	state := models.NewExecutionState()
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
	_ = m.store.SaveTask(snapshot)
}

func (m *TaskManager) execute(ctx context.Context, cfg *models.RuntimeConfig, state *models.ExecutionState, taskID string) error {
	config.MergeDefaults(cfg)
	if cfg.URL == "" {
		return errors.New("必须提供问卷链接")
	}

	e := engine.NewEngine(m.registry, nil, nil)
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
	state.Config = execCfg
	m.updateTask(taskID, func(t *TaskRecord) {
		t.Config = cloneRuntimeConfig(cfg)
		t.State = state
	})

	var pool *proxy.Pool
	if cfg.RandomIPEnabled {
		pool = NewProxyPoolFromRuntimeConfig(cfg)
		m.logTask(taskID, logging.LevelInfo, "随机 IP 已启用", logging.F("proxy_source", cfg.ProxySource))
	}

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
	runner := engine.NewEngine(m.registry, pool, handler)
	if err := runner.Run(ctx, execCfg, state); err != nil {
		return err
	}
	return nil
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
	_ = m.store.SaveTask(snapshot)
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

func (m *TaskManager) logTask(id string, level logging.Level, message string, fields ...logging.Field) {
	m.consoleLog(level, message, id, fields...)
	entry := TaskLog{Timestamp: time.Now(), Level: logLevelName(level), Message: message, Fields: map[string]any{"task_id": id}}
	for _, field := range fields {
		entry.Fields[field.Key] = field.Value
	}
	_ = m.store.AppendLog(id, entry)
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
	_ = m.store.AppendLog(id, entry)
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

func newTaskID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func cloneRuntimeConfig(cfg *models.RuntimeConfig) *models.RuntimeConfig {
	if cfg == nil {
		return nil
	}
	data, err := models.SerializeRuntimeConfig(cfg)
	if err != nil {
		copy := *cfg
		return &copy
	}
	cloned, err := models.DeserializeRuntimeConfig(data)
	if err != nil {
		copy := *cfg
		return &copy
	}
	return cloned
}

func cloneTask(task *TaskRecord) *TaskRecord {
	if task == nil {
		return nil
	}
	copy := *task
	copy.Config = cloneRuntimeConfig(task.Config)
	copy.State = task.State.Snapshot()
	return &copy
}

// NewProxyPoolFromRuntimeConfig builds a proxy pool from user config.
func NewProxyPoolFromRuntimeConfig(cfg *models.RuntimeConfig) *proxy.Pool {
	areaCode := ""
	if cfg.ProxyAreaCode != nil {
		areaCode = *cfg.ProxyAreaCode
	}
	return proxy.NewPool(
		cfg.ProxySource,
		cfg.CustomProxyAPI,
		proxy.WithOfficialAreaCode(areaCode),
		proxy.WithOfficialCredentials(cfg.RandomIPUserID, cfg.RandomIPDeviceID),
		proxy.WithOfficialEndpoint(cfg.IPExtractEndpoint),
		proxy.WithOfficialMinute(cfg.RandomIPLeaseMinute),
	)
}

func DefaultTaskManager() (*TaskManager, error) {
	store := NewStore("data/tasks")
	if err := store.Init(); err != nil {
		return nil, err
	}
	manager := NewTaskManager(store, providers.Default())
	return manager, nil
}
