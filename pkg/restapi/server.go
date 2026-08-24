package restapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	surveyio "github.com/SurveyController/SurveyCore/internal/io"
	"github.com/SurveyController/SurveyCore/internal/service"
	"github.com/SurveyController/SurveyCore/pkg/surveycore"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/proxy"
)

type Config struct {
	DBPath    string
	Version   string
	AIKey     string
	AIBaseURL string
	AIModel   string
}
type Server struct {
	manager      *service.Manager
	store        *service.Store
	official     *proxy.OfficialClient
	buildVersion string
	mux          http.Handler
}

func New(cfg Config) (*Server, error) {
	if strings.TrimSpace(cfg.DBPath) == "" {
		cfg.DBPath = "data/surveycore-v1.db"
	}
	version := strings.TrimSpace(cfg.Version)
	if version == "" {
		version = "dev"
	}
	store := service.NewStore(cfg.DBPath)
	if err := store.Init(); err != nil {
		return nil, err
	}
	client := surveycore.New(surveycore.WithAI(cfg.AIKey, cfg.AIBaseURL, cfg.AIModel))
	m := service.NewManager(store, client)
	if err := m.Load(); err != nil {
		return nil, err
	}
	proxySession := proxy.NewOfficialSessionManager(proxy.OfficialSessionManagerOptions{Store: store})
	official := proxy.NewOfficialClient(proxy.OfficialClientOptions{SessionManager: proxySession})
	s := &Server{manager: m, store: store, official: official, buildVersion: version}
	s.mux = s.routes()
	return s, nil
}
func (s *Server) Handler() http.Handler { return s.mux }
func (s *Server) Close() error          { s.manager.StopAll(); return s.manager.Close() }
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("GET /api/v1/version", s.version)
	mux.HandleFunc("POST /api/v1/surveys/parse", s.parse)
	mux.HandleFunc("POST /api/v1/configs", s.config)
	mux.HandleFunc("POST /api/v1/tasks", s.create)
	mux.HandleFunc("GET /api/v1/tasks", s.list)
	mux.HandleFunc("GET /api/v1/tasks/{id}", s.get)
	mux.HandleFunc("POST /api/v1/tasks/{id}/stop", s.stop)
	mux.HandleFunc("GET /api/v1/tasks/{id}/logs", s.logs)
	mux.HandleFunc("POST /api/v1/qrcode/decode", s.qr)
	mux.HandleFunc("POST /api/v1/proxy/session", s.proxySession)
	mux.HandleFunc("GET /api/v1/proxy/usage", s.proxyUsage)
	mux.HandleFunc("POST /api/v1/proxy/extract", s.proxyExtract)
	mux.HandleFunc("POST /api/v1/proxy/bonus", s.proxyBonus)
	mux.HandleFunc("POST /api/v1/proxy/redeem", s.proxyRedeem)
	return mux
}
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	write(w, 200, map[string]any{"status": "ok"})
}
func (s *Server) version(w http.ResponseWriter, _ *http.Request) {
	write(w, 200, map[string]any{"version": s.buildVersion})
}

type urlRequest struct {
	URL string `json:"url"`
}

func decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 8<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}
func (s *Server) parse(w http.ResponseWriter, r *http.Request) {
	var req urlRequest
	if err := decode(r, &req); err != nil {
		fail(w, 400, "invalid_json", err)
		return
	}
	v, e := s.manager.Parse(r.Context(), req.URL)
	if e != nil {
		fail(w, 502, "upstream_error", e)
		return
	}
	write(w, 200, v)
}
func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	var req urlRequest
	if err := decode(r, &req); err != nil {
		fail(w, 400, "invalid_json", err)
		return
	}
	v, e := s.manager.DefaultConfig(r.Context(), req.URL)
	if e != nil {
		fail(w, 502, "upstream_error", e)
		return
	}
	write(w, 200, v)
}
func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	var cfg model.RunRequest
	if err := decode(r, &cfg); err != nil {
		fail(w, 400, "invalid_json", err)
		return
	}
	t, e := s.manager.Create(r.Context(), &cfg)
	if e != nil {
		fail(w, 422, "validation_error", e)
		return
	}
	write(w, 202, t)
}
func (s *Server) list(w http.ResponseWriter, _ *http.Request) {
	write(w, 200, map[string]any{"tasks": s.manager.List()})
}
func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	t, ok := s.manager.Get(r.PathValue("id"))
	if !ok {
		fail(w, 404, "not_found", service.ErrTaskNotFound)
		return
	}
	write(w, 200, t)
}
func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	t, e := s.manager.Stop(r.PathValue("id"))
	if errors.Is(e, service.ErrTaskNotFound) {
		fail(w, 404, "not_found", e)
		return
	}
	if e != nil {
		fail(w, 500, "internal_error", e)
		return
	}
	write(w, 200, t)
}
func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	after, e := queryInt(r, "after", 0)
	if e != nil {
		fail(w, 400, "invalid_query", e)
		return
	}
	limit, e := queryInt(r, "limit", 200)
	if e != nil {
		fail(w, 400, "invalid_query", e)
		return
	}
	p, e := s.manager.Logs(r.PathValue("id"), int64(after), limit)
	if errors.Is(e, service.ErrTaskNotFound) {
		fail(w, 404, "not_found", e)
		return
	}
	if e != nil {
		fail(w, 500, "internal_error", e)
		return
	}
	write(w, 200, p)
}
func (s *Server) qr(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		fail(w, 400, "invalid_request", err)
		return
	}
	f, _, e := r.FormFile("file")
	if e != nil {
		fail(w, 400, "invalid_request", e)
		return
	}
	defer f.Close()
	tmp, e := os.CreateTemp("", "surveycore-qr-*")
	if e == nil {
		_, e = io.Copy(tmp, f)
		tmp.Close()
	}
	if e == nil {
		defer os.Remove(tmp.Name())
		var u string
		u, e = surveyio.DecodeSurveyURLFromFile(tmp.Name())
		if e == nil {
			write(w, 200, map[string]string{"url": u})
			return
		}
	}
	fail(w, 422, "validation_error", e)
}
func (s *Server) proxySession(w http.ResponseWriter, r *http.Request) {
	v, e := s.official.ActivateTrial(r.Context())
	if e != nil {
		fail(w, 502, "upstream_error", e)
		return
	}
	write(w, 200, v)
}
func (s *Server) proxyUsage(w http.ResponseWriter, r *http.Request) {
	v, e := s.official.Usage(r.Context())
	if e != nil {
		fail(w, 502, "upstream_error", e)
		return
	}
	write(w, 200, v)
}
func (s *Server) proxyExtract(w http.ResponseWriter, r *http.Request) {
	var req proxy.OfficialExtractRequest
	if err := decode(r, &req); err != nil {
		fail(w, 400, "invalid_json", err)
		return
	}
	v, e := s.official.ExtractProxy(r.Context(), req)
	if e != nil {
		fail(w, 502, "upstream_error", e)
		return
	}
	write(w, 200, v)
}

type codeRequest struct {
	Code string `json:"code"`
}

func (s *Server) proxyBonus(w http.ResponseWriter, r *http.Request) {
	var req codeRequest
	if err := decode(r, &req); err != nil {
		fail(w, 400, "invalid_json", err)
		return
	}
	v, e := s.official.ClaimBonus(r.Context(), req.Code)
	if e != nil {
		fail(w, 502, "upstream_error", e)
		return
	}
	write(w, 200, v)
}
func (s *Server) proxyRedeem(w http.ResponseWriter, r *http.Request) {
	var req codeRequest
	if err := decode(r, &req); err != nil {
		fail(w, 400, "invalid_json", err)
		return
	}
	v, e := s.official.RedeemCard(r.Context(), req.Code)
	if e != nil {
		fail(w, 502, "upstream_error", e)
		return
	}
	write(w, 200, v)
}
func queryInt(r *http.Request, k string, d int) (int, error) {
	v := r.URL.Query().Get(k)
	if v == "" {
		return d, nil
	}
	n, e := strconv.Atoi(v)
	if e != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", k)
	}
	return n, nil
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, code string, e error) {
	detail := ""
	if e != nil {
		detail = e.Error()
	}
	write(w, status, map[string]any{"code": code, "message": detail, "detail": detail})
}
