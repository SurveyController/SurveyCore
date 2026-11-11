package restapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SurveyController/SurveyCore/internal/service"
	"github.com/SurveyController/SurveyCore/pkg/surveycore"
)

const taskResultsHTML = `<html><head><title>Task results</title></head><body><div id="divQuestion"><fieldset><div topic="1" id="div1" type="3"><div class="topichtml">Choice</div><div class="ui-controlgroup"><div><span class="label">A</span></div><div><span class="label">B</span></div></div></div></fieldset></div></body></html>`

func taskResultsServer(t *testing.T, path string, transport submissionTestTransport) *Server {
	t.Helper()
	s, err := New(Config{DBPath: path})
	if err != nil {
		t.Fatal(err)
	}
	s.manager = service.NewManager(s.store, surveycore.New(surveycore.WithHTTPClient(&http.Client{Transport: transport})))
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func createResultTask(t *testing.T, serverURL string, target int) service.Task {
	t.Helper()
	body := fmt.Sprintf(`{"schema_version":1,"url":"https://www.wjx.cn/vm/demo.aspx","target":%d,"concurrency":1,"answer_duration":[0,0]}`, target)
	response, err := http.Post(serverURL+"/api/v1/tasks", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var task service.Task
	if response.StatusCode != http.StatusAccepted {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("create status=%d body=%s", response.StatusCode, data)
	}
	if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	return task
}

func waitForTask(t *testing.T, s *Server, id string) *service.Task {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if task, ok := s.manager.Get(id); ok && task.FinishedAt != nil {
			return task
		}
		select {
		case <-deadline.C:
			t.Fatal("task did not finish")
		case <-ticker.C:
		}
	}
}

func readResultPage(t *testing.T, s *Server, id string, after int64, limit int) service.TaskResultPage {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/tasks/%s/results?after=%d&limit=%d", id, after, limit), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("page status=%d body=%s", rec.Code, rec.Body.String())
	}
	var page service.TaskResultPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func TestTaskResultsSurviveRequestCompletionAndServiceRestart(t *testing.T) {
	path := t.TempDir() + "/results.db"
	release := make(chan struct{})
	var posts atomic.Int32
	s := taskResultsServer(t, path, func(request *http.Request) (*http.Response, error) {
		body := taskResultsHTML
		if request.Method == http.MethodPost {
			if posts.Add(1) == 1 {
				select {
				case <-release:
					body = "1〒1〒失败"
				case <-request.Context().Done():
					return nil, request.Context().Err()
				}
			} else {
				body = "10"
			}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	httpServer := httptest.NewServer(s.Handler())
	defer httpServer.Close()
	created := createResultTask(t, httpServer.URL, 3)
	// The HTTP creation request has ended before any attempt can complete.
	close(release)
	finished := waitForTask(t, s, created.ID)
	if finished.Status != service.TaskFailed || finished.Result == nil || finished.Result.Success != 2 || finished.Result.Fail != 1 || posts.Load() != 3 {
		t.Fatalf("finished=%#v aggregate=%#v posts=%d", finished, finished.Result, posts.Load())
	}
	first := readResultPage(t, s, created.ID, 0, 2)
	if len(first.Results) != 2 || !first.HasMore || first.NextCursor <= 0 || first.Results[0].Attempt != 1 || first.Results[0].Success || first.Results[0].Error == nil || first.Results[1].Attempt != 2 || !first.Results[1].Success {
		t.Fatalf("first page=%#v", first)
	}
	second := readResultPage(t, s, created.ID, first.NextCursor, 2)
	if len(second.Results) != 1 || second.HasMore || second.NextCursor <= first.NextCursor || second.Results[0].Attempt != 3 || !second.Results[0].Success {
		t.Fatalf("second page=%#v", second)
	}
	empty := readResultPage(t, s, created.ID, second.NextCursor, 2)
	if empty.Results == nil || len(empty.Results) != 0 || empty.HasMore || empty.NextCursor != second.NextCursor {
		t.Fatalf("empty page=%#v", empty)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(Config{DBPath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	page := readResultPage(t, reopened, created.ID, 0, 200)
	if len(page.Results) != 3 || page.NextCursor != second.NextCursor || page.HasMore {
		t.Fatalf("persisted page=%#v", page)
	}
	if task, ok := reopened.manager.Get(created.ID); !ok || task.Status != service.TaskFailed || task.FinishedAt == nil {
		t.Fatalf("persisted task=%#v", task)
	}
}

func TestStoppingTaskCancelsInflightSubmissionWithoutFailureResult(t *testing.T) {
	started := make(chan struct{})
	var posts atomic.Int32
	s := taskResultsServer(t, t.TempDir()+"/stop.db", func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodPost {
			posts.Add(1)
			close(started)
			<-request.Context().Done()
			return nil, request.Context().Err()
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(taskResultsHTML)), Request: request}, nil
	})
	httpServer := httptest.NewServer(s.Handler())
	defer httpServer.Close()
	created := createResultTask(t, httpServer.URL, 100)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("submission did not start")
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+created.ID+"/stop", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("stop status=%d body=%s", rec.Code, rec.Body.String())
	}
	finished := waitForTask(t, s, created.ID)
	page := readResultPage(t, s, created.ID, 0, 200)
	if finished.Status != service.TaskStopped || finished.Result == nil || !finished.Result.Stopped || finished.Result.Fail != 0 || finished.Result.Success != 0 || len(page.Results) != 0 || posts.Load() != 1 {
		t.Fatalf("finished=%#v aggregate=%#v page=%#v posts=%d", finished, finished.Result, page, posts.Load())
	}
	s.manager.StopAll()
	saved, errs := s.store.LoadTasks()
	if len(errs) != 0 || len(saved) != 1 || saved[0].Status != service.TaskStopped || saved[0].FinishedAt == nil || saved[0].Result == nil || !saved[0].Result.Stopped {
		t.Fatalf("persisted stopped task=%#v errors=%v", saved, errs)
	}
}

func TestTaskResultPageRejectsInvalidQueries(t *testing.T) {
	s, _ := submissionTestServer(t, true)
	for _, test := range []struct {
		query  string
		status int
	}{
		{"?after=-1", 400}, {"?after=x", 400}, {"?limit=-1", 400}, {"?limit=x", 400}, {"", 404},
	} {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/missing/results"+test.query, nil))
		if rec.Code != test.status {
			t.Fatalf("query=%s status=%d body=%s", test.query, rec.Code, rec.Body.String())
		}
	}
}
