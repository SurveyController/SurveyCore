package runtime

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/internal/runerror"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

func tagAttempt(_ context.Context, index, _ int, job *JobRequest) error {
	job.Submission.Source.URL = fmt.Sprintf("https://example.test/%d", index+1)
	return nil
}

func attemptNumber(request *model.SubmissionRequest) int {
	value, _ := strconv.Atoi(strings.TrimPrefix(request.Source.URL, "https://example.test/"))
	return value
}

func TestExecutionResultsUseCompletionOrderAndBoundedConcurrency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var active, maximum atomic.Int32
	var results []model.SubmissionResult
	config := executionTestConfig(3, 2)
	config.SurveySource.Provider = ProviderWJX
	options := ExecutionOptionsFromConfig(config)
	options.ConfigureJob = tagAttempt
	result, err := RunExecution(ctx, config, func(ctx context.Context, request *model.SubmissionRequest, _ EventHandler) (*RunResult, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); current > old && !maximum.CompareAndSwap(old, current); old = maximum.Load() {
		}
		if attemptNumber(request) == 1 {
			close(firstStarted)
			select {
			case <-releaseFirst:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		} else {
			select {
			case <-firstStarted:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return &RunResult{Success: 1}, nil
	}, func(event Event) {
		if event.Result == nil {
			return
		}
		results = append(results, *event.Result)
		if event.Result.Attempt == 3 {
			close(releaseFirst)
		}
	}, options)
	if err != nil || result.Success != 3 || maximum.Load() != 2 || active.Load() != 0 {
		t.Fatalf("result=%#v error=%v maximum=%d active=%d", result, err, maximum.Load(), active.Load())
	}
	if len(results) != 3 || results[0].Attempt != 2 || results[1].Attempt != 3 || results[2].Attempt != 1 {
		t.Fatalf("completion order = %#v", results)
	}
	for _, item := range results {
		if !item.Success || item.Error != nil || item.Provider != ProviderWJX || item.ElapsedSeconds < 0 {
			t.Fatalf("result metadata = %#v", item)
		}
	}
}

func TestDefaultRunContinuesAfterErrorsAndWaitsBeforeEachLaterAttempt(t *testing.T) {
	config := executionTestConfig(4, 1)
	config.PsychometricPolicy.Enabled = true
	config.SubmitInterval = [2]int{2, 4}
	options := ExecutionOptionsFromConfig(config)
	options.ConfigureJob = tagAttempt
	var waits []time.Duration
	options.WaitInterval = func(_ context.Context, duration time.Duration) error {
		waits = append(waits, duration)
		return nil
	}
	var calls int
	var results []model.SubmissionResult
	result, err := RunExecution(context.Background(), config, func(_ context.Context, request *model.SubmissionRequest, _ EventHandler) (*RunResult, error) {
		calls++
		if attemptNumber(request)%2 == 1 {
			return nil, runerror.Wrap(runerror.KindConfig, errors.New("attempt configuration failed"))
		}
		return &RunResult{Success: 1}, nil
	}, func(event Event) {
		if event.Result != nil {
			results = append(results, *event.Result)
		}
	}, options)
	if err == nil || calls != 4 || len(waits) != 3 || len(results) != 4 || result.Success != 2 || result.Fail != 2 || options.MaxRetries != 0 {
		t.Fatalf("result=%#v error=%v calls=%d waits=%v results=%#v", result, err, calls, waits, results)
	}
	for _, duration := range waits {
		if duration < 2*time.Second || duration > 4*time.Second {
			t.Fatalf("interval outside Python range: %v", duration)
		}
	}
	if results[0].Success || results[0].Error == nil || !results[1].Success || results[1].Error != nil {
		t.Fatalf("failure did not remain in its own result: %#v", results)
	}
}

func TestLaterAttemptWaitsEvenOnANewWorker(t *testing.T) {
	config := executionTestConfig(2, 2)
	config.SubmitInterval = [2]int{1, 1}
	options := ExecutionOptionsFromConfig(config)
	options.ConfigureJob = tagAttempt
	waiting := make(chan struct{})
	release := make(chan struct{})
	options.WaitInterval = func(ctx context.Context, duration time.Duration) error {
		if duration != time.Second {
			t.Errorf("interval=%v", duration)
		}
		close(waiting)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := RunExecution(ctx, config, func(ctx context.Context, request *model.SubmissionRequest, _ EventHandler) (*RunResult, error) {
		if attemptNumber(request) == 1 {
			select {
			case <-waiting:
				close(release)
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return &RunResult{Success: 1}, nil
	}, nil, options)
	if err != nil || result.Success != 2 {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}

func TestCancellationDuringIntervalDoesNotCreateAResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	config := executionTestConfig(3, 1)
	config.SubmitInterval = [2]int{100, 100}
	options := ExecutionOptionsFromConfig(config)
	options.WaitInterval = func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}
	calls, records := 0, 0
	result, err := RunExecution(ctx, config, func(context.Context, *model.SubmissionRequest, EventHandler) (*RunResult, error) {
		calls++
		return &RunResult{Success: 1}, nil
	}, func(event Event) {
		if event.Result != nil {
			records++
		}
	}, options)
	if !errors.Is(err, context.Canceled) || !result.Stopped || result.Success != 1 || result.Fail != 0 || calls != 1 || records != 1 {
		t.Fatalf("result=%#v error=%v calls=%d records=%d", result, err, calls, records)
	}
}

func TestCancellationTakesPriorityOverAnEarlierAttemptError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	config := executionTestConfig(3, 1)
	calls, records := 0, 0
	result, err := RunExecution(ctx, config, func(context.Context, *model.SubmissionRequest, EventHandler) (*RunResult, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("first attempt failed")
		}
		cancel()
		return nil, ctx.Err()
	}, func(event Event) {
		if event.Result != nil {
			records++
		}
	}, ExecutionOptionsFromConfig(config))
	if !errors.Is(err, context.Canceled) || !result.Stopped || result.Fail != 1 || calls != 2 || records != 1 {
		t.Fatalf("result=%#v error=%v calls=%d records=%d", result, err, calls, records)
	}
}
