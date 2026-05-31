package api

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SurveyController/SurveyCore/internal/config"
	"github.com/SurveyController/SurveyCore/internal/models"
	"github.com/SurveyController/SurveyCore/internal/tasks"
)

const reportLogPageSize = 1000

type configImportEnvelope struct {
	Config *models.RuntimeConfig `json:"config"`
}

type taskReport struct {
	TaskID               string              `json:"task_id"`
	Status               string              `json:"status"`
	CreatedAt            time.Time           `json:"created_at"`
	StartedAt            *time.Time          `json:"started_at,omitempty"`
	FinishedAt           *time.Time          `json:"finished_at,omitempty"`
	DurationMS           int64               `json:"duration_ms,omitempty"`
	ErrorCode            string              `json:"error_code,omitempty"`
	FailureReason        string              `json:"failure_reason,omitempty"`
	TerminalStopCategory string              `json:"terminal_stop_category,omitempty"`
	Error                string              `json:"error,omitempty"`
	StopMessage          string              `json:"stop_message,omitempty"`
	Config               taskReportConfig    `json:"config"`
	Progress             *tasks.TaskProgress `json:"progress,omitempty"`
	ThreadProgress       []map[string]any    `json:"thread_progress,omitempty"`
	Logs                 []tasks.TaskLog     `json:"logs"`
}

type taskReportConfig struct {
	URL                string `json:"url,omitempty"`
	SurveyTitle        string `json:"survey_title,omitempty"`
	SurveyProvider     string `json:"survey_provider,omitempty"`
	Target             int    `json:"target,omitempty"`
	Threads            int    `json:"threads,omitempty"`
	RandomIPEnabled    bool   `json:"random_ip_enabled,omitempty"`
	ProxySource        string `json:"proxy_source,omitempty"`
	RandomUAEnabled    bool   `json:"random_ua_enabled,omitempty"`
	AIMode             string `json:"ai_mode,omitempty"`
	AIProvider         string `json:"ai_provider,omitempty"`
	AIAPIProtocol      string `json:"ai_api_protocol,omitempty"`
	ReverseFillEnabled bool   `json:"reverse_fill_enabled,omitempty"`
}

func (s *Server) handleImportConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := readCompatibleRuntimeConfig(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "配置 JSON 请求体无效", err)
		return
	}
	config.MergeDefaults(cfg)
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleExportConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := readCompatibleRuntimeConfig(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "配置 JSON 请求体无效", err)
		return
	}
	config.MergeDefaults(cfg)
	writeRuntimeConfigDownload(w, cfg, "surveycore-config.json")
}

func (s *Server) handleExportTaskConfig(w http.ResponseWriter, r *http.Request) {
	task, ok := s.manager.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "任务不存在", nil)
		return
	}
	writeRuntimeConfigDownload(w, task.Config, "surveycore-task-"+task.ID+"-config.json")
}

func (s *Server) handleExportTaskReport(w http.ResponseWriter, r *http.Request) {
	task, ok := s.manager.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "任务不存在", nil)
		return
	}
	logs, err := s.loadAllTaskLogs(task.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "任务日志读取失败", err)
		return
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" || format == "json" {
		writeJSONDownload(w, http.StatusOK, buildTaskReport(task, logs), "surveycore-task-"+task.ID+"-report.json")
		return
	}
	if format == "csv" {
		writeCSVReport(w, task, logs)
		return
	}
	writeError(w, http.StatusBadRequest, "invalid_query", "报告格式无效", errors.New("format 仅支持 json 或 csv"))
}

func readCompatibleRuntimeConfig(r *http.Request) (*models.RuntimeConfig, error) {
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, errors.New("请求体为空")
	}
	var envelope configImportEnvelope
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Config != nil {
		return envelope.Config, nil
	}
	var cfg models.RuntimeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func writeRuntimeConfigDownload(w http.ResponseWriter, cfg *models.RuntimeConfig, filename string) {
	data, err := models.SerializeRuntimeConfig(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "配置序列化失败", err)
		return
	}
	writeBytesDownload(w, http.StatusOK, "application/json; charset=utf-8", filename, data)
}

func writeJSONDownload(w http.ResponseWriter, status int, value any, filename string) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "报告序列化失败", err)
		return
	}
	data = append(data, '\n')
	writeBytesDownload(w, status, "application/json; charset=utf-8", filename, data)
}

func writeBytesDownload(w http.ResponseWriter, status int, contentType, filename string, data []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func (s *Server) loadAllTaskLogs(taskID string) ([]tasks.TaskLog, error) {
	var (
		all   []tasks.TaskLog
		after int64
	)
	for {
		page, err := s.manager.Logs(taskID, after, reportLogPageSize)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Logs...)
		if !page.HasMore || page.NextCursor == 0 {
			return all, nil
		}
		after = page.NextCursor
	}
}

func buildTaskReport(task *tasks.TaskRecord, logs []tasks.TaskLog) taskReport {
	report := taskReport{
		TaskID:               task.ID,
		Status:               task.Status,
		CreatedAt:            task.CreatedAt,
		StartedAt:            task.StartedAt,
		FinishedAt:           task.FinishedAt,
		ErrorCode:            task.ErrorCode,
		FailureReason:        task.FailureReason,
		TerminalStopCategory: task.TerminalStopCategory,
		Error:                task.Error,
		StopMessage:          task.StopMessage,
		Config:               summarizeReportConfig(task.Config),
		Progress:             task.Progress,
		Logs:                 logs,
	}
	if task.StartedAt != nil {
		end := time.Now()
		if task.FinishedAt != nil {
			end = *task.FinishedAt
		}
		report.DurationMS = end.Sub(*task.StartedAt).Milliseconds()
	}
	if task.State != nil {
		report.ThreadProgress = task.State.SnapshotThreadProgress()
	}
	return report
}

func summarizeReportConfig(cfg *models.RuntimeConfig) taskReportConfig {
	if cfg == nil {
		return taskReportConfig{}
	}
	return taskReportConfig{
		URL:                cfg.URL,
		SurveyTitle:        cfg.SurveyTitle,
		SurveyProvider:     cfg.SurveyProvider,
		Target:             cfg.Target,
		Threads:            cfg.Threads,
		RandomIPEnabled:    cfg.RandomIPEnabled,
		ProxySource:        cfg.ProxySource,
		RandomUAEnabled:    cfg.RandomUAEnabled,
		AIMode:             cfg.AIMode,
		AIProvider:         cfg.AIProvider,
		AIAPIProtocol:      cfg.AIAPIProtocol,
		ReverseFillEnabled: cfg.ReverseFillEnabled,
	}
}

func writeCSVReport(w http.ResponseWriter, task *tasks.TaskRecord, logs []tasks.TaskLog) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	_ = writer.Write([]string{"task_id", "status", "log_id", "timestamp", "level", "message", "worker", "current", "total", "success", "fail"})
	for _, entry := range logs {
		worker, current, total, success, fail := eventFields(entry)
		_ = writer.Write([]string{
			task.ID,
			task.Status,
			strconv.FormatInt(entry.ID, 10),
			entry.Timestamp.Format(time.RFC3339Nano),
			entry.Level,
			entry.Message,
			worker,
			current,
			total,
			success,
			fail,
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "CSV 报告生成失败", err)
		return
	}
	writeBytesDownload(w, http.StatusOK, "text/csv; charset=utf-8", "surveycore-task-"+task.ID+"-report.csv", buf.Bytes())
}

func eventFields(entry tasks.TaskLog) (worker, current, total, success, fail string) {
	if entry.Event == nil {
		return fieldString(entry.Fields, "worker"), fieldString(entry.Fields, "current"), fieldString(entry.Fields, "total"), "", ""
	}
	return entry.Event.ThreadName,
		strconv.Itoa(entry.Event.Current),
		strconv.Itoa(entry.Event.Total),
		strconv.FormatBool(entry.Event.Success),
		strconv.FormatBool(entry.Event.Fail)
}

func fieldString(fields map[string]any, key string) string {
	if fields == nil {
		return ""
	}
	value, ok := fields[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
