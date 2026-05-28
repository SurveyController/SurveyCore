package models

import "time"

// ProxyLease represents a proxy IP lease.
type ProxyLease struct {
	Address  string  `json:"address"`
	ExpireAt string  `json:"expire_at,omitempty"`
	ExpireTS float64 `json:"expire_ts,omitempty"`
	Poolable bool    `json:"poolable"`
	Source   string  `json:"source,omitempty"`
}

// IsExpired checks if the proxy lease has expired.
func (p *ProxyLease) IsExpired() bool {
	if p.ExpireTS <= 0 {
		return false
	}
	return time.Now().Unix() > int64(p.ExpireTS)
}

// HasSufficientTTL checks if the proxy has enough remaining TTL.
func (p *ProxyLease) HasSufficientTTL(minSeconds float64) bool {
	if p.ExpireTS <= 0 {
		return true // no expiry set = infinite
	}
	return float64(p.ExpireTS)-float64(time.Now().Unix()) > minSeconds
}

// RandomIPSession holds the state for a random IP session.
type RandomIPSession struct {
	DeviceID       string  `json:"device_id"`
	UserID         int     `json:"user_id"`
	RemainingQuota float64 `json:"remaining_quota"`
	TotalQuota     float64 `json:"total_quota"`
	UsedQuota      float64 `json:"used_quota"`
	QuotaKnown     bool    `json:"quota_known"`
}

// IsQuotaExhausted returns true if the used quota meets or exceeds total.
func (s *RandomIPSession) IsQuotaExhausted() bool {
	if !s.QuotaKnown {
		return false
	}
	return s.UsedQuota >= s.TotalQuota
}
