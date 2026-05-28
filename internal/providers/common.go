package providers

import (
	"net/url"
	"strings"
)

// Provider constants
const (
	ProviderWJX     = "wjx"
	ProviderQQ      = "qq"
	ProviderCredamo = "credamo"
)

// SupportedProviders is the set of all supported provider names.
var SupportedProviders = map[string]bool{
	ProviderWJX:     true,
	ProviderQQ:      true,
	ProviderCredamo: true,
}

// NormalizeSurveyProvider normalizes a provider string to a canonical name.
func NormalizeSurveyProvider(value, defaultProvider string) string {
	if defaultProvider == "" {
		defaultProvider = ProviderWJX
	}
	v := strings.ToLower(strings.TrimSpace(value))
	if SupportedProviders[v] {
		return v
	}
	return defaultProvider
}

// DetectSurveyProvider detects the provider from a URL.
func DetectSurveyProvider(urlValue, defaultProvider string) string {
	if defaultProvider == "" {
		defaultProvider = ProviderWJX
	}
	if IsWJXSurveyURL(urlValue) {
		return ProviderWJX
	}
	if IsQQSurveyURL(urlValue) {
		return ProviderQQ
	}
	if IsCredamoSurveyURL(urlValue) {
		return ProviderCredamo
	}
	return defaultProvider
}

// IsSupportedSurveyURL checks if the URL belongs to a supported survey platform.
func IsSupportedSurveyURL(urlValue string) bool {
	return IsWJXSurveyURL(urlValue) || IsQQSurveyURL(urlValue) || IsCredamoSurveyURL(urlValue)
}

// IsWJXDomain checks if the host is a WJX (问卷星) domain.
func IsWJXDomain(urlValue string) bool {
	host := extractHost(urlValue)
	return strings.HasSuffix(host, ".wjx.cn") || host == "wjx.cn" ||
		strings.HasSuffix(host, ".wjx.com") || host == "wjx.com"
}

// IsWJXSurveyURL checks if the URL is a WJX survey URL.
func IsWJXSurveyURL(urlValue string) bool {
	host, path := extractHostPath(urlValue)
	isWJX := strings.HasSuffix(host, ".wjx.cn") || host == "wjx.cn" ||
		strings.HasSuffix(host, ".wjx.com") || host == "wjx.com"
	if !isWJX {
		return false
	}
	return strings.Contains(path, "/vp/") || strings.Contains(path, "/vm/") || strings.Contains(path, "/v/")
}

// IsQQSurveyURL checks if the URL is a Tencent survey URL.
func IsQQSurveyURL(urlValue string) bool {
	host := extractHost(urlValue)
	return strings.HasSuffix(host, ".wj.qq.com") || host == "wj.qq.com"
}

// IsCredamoSurveyURL checks if the URL is a Credamo survey URL.
func IsCredamoSurveyURL(urlValue string) bool {
	host := extractHost(urlValue)
	return strings.HasSuffix(host, ".credamo.com") || host == "credamo.com" ||
		strings.HasSuffix(host, ".jianshu.com") || host == "jianshu.com"
}

// MakeProviderQuestionKey creates a unique key for a provider question.
func MakeProviderQuestionKey(provider, pageID, questionID string) string {
	return provider + ":" + pageID + ":" + questionID
}

// GetProviderQuestionKey is an alias for use from models package.
func GetProviderQuestionKey(provider, pageID, questionID string) string {
	return MakeProviderQuestionKey(provider, pageID, questionID)
}

func extractHost(rawURL string) string {
	host, _ := extractHostPath(rawURL)
	return host
}

func extractHostPath(rawURL string) (string, string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", ""
	}
	return strings.ToLower(u.Host), u.Path
}
