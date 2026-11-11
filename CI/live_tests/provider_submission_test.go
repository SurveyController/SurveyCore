package live_tests_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SurveyController/SurveyCore/pkg/surveycore"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

var (
	sensitiveHeaderValue = regexp.MustCompile(`(?i)(authorization|proxy-authorization|cookie|set-cookie)(\s*[=:]\s*|\s+)(?:bearer\s+)?[^\r\n]+`)
	sensitiveFieldValue  = regexp.MustCompile(`(?i)(access[_-]?token|refresh[_-]?token|api[_-]?key|password|passwd)(\s*[=:]\s*|\s+)[^\s,;}&]+`)
)

func TestLiveProviderParse(t *testing.T) {
	if os.Getenv("SURVEYCORE_LIVE_PARSE") != "1" {
		t.Skip("set SURVEYCORE_LIVE_PARSE=1 to enable live parser checks")
	}
	for _, survey := range []struct {
		provider   string
		url        string
		parseError string
	}{
		{model.ProviderWJX, "https://v.wjx.cn/vm/ei3sVrE.aspx", ""},
		// This published fixture includes rating types the parser explicitly rejects.
		{model.ProviderQQ, "https://wj.qq.com/s2/26070328/fa89/", "腾讯问卷当前版本暂不支持量表、矩阵量表题"},
		{model.ProviderCredamo, "https://www.credamo.com/s/A73QR3ano", ""},
	} {
		t.Run(survey.provider, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			if survey.parseError != "" {
				_, err := surveycore.New().Parse(ctx, survey.url)
				if !errors.Is(err, surveycore.ErrParseFailed) || !strings.Contains(err.Error(), survey.parseError) {
					t.Fatalf("expected the fixture's unsupported-type error, got %v", err)
				}
				t.Log("live questions fetched; unsupported rating types correctly reported")
				return
			}
			config := defaultConfigWithRetry(t, ctx, surveycore.New(), survey.url)
			if config.SurveySource.Provider != survey.provider || config.SurveyDefinition.Provider != survey.provider {
				t.Fatalf("provider mismatch: source=%q definition=%q", config.SurveySource.Provider, config.SurveyDefinition.Provider)
			}
			if strings.TrimSpace(config.Title) == "" || len(config.SurveyDefinition.Questions) == 0 || len(config.AnswerPlan.Strategies) == 0 {
				t.Fatal("live parser returned an empty title, questions, or answer strategies")
			}
		})
	}
}

func TestLiveProviderSubmission(t *testing.T) {
	if os.Getenv("SURVEYCORE_LIVE") != "1" {
		t.Skip("set SURVEYCORE_LIVE=1 to enable authorized live submissions")
	}
	provider := strings.TrimSpace(os.Getenv("SURVEYCORE_LIVE_PROVIDER"))
	surveyURL := strings.TrimSpace(os.Getenv("SURVEYCORE_LIVE_URL"))
	if !isKnownProvider(provider) {
		t.Fatalf("SURVEYCORE_LIVE_PROVIDER must be wjx, qq, or credamo; got %q", provider)
	}
	if surveyURL == "" {
		t.Fatal("SURVEYCORE_LIVE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute+30*time.Second)
	defer cancel()
	client := surveycore.New()
	cfg := defaultConfigWithRetry(t, ctx, client, surveyURL)
	if cfg.SurveySource.Provider != provider {
		t.Fatalf("detected source provider = %q, want %q", cfg.SurveySource.Provider, provider)
	}
	if cfg.SurveyDefinition.Provider != provider {
		t.Fatalf("detected definition provider = %q, want %q", cfg.SurveyDefinition.Provider, provider)
	}
	cfg.ExecutionPlan.Target = 1
	cfg.ExecutionPlan.Threads = 1
	cfg.ExecutionPlan.SubmitInterval = [2]int{0, 0}

	var receivedSuccess atomic.Bool
	result, err := client.RunWithEvents(ctx, cfg, func(event model.Event) {
		if event.Success {
			receivedSuccess.Store(true)
		}
	})
	if err != nil {
		logErrorChain(t, err)
		t.Fatal("live submission failed")
	}
	if result == nil {
		t.Fatal("live submission returned a nil result")
	}
	if result.Success != 1 || result.Fail != 0 || result.Stopped {
		t.Fatalf("unexpected result: success=%d fail=%d stopped=%t", result.Success, result.Fail, result.Stopped)
	}
	if !receivedSuccess.Load() {
		t.Fatal("live submission completed without a success event")
	}
}

func defaultConfigWithRetry(t *testing.T, ctx context.Context, client *surveycore.Client, surveyURL string) *model.RunRequest {
	t.Helper()
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		cfg, err := client.DefaultConfig(ctx, surveyURL)
		if err == nil {
			return cfg
		}
		lastErr = err
		t.Logf("default config attempt %d failed", attempt)
		logErrorChain(t, err)
		if attempt == 1 {
			select {
			case <-ctx.Done():
				logErrorChain(t, ctx.Err())
				t.Fatal("context ended before the default config retry")
			case <-time.After(5 * time.Second):
			}
		}
	}
	t.Fatalf("default config failed after two attempts: %s", redactErrorText(lastErr.Error()))
	return nil
}

func isKnownProvider(provider string) bool {
	return provider == model.ProviderWJX || provider == model.ProviderQQ || provider == model.ProviderCredamo
}

func logErrorChain(t *testing.T, err error) {
	t.Helper()
	for depth := 0; err != nil; depth++ {
		t.Logf("error[%d]: %s", depth, redactErrorText(err.Error()))
		err = errors.Unwrap(err)
	}
}

func redactErrorText(value string) string {
	if value == "" {
		return ""
	}
	value = sensitiveHeaderValue.ReplaceAllString(value, "$1$2[REDACTED]")
	return sensitiveFieldValue.ReplaceAllString(value, "$1$2[REDACTED]")
}

func TestRedactErrorText(t *testing.T) {
	input := "Authorization: Bearer secret\npassword=plain access_token=opaquevalue"
	redacted := redactErrorText(input)
	for _, secret := range []string{"secret", "plain", "opaquevalue"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("redacted error still contains %q: %s", secret, redacted)
		}
	}
}

func Example_liveProviderContract() {
	provider := model.ProviderWJX
	fmt.Println(provider)
	// Output: wjx
}
