package domain

import "time"

type ProxyLease struct {
	Address  string  `json:"address"`
	ExpireAt string  `json:"expire_at,omitempty"`
	ExpireTS float64 `json:"expire_ts,omitempty"`
	Poolable bool    `json:"poolable"`
	Source   string  `json:"source,omitempty"`
}

func (p *ProxyLease) IsExpired() bool {
	if p.ExpireTS <= 0 {
		return false
	}
	return time.Now().Unix() > int64(p.ExpireTS)
}

func (p *ProxyLease) HasSufficientTTL(minSeconds float64) bool {
	if p.ExpireTS <= 0 {
		return true
	}
	return float64(p.ExpireTS)-float64(time.Now().Unix()) > minSeconds
}
