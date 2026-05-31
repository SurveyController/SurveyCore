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
	"github.com/SurveyController/SurveyCore/internal/network/proxy"
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

func TestCreateTaskAcceptsPythonConfigEnvelope(t *testing.T) {
	server := newTestServer(t)
	reqBody := `{
		"config":{
			"url":"https://www.wjx.cn/vm/test.aspx",
			"target":1,
			"_ai_config_present":true,
			"python_only_future_field":"task-envelope"
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	taskID, _ := created["task_id"].(string)
	if taskID == "" {
		t.Fatalf("response = %#v, want task_id", created)
	}
	task, ok := server.manager.Get(taskID)
	if !ok {
		t.Fatalf("created task %q not found", taskID)
	}
	if task.Config == nil || task.Config.URL != "https://www.wjx.cn/vm/test.aspx" {
		t.Fatalf("task config = %#v, want envelope config", task.Config)
	}
	if string(task.Config.ExtraFields["python_only_future_field"]) != `"task-envelope"` {
		t.Fatalf("extra fields = %#v, want envelope extras preserved", task.Config.ExtraFields)
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

func TestAITestEndpointReturnsPreview(t *testing.T) {
	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"连接成功"}`))
	}))
	defer aiServer.Close()

	server := newTestServer(t)
	reqBody := `{
		"ai_mode":"provider",
		"ai_provider":"custom",
		"ai_api_key":"test-key",
		"ai_base_url":"` + aiServer.URL + `/v1",
		"ai_api_protocol":"responses",
		"ai_model":"test-model"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/ai/test", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "连接成功") {
		t.Fatalf("response body = %s, want AI preview", rec.Body.String())
	}
}

func TestAITestEndpointSupportsFreeAIService(t *testing.T) {
	var gotDeviceID string
	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotDeviceID = r.Header.Get("X-Device-ID")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answers":["免费连接成功"]}`))
	}))
	defer aiServer.Close()

	server := newTestServer(t)
	reqBody := `{
		"ai_mode":"free",
		"ai_free_endpoint":"` + aiServer.URL + `",
		"random_ip_user_id":88,
		"random_ip_device_id":"device-88"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/ai/test", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	if gotDeviceID != "device-88" {
		t.Fatalf("X-Device-ID = %q, want device-88", gotDeviceID)
	}
	if !strings.Contains(rec.Body.String(), "免费连接成功") {
		t.Fatalf("response body = %s, want free AI preview", rec.Body.String())
	}
}

func TestRandomIPTrialEndpointReturnsSession(t *testing.T) {
	randomIPServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Device-ID") == "" {
			t.Fatal("missing X-Device-ID")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_id":99,"remaining_quota":8,"total_quota":10,"used_quota":2}`))
	}))
	defer randomIPServer.Close()

	server := newTestServer(t)
	server.randomIP = proxy.NewRandomIPService(filepath.Join(t.TempDir(), "random-ip.json"), proxy.RandomIPEndpoints{TrialEndpoint: randomIPServer.URL})
	req := httptest.NewRequest(http.MethodPost, "/api/random-ip/trial", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var snapshot proxy.RandomIPSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if !snapshot.Authenticated || snapshot.UserID != 99 || snapshot.RemainingQuota != 8 {
		t.Fatalf("snapshot = %#v, want authenticated random IP session", snapshot)
	}
}

func TestRandomIPSyncRequiresSession(t *testing.T) {
	server := newTestServer(t)
	server.randomIP = proxy.NewRandomIPService(filepath.Join(t.TempDir(), "random-ip.json"), proxy.RandomIPEndpoints{})
	req := httptest.NewRequest(http.MethodPost, "/api/random-ip/quota/sync", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s, want 401", rec.Code, rec.Body.String())
	}
	apiErr := decodeAPIError(t, rec)
	if apiErr.Code != "random_ip_not_authenticated" {
		t.Fatalf("code = %q, want random_ip_not_authenticated", apiErr.Code)
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

func TestImportConfigAcceptsPythonCompatibleJSON(t *testing.T) {
	server := newTestServer(t)
	reqBody := `{
		"config":{
			"url":"https://www.wjx.cn/vm/test.aspx",
			"target":2,
			"_ai_config_present":true,
			"python_only_future_field":"ignored"
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/configs/import", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var cfg models.RuntimeConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Target != 2 || cfg.Threads != 1 || cfg.AIMode != "free" {
		t.Fatalf("config = %#v, want imported target with defaults", cfg)
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["_ai_config_present"] != true || raw["python_only_future_field"] != "ignored" {
		t.Fatalf("raw config = %#v, want python-only fields preserved", raw)
	}
}

func TestExportConfigReturnsDownload(t *testing.T) {
	server := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/configs/export", strings.NewReader(`{"url":"https://www.wjx.cn/vm/test.aspx","target":3,"config_schema_version":6}`))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "surveycore-config.json") {
		t.Fatalf("Content-Disposition = %q, want config filename", got)
	}
	var cfg models.RuntimeConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Target != 3 || cfg.Threads != 1 {
		t.Fatalf("config = %#v, want exported config with defaults", cfg)
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["config_schema_version"] != float64(6) {
		t.Fatalf("raw config = %#v, want preserved schema version", raw)
	}
}

func TestExportTaskConfigReturnsPersistedConfig(t *testing.T) {
	server := newTestServer(t)
	importedCfg, err := models.DeserializeRuntimeConfig([]byte(`{
		"url":"https://www.wjx.cn/vm/test.aspx",
		"survey_title":"问卷",
		"target":1,
		"threads":1,
		"python_only_future_field":"task-keep"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	task, err := server.manager.Create(t.Context(), &models.RuntimeConfig{
		URL:         importedCfg.URL,
		SurveyTitle: importedCfg.SurveyTitle,
		Target:      importedCfg.Target,
		Threads:     importedCfg.Threads,
		ExtraFields: importedCfg.ExtraFields,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/tasks/"+task.ID+"/config", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var exportedCfg models.RuntimeConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &exportedCfg); err != nil {
		t.Fatal(err)
	}
	if exportedCfg.SurveyTitle != "问卷" || exportedCfg.Target != 1 {
		t.Fatalf("config = %#v, want task config", exportedCfg)
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["python_only_future_field"] != "task-keep" {
		t.Fatalf("raw task config = %#v, want persisted extra field", raw)
	}
}

func TestExportTaskReportReturnsProgressAndLogs(t *testing.T) {
	server := newTestServer(t)
	task, err := server.manager.Create(t.Context(), nilRuntimeConfig())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/tasks/"+task.ID+"/report", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "report.json") {
		t.Fatalf("Content-Disposition = %q, want report filename", got)
	}
	var report taskReport
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.TaskID != task.ID || report.Progress == nil || report.Progress.Target != 1 || len(report.Logs) == 0 {
		t.Fatalf("report = %#v, want task progress and logs", report)
	}
}

func TestExportTaskReportCSV(t *testing.T) {
	server := newTestServer(t)
	task, err := server.manager.Create(t.Context(), nilRuntimeConfig())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/tasks/"+task.ID+"/report?format=csv", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "task_id,status,log_id") {
		t.Fatalf("csv = %s, want header", rec.Body.String())
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
