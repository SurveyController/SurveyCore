package engine

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SurveyController/SurveyCore/internal/execution"
	runstate "github.com/SurveyController/SurveyCore/internal/runtime"

	"github.com/SurveyController/SurveyCore/internal/logging"
	"github.com/SurveyController/SurveyCore/internal/models"
)

// StatusEvent represents a status update from the engine.
type StatusEvent struct {
	ThreadName string
	StatusText string
	Success    bool
	Fail       bool
	Current    int
	Total      int
	Timestamp  time.Time
}

// StatusHandler is called when a status event occurs.
type StatusHandler func(event StatusEvent)

// ProviderRegistry is the interface for provider lookup.
type ProviderRegistry interface {
	Get(name string) (models.ProviderAdapter, error)
	GetByURL(url string) (models.ProviderAdapter, error)
}

// Engine manages the concurrent execution of survey submissions.
type Engine struct {
	registry ProviderRegistry
	handler  StatusHandler
	paused   atomic.Bool
}

// NewEngine creates a new execution engine.
func NewEngine(registry ProviderRegistry, handler StatusHandler) *Engine {
	return &Engine{
		registry: registry,
		handler:  handler,
	}
}

// Pause pauses the engine. Workers will pause between submissions.
func (e *Engine) Pause() {
	e.paused.Store(true)
}

// Resume resumes the engine after a pause.
func (e *Engine) Resume() {
	e.paused.Store(false)
}

// IsPaused returns whether the engine is currently paused.
func (e *Engine) IsPaused() bool {
	return e.paused.Load()
}

// Run starts the survey submission run. It blocks until complete or stopped.
func (e *Engine) Run(ctx context.Context, cfg *execution.ExecutionConfig, state *runstate.ExecutionState) error {
	adapter, err := e.registry.Get(cfg.SurveyProvider)
	if err != nil {
		return fmt.Errorf("获取 provider 失败: %w", err)
	}

	concurrency := cfg.NumThreads
	if concurrency <= 0 {
		concurrency = 1
	}

	state.EnsureWorkerThreads(concurrency, "Worker")
	state.InitializeReverseFillRuntime()

	scheduler := NewScheduler(concurrency)
	scheduler.Start()
	defer scheduler.Close()

	var wg sync.WaitGroup

	// Start worker goroutines
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			e.worker(ctx, adapter, cfg, state, scheduler, workerID)
		}(i)
	}

	// Wait for completion or stop
	doneChan := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneChan)
	}()

	select {
	case <-doneChan:
		return nil
	case <-ctx.Done():
		state.SignalStop()
		select {
		case <-doneChan:
		case <-time.After(30 * time.Second):
		}
		return nil
	case <-state.StopChan:
		select {
		case <-doneChan:
		case <-time.After(30 * time.Second):
		}
		return nil
	}
}

func (e *Engine) worker(ctx context.Context, adapter models.ProviderAdapter, cfg *execution.ExecutionConfig, state *runstate.ExecutionState, scheduler *Scheduler, workerID int) {
	threadName := fmt.Sprintf("Worker-%d", workerID+1)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if state.IsStopped() {
			return
		}

		// Check pause state
		for e.paused.Load() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
			if state.IsStopped() {
				return
			}
		}

		// Check if target reached
		if state.GetCurNum() >= cfg.TargetNum {
			return
		}

		// Acquire a scheduler token
		tokenID := scheduler.Acquire()
		if tokenID < 0 {
			return
		}

		if !state.TryStartSubmission(cfg.TargetNum) {
			scheduler.Release(tokenID, 0)
			return
		}
		if state.HasReverseFillRuntime() {
			acquired := state.AcquireReverseFillSample(threadName)
			if acquired.Status != "acquired" {
				state.AbortSubmissionReservation()
				scheduler.Release(tokenID, 0)
				if acquired.Status == "exhausted" {
					state.MarkTerminalStop("reverse_fill_exhausted", "reverse_fill_exhausted", "反填样本已耗尽，剩余样本不足以完成目标份数")
					state.SignalStop()
				}
				return
			}
		}

		// Execute one submission
		success := e.executeOne(ctx, adapter, cfg, state, threadName)

		if state.IsStopped() && !success {
			state.AbortSubmissionReservation()
			state.ReleaseReverseFillSample(threadName, true)
			scheduler.Release(tokenID, 0)
			return
		}

		// Update counters
		state.CompleteSubmission(success)
		if success {
			state.IncrementThreadSuccess(threadName)
			state.CommitReverseFillSample(threadName)
			e.emit(threadName, "提交成功", true, false, state)
		} else {
			state.IncrementThreadFail(threadName)
			if _, discarded := state.MarkReverseFillSubmissionFailed(threadName, 1); discarded && state.IsReverseFillTargetUnreachable() {
				state.MarkTerminalStop("reverse_fill_exhausted", "reverse_fill_exhausted", "反填样本已耗尽，剩余样本不足以完成目标份数")
				state.SignalStop()
			}
			e.emit(threadName, "提交失败", false, true, state)
		}

		// Release token with optional delay
		delay := sampleIntervalDelay(cfg.SubmitIntervalRangeSeconds)
		scheduler.Release(tokenID, delay)

		// Check fail threshold
		if state.GetCurFail() >= cfg.FailThreshold {
			state.MarkTerminalStop("fail_threshold", "fail_threshold", fmt.Sprintf("累计失败 %d 次，已停止", state.GetCurFail()))
			state.SignalStop()
			return
		}
	}
}

func (e *Engine) executeOne(ctx context.Context, adapter models.ProviderAdapter, cfg *execution.ExecutionConfig, state *runstate.ExecutionState, threadName string) bool {
	running := true
	state.UpdateThreadStatus(threadName, "构造答案", &running)
	defer func() {
		running = false
		state.UpdateThreadStatus(threadName, "等待中", &running)
	}()

	opts := models.FillOptions{
		ThreadName: threadName,
		UserAgent:  sampleUserAgent(cfg),
		StopChan:   state.StopChan,
	}

	state.UpdateThreadStatus(threadName, "提交问卷", &running)

	success, err := adapter.FillSurveyHTTP(ctx, cfg, state, opts)
	if err != nil {
		logging.WarnFields("提交失败", logging.F("worker", threadName), logging.F("error", err))
		return false
	}

	if success {
		state.UpdateThreadStatus(threadName, "提交成功", &running)
	}
	return success
}

func (e *Engine) emit(threadName, statusText string, success, fail bool, state *runstate.ExecutionState) {
	if e.handler == nil {
		return
	}
	total := 0
	if state.Config != nil {
		total = state.Config.TargetNum
	}
	e.handler(StatusEvent{
		ThreadName: threadName,
		StatusText: statusText,
		Success:    success,
		Fail:       fail,
		Current:    state.GetCurNum(),
		Total:      total,
		Timestamp:  time.Now(),
	})
}

func sampleIntervalDelay(bounds [2]int) time.Duration {
	minSeconds := bounds[0]
	maxSeconds := bounds[1]
	if minSeconds < 0 {
		minSeconds = 0
	}
	if maxSeconds < 0 {
		maxSeconds = 0
	}
	if maxSeconds < minSeconds {
		minSeconds, maxSeconds = maxSeconds, minSeconds
	}
	if maxSeconds == 0 {
		return 0
	}
	seconds := minSeconds
	if maxSeconds > minSeconds {
		seconds = minSeconds + rand.Intn(maxSeconds-minSeconds+1)
	}
	return time.Duration(seconds) * time.Second
}

var userAgentProfiles = map[string]string{
	"wechat": "Mozilla/5.0 (Linux; Android 14; Pixel 8 Pro) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36 MicroMessenger/8.0.44",
	"mobile": "Mozilla/5.0 (Linux; Android 14; Pixel 8 Pro) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
	"pc":     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
}

func sampleUserAgent(cfg *execution.ExecutionConfig) string {
	if !cfg.RandomUserAgentEnabled {
		return ""
	}
	keys := cfg.RandomUserAgentKeys
	if len(keys) == 0 {
		keys = []string{"wechat", "mobile", "pc"}
	}

	total := 0
	for _, key := range keys {
		if _, ok := userAgentProfiles[key]; !ok {
			continue
		}
		weight := cfg.UserAgentRatios[key]
		if weight <= 0 {
			weight = 1
		}
		total += weight
	}
	if total <= 0 {
		return ""
	}

	pick := rand.Intn(total)
	for _, key := range keys {
		ua, ok := userAgentProfiles[key]
		if !ok {
			continue
		}
		weight := cfg.UserAgentRatios[key]
		if weight <= 0 {
			weight = 1
		}
		if pick < weight {
			return ua
		}
		pick -= weight
	}
	return ""
}

// ParseSurvey parses a survey URL using the appropriate provider.
func (e *Engine) ParseSurvey(ctx context.Context, surveyURL string) (*models.SurveyDefinition, error) {
	adapter, err := e.registry.GetByURL(surveyURL)
	if err != nil {
		return nil, err
	}
	return adapter.ParseSurvey(ctx, surveyURL)
}
