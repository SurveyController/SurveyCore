package api

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SurveyController/SurveyConsole/internal/models"
	"github.com/SurveyController/SurveyConsole/internal/tasks"
)

func TestHealth(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestCreateTaskReturnsTaskID(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(`{"url":"https://www.wjx.cn/vm/test.aspx","target":1}`))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "task_id") {
		t.Fatalf("response body = %s, want task_id", rec.Body.String())
	}
}

func TestGetAndStopTask(t *testing.T) {
	server := newTestServer(t)
	task, err := server.manager.Create(t.Context(), nilRuntimeConfig())
	if err != nil {
		t.Fatal(err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/tasks/"+task.ID, nil)
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d", getRec.Code)
	}

	stopReq := httptest.NewRequest(http.MethodPost, "/api/tasks/"+task.ID+"/stop", nil)
	stopRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(stopRec, stopReq)
	if stopRec.Code != http.StatusOK {
		t.Fatalf("stop status = %d body=%s", stopRec.Code, stopRec.Body.String())
	}
}

func TestParseSurveyMissingURLReturns400(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/surveys/parse", strings.NewReader(`{"url":""}`))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestDecodeQRMissingImageReturns400(t *testing.T) {
	server := newTestServer(t)
	body := bytes.NewBufferString("--boundary--\r\n")
	req := httptest.NewRequest(http.MethodPost, "/api/qrcode/decode", body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	store := tasks.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	manager := tasks.NewTaskManager(store, nilRegistry{})
	t.Cleanup(manager.StopAll)
	return NewServer(manager, "test")
}

func nilRuntimeConfig() *models.RuntimeConfig {
	return &models.RuntimeConfig{URL: "https://www.wjx.cn/vm/test.aspx", Target: 1, Threads: 1}
}

type nilRegistry struct{}

func (nilRegistry) Get(name string) (models.ProviderAdapter, error) {
	return nil, errors.New("test registry has no providers")
}

func (nilRegistry) GetByURL(url string) (models.ProviderAdapter, error) {
	return nil, errors.New("test registry has no providers")
}
