package providers

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/SurveyController/SurveyConsole/internal/domain"
)

// Provider constants
const (
	ProviderWJX     = domain.ProviderWJX
	ProviderQQ      = domain.ProviderQQ
	ProviderCredamo = domain.ProviderCredamo
)

// SupportedProviders is the set of all supported provider names.
var SupportedProviders = map[string]bool{
	ProviderWJX:     true,
	ProviderQQ:      true,
	ProviderCredamo: true,
}

var (
	qqSurveyPathRe           = regexp.MustCompile(`(?i)^/s\d+/\d+/[A-Za-z0-9_-]+/?$`)
	credamoSurveyPathRe      = regexp.MustCompile(`(?i)^/answer\.html`)
	credamoShortSurveyPathRe = regexp.MustCompile(`(?i)^/s/[A-Za-z0-9_-]+/?$`)
)

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
	if IsCredamoSurveyURL(urlValue) {
		return ProviderCredamo
	}
	if IsQQSurveyURL(urlValue) {
		return ProviderQQ
	}
	if IsWJXDomain(urlValue) {
		return ProviderWJX
	}
	return NormalizeSurveyProvider("", defaultProvider)
}

// IsSupportedSurveyURL checks if the URL belongs to a supported survey platform.
func IsSupportedSurveyURL(urlValue string) bool {
	return IsWJXDomain(urlValue) || IsQQSurveyURL(urlValue) || IsCredamoSurveyURL(urlValue)
}

// IsWJXDomain checks if the host is a WJX (问卷星) domain.
func IsWJXDomain(urlValue string) bool {
	host := extractHost(urlValue)
	return hostMatches(host, "wjx.top", "wjx.cn", "wjx.com")
}

// IsWJXSurveyURL checks if the URL is a WJX survey URL.
func IsWJXSurveyURL(urlValue string) bool {
	return IsWJXDomain(urlValue)
}

// IsQQSurveyURL checks if the URL is a Tencent survey URL.
func IsQQSurveyURL(urlValue string) bool {
	host, path := extractHostPath(urlValue)
	return host == "wj.qq.com" && qqSurveyPathRe.MatchString(path)
}

// IsCredamoSurveyURL checks if the URL is a Credamo survey URL.
func IsCredamoSurveyURL(urlValue string) bool {
	host, path := extractHostPath(urlValue)
	if !hostMatches(host, "credamo.com", "credamo.cn") {
		return false
	}
	return credamoSurveyPathRe.MatchString(path) || credamoShortSurveyPathRe.MatchString(path)
}

// MakeProviderQuestionKey creates a unique key for a provider question.
func MakeProviderQuestionKey(provider, pageID, questionID string) string {
	provider = NormalizeSurveyProvider(provider, ProviderWJX)
	return domain.MakeProviderQuestionKey(provider, pageID, questionID)
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
	text := strings.TrimSpace(rawURL)
	if text == "" {
		return "", ""
	}
	if !strings.Contains(text, "://") {
		text = "https://" + text
	}
	u, err := url.Parse(text)
	if err != nil {
		return "", ""
	}
	host := strings.ToLower(u.Hostname())
	return host, u.Path
}

func hostMatches(host string, domains ...string) bool {
	if host == "" {
		return false
	}
	for _, domain := range domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}
