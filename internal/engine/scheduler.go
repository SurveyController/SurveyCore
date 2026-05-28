package engine

import (
	"container/heap"
	"sync"
	"time"
)

// scheduledToken represents a token with a ready time for delayed requeue.
type scheduledToken struct {
	readyAt time.Time
	order   int
	tokenID int
}

// tokenHeap implements heap.Interface for delayed tokens.
type tokenHeap []scheduledToken

func (h tokenHeap) Len() int            { return len(h) }
func (h tokenHeap) Less(i, j int) bool  { return h[i].readyAt.Before(h[j].readyAt) }
func (h tokenHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *tokenHeap) Push(x interface{}) { *h = append(*h, x.(scheduledToken)) }
func (h *tokenHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

// Scheduler implements a bounded async concurrency scheduler with delayed requeue.
type Scheduler struct {
	concurrency int
	ready       chan int
	delayed     tokenHeap
	mu          sync.Mutex
	closed      bool
	closeChan   chan struct{}
	order       int
}

// NewScheduler creates a new scheduler with the given concurrency limit.
func NewScheduler(concurrency int) *Scheduler {
	if concurrency <= 0 {
		concurrency = 1
	}
	s := &Scheduler{
		concurrency: concurrency,
		ready:       make(chan int, concurrency),
		closeChan:   make(chan struct{}),
	}
	// Pre-fill tokens
	for i := 0; i < concurrency; i++ {
		s.ready <- i
	}
	return s
}

// Start begins the scheduler's delayed token waker goroutine.
func (s *Scheduler) Start() {
	go s.wakerLoop()
}

// Acquire blocks until a token is available or the scheduler is closed.
// Returns the token ID, or -1 if closed.
func (s *Scheduler) Acquire() int {
	// Check if already closed
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return -1
	}
	s.mu.Unlock()

	select {
	case tokenID := <-s.ready:
		// Double-check closed state after acquiring token
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return -1
		}
		s.mu.Unlock()
		return tokenID
	case <-s.closeChan:
		return -1
	}
}

// Release returns a token to the scheduler. If delay > 0, the token will be
// re-issued after the delay.
func (s *Scheduler) Release(tokenID int, delay time.Duration) {
	if delay <= 0 {
		s.releaseNow(tokenID)
		return
	}

	s.mu.Lock()
	heap.Push(&s.delayed, scheduledToken{
		readyAt: time.Now().Add(delay),
		order:   s.order,
		tokenID: tokenID,
	})
	s.order++
	s.mu.Unlock()
}

func (s *Scheduler) releaseNow(tokenID int) {
	select {
	case s.ready <- tokenID:
	case <-s.closeChan:
	}
}

// Close shuts down the scheduler.
func (s *Scheduler) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.closeChan)
	s.mu.Unlock()
}

func (s *Scheduler) wakerLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.closeChan:
			return
		case <-ticker.C:
			s.processDelayed()
		}
	}
}

func (s *Scheduler) processDelayed() {
	now := time.Now()
	s.mu.Lock()
	for s.delayed.Len() > 0 {
		item := s.delayed[0]
		if item.readyAt.After(now) {
			break
		}
		heap.Pop(&s.delayed)
		s.mu.Unlock()
		s.releaseNow(item.tokenID)
		s.mu.Lock()
	}
	s.mu.Unlock()
}
