package tasks

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SurveyController/SurveyCore/internal/execution"
	runstate "github.com/SurveyController/SurveyCore/internal/runtime"

	"github.com/SurveyController/SurveyCore/internal/models"
)

func TestStorePersistsTaskAndLogs(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "surveycore.db"))
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	task := &TaskRecord{ID: "task-1", Status: TaskRunning, Config: &models.RuntimeConfig{URL: "https://example.com"}}
	if err := store.SaveTask(task); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendLog("task-1", TaskLog{Level: "INFO", Message: "hello"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "surveycore.db")); err != nil {
		t.Fatalf("sqlite database missing: %v", err)
	}
	tasks, errs := store.LoadTasks()
	if len(errs) != 0 || len(tasks) != 1 || tasks[0].ID != "task-1" {
		t.Fatalf("tasks = %#v errs = %#v, want persisted task", tasks, errs)
	}
	page, err := store.LoadLogs("task-1", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Logs) != 1 || page.Logs[0].Message != "hello" || page.Logs[0].ID == 0 {
		t.Fatalf("logs = %#v, want persisted log with cursor", page.Logs)
	}
}

func TestTaskManagerLoadMarksRunningInterruptedAndSkipsBadRecord(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "surveycore.db"))
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.SaveTask(&TaskRecord{ID: "running", Status: TaskRunning, Config: &models.RuntimeConfig{URL: "x"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.database().Exec(`
		INSERT INTO tasks(id, status, record_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, "bad", TaskRunning, "{bad", time.Now().Format(time.RFC3339Nano), time.Now().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}

	manager := NewTaskManager(store, nil)
	errs := manager.Load()
	if len(errs) == 0 {
		t.Fatal("expected bad json error")
	}
	task, ok := manager.Get("running")
	if !ok {
		t.Fatal("running task not loaded")
	}
	if task.Status != TaskInterrupted {
		t.Fatalf("status = %q, want interrupted", task.Status)
	}
}

func TestTaskManagerAppliesExecutionDefaults(t *testing.T) {
	manager := NewTaskManagerWithExecutionDefaults(nil, nil, func(cfg *execution.ExecutionConfig) {
		cfg.AIBaseURL = "https://ai.example.test/v1"
		cfg.AIModel = "test-model"
		cfg.AIAPIKey = "test-key"
	})
	cfg := &execution.ExecutionConfig{}

	manager.applyExecutionDefaults(cfg)

	if cfg.AIBaseURL != "https://ai.example.test/v1" || cfg.AIModel != "test-model" || cfg.AIAPIKey != "test-key" {
		t.Fatalf("execution defaults = %#v, want AI defaults", cfg)
	}
}

func TestStoreLoadLogsUsesCursorPagination(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "surveycore.db"))
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.SaveTask(&TaskRecord{ID: "task-1", Status: TaskRunning}); err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{"one", "two", "three"} {
		if err := store.AppendLog("task-1", TaskLog{Timestamp: time.Now(), Level: "INFO", Message: message}); err != nil {
			t.Fatal(err)
		}
	}

	first, err := store.LoadLogs("task-1", 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Logs) != 2 || !first.HasMore || first.NextCursor != first.Logs[1].ID {
		t.Fatalf("first page = %#v, want two logs and next page", first)
	}
	second, err := store.LoadLogs("task-1", first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Logs) != 1 || second.HasMore || second.Logs[0].Message != "three" {
		t.Fatalf("second page = %#v, want final log", second)
	}
}

func TestCloneTaskSnapshotsExecutionState(t *testing.T) {
	running := true
	state := runstate.NewExecutionState()
	state.UpdateThreadStatus("Worker-1", "运行中", &running)
	state.IncrementSuccess()

	task := &TaskRecord{
		ID:     "task-1",
		Status: TaskRunning,
		State:  state,
	}
	cloned := cloneTask(task)

	state.UpdateThreadStatus("Worker-1", "已变化", &running)
	state.IncrementSuccess()

	if cloned.State == state {
		t.Fatal("cloned task keeps the original execution state pointer")
	}
	if cloned.State.GetCurNum() != 1 {
		t.Fatalf("cloned cur num = %d, want snapshot value 1", cloned.State.GetCurNum())
	}
	got := cloned.State.ThreadProgress["Worker-1"].StatusText
	if got != "运行中" {
		t.Fatalf("cloned status = %q, want snapshot value", got)
	}
}

func TestCloneTaskAddsProgressAndFailureFields(t *testing.T) {
	state := runstate.NewExecutionState()
	state.IncrementSuccess()
	state.MarkTerminalStop("fill_failed", "fill_failed", "填写失败")

	task := &TaskRecord{
		ID:     "task-1",
		Status: TaskFailed,
		Config: &models.RuntimeConfig{Target: 4},
		State:  state,
		Error:  "执行失败",
	}
	cloned := cloneTask(task)

	if cloned.Progress == nil || cloned.Progress.Target != 4 || cloned.Progress.Success != 1 || cloned.Progress.Percent != 0.25 {
		t.Fatalf("progress = %#v, want stable summary", cloned.Progress)
	}
	if cloned.ErrorCode != "fill_failed" || cloned.FailureReason != "fill_failed" || cloned.TerminalStopCategory != "fill_failed" {
		t.Fatalf("error fields = %q/%q/%q, want standardized fill failure", cloned.ErrorCode, cloned.FailureReason, cloned.TerminalStopCategory)
	}
}

func TestTaskErrorCodeUsesTerminalCategoryAndErrorFallback(t *testing.T) {
	tests := []struct {
		name       string
		task       *TaskRecord
		wantCode   string
		wantReason string
	}{
		{
			name:       "submission verification",
			task:       terminalTask("submission_verification", "submission_verification_required", "触发智能验证"),
			wantCode:   "submission_verification_required",
			wantReason: "submission_verification_required",
		},
		{
			name:       "reverse fill exhausted category",
			task:       terminalTask("reverse_fill_exhausted", "", "反填样本已耗尽"),
			wantCode:   "reverse_fill_exhausted",
			wantReason: "反填样本已耗尽",
		},
		{
			name:       "parse failure fallback",
			task:       &TaskRecord{ID: "task-1", Status: TaskFailed, Error: "解析问卷失败: upstream timeout"},
			wantCode:   "survey_parse_failed",
			wantReason: "解析问卷失败: upstream timeout",
		},
		{
			name:       "config failure fallback",
			task:       &TaskRecord{ID: "task-1", Status: TaskFailed, Error: "准备执行配置失败: answer window invalid"},
			wantCode:   "config_error",
			wantReason: "准备执行配置失败: answer window invalid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloned := cloneTask(tt.task)
			if cloned.ErrorCode != tt.wantCode || cloned.FailureReason != tt.wantReason {
				t.Fatalf("error fields = %q/%q, want %q/%q", cloned.ErrorCode, cloned.FailureReason, tt.wantCode, tt.wantReason)
			}
		})
	}
}

func terminalTask(category, reason, message string) *TaskRecord {
	state := runstate.NewExecutionState()
	state.MarkTerminalStop(category, reason, message)
	return &TaskRecord{ID: "task-1", Status: TaskFailed, State: state, Error: message}
}
