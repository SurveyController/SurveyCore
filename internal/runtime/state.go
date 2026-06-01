package runstate

import (
	"fmt"
	"sync"
	"time"

	"github.com/SurveyController/SurveyCore/internal/domain"
	"github.com/SurveyController/SurveyCore/internal/execution"
)

// ThreadProgressState tracks per-worker-thread progress.
type ThreadProgressState struct {
	ThreadName   string  `json:"thread_name"`
	ThreadIndex  int     `json:"thread_index"`
	OwnerID      int     `json:"owner_id"`
	SuccessCount int     `json:"success_count"`
	FailCount    int     `json:"fail_count"`
	StepCurrent  int     `json:"step_current"`
	StepTotal    int     `json:"step_total"`
	StatusText   string  `json:"status_text"`
	Running      bool    `json:"running"`
	LastUpdateTS float64 `json:"last_update_ts"`
}

// ExecutionState holds the mutable runtime state for a task run.
type ExecutionState struct {
	Config *execution.ExecutionConfig `json:"-"`

	CurNum                int    `json:"cur_num"`
	CurFail               int    `json:"cur_fail"`
	TerminalStopCategory  string `json:"terminal_stop_category"`
	TerminalFailureReason string `json:"terminal_failure_reason"`
	TerminalStopMessage   string `json:"terminal_stop_message"`

	ThreadProgress     map[string]*ThreadProgressState `json:"thread_progress"`
	InFlight           int                             `json:"-"`
	ReverseFillRuntime *domain.ReverseFillRuntimeState `json:"-"`

	StopChan   chan struct{} `json:"-"`
	PauseChan  chan struct{} `json:"-"`
	ResumeChan chan struct{} `json:"-"`

	terminalStopOnce sync.Once
	mu               sync.RWMutex
}

// NewExecutionState creates a new ExecutionState with initialized maps.
func NewExecutionState() *ExecutionState {
	return &ExecutionState{
		ThreadProgress: make(map[string]*ThreadProgressState),
		StopChan:       make(chan struct{}),
		PauseChan:      make(chan struct{}, 1),
		ResumeChan:     make(chan struct{}, 1),
	}
}

// Snapshot returns a JSON-safe copy of the mutable runtime state.
func (s *ExecutionState) Snapshot() *ExecutionState {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	copy := &ExecutionState{
		CurNum:                s.CurNum,
		CurFail:               s.CurFail,
		TerminalStopCategory:  s.TerminalStopCategory,
		TerminalFailureReason: s.TerminalFailureReason,
		TerminalStopMessage:   s.TerminalStopMessage,
		ThreadProgress:        make(map[string]*ThreadProgressState, len(s.ThreadProgress)),
	}
	for key, value := range s.ThreadProgress {
		if value == nil {
			copy.ThreadProgress[key] = nil
			continue
		}
		threadCopy := *value
		copy.ThreadProgress[key] = &threadCopy
	}
	return copy
}

// MarkTerminalStop records a terminal stop condition (first-write-wins).
func (s *ExecutionState) MarkTerminalStop(category, failureReason, message string) {
	s.terminalStopOnce.Do(func() {
		s.mu.Lock()
		s.TerminalStopCategory = category
		s.TerminalFailureReason = failureReason
		s.TerminalStopMessage = message
		s.mu.Unlock()
	})
}

// GetTerminalStopSnapshot returns the terminal stop fields.
func (s *ExecutionState) GetTerminalStopSnapshot() (category, reason, message string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TerminalStopCategory, s.TerminalFailureReason, s.TerminalStopMessage
}

// EnsureWorkerThreads initializes thread progress entries for the expected count.
func (s *ExecutionState) EnsureWorkerThreads(expectedCount int, prefix string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if prefix == "" {
		prefix = "Worker"
	}
	for i := 0; i < expectedCount; i++ {
		name := prefix + "-" + itoa(i+1)
		if _, ok := s.ThreadProgress[name]; !ok {
			s.ThreadProgress[name] = &ThreadProgressState{
				ThreadName:  name,
				ThreadIndex: i,
				StatusText:  "等待中",
			}
		}
	}
}

// UpdateThreadStatus updates a thread's status text and running flag.
func (s *ExecutionState) UpdateThreadStatus(threadName, statusText string, running *bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tp, ok := s.ThreadProgress[threadName]
	if !ok {
		tp = &ThreadProgressState{ThreadName: threadName, StatusText: statusText}
		s.ThreadProgress[threadName] = tp
	}
	tp.StatusText = statusText
	tp.LastUpdateTS = float64(time.Now().Unix())
	if running != nil {
		tp.Running = *running
	}
}

// IncrementThreadSuccess increments the success counter for a thread.
func (s *ExecutionState) IncrementThreadSuccess(threadName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tp, ok := s.ThreadProgress[threadName]
	if !ok {
		tp = &ThreadProgressState{ThreadName: threadName}
		s.ThreadProgress[threadName] = tp
	}
	tp.SuccessCount++
	tp.StatusText = "提交成功"
	tp.LastUpdateTS = float64(time.Now().Unix())
}

// IncrementThreadFail increments the fail counter for a thread.
func (s *ExecutionState) IncrementThreadFail(threadName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tp, ok := s.ThreadProgress[threadName]
	if !ok {
		tp = &ThreadProgressState{ThreadName: threadName}
		s.ThreadProgress[threadName] = tp
	}
	tp.FailCount++
	tp.StatusText = "失败重试"
	tp.LastUpdateTS = float64(time.Now().Unix())
}

// IsStopped checks if the stop signal has been sent.
func (s *ExecutionState) IsStopped() bool {
	select {
	case <-s.StopChan:
		return true
	default:
		return false
	}
}

// SignalStop sends the stop signal.
func (s *ExecutionState) SignalStop() {
	s.terminalStopOnce.Do(func() {}) // ensure once is claimed
	select {
	case <-s.StopChan:
	default:
		close(s.StopChan)
	}
}

// GetCurNum returns the current success count.
func (s *ExecutionState) GetCurNum() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CurNum
}

// GetCurFail returns the current fail count.
func (s *ExecutionState) GetCurFail() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CurFail
}

// IncrementSuccess increments the success counter.
func (s *ExecutionState) IncrementSuccess() {
	s.mu.Lock()
	s.CurNum++
	s.mu.Unlock()
}

// IncrementFail increments the fail counter.
func (s *ExecutionState) IncrementFail() {
	s.mu.Lock()
	s.CurFail++
	s.mu.Unlock()
}

// TryStartSubmission reserves one target slot before a worker submits.
func (s *ExecutionState) TryStartSubmission(target int) bool {
	if target <= 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.CurNum+s.InFlight >= target {
		return false
	}
	s.InFlight++
	return true
}

// AbortSubmissionReservation releases a reserved target slot without counting it.
func (s *ExecutionState) AbortSubmissionReservation() {
	s.mu.Lock()
	if s.InFlight > 0 {
		s.InFlight--
	}
	s.mu.Unlock()
}

// CompleteSubmission releases a reservation and updates the success/fail counters.
func (s *ExecutionState) CompleteSubmission(success bool) {
	s.mu.Lock()
	if s.InFlight > 0 {
		s.InFlight--
	}
	if success {
		s.CurNum++
	} else {
		s.CurFail++
	}
	s.mu.Unlock()
}

// InitializeReverseFillRuntime initializes reverse-fill sample queues from config.
func (s *ExecutionState) InitializeReverseFillRuntime() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Config == nil || s.Config.ReverseFillSpec == nil {
		s.ReverseFillRuntime = nil
		return
	}
	runtime := &domain.ReverseFillRuntimeState{
		Spec:                s.Config.ReverseFillSpec,
		QueuedRowNumbers:    make([]int, 0, len(s.Config.ReverseFillSpec.Samples)),
		SamplesByRowNumber:  make(map[int]domain.ReverseFillSampleRow),
		ReservedRowByThread: make(map[string]int),
		FailureCountByRow:   make(map[int]int),
		CommittedRowNumbers: make(map[int]bool),
		DiscardedRowNumbers: make(map[int]bool),
	}
	for _, sample := range s.Config.ReverseFillSpec.Samples {
		runtime.SamplesByRowNumber[sample.DataRowNumber] = sample
		runtime.QueuedRowNumbers = append(runtime.QueuedRowNumbers, sample.DataRowNumber)
	}
	s.ReverseFillRuntime = runtime
}

// HasReverseFillRuntime returns whether reverse-fill is active.
func (s *ExecutionState) HasReverseFillRuntime() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ReverseFillRuntime != nil
}

func reverseFillThreadKey(threadName string) string {
	if threadName == "" {
		return "Worker-?"
	}
	return threadName
}

// AcquireReverseFillSample reserves one source sample for a worker.
func (s *ExecutionState) AcquireReverseFillSample(threadName string) domain.ReverseFillAcquireResult {
	key := reverseFillThreadKey(threadName)
	s.mu.Lock()
	defer s.mu.Unlock()
	runtime := s.ReverseFillRuntime
	if runtime == nil {
		return domain.ReverseFillAcquireResult{Status: "disabled", Message: "reverse_fill_disabled"}
	}
	if existingRow, ok := runtime.ReservedRowByThread[key]; ok {
		if sample, exists := runtime.SamplesByRowNumber[existingRow]; exists {
			return domain.ReverseFillAcquireResult{Status: "acquired", Sample: &sample, Message: "already_reserved"}
		}
		delete(runtime.ReservedRowByThread, key)
	}
	for len(runtime.QueuedRowNumbers) > 0 {
		rowNumber := runtime.QueuedRowNumbers[0]
		runtime.QueuedRowNumbers = runtime.QueuedRowNumbers[1:]
		sample, ok := runtime.SamplesByRowNumber[rowNumber]
		if !ok {
			continue
		}
		runtime.ReservedRowByThread[key] = rowNumber
		return domain.ReverseFillAcquireResult{Status: "acquired", Sample: &sample, Message: "reserved"}
	}
	if s.reverseFillPossibleTotalLocked() < maxInt(0, s.Config.TargetNum) {
		return domain.ReverseFillAcquireResult{Status: "exhausted", Message: "reverse_fill_target_unreachable"}
	}
	return domain.ReverseFillAcquireResult{Status: "waiting", Message: "reverse_fill_waiting"}
}

// ReleaseReverseFillSample releases a reserved source sample.
func (s *ExecutionState) ReleaseReverseFillSample(threadName string, requeue bool) *int {
	key := reverseFillThreadKey(threadName)
	s.mu.Lock()
	defer s.mu.Unlock()
	runtime := s.ReverseFillRuntime
	if runtime == nil {
		return nil
	}
	rowNumber, ok := runtime.ReservedRowByThread[key]
	if !ok {
		return nil
	}
	delete(runtime.ReservedRowByThread, key)
	if requeue && !runtime.CommittedRowNumbers[rowNumber] && !runtime.DiscardedRowNumbers[rowNumber] {
		runtime.QueuedRowNumbers = append([]int{rowNumber}, runtime.QueuedRowNumbers...)
	}
	return &rowNumber
}

// CommitReverseFillSample marks a worker's reserved source sample as consumed.
func (s *ExecutionState) CommitReverseFillSample(threadName string) *int {
	key := reverseFillThreadKey(threadName)
	s.mu.Lock()
	defer s.mu.Unlock()
	runtime := s.ReverseFillRuntime
	if runtime == nil {
		return nil
	}
	rowNumber, ok := runtime.ReservedRowByThread[key]
	if !ok {
		return nil
	}
	delete(runtime.ReservedRowByThread, key)
	runtime.CommittedRowNumbers[rowNumber] = true
	delete(runtime.FailureCountByRow, rowNumber)
	return &rowNumber
}

// MarkReverseFillSubmissionFailed requeues or discards a failed sample.
func (s *ExecutionState) MarkReverseFillSubmissionFailed(threadName string, maxRetries int) (*int, bool) {
	key := reverseFillThreadKey(threadName)
	s.mu.Lock()
	defer s.mu.Unlock()
	runtime := s.ReverseFillRuntime
	if runtime == nil {
		return nil, false
	}
	rowNumber, ok := runtime.ReservedRowByThread[key]
	if !ok {
		return nil, false
	}
	delete(runtime.ReservedRowByThread, key)
	nextCount := runtime.FailureCountByRow[rowNumber] + 1
	runtime.FailureCountByRow[rowNumber] = nextCount
	if nextCount <= maxInt(0, maxRetries) {
		runtime.QueuedRowNumbers = append([]int{rowNumber}, runtime.QueuedRowNumbers...)
		return &rowNumber, false
	}
	runtime.DiscardedRowNumbers[rowNumber] = true
	return &rowNumber, true
}

// GetReverseFillAnswer returns the reserved sample answer for a question.
func (s *ExecutionState) GetReverseFillAnswer(questionNum int, threadName string) *domain.ReverseFillAnswer {
	key := reverseFillThreadKey(threadName)
	s.mu.RLock()
	defer s.mu.RUnlock()
	runtime := s.ReverseFillRuntime
	if runtime == nil {
		return nil
	}
	rowNumber, ok := runtime.ReservedRowByThread[key]
	if !ok {
		return nil
	}
	sample, ok := runtime.SamplesByRowNumber[rowNumber]
	if !ok {
		return nil
	}
	answer, ok := sample.Answers[questionNum]
	if !ok {
		return nil
	}
	return &answer
}

// IsReverseFillTargetUnreachable reports whether remaining samples cannot hit target.
func (s *ExecutionState) IsReverseFillTargetUnreachable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ReverseFillRuntime == nil || s.Config == nil || s.Config.TargetNum <= 0 {
		return false
	}
	return s.reverseFillPossibleTotalLocked() < s.Config.TargetNum
}

func (s *ExecutionState) reverseFillPossibleTotalLocked() int {
	if s.ReverseFillRuntime == nil {
		return s.CurNum
	}
	return s.CurNum + len(s.ReverseFillRuntime.QueuedRowNumbers) + len(s.ReverseFillRuntime.ReservedRowByThread)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return fmt.Sprintf("%d", i)
}

// SnapshotThreadProgress returns a snapshot of all thread progress as maps.
func (s *ExecutionState) SnapshotThreadProgress() []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]map[string]any, 0, len(s.ThreadProgress))
	for _, tp := range s.ThreadProgress {
		result = append(result, map[string]any{
			"thread_name":   tp.ThreadName,
			"thread_index":  tp.ThreadIndex,
			"success_count": tp.SuccessCount,
			"fail_count":    tp.FailCount,
			"step_current":  tp.StepCurrent,
			"step_total":    tp.StepTotal,
			"status_text":   tp.StatusText,
			"running":       tp.Running,
			"last_update":   tp.LastUpdateTS,
		})
	}
	return result
}
