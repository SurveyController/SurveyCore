package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SurveyController/SurveyCore/internal/models"
)

const (
	defaultTrialEndpoint      = "https://api-wjx.hungrym0.top/api/auth/trial"
	defaultBonusEndpoint      = "https://api-wjx.hungrym0.top/api/bonus"
	defaultCardRedeemEndpoint = "https://api-wjx.hungrym0.top/api/cards/redeem"
	defaultSessionPath        = "data/random_ip_session.json"
)

// RandomIPAuthError carries the server detail field used by the Python client.
type RandomIPAuthError struct {
	Detail            string
	StatusCode        int
	RetryAfterSeconds int
}

func (e *RandomIPAuthError) Error() string {
	if e == nil {
		return "随机 IP 请求失败"
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("随机 IP %s (HTTP %d)", e.Detail, e.StatusCode)
	}
	return fmt.Sprintf("随机 IP %s", e.Detail)
}

// RandomIPEndpoints groups official random-IP service endpoints.
type RandomIPEndpoints struct {
	TrialEndpoint      string
	BonusEndpoint      string
	CardRedeemEndpoint string
}

// RandomIPSnapshot is the API-facing session snapshot.
type RandomIPSnapshot struct {
	Authenticated  bool    `json:"authenticated"`
	DeviceID       string  `json:"device_id"`
	UserID         int     `json:"user_id"`
	RemainingQuota float64 `json:"remaining_quota"`
	TotalQuota     float64 `json:"total_quota"`
	UsedQuota      float64 `json:"used_quota"`
	QuotaKnown     bool    `json:"quota_known"`
	HasValidUserID bool    `json:"has_valid_user_id"`
	SessionState   string  `json:"session_state"`
}

// RandomIPService persists and syncs the official random-IP session.
type RandomIPService struct {
	mu        sync.Mutex
	path      string
	endpoints RandomIPEndpoints
	client    *http.Client
	loaded    bool
	session   models.RandomIPSession
}

var defaultRandomIPService = NewRandomIPService("", RandomIPEndpoints{})

// DefaultRandomIPService returns the process-wide random-IP session service.
func DefaultRandomIPService() *RandomIPService {
	return defaultRandomIPService
}

// NewRandomIPService creates a random-IP session service.
func NewRandomIPService(path string, endpoints RandomIPEndpoints) *RandomIPService {
	if strings.TrimSpace(path) == "" {
		path = firstEnv("SURVEYCORE_RANDOM_IP_SESSION_PATH", "RANDOM_IP_SESSION_PATH")
	}
	if strings.TrimSpace(path) == "" {
		path = defaultSessionPath
	}
	endpoints = normalizeRandomIPEndpoints(endpoints)
	return &RandomIPService{
		path:      path,
		endpoints: endpoints,
		client:    &http.Client{Timeout: 12 * time.Second},
	}
}

// Snapshot returns the current local session, generating a stable device ID when needed.
func (s *RandomIPService) Snapshot() (RandomIPSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoadedLocked(); err != nil {
		return RandomIPSnapshot{}, err
	}
	return snapshotFromSession(s.session), nil
}

// AuthenticatedSnapshot returns a loaded or persisted authenticated session without creating a new device ID.
func (s *RandomIPService) AuthenticatedSnapshot() (RandomIPSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.loaded {
		data, err := os.ReadFile(s.path)
		if err != nil {
			return RandomIPSnapshot{}, false
		}
		if err := json.Unmarshal(data, &s.session); err != nil {
			return RandomIPSnapshot{}, false
		}
		s.session.RemainingQuota, s.session.TotalQuota, s.session.UsedQuota = normalizeQuotaState(s.session.RemainingQuota, s.session.TotalQuota, s.session.UsedQuota)
		s.session.QuotaKnown = normalizeQuotaKnown(s.session)
		s.loaded = true
	}
	if s.session.UserID <= 0 || strings.TrimSpace(s.session.DeviceID) == "" {
		return RandomIPSnapshot{}, false
	}
	return snapshotFromSession(s.session), true
}

// ActivateTrial claims or refreshes the trial session.
func (s *RandomIPService) ActivateTrial(ctx context.Context) (RandomIPSnapshot, error) {
	session, err := s.postSession(ctx, s.endpoints.TrialEndpoint, map[string]any{}, nil)
	if err != nil {
		return RandomIPSnapshot{}, err
	}
	if err := s.setSession(session); err != nil {
		return RandomIPSnapshot{}, err
	}
	return snapshotFromSession(session), nil
}

// SyncQuota refreshes quota numbers for the authenticated session.
func (s *RandomIPService) SyncQuota(ctx context.Context) (RandomIPSnapshot, error) {
	current, err := s.requireSession()
	if err != nil {
		return RandomIPSnapshot{}, err
	}
	session, err := s.postSession(ctx, s.endpoints.TrialEndpoint, map[string]any{}, &current)
	if err != nil {
		return RandomIPSnapshot{}, err
	}
	if err := s.setSession(session); err != nil {
		return RandomIPSnapshot{}, err
	}
	return snapshotFromSession(session), nil
}

// ClaimBonus claims the Easter egg bonus and updates local quota.
func (s *RandomIPService) ClaimBonus(ctx context.Context) (map[string]any, error) {
	current, err := s.requireSession()
	if err != nil {
		return nil, err
	}
	data, err := s.postJSON(ctx, s.endpoints.BonusEndpoint, map[string]any{
		"user_id":    current.UserID,
		"bonus_code": "fuck-you-hacker",
	})
	if err != nil {
		return nil, err
	}
	updated := applyQuotaPayload(current, data)
	if err := s.setSession(updated); err != nil {
		return nil, err
	}
	return map[string]any{
		"claimed":         boolFromAny(data["claimed"]),
		"bonus_quota":     nonNegativeFloat(data["bonus_quota"], 0),
		"detail":          strings.TrimSpace(fmt.Sprint(data["detail"])),
		"used_quota":      updated.UsedQuota,
		"remaining_quota": updated.RemainingQuota,
		"total_quota":     updated.TotalQuota,
	}, nil
}

// RedeemCard redeems a quota card and updates local quota.
func (s *RandomIPService) RedeemCard(ctx context.Context, cardCode string) (map[string]any, error) {
	current, err := s.requireSession()
	if err != nil {
		return nil, err
	}
	data, err := s.postJSON(ctx, s.endpoints.CardRedeemEndpoint, map[string]any{
		"user_id":   current.UserID,
		"card_code": strings.TrimSpace(cardCode),
	})
	if err != nil {
		return nil, err
	}
	updated := applyQuotaPayload(current, data)
	if err := s.setSession(updated); err != nil {
		return nil, err
	}
	return map[string]any{
		"redeemed":        boolFromAny(data["redeemed"]),
		"card_quota":      nonNegativeFloat(data["card_quota"], 0),
		"detail":          strings.TrimSpace(fmt.Sprint(data["detail"])),
		"used_quota":      updated.UsedQuota,
		"remaining_quota": updated.RemainingQuota,
		"total_quota":     updated.TotalQuota,
	}, nil
}

func (s *RandomIPService) requireSession() (models.RandomIPSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoadedLocked(); err != nil {
		return models.RandomIPSession{}, err
	}
	if s.session.UserID <= 0 {
		return models.RandomIPSession{}, &RandomIPAuthError{Detail: "not_authenticated"}
	}
	return s.session, nil
}

func (s *RandomIPService) setSession(session models.RandomIPSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoadedLocked(); err != nil {
		return err
	}
	session.DeviceID = strings.TrimSpace(session.DeviceID)
	if session.DeviceID == "" {
		session.DeviceID = s.session.DeviceID
	}
	session.RemainingQuota, session.TotalQuota, session.UsedQuota = normalizeQuotaState(session.RemainingQuota, session.TotalQuota, session.UsedQuota)
	session.QuotaKnown = normalizeQuotaKnown(session)
	s.session = session
	return s.persistLocked()
}

func (s *RandomIPService) ensureLoadedLocked() error {
	if s.loaded {
		return nil
	}
	if data, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(data, &s.session)
	} else if !os.IsNotExist(err) {
		return err
	}
	if strings.TrimSpace(s.session.DeviceID) == "" {
		deviceID, err := generateDeviceID()
		if err != nil {
			return err
		}
		s.session.DeviceID = deviceID
	}
	s.session.RemainingQuota, s.session.TotalQuota, s.session.UsedQuota = normalizeQuotaState(s.session.RemainingQuota, s.session.TotalQuota, s.session.UsedQuota)
	s.session.QuotaKnown = normalizeQuotaKnown(s.session)
	s.loaded = true
	return s.persistLocked()
}

func (s *RandomIPService) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

func (s *RandomIPService) postSession(ctx context.Context, url string, body map[string]any, fallback *models.RandomIPSession) (models.RandomIPSession, error) {
	data, err := s.postJSON(ctx, url, body)
	if err != nil {
		return models.RandomIPSession{}, err
	}
	deviceID := ""
	if fallback != nil {
		deviceID = fallback.DeviceID
	}
	if deviceID == "" {
		s.mu.Lock()
		if err := s.ensureLoadedLocked(); err == nil {
			deviceID = s.session.DeviceID
		}
		s.mu.Unlock()
	}
	return parseRandomIPSessionPayload(data, deviceID, fallback)
}

func (s *RandomIPService) postJSON(ctx context.Context, url string, body map[string]any) (map[string]any, error) {
	s.mu.Lock()
	if err := s.ensureLoadedLocked(); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	deviceID := s.session.DeviceID
	s.mu.Unlock()

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Device-ID", deviceID)
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, &RandomIPAuthError{Detail: "network_error:" + err.Error()}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &RandomIPAuthError{Detail: "network_error:" + err.Error()}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseRandomIPError(resp, respBody)
	}
	var data map[string]any
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, &RandomIPAuthError{Detail: "invalid_response:" + err.Error()}
	}
	return data, nil
}

func parseRandomIPSessionPayload(data map[string]any, deviceID string, fallback *models.RandomIPSession) (models.RandomIPSession, error) {
	userID := nonNegativeInt(data["user_id"], 0)
	if userID <= 0 {
		return models.RandomIPSession{}, &RandomIPAuthError{Detail: "invalid_response:user_id_invalid"}
	}
	fallbackSession := models.RandomIPSession{DeviceID: deviceID}
	if fallback != nil {
		fallbackSession = *fallback
	}
	remaining, total, used, known := resolveQuotaFromPayload(data, fallbackSession)
	return models.RandomIPSession{
		DeviceID:       deviceID,
		UserID:         userID,
		RemainingQuota: remaining,
		TotalQuota:     total,
		UsedQuota:      used,
		QuotaKnown:     known,
	}, nil
}

func parseRandomIPError(resp *http.Response, body []byte) *RandomIPAuthError {
	retryAfter := nonNegativeInt(resp.Header.Get("Retry-After"), 0)
	detail := ""
	var data map[string]any
	if err := json.Unmarshal(body, &data); err == nil {
		detail = strings.TrimSpace(fmt.Sprint(data["detail"]))
		retryAfter = max(retryAfter, nonNegativeInt(data["retry_after_seconds"], retryAfter))
	}
	if detail == "" {
		detail = fmt.Sprintf("http_%d", resp.StatusCode)
	}
	return &RandomIPAuthError{Detail: detail, StatusCode: resp.StatusCode, RetryAfterSeconds: retryAfter}
}

func resolveQuotaFromPayload(data map[string]any, fallback models.RandomIPSession) (float64, float64, float64, bool) {
	fallbackRemaining, fallbackTotal, fallbackUsed := normalizeQuotaState(fallback.RemainingQuota, fallback.TotalQuota, fallback.UsedQuota)
	remaining, okRemaining := optionalNonNegativeFloat(data["remaining_quota"])
	total, okTotal := optionalNonNegativeFloat(data["total_quota"])
	used, okUsed := optionalNonNegativeFloat(data["used_quota"])
	validCount := 0
	for _, ok := range []bool{okRemaining, okTotal, okUsed} {
		if ok {
			validCount++
		}
	}
	if validCount >= 2 {
		r, t, u := normalizeQuotaState(optionalOr(remaining, okRemaining, 0), optionalOr(total, okTotal, fallbackTotal), optionalOr(used, okUsed, 0))
		return r, t, u, canTrustQuota(t, u)
	}
	if validCount == 1 && fallback.QuotaKnown {
		switch {
		case okRemaining:
			r, t, u := normalizeQuotaState(remaining, fallbackTotal, 0)
			return r, t, u, canTrustQuota(t, u)
		case okTotal:
			r, t, u := normalizeQuotaState(0, total, fallbackUsed)
			return r, t, u, canTrustQuota(t, u)
		case okUsed:
			r, t, u := normalizeQuotaState(0, fallbackTotal, used)
			return r, t, u, canTrustQuota(t, u)
		}
	}
	return fallbackRemaining, fallbackTotal, fallbackUsed, false
}

func applyQuotaPayload(session models.RandomIPSession, data map[string]any) models.RandomIPSession {
	remaining, total, used, known := resolveQuotaFromPayload(data, session)
	session.RemainingQuota = remaining
	session.TotalQuota = total
	session.UsedQuota = used
	session.QuotaKnown = known
	return session
}

func snapshotFromSession(session models.RandomIPSession) RandomIPSnapshot {
	remaining, total, used := normalizeQuotaState(session.RemainingQuota, session.TotalQuota, session.UsedQuota)
	authenticated := session.UserID > 0
	state := "anonymous"
	if authenticated {
		state = "ready"
	}
	return RandomIPSnapshot{
		Authenticated:  authenticated,
		DeviceID:       session.DeviceID,
		UserID:         session.UserID,
		RemainingQuota: remaining,
		TotalQuota:     total,
		UsedQuota:      used,
		QuotaKnown:     normalizeQuotaKnown(session),
		HasValidUserID: session.UserID > 0,
		SessionState:   state,
	}
}

func normalizeRandomIPEndpoints(endpoints RandomIPEndpoints) RandomIPEndpoints {
	if strings.TrimSpace(endpoints.TrialEndpoint) == "" {
		endpoints.TrialEndpoint = firstEnv("AUTH_TRIAL_ENDPOINT", "WJX_AUTH_TRIAL_ENDPOINT")
	}
	if strings.TrimSpace(endpoints.BonusEndpoint) == "" {
		endpoints.BonusEndpoint = firstEnv("AUTH_BONUS_CLAIM_ENDPOINT", "WJX_AUTH_BONUS_CLAIM_ENDPOINT")
	}
	if strings.TrimSpace(endpoints.CardRedeemEndpoint) == "" {
		endpoints.CardRedeemEndpoint = firstEnv("CARD_REDEEM_ENDPOINT", "WJX_CARD_REDEEM_ENDPOINT")
	}
	if strings.TrimSpace(endpoints.TrialEndpoint) == "" {
		endpoints.TrialEndpoint = defaultTrialEndpoint
	}
	if strings.TrimSpace(endpoints.BonusEndpoint) == "" {
		endpoints.BonusEndpoint = defaultBonusEndpoint
	}
	if strings.TrimSpace(endpoints.CardRedeemEndpoint) == "" {
		endpoints.CardRedeemEndpoint = defaultCardRedeemEndpoint
	}
	return endpoints
}

func generateDeviceID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "surveycore-" + hex.EncodeToString(buf), nil
}

func normalizeQuotaState(remainingQuota, totalQuota, usedQuota float64) (float64, float64, float64) {
	remaining := maxFloat(0, remainingQuota)
	total := maxFloat(0, totalQuota)
	used := maxFloat(0, usedQuota)
	if used > 0 {
		total = maxFloat(total, used)
		remaining = maxFloat(0, total-used)
		return remaining, total, used
	}
	if remaining > 0 {
		total = maxFloat(total, remaining)
		used = maxFloat(0, total-remaining)
		return remaining, total, used
	}
	return total, total, 0
}

func normalizeQuotaKnown(session models.RandomIPSession) bool {
	return session.UserID > 0 && canTrustQuota(session.TotalQuota, session.UsedQuota)
}

func canTrustQuota(total, used float64) bool {
	return total > 0 || used > 0
}

func optionalNonNegativeFloat(value any) (float64, bool) {
	if value == nil {
		return 0, false
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil || parsed < 0 {
		return 0, false
	}
	return parsed, true
}

func nonNegativeFloat(value any, defaultValue float64) float64 {
	parsed, ok := optionalNonNegativeFloat(value)
	if !ok {
		return maxFloat(0, defaultValue)
	}
	return parsed
}

func nonNegativeInt(value any, defaultValue int) int {
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return max(0, defaultValue)
	}
	parsed, err := strconv.Atoi(text)
	if err != nil || parsed < 0 {
		return max(0, defaultValue)
	}
	return parsed
}

func optionalOr(value float64, ok bool, fallback float64) float64 {
	if ok {
		return value
	}
	return fallback
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
