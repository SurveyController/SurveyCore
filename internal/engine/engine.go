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
	"github.com/SurveyController/SurveyCore/internal/network/proxy"
)

// StatusEvent represents a status update from the engine.
type StatusEvent struct {
	ThreadName string    `json:"thread_name"`
	StatusText string    `json:"status_text"`
	Success    bool      `json:"success"`
	Fail       bool      `json:"fail"`
	Current    int       `json:"current"`
	Total      int       `json:"total"`
	Timestamp  time.Time `json:"timestamp"`
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
	pool     *proxy.Pool
	handler  StatusHandler
	paused   atomic.Bool
}

// NewEngine creates a new execution engine.
func NewEngine(registry ProviderRegistry, pool *proxy.Pool, handler StatusHandler) *Engine {
	return &Engine{
		registry: registry,
		pool:     pool,
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

	// Start proxy prefetch loop if proxy pool is configured
	if e.pool != nil && cfg.RandomProxyIPEnabled {
		e.prefetchProxyBatch(e.pool, concurrency*2)
		go e.proxyPrefetchLoop(ctx, e.pool, cfg, concurrency)
	}

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

func (e *Engine) prefetchProxyBatch(pool *proxy.Pool, count int) {
	if count <= 0 {
		count = 1
	}
	leases, err := pool.FetchBatch(count)
	if err != nil {
		logging.WarnFields("获取代理失败", logging.F("component", "proxy_prefetch"), logging.F("error", err))
		return
	}
	pool.AddLeases(leases)
}

func (e *Engine) proxyPrefetchLoop(ctx context.Context, pool *proxy.Pool, cfg *execution.ExecutionConfig, concurrency int) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Prefetch if pool is running low
			bufferSize := concurrency * 2
			if pool.Size() < bufferSize {
				e.prefetchProxyBatch(pool, bufferSize)
			}
			pool.CleanupExpired()
		}
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
		if cfg.StopOnFailEnabled && state.GetCurFail() >= cfg.FailThreshold {
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

	// Get proxy if enabled
	var proxyAddr, rawProxyAddr string
	if cfg.RandomProxyIPEnabled && e.pool != nil {
		lease := e.pool.Pop()
		if lease == nil {
			e.prefetchProxyBatch(e.pool, 1)
			lease = e.pool.Pop()
		}
		if lease == nil {
			logging.WarnFields("随机 IP 已启用但没有可用代理", logging.F("worker", threadName))
			return false
		}
		if lease != nil {
			rawProxyAddr = lease.Address
			proxyAddr = proxy.ExtractProxyAddress(lease.Address)
		}
	}

	opts := models.FillOptions{
		ThreadName:   threadName,
		ProxyAddress: proxyAddr,
		UserAgent:    sampleUserAgent(cfg),
		StopChan:     state.StopChan,
	}

	state.UpdateThreadStatus(threadName, "提交问卷", &running)

	success, err := adapter.FillSurveyHTTP(ctx, cfg, state, opts)
	if err != nil {
		logging.WarnFields("提交失败", logging.F("worker", threadName), logging.F("error", err))
		if cfg.RandomProxyIPEnabled && e.pool != nil && rawProxyAddr != "" {
			e.pool.MarkBad(rawProxyAddr)
		}
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
	"pc_web":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
	"mobile_android": "Mozilla/5.0 (Linux; Android 16; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Mobile Safari/537.36",
	"wechat_android": "Mozilla/5.0 (Linux; Android 16; Pixel 8 Build/BP22.250124.009; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/121.0.0.0 Mobile Safari/537.36 MicroMessenger/8.0.43.2460(0x28002B3B) Process/appbrand0 WeChat/arm64 Weixin NetType/WIFI Language/zh_CN ABI/arm64",
	"wechat":         "Mozilla/5.0 (Linux; Android 16; Pixel 8 Build/BP22.250124.009; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/121.0.0.0 Mobile Safari/537.36 MicroMessenger/8.0.43.2460(0x28002B3B) Process/appbrand0 WeChat/arm64 Weixin NetType/WIFI Language/zh_CN ABI/arm64",
	"mobile":         "Mozilla/5.0 (Linux; Android 16; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Mobile Safari/537.36",
	"pc":             "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
}

func sampleUserAgent(cfg *execution.ExecutionConfig) string {
	if !cfg.RandomUserAgentEnabled {
		return ""
	}
	keys := cfg.RandomUserAgentKeys
	if len(keys) == 0 {
		keys = []string{"wechat_android", "mobile_android", "pc_web"}
	}

	total := 0
	for _, key := range keys {
		if _, ok := userAgentProfiles[key]; !ok {
			continue
		}
		weight := cfg.UserAgentRatios[userAgentRatioKey(key)]
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
		weight := cfg.UserAgentRatios[userAgentRatioKey(key)]
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

func userAgentRatioKey(key string) string {
	switch key {
	case "wechat_android":
		return "wechat"
	case "mobile_android":
		return "mobile"
	case "pc_web":
		return "pc"
	default:
		return key
	}
}

// ParseSurvey parses a survey URL using the appropriate provider.
func (e *Engine) ParseSurvey(ctx context.Context, surveyURL string) (*models.SurveyDefinition, error) {
	adapter, err := e.registry.GetByURL(surveyURL)
	if err != nil {
		return nil, err
	}
	return adapter.ParseSurvey(ctx, surveyURL)
}
