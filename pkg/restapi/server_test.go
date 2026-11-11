package restapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SurveyController/SurveyCore/internal/service"
)

func TestV1VersionUsesConfiguredBuildVersion(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Config{DBPath: dir + "\\test.db", Version: "v0.2.0-test"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["version"] != "v0.2.0-test" {
		t.Fatalf("version=%q", payload["version"])
	}
}

func TestV1CreateTaskAcceptsPythonConfig(t *testing.T) {
	s, err := New(Config{DBPath: t.TempDir() + "/tasks.db"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// A cancelled context exercises task creation and persistence without upstream requests.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	body := `{"schema_version":1,"url":"https://www.wjx.cn/vm/demo.aspx","target":3,"concurrency":2,"random_user_agent":true,"user_agent_ratios":{"pc":100},"questions":[{"question_type":"single","probabilities":[0,1]}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var task service.Task
	if err := json.Unmarshal(rec.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if task.Config.ExecutionPlan.Threads != 2 || task.Config.Target != 3 || task.Config.UserAgent == nil || !task.Config.UserAgent.Enabled || task.Config.AnswerPlan.Strategies[0].Probabilities.Options[1] != 1 {
		t.Fatalf("task config = %#v", task.Config)
	}
}

func TestV1CreateTaskRejectsInvalidPythonConfig(t *testing.T) {
	s, err := New(Config{DBPath: t.TempDir() + "/tasks.db"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, body := range []string{
		`{"schema_version":2,"url":"x"}`,
		`{"schema_version":1,"url":"x","concurrency":0}`,
		`{"schema_version":1,"url":"x","execution":{}}`,
		`{"schema_version":1,"url":"x","questions":[{"question_type":"single","unknown":1}]}`,
		`{"schema_version":1,"url":"x"} {}`,
		`null`,
	} {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"invalid_json"`) {
			t.Fatalf("body=%s status=%d error=%s", body, rec.Code, rec.Body.String())
		}
	}
}

func TestV1HealthAndNotFound(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Config{DBPath: dir + "\\test.db"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	resp, err := http.Get(server.URL + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status=%d", resp.StatusCode)
	}
	resp, err = http.Get(server.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("legacy health status=%d, want 404", resp.StatusCode)
	}
	resp, err = http.Get(server.URL + "/api/v1/tasks/missing")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status=%d", resp.StatusCode)
	}
}

func TestV1CreateTaskRejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Config{DBPath: dir + "\\test.db"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	body := `{"unknown":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != "invalid_json" {
		t.Fatalf("code=%v", payload["code"])
	}
}
