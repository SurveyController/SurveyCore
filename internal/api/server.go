package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	surveyio "github.com/SurveyController/SurveyCore/internal/io"
	"github.com/SurveyController/SurveyCore/internal/logging"
	"github.com/SurveyController/SurveyCore/internal/models"
	"github.com/SurveyController/SurveyCore/internal/network/proxy"
	"github.com/SurveyController/SurveyCore/internal/tasks"
)

type TaskService interface {
	Create(ctx context.Context, cfg *models.RuntimeConfig) (*tasks.TaskRecord, error)
	List() []*tasks.TaskRecord
	Get(id string) (*tasks.TaskRecord, bool)
	Stop(id string) (*tasks.TaskRecord, error)
	Logs(id string, afterID int64, limit int) (*tasks.TaskLogPage, error)
	ParseSurvey(ctx context.Context, surveyURL string) (*models.SurveyDefinition, error)
	BuildDefaultConfig(ctx context.Context, surveyURL string) (*models.RuntimeConfig, error)
}

type Server struct {
	manager  TaskService
	version  string
	randomIP *proxy.RandomIPService
}

func NewServer(manager TaskService, version string) *Server {
	return &Server{
		manager:  manager,
		version:  version,
		randomIP: proxy.DefaultRandomIPService(),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/version", s.handleVersion)
	mux.HandleFunc("POST /api/tasks", s.handleCreateTask)
	mux.HandleFunc("GET /api/tasks", s.handleListTasks)
	mux.HandleFunc("GET /api/tasks/{id}", s.handleGetTask)
	mux.HandleFunc("POST /api/tasks/{id}/stop", s.handleStopTask)
	mux.HandleFunc("GET /api/tasks/{id}/logs", s.handleTaskLogs)
	mux.HandleFunc("GET /api/tasks/{id}/config", s.handleExportTaskConfig)
	mux.HandleFunc("GET /api/tasks/{id}/report", s.handleExportTaskReport)
	mux.HandleFunc("POST /api/surveys/parse", s.handleParseSurvey)
	mux.HandleFunc("POST /api/configs", s.handleCreateConfig)
	mux.HandleFunc("POST /api/configs/import", s.handleImportConfig)
	mux.HandleFunc("POST /api/configs/export", s.handleExportConfig)
	mux.HandleFunc("POST /api/ai/test", s.handleTestAI)
	mux.HandleFunc("GET /api/random-ip/session", s.handleGetRandomIPSession)
	mux.HandleFunc("POST /api/random-ip/trial", s.handleActivateRandomIPTrial)
	mux.HandleFunc("POST /api/random-ip/quota/sync", s.handleSyncRandomIPQuota)
	mux.HandleFunc("POST /api/random-ip/redeem", s.handleRedeemRandomIPCard)
	mux.HandleFunc("POST /api/random-ip/bonus", s.handleClaimRandomIPBonus)
	mux.HandleFunc("POST /api/qrcode/decode", s.handleDecodeQR)
	return loggingMiddleware(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"version": s.version})
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var cfg models.RuntimeConfig
	if err := decodeCompatibleJSON(r, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "配置 JSON 请求体无效", err)
		return
	}
	task, err := s.manager.Create(context.Background(), &cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "任务配置无效", err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"task_id": task.ID, "status": task.Status})
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"tasks": s.manager.List()})
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	task, ok := s.manager.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "任务不存在", nil)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleStopTask(w http.ResponseWriter, r *http.Request) {
	task, err := s.manager.Stop(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "任务不存在", err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleTaskLogs(w http.ResponseWriter, r *http.Request) {
	afterID, err := tasks.ParseLogCursor(r.URL.Query().Get("after"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_query", "日志查询参数无效", err)
		return
	}
	limit, err := tasks.ParseLogLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_query", "日志查询参数无效", err)
		return
	}
	page, err := s.manager.Logs(r.PathValue("id"), afterID, limit)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "任务不存在", err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleParseSurvey(w http.ResponseWriter, r *http.Request) {
	var req parseSurveyRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "JSON 请求体无效", err)
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "url 不能为空", nil)
		return
	}
	def, err := s.manager.ParseSurvey(r.Context(), req.URL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_error", "问卷解析失败", err)
		return
	}
	writeJSON(w, http.StatusOK, def)
}

func (s *Server) handleCreateConfig(w http.ResponseWriter, r *http.Request) {
	var req createConfigRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "JSON 请求体无效", err)
		return
	}
	cfg, err := s.manager.BuildDefaultConfig(r.Context(), strings.TrimSpace(req.URL))
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_error", "配置生成失败", err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleDecodeQR(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "二维码请求无效", err)
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "缺少 image 文件", err)
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "surveycore-qr-*"+filepath.Ext(header.Filename))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "临时文件创建失败", err)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.ReadFrom(file); err != nil {
		tmp.Close()
		writeError(w, http.StatusInternalServerError, "internal_error", "二维码文件读取失败", err)
		return
	}
	if err := tmp.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "二维码文件保存失败", err)
		return
	}

	url, err := surveyio.DecodeSurveyURLFromFile(tmpPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "二维码解析失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": url})
}

func decodeStrictJSON(r *http.Request, dst any) error {
	return decodeJSON(r, dst, true)
}

func decodeCompatibleJSON(r *http.Request, dst any) error {
	return decodeJSON(r, dst, false)
}

func decodeJSON(r *http.Request, dst any, strict bool) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("JSON 请求体包含多个顶层值")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type errorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

func writeError(w http.ResponseWriter, status int, code string, message string, err error) {
	resp := errorResponse{
		Error:   message,
		Code:    code,
		Message: message,
	}
	if err != nil {
		resp.Detail = err.Error()
	}
	writeJSON(w, status, resp)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logging.InfoFields("HTTP 请求", logging.F("method", r.Method), logging.F("path", r.URL.Path), logging.F("duration_ms", time.Since(start).Milliseconds()))
	})
}
