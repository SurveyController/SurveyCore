package api

import (
	"errors"
	"net/http"

	"github.com/SurveyController/SurveyCore/internal/network/proxy"
)

type randomIPRedeemRequest struct {
	CardCode string `json:"card_code"`
}

func (s *Server) handleGetRandomIPSession(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.randomIP.Snapshot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "随机 IP 会话读取失败", err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleActivateRandomIPTrial(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.randomIP.ActivateTrial(r.Context())
	if err != nil {
		writeRandomIPError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleSyncRandomIPQuota(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.randomIP.SyncQuota(r.Context())
	if err != nil {
		writeRandomIPError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleRedeemRandomIPCard(w http.ResponseWriter, r *http.Request) {
	var req randomIPRedeemRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "JSON 请求体无效", err)
		return
	}
	result, err := s.randomIP.RedeemCard(r.Context(), req.CardCode)
	if err != nil {
		writeRandomIPError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleClaimRandomIPBonus(w http.ResponseWriter, r *http.Request) {
	result, err := s.randomIP.ClaimBonus(r.Context())
	if err != nil {
		writeRandomIPError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeRandomIPError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	code := "random_ip_upstream_error"
	message := "随机 IP 服务调用失败"
	var authErr *proxy.RandomIPAuthError
	if errors.As(err, &authErr) {
		if authErr.Detail == "not_authenticated" {
			status = http.StatusUnauthorized
			code = "random_ip_not_authenticated"
			message = "随机 IP 尚未认证"
		} else if authErr.StatusCode >= 400 && authErr.StatusCode < 500 {
			status = http.StatusBadRequest
			code = "random_ip_auth_error"
		}
	}
	writeError(w, status, code, message, err)
}
