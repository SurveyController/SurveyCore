package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/SurveyController/SurveyController-Go/internal/models"
)

// Pool manages a pool of proxy leases.
type Pool struct {
	mu       sync.RWMutex
	leases   []models.ProxyLease
	cooldown map[string]time.Time // address -> cooldown until
	source   string
	apiURL   string
}

// NewPool creates a new proxy pool.
func NewPool(source, apiURL string) *Pool {
	return &Pool{
		leases:   make([]models.ProxyLease, 0),
		cooldown: make(map[string]time.Time),
		source:   source,
		apiURL:   apiURL,
	}
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

// Push adds a proxy lease back to the pool.
func (p *Pool) Push(lease models.ProxyLease) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.leases = append(p.leases, lease)
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
	case "default", "benefit":
		return fetchFromOfficial(source, count)
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

var ipPortRe = regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}:\d{2,5}`)

func fetchFromOfficial(source string, count int) ([]models.ProxyLease, error) {
	// Placeholder: In production, this would call the actual proxy API
	// For now, return empty - the real implementation requires API credentials
	return nil, fmt.Errorf("官方代理 API 暂未配置，请设置 custom_proxy_api")
}

func fetchFromCustom(apiURL string, count int) ([]models.ProxyLease, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("获取代理失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取代理响应失败: %w", err)
	}

	text := string(body)
	matches := ipPortRe.FindAllString(text, -1)
	if len(matches) == 0 {
		// Try JSON parsing
		return parseProxyFromJSON(text)
	}

	leases := make([]models.ProxyLease, 0, len(matches))
	for _, addr := range matches {
		leases = append(leases, models.ProxyLease{
			Address:  addr,
			Poolable: true,
			Source:   "custom",
		})
	}
	return leases, nil
}

func parseProxyFromJSON(text string) ([]models.ProxyLease, error) {
	// Try parsing as a JSON array of strings
	var arr []string
	if err := json.Unmarshal([]byte(text), &arr); err == nil {
		leases := make([]models.ProxyLease, 0, len(arr))
		for _, addr := range arr {
			if ipPortRe.MatchString(addr) {
				leases = append(leases, models.ProxyLease{
					Address:  addr,
					Poolable: true,
					Source:   "custom",
				})
			}
		}
		return leases, nil
	}

	// Try parsing as JSON object with items/data/proxies field
	var obj map[string]any
	if err := json.Unmarshal([]byte(text), &obj); err == nil {
		for _, key := range []string{"items", "data", "proxies", "list"} {
			if arr, ok := obj[key]; ok {
				return extractProxiesFromAny(arr)
			}
		}
	}

	return nil, fmt.Errorf("无法从响应中解析代理地址")
}

func extractProxiesFromAny(v any) ([]models.ProxyLease, error) {
	switch arr := v.(type) {
	case []any:
		leases := make([]models.ProxyLease, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok && ipPortRe.MatchString(s) {
				leases = append(leases, models.ProxyLease{
					Address:  s,
					Poolable: true,
					Source:   "custom",
				})
			}
			if m, ok := item.(map[string]any); ok {
				if ip, ok := m["ip"].(string); ok {
					port := ""
					if p, ok := m["port"].(string); ok {
						port = p
					} else if p, ok := m["port"].(float64); ok {
						port = fmt.Sprintf("%.0f", p)
					}
					if ip != "" && port != "" {
						leases = append(leases, models.ProxyLease{
							Address:  ip + ":" + port,
							Poolable: true,
							Source:   "custom",
						})
					}
				}
			}
		}
		return leases, nil
	}
	return nil, fmt.Errorf("无法解析代理数据")
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
