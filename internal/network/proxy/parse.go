package proxy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/SurveyController/SurveyConsole/internal/models"
)

var ipPortRe = regexp.MustCompile(`(?:https?://)?(?:([^\s:@/,\[\]"]+):([^\s:@/,\[\]"]+)@)?((?:\d{1,3}\.){3}\d{1,3}):(\d{2,5})`)

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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
