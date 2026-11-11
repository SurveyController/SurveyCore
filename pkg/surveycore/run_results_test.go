package surveycore_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SurveyController/SurveyCore/pkg/surveycore"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

const runResultsHTML = `<html><head><title>Run results</title></head><body><div id="divQuestion"><fieldset><div topic="1" id="div1" type="3"><div class="topichtml">Choice</div><div class="ui-controlgroup"><div><span class="label">A</span></div><div><span class="label">B</span></div></div></div></fieldset></div></body></html>`

func runResultsResponse(request *http.Request, body string) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

func TestRunResultsContinuesAfterFailureWithoutModifyingConfig(t *testing.T) {
	var gets, posts atomic.Int32
	client := surveycore.New(surveycore.WithHTTPClient(&http.Client{Transport: submissionTransport(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost {
			gets.Add(1)
			return runResultsResponse(request, runResultsHTML)
		}
		if posts.Add(1) == 1 {
			return runResultsResponse(request, "1〒1〒失败")
		}
		return runResultsResponse(request, "10")
	})}))
	config := &model.RunRequest{SurveySource: model.SurveySource{URL: "https://www.wjx.cn/vm/demo.aspx"}, ExecutionPlan: model.ExecutionPlan{Target: 3, Threads: 1}}
	original, _ := json.Marshal(config)
	var results []surveycore.SubmissionResult
	for result, err := range client.RunResults(context.Background(), config) {
		if err != nil {
			t.Fatal(err)
		}
		results = append(results, result)
	}
	if len(results) != 3 || gets.Load() != 1 || posts.Load() != 3 {
		t.Fatalf("results=%#v gets=%d posts=%d", results, gets.Load(), posts.Load())
	}
	for index, result := range results {
		if result.Attempt != index+1 || result.Provider != "wjx" || result.Success != (index > 0) || (result.Error == nil) != result.Success {
			t.Fatalf("result=%#v", result)
		}
	}
	current, _ := json.Marshal(config)
	if string(original) != string(current) {
		t.Fatal("run modified caller config")
	}
}

func TestBreakingRunResultsCancelsAndDrainsWorkers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	secondStarted := make(chan struct{})
	var posts, active atomic.Int32
	client := surveycore.New(surveycore.WithHTTPClient(&http.Client{Transport: submissionTransport(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost {
			return runResultsResponse(request, runResultsHTML)
		}
		active.Add(1)
		defer active.Add(-1)
		switch posts.Add(1) {
		case 1:
			select {
			case <-secondStarted:
				return runResultsResponse(request, "10")
			case <-request.Context().Done():
				return nil, request.Context().Err()
			}
		case 2:
			close(secondStarted)
		}
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}))
	config := &model.RunRequest{SurveySource: model.SurveySource{URL: "https://www.wjx.cn/vm/demo.aspx"}, ExecutionPlan: model.ExecutionPlan{Target: 100, Threads: 2}}
	count := 0
	for result, err := range client.RunResults(ctx, config) {
		if err != nil || !result.Success {
			t.Fatalf("result=%#v error=%v", result, err)
		}
		count++
		break
	}
	if count != 1 || active.Load() != 0 || posts.Load() < 2 || posts.Load() >= 100 || ctx.Err() != nil {
		t.Fatalf("count=%d active=%d posts=%d context=%v", count, active.Load(), posts.Load(), ctx.Err())
	}
}

func TestRunResultsReportsSetupAndContextErrors(t *testing.T) {
	count := 0
	for _, err := range surveycore.RunResults(context.Background(), nil) {
		count++
		if !errors.Is(err, surveycore.ErrInvalidConfig) {
			t.Fatalf("setup error=%v", err)
		}
	}
	if count != 1 {
		t.Fatalf("setup error count=%d", count)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var posts atomic.Int32
	client := surveycore.New(surveycore.WithHTTPClient(&http.Client{Transport: submissionTransport(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost {
			return runResultsResponse(request, runResultsHTML)
		}
		posts.Add(1)
		cancel()
		return nil, request.Context().Err()
	})}))
	config := &model.RunRequest{SurveySource: model.SurveySource{URL: "https://www.wjx.cn/vm/demo.aspx"}, ExecutionPlan: model.ExecutionPlan{Target: 3, Threads: 1}}
	count = 0
	for _, err := range client.RunResults(ctx, config) {
		count++
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("context error=%v", err)
		}
	}
	if count != 1 || posts.Load() != 1 {
		t.Fatalf("context error count=%d posts=%d", count, posts.Load())
	}
}
