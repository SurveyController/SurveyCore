package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/SurveyController/SurveyCore/pkg/surveycore"
)

type Manager struct {
	store    *Store
	client   *surveycore.Client
	mu       sync.RWMutex
	tasks    map[string]*Task
	runtimes map[string]*taskRuntime
	wg       sync.WaitGroup
}

func NewManager(store *Store, client *surveycore.Client) *Manager {
	if client == nil {
		client = surveycore.New()
	}
	return &Manager{store: store, client: client, tasks: map[string]*Task{}, runtimes: map[string]*taskRuntime{}}
}

func (m *Manager) Load() error {
	loaded, errs := m.store.LoadTasks()
	for _, task := range loaded {
		if task.Status == TaskPending || task.Status == TaskRunning {
			now := time.Now()
			task.Status = TaskInterrupted
			task.FinishedAt = &now
			task.Error = "服务重启，任务已中断"
			if err := m.store.SaveTask(task); err != nil {
				errs = append(errs, err)
			}
		}
		m.tasks[task.ID] = task
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (m *Manager) Create(ctx context.Context, cfg *surveycore.RunRequest) (*Task, error) {
	if cfg == nil {
		return nil, errors.New("配置不能为空")
	}
	id, err := taskID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	task := &Task{ID: id, Status: TaskPending, Config: cloneConfig(cfg), CreatedAt: now}
	if err := m.store.SaveTask(task); err != nil {
		return nil, err
	}
	m.appendLog(id, TaskLog{Timestamp: now, Level: "INFO", Message: "任务已创建"})
	runCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.tasks[id] = task
	m.runtimes[id] = &taskRuntime{cancel: cancel}
	m.wg.Add(1)
	m.mu.Unlock()
	go func() { defer m.wg.Done(); m.run(runCtx, id) }()
	return cloneTask(task), nil
}

func (m *Manager) run(ctx context.Context, id string) {
	task, ok := m.Get(id)
	if !ok {
		return
	}
	started := time.Now()
	m.update(id, func(t *Task) { t.Status = TaskRunning; t.StartedAt = &started })
	result, err := m.client.RunWithEvents(ctx, task.Config, func(event surveycore.Event) {
		m.appendLog(id, TaskLog{Timestamp: time.Now(), Level: "INFO", Message: event.Message, Event: &event})
	})
	finished := time.Now()
	m.mu.Lock()
	current := m.tasks[id]
	if current == nil {
		m.mu.Unlock()
		return
	}
	current.Result = result
	current.FinishedAt = &finished
	delete(m.runtimes, id)
	if current.Status == TaskStopped || errors.Is(ctx.Err(), context.Canceled) {
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
	_ = m.store.SaveTask(snapshot)
}

func (m *Manager) List() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		out = append(out, cloneTask(t))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}
func (m *Manager) Get(id string) (*Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	if !ok {
		return nil, false
	}
	return cloneTask(t), true
}
func (m *Manager) Stop(id string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return nil, ErrTaskNotFound
	}
	rt := m.runtimes[id]
	if rt == nil || (t.Status != TaskPending && t.Status != TaskRunning) {
		return cloneTask(t), nil
	}
	t.Status = TaskStopped
	t.StopMessage = "用户请求停止"
	rt.cancel()
	snap := cloneTask(t)
	go m.store.SaveTask(snap)
	return snap, nil
}
func (m *Manager) Logs(id string, after int64, limit int) (*TaskLogPage, error) {
	if _, ok := m.Get(id); !ok {
		return nil, ErrTaskNotFound
	}
	return m.store.LoadLogs(id, after, limit)
}
func (m *Manager) Parse(ctx context.Context, url string) (*surveycore.SurveyDefinition, error) {
	return m.client.Parse(ctx, url)
}
func (m *Manager) DefaultConfig(ctx context.Context, url string) (*surveycore.RunRequest, error) {
	return m.client.DefaultConfig(ctx, url)
}
func (m *Manager) StopAll() {
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
func (m *Manager) Close() error { return m.store.Close() }
func (m *Manager) update(id string, fn func(*Task)) {
	m.mu.Lock()
	t := m.tasks[id]
	if t == nil {
		m.mu.Unlock()
		return
	}
	fn(t)
	snap := cloneTask(t)
	m.mu.Unlock()
	_ = m.store.SaveTask(snap)
}
func (m *Manager) appendLog(id string, log TaskLog) { _ = m.store.AppendLog(id, log) }
func taskID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func cloneConfig(c *surveycore.RunRequest) *surveycore.RunRequest {
	if c == nil {
		return nil
	}
	b, e := json.Marshal(c)
	if e != nil {
		return c
	}
	var out surveycore.RunRequest
	if json.Unmarshal(b, &out) != nil {
		return c
	}
	return &out
}
func cloneTask(t *Task) *Task {
	if t == nil {
		return nil
	}
	out := *t
	out.Config = cloneConfig(t.Config)
	if t.Result != nil {
		b, _ := json.Marshal(t.Result)
		var r surveycore.RunResult
		if json.Unmarshal(b, &r) == nil {
			out.Result = &r
		}
	}
	return &out
}
