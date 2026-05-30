package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SurveyController/SurveyConsole/internal/models"
)

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
