package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SurveyController/SurveyCore/internal/models"
	"github.com/SurveyController/SurveyCore/internal/tasks"
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

func TestCreateTaskAcceptsPythonConfigExtraFields(t *testing.T) {
	server := newTestServer(t)
	reqBody := `{
		"url":"https://www.wjx.cn/vm/test.aspx",
		"target":1,
		"_ai_config_present":true,
		"python_only_future_field":"ignored"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateTaskRejectsInvalidJSONWithStructuredError(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(`{"url":`))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
	apiErr := decodeAPIError(t, rec)
	if apiErr.Code != "invalid_json" || apiErr.Message == "" || apiErr.Detail == "" {
		t.Fatalf("error = %#v, want invalid_json with message and detail", apiErr)
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
	apiErr := decodeAPIError(t, rec)
	if apiErr.Code != "validation_error" {
		t.Fatalf("code = %q, want validation_error", apiErr.Code)
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
	apiErr := decodeAPIError(t, rec)
	if apiErr.Code != "validation_error" {
		t.Fatalf("code = %q, want validation_error", apiErr.Code)
	}
}

func TestTaskLogsReturnsCursorPage(t *testing.T) {
	server := newTestServer(t)
	task, err := server.manager.Create(t.Context(), nilRuntimeConfig())
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/tasks/"+task.ID+"/logs?limit=1", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var page tasks.TaskLogPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Logs) != 1 || page.NextCursor == 0 {
		t.Fatalf("page = %#v, want one log with cursor", page)
	}
}

func TestTaskLogsRejectsInvalidCursor(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/tasks/task-1/logs?after=bad", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
	apiErr := decodeAPIError(t, rec)
	if apiErr.Code != "invalid_query" {
		t.Fatalf("code = %q, want invalid_query", apiErr.Code)
	}
}

func decodeAPIError(t *testing.T, rec *httptest.ResponseRecorder) errorResponse {
	t.Helper()
	var apiErr errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	return apiErr
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	store := tasks.NewStore(filepath.Join(t.TempDir(), "surveycore.db"))
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
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
