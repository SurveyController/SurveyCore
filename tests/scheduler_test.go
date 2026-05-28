package tests

import (
	"testing"
	"time"

	"github.com/SurveyController/SurveyController-Go/internal/engine"
)

func TestSchedulerConcurrency(t *testing.T) {
	s := engine.NewScheduler(3)
	s.Start()
	defer s.Close()

	// Should be able to acquire 3 tokens
	tokens := make([]int, 3)
	for i := 0; i < 3; i++ {
		tokens[i] = s.Acquire()
		if tokens[i] < 0 {
			t.Fatalf("Acquire() returned %d on iteration %d", tokens[i], i)
		}
	}

	// All tokens should be different
	seen := make(map[int]bool)
	for _, tok := range tokens {
		if seen[tok] {
			t.Errorf("Duplicate token: %d", tok)
		}
		seen[tok] = true
	}
}

func TestSchedulerRelease(t *testing.T) {
	s := engine.NewScheduler(2)
	s.Start()
	defer s.Close()

	tok1 := s.Acquire()
	tok2 := s.Acquire()

	// Release one
	s.Release(tok1, 0)

	// Should be able to acquire again
	tok3 := s.Acquire()
	if tok3 < 0 {
		t.Error("Should be able to acquire after release")
	}

	s.Release(tok2, 0)
	s.Release(tok3, 0)
}

func TestSchedulerDelayedRelease(t *testing.T) {
	s := engine.NewScheduler(1)
	s.Start()
	defer s.Close()

	tok := s.Acquire()

	// Release with delay
	s.Release(tok, 200*time.Millisecond)

	// After delay, should be available
	time.Sleep(300 * time.Millisecond)
	tok3 := s.Acquire()
	if tok3 < 0 {
		t.Error("Should be available after delay")
	}
	s.Release(tok3, 0)
}

func TestSchedulerClose(t *testing.T) {
	s := engine.NewScheduler(2)
	s.Start()

	tok := s.Acquire()
	s.Release(tok, 0)

	s.Close()

	// Acquire should return -1 after close
	tok2 := s.Acquire()
	if tok2 != -1 {
		t.Errorf("Acquire after close = %d, want -1", tok2)
	}
}
