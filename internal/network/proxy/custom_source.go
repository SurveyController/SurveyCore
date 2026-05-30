package proxy

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/SurveyController/SurveyConsole/internal/models"
)

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
