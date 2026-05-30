package credamo

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"time"

	"github.com/SurveyController/SurveyConsole/internal/models"
	"github.com/SurveyController/SurveyConsole/internal/network/httpclient"
)

func sampleAnswerStartTimeMS(cfg *models.ExecutionConfig, initStartedAtMS int64, durationSeconds int) int64 {
	if cfg == nil {
		return initStartedAtMS
	}
	windowStartMS, windowEndMS := cfg.AnswerDatetimeWindowMS[0], cfg.AnswerDatetimeWindowMS[1]
	if windowStartMS <= 0 || windowEndMS <= windowStartMS {
		return initStartedAtMS
	}
	durationMS := int64(durationSeconds) * 1000
	if durationMS <= 0 {
		durationMS = 1
	}
	latestStartMS := windowEndMS - durationMS
	if latestStartMS <= windowStartMS {
		return windowStartMS
	}
	return windowStartMS + rand.Int63n(latestStartMS-windowStartMS+1)
}

func extractShortURL(surveyURL string) string {
	text := strings.TrimSpace(surveyURL)
	if text == "" {
		return ""
	}
	parseText := text
	if !strings.Contains(parseText, "://") {
		parseText = "https://" + parseText
	}
	u, err := url.Parse(parseText)
	if err != nil {
		return ""
	}

	fragmentPath := strings.TrimRight(u.Fragment, "/")
	if strings.HasPrefix(strings.ToLower(fragmentPath), "/s/") {
		if id := lastPathSegment(fragmentPath); id != "" {
			return id
		}
	}

	path := strings.TrimRight(u.Path, "/")
	if strings.HasPrefix(strings.ToLower(path), "/s/") {
		return lastPathSegment(path)
	}
	if strings.EqualFold(path, "/answer.html") {
		return ""
	}

	if matches := shortURLRe.FindStringSubmatch(path); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func lastPathSegment(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

func fetchDetail(ctx context.Context, shortURL string) (map[string]any, error) {
	data, _, err := fetchDetailSession(ctx, shortURL, "")
	return data, err
}

func fetchDetailSession(ctx context.Context, shortURL, cookieHeader string) (map[string]any, string, error) {
	fetchURL := fmt.Sprintf("%s/v1/survey/noauth/detail/get/%s", apiOrigin, shortURL)
	headers := credamoHeaders(shortURL, defaultUA, "", false)
	if cookieHeader != "" {
		headers["Cookie"] = cookieHeader
	}
	resp, err := httpclient.Get(ctx, fetchURL, headers, nil, 15*time.Second)
	if err != nil {
		return nil, "", err
	}
	nextCookieHeader := responseCookieHeader(resp)

	var result map[string]any
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, nextCookieHeader, fmt.Errorf("解析响应失败: %w", err)
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		return nil, nextCookieHeader, fmt.Errorf("响应格式错误: %s", truncate(string(resp.Body), 200))
	}
	return data, nextCookieHeader, nil
}

func initAnswer(ctx context.Context, shortURL, cookieHeader string) (map[string]any, string, error) {
	timeCode := newTimeCode()
	initURL := fmt.Sprintf("%s/v1/survey/answer/noauth/init/%s?timeCode=%s&accountCode=CDM&resolution=%s",
		apiOrigin, shortURL, url.QueryEscape(timeCode), url.QueryEscape(resolution))
	headers := credamoHeaders(shortURL, defaultUA, "", false)
	if cookieHeader != "" {
		headers["Cookie"] = cookieHeader
	}
	resp, err := httpclient.Get(ctx, initURL, headers, nil, 10*time.Second)
	if err != nil {
		return nil, "", err
	}
	nextCookieHeader := responseCookieHeader(resp)

	var result map[string]any
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, nextCookieHeader, err
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		return nil, nextCookieHeader, fmt.Errorf("初始化响应格式错误: %s", truncate(string(resp.Body), 200))
	}
	if getString(data, "timeCode") == "" {
		data["timeCode"] = timeCode
	}
	return data, nextCookieHeader, nil
}

func credamoHeaders(shortURL, ua, answerToken string, jsonBody bool) map[string]string {
	headers := map[string]string{
		"User-Agent":      ua,
		"Accept":          "application/json, text/plain, */*",
		"Accept-Language": "zh-CN,zh;q=0.9",
		"Referer":         fmt.Sprintf("%s/answer.html#/s/%s", apiOrigin, shortURL),
	}
	for key, value := range buildSignatureHeaders(answerToken) {
		headers[key] = value
	}
	if jsonBody {
		headers["Origin"] = apiOrigin
		headers["Content-Type"] = "application/json"
	}
	return headers
}

func buildSignatureHeaders(answerToken string) map[string]string {
	nonce := randomCredamoToken(16)
	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	unionID := randomCredamoToken(10)
	return map[string]string{
		"unionId":   unionID,
		"nonce":     nonce,
		"timestamp": timestamp,
		"signature": computeSignature(answerToken, nonce, timestamp, unionID),
	}
}

func randomCredamoToken(length int) string {
	const chars = "ABCDEFGHJKMNPQRSTWXYZabcdefhijkmnprstwxyz2345678"
	if length <= 0 {
		length = 1
	}
	var b strings.Builder
	for i := 0; i < length; i++ {
		b.WriteByte(chars[rand.Intn(len(chars))])
	}
	return b.String()
}

func responseCookieHeader(resp *httpclient.Response) string {
	if resp == nil {
		return ""
	}
	var cookies []string
	for _, raw := range resp.Headers.Values("Set-Cookie") {
		part := strings.TrimSpace(strings.Split(raw, ";")[0])
		if part != "" {
			cookies = append(cookies, part)
		}
	}
	return strings.Join(cookies, "; ")
}

func mergeCookieHeaders(values ...string) string {
	seen := make(map[string]string)
	var order []string
	for _, value := range values {
		for _, part := range strings.Split(value, ";") {
			cookie := strings.TrimSpace(part)
			if cookie == "" {
				continue
			}
			name := cookie
			if idx := strings.Index(cookie, "="); idx >= 0 {
				name = cookie[:idx]
			}
			if _, ok := seen[name]; !ok {
				order = append(order, name)
			}
			seen[name] = cookie
		}
	}
	var cookies []string
	for _, name := range order {
		cookies = append(cookies, seen[name])
	}
	return strings.Join(cookies, "; ")
}

func newTimeCode() string {
	return fmt.Sprintf("%08x%04x%04x%04x%012x",
		rand.Int31(), rand.Int31n(0xffff), rand.Int31n(0xffff),
		rand.Int31n(0xffff), rand.Int63n(0xffffffffffff))
}

// computeSignature implements Credamo's double SHA1 signature scheme.
func computeSignature(token, nonce, timestamp, unionID string) string {
	// Inner hash: SHA1(token + nonce + timestamp + unionID + cipher)
	innerInput := token + nonce + timestamp + unionID + signCipher
	innerHash := sha1.Sum([]byte(innerInput))
	innerHex := strings.ToUpper(hex.EncodeToString(innerHash[:]))

	// Outer hash: SHA1(token + nonce + timestamp + innerHex + unionID + cipher)
	outerInput := token + nonce + timestamp + innerHex + unionID + signCipher
	outerHash := sha1.Sum([]byte(outerInput))
	return strings.ToUpper(hex.EncodeToString(outerHash[:]))
}
