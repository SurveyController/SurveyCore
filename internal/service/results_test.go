package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

func TestResultsExcludeOtherTasksAndProgressLogs(t *testing.T) {
	store := NewStore(t.TempDir() + "/results.db")
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, id := range []string{"one", "two"} {
		if err := store.SaveTask(&Task{ID: id, CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range []struct {
		id    string
		event *model.Event
	}{
		{"one", nil}, {"one", &model.Event{Message: "progress"}},
		{"two", &model.Event{Result: &model.SubmissionResult{Attempt: 2, Provider: "qq", Success: true}}},
		{"one", &model.Event{Result: &model.SubmissionResult{Attempt: 1, Provider: "wjx", Success: true}}},
	} {
		if err := store.AppendLog(entry.id, TaskLog{Timestamp: time.Now(), Event: entry.event}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := store.LoadResults("one", 0, 0)
	if err != nil || len(page.Results) != 1 || page.Results[0].Provider != "wjx" || page.NextCursor != 4 || page.HasMore {
		t.Fatalf("page=%#v error=%v", page, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	manager := NewManager(store, nil)
	if task, err := manager.Create(ctx, &model.RunRequest{}); task != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled creation task=%#v error=%v", task, err)
	}
	tasks, errs := store.LoadTasks()
	if len(tasks) != 2 || len(errs) != 0 {
		t.Fatalf("canceled creation persisted a task: count=%d errors=%v", len(tasks), errs)
	}
}
