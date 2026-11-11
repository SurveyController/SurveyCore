package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

type submitRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn submitRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestSubmitExecutesOneAttemptOnEachProvider(t *testing.T) {
	for _, provider := range []string{ProviderWJX, ProviderQQ, ProviderCredamo} {
		t.Run(provider, func(t *testing.T) {
			var server *httptest.Server
			var client *http.Client
			var surveyURL, submitPath string
			switch provider {
			case ProviderWJX:
				server = newWJXCoreTestServer(t, true)
				client = rewriteWJXHTTPClient(server.URL)
				surveyURL, submitPath = "https://www.wjx.cn/vm/demo.aspx", "/joinnew/processjq.ashx"
			case ProviderQQ:
				server = newTencentCoreTestServer(t)
				client = rewriteTencentHTTPClient(server.URL)
				surveyURL, submitPath = "https://wj.qq.com/s2/123/hashvalue/", "/api/v2/respondent/surveys/123/answers"
			case ProviderCredamo:
				server = newCredamoTestServer(t, true)
				client = server.Client()
				surveyURL, submitPath = server.URL+"/s/demo_", "/v1/survey/answer/noauth/save"
			}
			defer server.Close()
			var submissions atomic.Int32
			next := client.Transport
			client.Transport = submitRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.Path == submitPath {
					submissions.Add(1)
					if !strings.Contains(request.UserAgent(), "Windows NT") {
						t.Errorf("configured user-agent was lost: %q", request.UserAgent())
					}
				}
				return next.RoundTrip(request)
			})
			config := &RunRequest{
				SurveySource:       SurveySource{URL: surveyURL},
				ExecutionPlan:      ExecutionPlan{Target: 8, Threads: 4, SubmitInterval: [2]int{600, 600}},
				ReverseFillPlan:    ReverseFillPlan{Threads: 7},
				PsychometricPolicy: PsychometricPolicy{Enabled: true, TargetAlpha: 0.85},
				UserAgent:          &model.UserAgentSettings{Enabled: true, Ratios: map[string]int{"pc": 100}},
			}
			original, _ := json.Marshal(config)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result, err := New(WithHTTPClient(client)).Submit(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Success || result.Provider != provider || result.Attempt != 0 || result.Error != nil || result.ElapsedSeconds < 0 || submissions.Load() != 1 {
				t.Fatalf("result=%#v submissions=%d", result, submissions.Load())
			}
			current, _ := json.Marshal(config)
			if string(original) != string(current) {
				t.Fatal("Submit modified the caller's configuration")
			}
		})
	}
}

func TestSubmitFailureDoesNotRetryAndRetainsTypedError(t *testing.T) {
	server := newWJXCoreTestServer(t, false)
	defer server.Close()
	client := rewriteWJXHTTPClient(server.URL)
	var submissions atomic.Int32
	next := client.Transport
	client.Transport = submitRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodPost {
			submissions.Add(1)
		}
		return next.RoundTrip(request)
	})
	result, err := New(WithHTTPClient(client)).Submit(context.Background(), &RunRequest{
		SurveySource:       SurveySource{URL: "https://www.wjx.cn/vm/demo.aspx"},
		ExecutionPlan:      ExecutionPlan{Target: 9, Threads: 9},
		PsychometricPolicy: PsychometricPolicy{Enabled: true, TargetAlpha: 0.85},
	})
	if !errors.Is(err, ErrRunFailed) || ClassifyRunError(err) != ErrorKindRun {
		t.Fatalf("error lost its category: %v", err)
	}
	if result == nil || result.Success || result.Error == nil || *result.Error != err.Error() || submissions.Load() != 1 {
		t.Fatalf("result=%#v submissions=%d", result, submissions.Load())
	}
}

func TestSubmitPreservesCancellation(t *testing.T) {
	server := newWJXCoreTestServer(t, true)
	defer server.Close()
	for _, preCancelled := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		client := rewriteWJXHTTPClient(server.URL)
		var submissions atomic.Int32
		next := client.Transport
		client.Transport = submitRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			if preCancelled {
				t.Error("a pre-cancelled Submit performed a request")
			}
			if request.Method == http.MethodPost {
				submissions.Add(1)
				cancel()
				return nil, context.Canceled
			}
			return next.RoundTrip(request)
		})
		if preCancelled {
			cancel()
		}
		result, err := New(WithHTTPClient(client)).Submit(ctx, &RunRequest{SurveySource: SurveySource{URL: "https://www.wjx.cn/vm/demo.aspx"}})
		cancel()
		if !errors.Is(err, context.Canceled) || ClassifyRunError(err) != ErrorKindCanceled || result == nil || result.Success || result.Error == nil || submissions.Load() > 1 {
			t.Fatalf("preCancelled=%t result=%#v error=%v submissions=%d", preCancelled, result, err, submissions.Load())
		}
	}
}

func TestSubmitParseFailureIncludesAttemptMetadata(t *testing.T) {
	client := &http.Client{Transport: submitRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("parse transport failed")
	})}
	result, err := New(WithHTTPClient(client)).Submit(context.Background(), &RunRequest{SurveySource: SurveySource{URL: "https://www.wjx.cn/vm/demo.aspx"}})
	if !errors.Is(err, ErrParseFailed) || result == nil || result.Success || result.Error == nil || result.Provider != ProviderWJX {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}
