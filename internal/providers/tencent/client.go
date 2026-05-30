package tencent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/SurveyController/SurveyConsole/internal/models"
	"github.com/SurveyController/SurveyConsole/internal/network/httpclient"
)

func extractQQIdentifiers(surveyURL string) (string, string, error) {
	matches := urlRe.FindStringSubmatch(surveyURL)
	if len(matches) < 3 {
		return "", "", fmt.Errorf("无法从 URL 提取腾讯问卷 ID: %s", surveyURL)
	}
	return matches[1], matches[2], nil
}

func fetchSession(ctx context.Context, surveyID, hashValue string) (string, string, error) {
	url := fmt.Sprintf("%s/%s/session?_=%d&hash=%s", apiBase, surveyID, time.Now().UnixMilli(), hashValue)
	resp, err := httpclient.Get(ctx, url, qqAPIHeaders(surveyID, hashValue), nil, 10*time.Second)
	if err != nil {
		return "", "", err
	}
	cookieHeader := responseCookieHeader(resp)

	// Extract answer_session_id from response
	var result map[string]any
	if err := json.Unmarshal(resp.Body, &result); err == nil {
		if data, ok := result["data"].(map[string]any); ok {
			if sessionID, ok := data["answer_session_id"].(string); ok {
				return sessionID, cookieHeader, nil
			}
		}
	}
	// Session ID not found - continue without it
	return "", cookieHeader, nil
}

func fetchRawQuestionsWithSession(ctx context.Context, surveyID, hashValue, sessionID, cookieHeader string) ([]map[string]any, string, error) {
	var lastErr error
	for _, locale := range strings.Split(localeOrder, ",") {
		questionsURL := fmt.Sprintf("%s/%s/questions?_=%d&hash=%s&locale=%s",
			apiBase, surveyID, time.Now().UnixMilli(), hashValue, locale)

		headers := qqAPIHeaders(surveyID, hashValue)
		if sessionID != "" {
			headers["X-Answer-Session"] = sessionID
		}
		if cookieHeader != "" {
			headers["Cookie"] = cookieHeader
		}

		resp, err := httpclient.Get(ctx, questionsURL, headers, nil, 10*time.Second)
		if err != nil {
			lastErr = err
			continue
		}

		var result map[string]any
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			lastErr = err
			continue
		}
		if err := ensureQQAPIOK(result, "questions"); err != nil {
			lastErr = err
			continue
		}

		data, ok := result["data"].(map[string]any)
		if !ok {
			lastErr = fmt.Errorf("题目接口缺少 data 对象")
			continue
		}

		// Get title from meta
		title := ""
		metaURL := fmt.Sprintf("%s/%s/meta?_=%d&hash=%s&locale=%s",
			apiBase, surveyID, time.Now().UnixMilli(), hashValue, locale)
		metaResp, err := httpclient.Get(ctx, metaURL, headers, nil, 10*time.Second)
		if err == nil {
			var metaResult map[string]any
			if json.Unmarshal(metaResp.Body, &metaResult) == nil {
				_ = ensureQQAPIOK(metaResult, "meta")
				if metaData, ok := metaResult["data"].(map[string]any); ok {
					if t, ok := metaData["title"].(string); ok {
						title = t
					}
				}
			}
		}

		if questionsRaw, ok := data["questions"].([]any); ok {
			questions := make([]map[string]any, 0, len(questionsRaw))
			for _, q := range questionsRaw {
				if qm, ok := q.(map[string]any); ok {
					questions = append(questions, qm)
				}
			}
			return questions, title, nil
		}
		lastErr = fmt.Errorf("题目接口未返回 questions 数组")
	}

	if lastErr != nil {
		return nil, "", fmt.Errorf("无法获取腾讯问卷题目: %w", lastErr)
	}
	return nil, "", fmt.Errorf("无法获取腾讯问卷题目")
}

func qqAPIHeaders(surveyID, hashValue string) map[string]string {
	return map[string]string{
		"User-Agent":      defaultUA,
		"Accept":          "application/json, text/plain, */*",
		"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
		"Connection":      "close",
		"Origin":          "https://wj.qq.com",
		"Referer":         fmt.Sprintf("https://wj.qq.com/s2/%s/%s/", surveyID, hashValue),
	}
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

func ensureQQAPIOK(payload map[string]any, endpoint string) error {
	code := strings.ToUpper(strings.TrimSpace(fmt.Sprint(payload["code"])))
	if code == "" || code == "OK" || code == "0" {
		return nil
	}
	return fmt.Errorf("%s 接口返回异常: %s", endpoint, code)
}

func fetchQuestions(ctx context.Context, surveyID, hashValue string) ([]models.SurveyQuestionMeta, string, error) {
	rawQuestions, title, err := fetchRawQuestions(ctx, surveyID, hashValue)
	if err != nil {
		return nil, "", err
	}
	normalized := standardizeQuestions(rawQuestions)
	return normalized, title, nil
}

func fetchRawQuestions(ctx context.Context, surveyID, hashValue string) ([]map[string]any, string, error) {
	return fetchRawQuestionsWithSession(ctx, surveyID, hashValue, "", "")
}

func fetchRawQuestionsLegacy(ctx context.Context, surveyID, hashValue string) ([]map[string]any, string, error) {
	// Try different locales
	for _, locale := range strings.Split(localeOrder, ",") {
		questionsURL := fmt.Sprintf("%s/%s/questions?hash=%s&locale=%s&_=%d",
			apiBase, surveyID, hashValue, locale, time.Now().UnixMilli())

		resp, err := httpclient.Get(ctx, questionsURL, map[string]string{
			"User-Agent": defaultUA,
			"Referer":    fmt.Sprintf("https://wj.qq.com/s2/%s/%s/", surveyID, hashValue),
		}, nil, 10*time.Second)
		if err != nil {
			continue
		}

		var result map[string]any
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			continue
		}

		data, ok := result["data"].(map[string]any)
		if !ok {
			continue
		}

		// Get title
		title := ""
		if metaURL := fmt.Sprintf("%s/%s/meta?hash=%s&locale=%s&_=%d",
			apiBase, surveyID, hashValue, locale, time.Now().UnixMilli()); true {
			metaResp, err := httpclient.Get(ctx, metaURL, map[string]string{
				"User-Agent": defaultUA,
				"Referer":    fmt.Sprintf("https://wj.qq.com/s2/%s/%s/", surveyID, hashValue),
			}, nil, 10*time.Second)
			if err == nil {
				var metaResult map[string]any
				if json.Unmarshal(metaResp.Body, &metaResult) == nil {
					if metaData, ok := metaResult["data"].(map[string]any); ok {
						if t, ok := metaData["title"].(string); ok {
							title = t
						}
					}
				}
			}
		}

		// Extract questions array
		if questionsRaw, ok := data["questions"].([]any); ok {
			questions := make([]map[string]any, 0, len(questionsRaw))
			for _, q := range questionsRaw {
				if qm, ok := q.(map[string]any); ok {
					questions = append(questions, qm)
				}
			}
			return questions, title, nil
		}
	}

	return nil, "", fmt.Errorf("无法获取腾讯问卷题目")
}
