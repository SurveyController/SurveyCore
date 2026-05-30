package httpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Response wraps an HTTP response with the body already read.
type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

var (
	clientPool sync.Map // map[string]*http.Client
	poolMu     sync.Mutex
)

// getClient returns a pooled HTTP client for the given proxy address.
func getClient(proxyAddr *string) *http.Client {
	key := ""
	if proxyAddr != nil {
		key = *proxyAddr
	}

	if v, ok := clientPool.Load(key); ok {
		return v.(*http.Client)
	}

	poolMu.Lock()
	defer poolMu.Unlock()

	// Double-check after acquiring lock
	if v, ok := clientPool.Load(key); ok {
		return v.(*http.Client)
	}

	transport := &http.Transport{
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     300 * time.Second,
	}

	if proxyAddr != nil && *proxyAddr != "" {
		proxyURL, err := url.Parse(*proxyAddr)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
	clientPool.Store(key, client)
	return client
}

// Get performs an HTTP GET request.
func Get(ctx context.Context, reqURL string, headers map[string]string, proxyAddr *string, timeout time.Duration) (*Response, error) {
	return doRequest(ctx, http.MethodGet, reqURL, "", headers, proxyAddr, timeout)
}

// Post performs an HTTP POST request with a body string.
func Post(ctx context.Context, reqURL, body string, headers map[string]string, proxyAddr *string, timeout time.Duration) (*Response, error) {
	return doRequest(ctx, http.MethodPost, reqURL, body, headers, proxyAddr, timeout)
}

func doRequest(ctx context.Context, method, reqURL, body string, headers map[string]string, proxyAddr *string, timeout time.Duration) (*Response, error) {
	client := getClient(proxyAddr)

	// Use context timeout instead of mutating shared client (avoids data race)
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode >= 400 {
		return &Response{
			StatusCode: resp.StatusCode,
			Headers:    resp.Header,
			Body:       respBody,
		}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       respBody,
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
