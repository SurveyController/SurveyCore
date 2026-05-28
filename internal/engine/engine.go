package engine

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SurveyController/SurveyController-Go/internal/models"
	"github.com/SurveyController/SurveyController-Go/internal/network/proxy"
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
func (e *Engine) Run(ctx context.Context, cfg *models.ExecutionConfig, state *models.ExecutionState) error {
	adapter, err := e.registry.Get(cfg.SurveyProvider)
	if err != nil {
		return fmt.Errorf("获取 provider 失败: %w", err)
	}

	concurrency := cfg.NumThreads
	if concurrency <= 0 {
		concurrency = 1
	}

	state.EnsureWorkerThreads(concurrency, "Worker")

	scheduler := NewScheduler(concurrency)
	scheduler.Start()
	defer scheduler.Close()

	// Create pause channel
	pauseChan := make(chan struct{}, 1)
	// Initially unblocked (not paused)
	pauseChan <- struct{}{}

	// Start proxy prefetch loop if proxy pool is configured
	if e.pool != nil && cfg.RandomProxyIPEnabled {
		go e.proxyPrefetchLoop(ctx, e.pool, cfg, concurrency)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, concurrency)

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
	case <-state.StopChan:
		select {
		case <-doneChan:
		case <-time.After(30 * time.Second):
		}
		return nil
	case err := <-errChan:
		return err
	}
}

func (e *Engine) proxyPrefetchLoop(ctx context.Context, pool *proxy.Pool, cfg *models.ExecutionConfig, concurrency int) {
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
				leases, err := pool.FetchBatch(bufferSize)
				if err != nil {
					log.Printf("[ProxyPrefetch] 获取代理失败: %v", err)
					continue
				}
				pool.AddLeases(leases)
			}
			pool.CleanupExpired()
		}
	}
}

func (e *Engine) worker(ctx context.Context, adapter models.ProviderAdapter, cfg *models.ExecutionConfig, state *models.ExecutionState, scheduler *Scheduler, workerID int) {
	threadName := fmt.Sprintf("Worker-%d", workerID+1)

	for {
		if state.IsStopped() {
			return
		}

		// Check pause state
		for e.paused.Load() {
			time.Sleep(500 * time.Millisecond)
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

		// Execute one submission
		success := e.executeOne(ctx, adapter, cfg, state, threadName)

		// Update counters
		if success {
			state.IncrementSuccess()
			state.IncrementThreadSuccess(threadName)
		} else {
			state.IncrementFail()
			state.IncrementThreadFail(threadName)
		}

		// Release token with optional delay
		delay := time.Duration(0)
		if cfg.SubmitIntervalRangeSeconds[0] > 0 {
			delay = time.Duration(cfg.SubmitIntervalRangeSeconds[0]) * time.Second
		}
		scheduler.Release(tokenID, delay)

		// Check fail threshold
		if cfg.StopOnFailEnabled && state.GetCurFail() >= cfg.FailThreshold {
			state.MarkTerminalStop("fail_threshold", "fail_threshold", fmt.Sprintf("连续失败 %d 次，已停止", state.GetCurFail()))
			state.SignalStop()
			return
		}
	}
}

func (e *Engine) executeOne(ctx context.Context, adapter models.ProviderAdapter, cfg *models.ExecutionConfig, state *models.ExecutionState, threadName string) bool {
	running := true
	state.UpdateThreadStatus(threadName, "构造答案", &running)
	defer func() {
		running = false
		state.UpdateThreadStatus(threadName, "等待中", &running)
	}()

	// Get proxy if enabled
	var proxyAddr string
	if cfg.RandomProxyIPEnabled && e.pool != nil {
		lease := e.pool.Pop()
		if lease != nil {
			proxyAddr = lease.Address
		}
	}

	opts := models.FillOptions{
		ThreadName:   threadName,
		ProxyAddress: proxyAddr,
		StopChan:     state.StopChan,
	}

	state.UpdateThreadStatus(threadName, "提交问卷", &running)

	success, err := adapter.FillSurveyHTTP(ctx, cfg, state, opts)
	if err != nil {
		log.Printf("[Worker %s] 提交失败: %v", threadName, err)
		if cfg.RandomProxyIPEnabled && e.pool != nil && proxyAddr != "" {
			e.pool.MarkBad(proxyAddr)
		}
		return false
	}

	if success {
		state.UpdateThreadStatus(threadName, "提交成功", &running)
	}
	return success
}

// ParseSurvey parses a survey URL using the appropriate provider.
func (e *Engine) ParseSurvey(ctx context.Context, surveyURL string) (*models.SurveyDefinition, error) {
	adapter, err := e.registry.GetByURL(surveyURL)
	if err != nil {
		return nil, err
	}
	return adapter.ParseSurvey(ctx, surveyURL)
}
