package models

const (
	ReverseFillFormatAuto        = "auto"
	ReverseFillFormatWJXSequence = "wjx_sequence"
	ReverseFillFormatWJXScore    = "wjx_score"
	ReverseFillFormatWJXText     = "wjx_text"

	ReverseFillStatusReverse  = "reverse_fill"
	ReverseFillStatusFallback = "fallback_config"
	ReverseFillStatusBlocked  = "blocked"

	ReverseFillKindChoice    = "choice"
	ReverseFillKindText      = "text"
	ReverseFillKindMultiText = "multi_text"
	ReverseFillKindMatrix    = "matrix"
)

// ReverseFillAnswer is one parsed answer from a reverse-fill sample row.
type ReverseFillAnswer struct {
	QuestionNum         int      `json:"question_num"`
	Kind                string   `json:"kind"`
	ChoiceIndex         *int     `json:"choice_index,omitempty"`
	TextValue           string   `json:"text_value,omitempty"`
	TextValues          []string `json:"text_values,omitempty"`
	MatrixChoiceIndexes []int    `json:"matrix_choice_indexes,omitempty"`
}

// ReverseFillSampleRow is one reusable source row.
type ReverseFillSampleRow struct {
	DataRowNumber      int                       `json:"data_row_number"`
	WorksheetRowNumber int                       `json:"worksheet_row_number"`
	Answers            map[int]ReverseFillAnswer `json:"answers"`
}

// ReverseFillIssue describes a validation or mapping problem.
type ReverseFillIssue struct {
	QuestionNum int    `json:"question_num"`
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Reason      string `json:"reason"`
	Suggestion  string `json:"suggestion"`
	SampleRows  []int  `json:"sample_rows,omitempty"`
}

// ReverseFillQuestionPlan describes how a question is handled.
type ReverseFillQuestionPlan struct {
	QuestionNum      int      `json:"question_num"`
	Title            string   `json:"title"`
	QuestionType     string   `json:"question_type"`
	Status           string   `json:"status"`
	ColumnHeaders    []string `json:"column_headers,omitempty"`
	Detail           string   `json:"detail,omitempty"`
	FallbackReady    bool     `json:"fallback_ready,omitempty"`
	FallbackResolved bool     `json:"fallback_resolved,omitempty"`
}

// ReverseFillSpec is the immutable reverse-fill plan for a run.
type ReverseFillSpec struct {
	SourcePath       string                    `json:"source_path"`
	SelectedFormat   string                    `json:"selected_format"`
	DetectedFormat   string                    `json:"detected_format"`
	StartRow         int                       `json:"start_row"`
	TotalSamples     int                       `json:"total_samples"`
	AvailableSamples int                       `json:"available_samples"`
	TargetNum        int                       `json:"target_num"`
	QuestionPlans    []ReverseFillQuestionPlan `json:"question_plans,omitempty"`
	Issues           []ReverseFillIssue        `json:"issues,omitempty"`
	Samples          []ReverseFillSampleRow    `json:"samples,omitempty"`
}

// BlockingIssues returns all blocking validation issues.
func (s *ReverseFillSpec) BlockingIssues() []ReverseFillIssue {
	if s == nil {
		return nil
	}
	issues := make([]ReverseFillIssue, 0)
	for _, issue := range s.Issues {
		if issue.Severity == "block" {
			issues = append(issues, issue)
		}
	}
	return issues
}

// ReverseFillRuntimeState is mutable runtime state for sample reservation.
type ReverseFillRuntimeState struct {
	Spec                *ReverseFillSpec
	QueuedRowNumbers    []int
	SamplesByRowNumber  map[int]ReverseFillSampleRow
	ReservedRowByThread map[string]int
	FailureCountByRow   map[int]int
	CommittedRowNumbers map[int]bool
	DiscardedRowNumbers map[int]bool
}

// ReverseFillAcquireResult is returned by sample acquisition.
type ReverseFillAcquireResult struct {
	Status  string
	Sample  *ReverseFillSampleRow
	Message string
}
