package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SurveyController/SurveyConsole/internal/models"
)

// Pool manages a pool of proxy leases.
type Pool struct {
	mu              sync.RWMutex
	leases          []models.ProxyLease
	cooldown        map[string]time.Time // address -> cooldown until
	source          string
	apiURL          string
	officialOptions officialOptions
}

type officialOptions struct {
	Endpoint string
	UserID   int
	DeviceID string
	AreaCode string
	Minute   int
	Pool     string
}

// Option configures a proxy pool.
type Option func(*Pool)

// WithOfficialEndpoint overrides the official random-IP endpoint.
func WithOfficialEndpoint(endpoint string) Option {
	return func(p *Pool) {
		p.officialOptions.Endpoint = strings.TrimSpace(endpoint)
	}
}

// WithOfficialCredentials sets official random-IP session credentials.
func WithOfficialCredentials(userID int, deviceID string) Option {
	return func(p *Pool) {
		p.officialOptions.UserID = userID
		p.officialOptions.DeviceID = strings.TrimSpace(deviceID)
	}
}

// WithOfficialAreaCode sets the official random-IP area code.
func WithOfficialAreaCode(areaCode string) Option {
	return func(p *Pool) {
		p.officialOptions.AreaCode = normalizeAreaCode(areaCode)
	}
}

// WithOfficialMinute sets the official random-IP lease minute.
func WithOfficialMinute(minute int) Option {
	return func(p *Pool) {
		if isAllowedMinute(minute) {
			p.officialOptions.Minute = minute
		}
	}
}

// NewPool creates a new proxy pool.
func NewPool(source, apiURL string, opts ...Option) *Pool {
	p := &Pool{
		leases:   make([]models.ProxyLease, 0),
		cooldown: make(map[string]time.Time),
		source:   source,
		apiURL:   apiURL,
		officialOptions: officialOptions{
			Endpoint: defaultOfficialEndpoint(),
			UserID:   officialUserIDFromEnv(),
			DeviceID: firstEnv("WJX_RANDOM_IP_DEVICE_ID", "RANDOM_IP_DEVICE_ID"),
			AreaCode: normalizeAreaCode(firstEnv("WJX_PROXY_AREA_CODE", "PROXY_AREA_CODE")),
			Minute:   1,
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	if p.officialOptions.Pool == "" {
		p.officialOptions.Pool = resolveOfficialPool(p.officialOptions.AreaCode)
	}
	if p.officialOptions.Minute <= 0 {
		p.officialOptions.Minute = 1
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
	officialOpts := p.officialOptions
	p.mu.RUnlock()

	switch source {
	case "default", "benefit":
		return fetchFromOfficial(source, count, officialOpts)
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

var ipPortRe = regexp.MustCompile(`(?:https?://)?(?:([^\s:@/,\[\]"]+):([^\s:@/,\[\]"]+)@)?((?:\d{1,3}\.){3}\d{1,3}):(\d{2,5})`)

func fetchFromOfficial(source string, count int, opts officialOptions) ([]models.ProxyLease, error) {
	count = normalizeProxyCount(count)
	opts = normalizeOfficialOptions(opts)
	if opts.Endpoint == "" {
		return nil, fmt.Errorf("官方随机 IP 提取接口未配置")
	}
	if opts.UserID <= 0 {
		return nil, fmt.Errorf("官方随机 IP 用户 ID 未配置，请设置 -random-ip-user-id 或 WJX_RANDOM_IP_USER_ID")
	}
	if opts.DeviceID == "" {
		return nil, fmt.Errorf("官方随机 IP 设备 ID 未配置，请设置 -random-ip-device-id 或 WJX_RANDOM_IP_DEVICE_ID")
	}

	upstream := "default"
	if source == "benefit" {
		upstream = "idiot"
		opts.Minute = 1
	}
	body := map[string]any{
		"user_id":  opts.UserID,
		"minute":   opts.Minute,
		"pool":     opts.Pool,
		"upstream": upstream,
	}
	if count > 1 {
		body["num"] = count
	}
	if opts.AreaCode != "" {
		body["area"] = opts.AreaCode
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, opts.Endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Device-ID", opts.DeviceID)
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("官方随机 IP 请求失败: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取官方随机 IP 响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("官方随机 IP HTTP %d: %s", resp.StatusCode, officialErrorDetail(respBody))
	}

	leases, err := parseOfficialProxyPayload(respBody, source)
	if err != nil {
		return nil, err
	}
	if len(leases) > count {
		leases = leases[:count]
	}
	return leases, nil
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
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("代理 API HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	text := string(body)
	if leases := buildProxyLeases(findProxyAddressesInText(text), "custom"); len(leases) > 0 {
		return leases, nil
	}

	return parseProxyFromJSON(text)
}

func parseProxyFromJSON(text string) ([]models.ProxyLease, error) {
	var data any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w", err)
	}
	addresses := make([]string, 0)
	recursiveFindProxies(data, &addresses, 0)
	leases := buildProxyLeases(addresses, "custom")
	if len(leases) == 0 {
		return nil, fmt.Errorf("无法从响应中解析代理地址")
	}
	return leases, nil
}

func recursiveFindProxies(data any, results *[]string, depth int) {
	if depth > 10 {
		return
	}
	switch value := data.(type) {
	case map[string]any:
		if proxy := extractProxyFromMap(value); proxy != "" {
			*results = append(*results, proxy)
			return
		}
		for _, child := range value {
			recursiveFindProxies(child, results, depth+1)
		}
	case []any:
		for _, item := range value {
			recursiveFindProxies(item, results, depth+1)
		}
	case string:
		if proxy := extractProxyFromString(value); proxy != "" {
			*results = append(*results, proxy)
		}
	}
}

func extractProxyFromMap(obj map[string]any) string {
	host := firstString(obj, "ip", "IP", "host", "address", "proxy")
	port := firstString(obj, "port", "Port", "PORT")
	if host != "" && port != "" && !strings.Contains(host, ":") {
		user := firstString(obj, "account", "username", "user")
		password := firstString(obj, "password", "pwd", "pass")
		if user != "" && password != "" {
			return fmt.Sprintf("%s:%s@%s:%s", user, password, host, port)
		}
		return host + ":" + port
	}
	for _, value := range obj {
		if text, ok := value.(string); ok {
			if proxy := extractProxyFromString(text); proxy != "" {
				return proxy
			}
		}
	}
	return ""
}

func firstString(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := obj[key]; ok {
			switch typed := value.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					return strings.TrimSpace(typed)
				}
			case float64:
				if typed > 0 {
					return fmt.Sprintf("%.0f", typed)
				}
			case int:
				if typed > 0 {
					return fmt.Sprintf("%d", typed)
				}
			}
		}
	}
	return ""
}

func findProxyAddressesInText(text string) []string {
	matches := ipPortRe.FindAllStringSubmatch(text, -1)
	results := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 5 {
			continue
		}
		user, password, host, port := match[1], match[2], match[3], match[4]
		if user != "" && password != "" {
			results = append(results, fmt.Sprintf("%s:%s@%s:%s", user, password, host, port))
			continue
		}
		results = append(results, host+":"+port)
	}
	return results
}

func extractProxyFromString(text string) string {
	addresses := findProxyAddressesInText(text)
	if len(addresses) == 0 {
		return ""
	}
	return addresses[0]
}

func buildProxyLeases(addresses []string, source string) []models.ProxyLease {
	seen := make(map[string]bool)
	leases := make([]models.ProxyLease, 0, len(addresses))
	for _, addr := range addresses {
		addr = strings.TrimSpace(addr)
		if addr == "" || seen[addr] {
			continue
		}
		seen[addr] = true
		leases = append(leases, models.ProxyLease{
			Address:  addr,
			Poolable: true,
			Source:   source,
		})
	}
	return leases
}

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"

func parseOfficialProxyPayload(body []byte, fallbackSource string) ([]models.ProxyLease, error) {
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("解析官方随机 IP 响应失败: %w", err)
	}
	source := officialSourceFromProvider(firstString(data, "provider"), fallbackSource)
	if rawItems, ok := data["items"].([]any); ok {
		leases := make([]models.ProxyLease, 0, len(rawItems))
		for _, raw := range rawItems {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if lease, ok := officialLeaseFromMap(item, source); ok {
				leases = append(leases, lease)
			}
		}
		if len(leases) == 0 {
			return nil, fmt.Errorf("官方随机 IP 批量响应中无有效代理")
		}
		return leases, nil
	}
	if lease, ok := officialLeaseFromMap(data, source); ok {
		return []models.ProxyLease{lease}, nil
	}
	return nil, fmt.Errorf("官方随机 IP 响应缺少 host/port/account/password")
}

func officialLeaseFromMap(data map[string]any, source string) (models.ProxyLease, bool) {
	host := firstString(data, "host", "ip", "IP")
	port := firstString(data, "port", "Port", "PORT")
	account := firstString(data, "account", "username", "user")
	password := firstString(data, "password", "pwd", "pass")
	if host == "" || port == "" || account == "" || password == "" {
		return models.ProxyLease{}, false
	}
	expireAt := firstString(data, "expire_at", "expireAt", "expire")
	return models.ProxyLease{
		Address:  fmt.Sprintf("%s:%s@%s:%s", account, password, host, port),
		ExpireAt: expireAt,
		ExpireTS: parseExpireAtToUnix(expireAt),
		Poolable: expireAt != "",
		Source:   source,
	}, true
}

func officialSourceFromProvider(provider, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "idiot", "benefit":
		return "benefit"
	case "default":
		return "default"
	default:
		if strings.TrimSpace(fallback) != "" {
			return strings.TrimSpace(fallback)
		}
		return "default"
	}
}

func officialErrorDetail(body []byte) string {
	var data map[string]any
	if err := json.Unmarshal(body, &data); err == nil {
		for _, key := range []string{"detail", "message", "error"} {
			if value := firstString(data, key); value != "" {
				return value
			}
		}
	}
	return truncate(string(body), 200)
}

func parseExpireAtToUnix(value string) float64 {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, text)
		if err == nil {
			return float64(parsed.Unix())
		}
	}
	return 0
}

func normalizeOfficialOptions(opts officialOptions) officialOptions {
	opts.Endpoint = strings.TrimSpace(opts.Endpoint)
	if opts.Endpoint == "" {
		opts.Endpoint = defaultOfficialEndpoint()
	}
	if opts.UserID <= 0 {
		opts.UserID = officialUserIDFromEnv()
	}
	if opts.DeviceID == "" {
		opts.DeviceID = firstEnv("WJX_RANDOM_IP_DEVICE_ID", "RANDOM_IP_DEVICE_ID")
	}
	opts.AreaCode = normalizeAreaCode(opts.AreaCode)
	if !isAllowedMinute(opts.Minute) {
		opts.Minute = 1
	}
	if opts.Pool == "" {
		opts.Pool = resolveOfficialPool(opts.AreaCode)
	}
	return opts
}

func normalizeProxyCount(count int) int {
	if count <= 0 {
		return 1
	}
	if count > 80 {
		return 80
	}
	return count
}

func defaultOfficialEndpoint() string {
	if value := firstEnv("IP_EXTRACT_ENDPOINT", "WJX_IP_EXTRACT_ENDPOINT"); value != "" {
		return value
	}
	return "https://api-wjx.hungrym0.top/api/ip/extract"
}

func officialUserIDFromEnv() int {
	value := firstEnv("WJX_RANDOM_IP_USER_ID", "RANDOM_IP_USER_ID")
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func isAllowedMinute(minute int) bool {
	switch minute {
	case 1, 3, 5, 10, 15, 30:
		return true
	default:
		return false
	}
}

func normalizeAreaCode(areaCode string) string {
	areaCode = strings.TrimSpace(areaCode)
	if len(areaCode) != 6 {
		return ""
	}
	for _, ch := range areaCode {
		if ch < '0' || ch > '9' {
			return ""
		}
	}
	return areaCode
}

func resolveOfficialPool(areaCode string) string {
	if areaCode == "" {
		return "ordinary"
	}
	if strings.HasSuffix(areaCode, "0000") && ordinaryPoolProvinceCodes[areaCode] {
		return "ordinary"
	}
	return "quality"
}

var ordinaryPoolProvinceCodes = map[string]bool{
	"110000": true, "120000": true, "130000": true, "140000": true, "150000": true,
	"210000": true, "220000": true, "230000": true, "320000": true, "330000": true,
	"340000": true, "350000": true, "360000": true, "370000": true, "410000": true,
	"420000": true, "430000": true, "440000": true, "460000": true, "500000": true,
	"510000": true, "610000": true, "620000": true, "640000": true,
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
