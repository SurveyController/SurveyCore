package models

import (
	"fmt"
	"sync"
	"time"
)

// ThreadProgressState tracks per-worker-thread progress.
type ThreadProgressState struct {
	ThreadName    string  `json:"thread_name"`
	ThreadIndex   int     `json:"thread_index"`
	OwnerID       int     `json:"owner_id"`
	SuccessCount  int     `json:"success_count"`
	FailCount     int     `json:"fail_count"`
	StepCurrent   int     `json:"step_current"`
	StepTotal     int     `json:"step_total"`
	StatusText    string  `json:"status_text"`
	Running       bool    `json:"running"`
	LastUpdateTS  float64 `json:"last_update_ts"`
}

// ExecutionConfig is the static, thread-safe snapshot used at runtime.
type ExecutionConfig struct {
	URL             string `json:"url"`
	SurveyTitle     string `json:"survey_title"`
	SurveyProvider  string `json:"survey_provider"`

	SingleProb    []any    `json:"single_prob,omitempty"`
	DroplistProb  []any    `json:"droplist_prob,omitempty"`
	MultipleProb  [][]float64 `json:"multiple_prob,omitempty"`
	MatrixProb    []any    `json:"matrix_prob,omitempty"`
	ScaleProb     []any    `json:"scale_prob,omitempty"`
	SliderTargets []float64   `json:"slider_targets,omitempty"`

	Texts         [][]string `json:"texts,omitempty"`
	TextsProb     [][]float64 `json:"texts_prob,omitempty"`
	TextEntryTypes []string   `json:"text_entry_types,omitempty"`
	TextAIFlags   []bool     `json:"text_ai_flags,omitempty"`
	TextTitles    []string   `json:"text_titles,omitempty"`
	LocationParts map[int][]string `json:"location_parts,omitempty"`

	MultiTextBlankModes     [][]string     `json:"multi_text_blank_modes,omitempty"`
	MultiTextBlankAIFlags   [][]bool       `json:"multi_text_blank_ai_flags,omitempty"`
	MultiTextBlankIntRanges [][][]int      `json:"multi_text_blank_int_ranges,omitempty"`

	SingleOptionFillTexts        [][]*string        `json:"single_option_fill_texts,omitempty"`
	SingleAttachedOptionSelects  [][]map[string]any `json:"single_attached_option_selects,omitempty"`
	DroplistOptionFillTexts      [][]*string        `json:"droplist_option_fill_texts,omitempty"`
	MultipleOptionFillTexts      [][]*string        `json:"multiple_option_fill_texts,omitempty"`

	AnswerRules      []map[string]any `json:"answer_rules,omitempty"`
	ReverseFillSpec  any              `json:"reverse_fill_spec,omitempty"`

	QuestionConfigIndexMap         map[int]string    `json:"question_config_index_map,omitempty"`
	ProviderQuestionConfigIndexMap map[string]string `json:"provider_question_config_index_map,omitempty"`
	QuestionDimensionMap           map[int]*string   `json:"question_dimension_map,omitempty"`
	QuestionOrdinalScoreMap        map[int][]int     `json:"question_ordinal_score_map,omitempty"`
	QuestionStrictRatioMap         map[int]bool      `json:"question_strict_ratio_map,omitempty"`
	QuestionsMetadata              map[int]SurveyQuestionMeta `json:"questions_metadata,omitempty"`
	ProviderQuestionMetadataMap    map[string]SurveyQuestionMeta `json:"provider_question_metadata_map,omitempty"`
	JointPsychometricAnswerPlan    any               `json:"joint_psychometric_answer_plan,omitempty"`

	PsychoTargetAlpha float64 `json:"psycho_target_alpha"`

	NumThreads        int  `json:"num_threads"`
	TargetNum         int  `json:"target_num"`
	FailThreshold     int  `json:"fail_threshold"`
	StopOnFailEnabled bool `json:"stop_on_fail_enabled"`

	SubmitIntervalRangeSeconds [2]int `json:"submit_interval_range_seconds"`
	AnswerDurationRangeSeconds [2]int `json:"answer_duration_range_seconds"`

	RandomProxyIPEnabled  bool              `json:"random_proxy_ip_enabled"`
	ProxySource           string            `json:"proxy_source"`
	RandomUserAgentEnabled bool             `json:"random_user_agent_enabled"`
	UserAgentRatios       map[string]int    `json:"user_agent_ratios"`
	PauseOnAliyunCaptcha  bool              `json:"pause_on_aliyun_captcha"`
}

// ExecutionState holds the mutable runtime state for a task run.
type ExecutionState struct {
	Config *ExecutionConfig `json:"-"`

	CurNum                  int    `json:"cur_num"`
	CurFail                 int    `json:"cur_fail"`
	ProxyUnavailableFailCount int  `json:"proxy_unavailable_fail_count"`
	DeviceQuotaFailCount    int    `json:"device_quota_fail_count"`
	TerminalStopCategory    string `json:"terminal_stop_category"`
	TerminalFailureReason   string `json:"terminal_failure_reason"`
	TerminalStopMessage     string `json:"terminal_stop_message"`

	ThreadProgress map[string]*ThreadProgressState `json:"thread_progress"`

	ProxyWaitingThreads      int                  `json:"proxy_waiting_threads"`
	ProxyInUseByThread       map[string]ProxyLease `json:"proxy_in_use_by_thread"`
	SuccessfulProxyAddresses map[string]bool       `json:"successful_proxy_addresses"`
	ProxyCooldownUntil       map[string]float64    `json:"proxy_cooldown_until"`

	StopChan   chan struct{} `json:"-"`
	PauseChan  chan struct{} `json:"-"`
	ResumeChan chan struct{} `json:"-"`

	terminalStopOnce sync.Once
	mu               sync.RWMutex
}

// NewExecutionState creates a new ExecutionState with initialized maps.
func NewExecutionState() *ExecutionState {
	return &ExecutionState{
		ThreadProgress:           make(map[string]*ThreadProgressState),
		ProxyInUseByThread:       make(map[string]ProxyLease),
		SuccessfulProxyAddresses: make(map[string]bool),
		ProxyCooldownUntil:       make(map[string]float64),
		StopChan:                 make(chan struct{}),
		PauseChan:                make(chan struct{}, 1),
		ResumeChan:               make(chan struct{}, 1),
	}
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
