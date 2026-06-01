package proxy

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/SurveyController/SurveyCore/internal/models"
)

// Pool manages a pool of proxy leases.
type Pool struct {
	mu       sync.RWMutex
	leases   []models.ProxyLease
	cooldown map[string]time.Time // address -> cooldown until
	source   string
	apiURL   string
}

// Option configures a proxy pool.
type Option func(*Pool)

// NewPool creates a new proxy pool.
func NewPool(source, apiURL string, opts ...Option) *Pool {
	p := &Pool{
		leases:   make([]models.ProxyLease, 0),
		cooldown: make(map[string]time.Time),
		source:   source,
		apiURL:   apiURL,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

// Pop removes and returns the next available proxy lease.
// Returns nil if no proxy is available.
func (p *Pool) Pop() *models.ProxyLease {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	for i, lease := range p.leases {
		// Skip expired
		if lease.ExpireTS > 0 && now.Unix() > int64(lease.ExpireTS) {
			continue
		}
		// Skip in cooldown
		if until, ok := p.cooldown[lease.Address]; ok && now.Before(until) {
			continue
		}
		// Found a valid lease
		p.leases = append(p.leases[:i], p.leases[i+1:]...)
		return &lease
	}
	return nil
}

// MarkBad puts a proxy address into cooldown for 180 seconds.
func (p *Pool) MarkBad(address string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cooldown[address] = time.Now().Add(180 * time.Second)
}

// Size returns the number of available leases.
func (p *Pool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.leases)
}

// FetchBatch fetches a batch of proxy leases from the configured source.
func (p *Pool) FetchBatch(count int) ([]models.ProxyLease, error) {
	p.mu.RLock()
	source := p.source
	apiURL := p.apiURL
	p.mu.RUnlock()

	switch source {
	case "custom":
		if apiURL == "" {
			return nil, fmt.Errorf("自定义代理 API URL 未配置")
		}
		return fetchFromCustom(apiURL, count)
	default:
		return nil, fmt.Errorf("不支持的代理源: %s", source)
	}
}

// AddLeases adds fetched leases to the pool.
func (p *Pool) AddLeases(leases []models.ProxyLease) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.leases = append(p.leases, leases...)
}

// CleanupExpired removes expired leases and cooldowns.
func (p *Pool) CleanupExpired() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	valid := p.leases[:0]
	for _, lease := range p.leases {
		if lease.ExpireTS <= 0 || now.Unix() <= int64(lease.ExpireTS) {
			valid = append(valid, lease)
		}
	}
	p.leases = valid

	// Cleanup cooldowns
	for addr, until := range p.cooldown {
		if now.After(until) {
			delete(p.cooldown, addr)
		}
	}
}

// ExtractProxyAddress extracts a clean proxy address string.
func ExtractProxyAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	// Add protocol if missing
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		return "http://" + addr
	}
	return addr
}
