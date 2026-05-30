package tencent

import "regexp"

const (
	SubmitSuccess  = "success"
	SubmitRejected = "rejected"
)

var (
	urlRe            = regexp.MustCompile(`/s\d+/(\d+)/([A-Za-z0-9_-]+)/?$`)
	qqQuestionIDRe   = regexp.MustCompile(`\bq-[A-Za-z0-9_-]+\b`)
	qqPageIDRe       = regexp.MustCompile(`\bp-[A-Za-z0-9_-]+\b`)
	qqLogicEndTokens = []string{"submit", "finish", "complete", "end", "结束", "提交", "完成"}
)

// Type code mapping from QQ types
var qqTypeMap = map[string]string{
	"radio":        "3",
	"checkbox":     "4",
	"text":         "1",
	"textarea":     "1",
	"nps":          "5",
	"star":         "5",
	"matrix_radio": "6",
	"matrix_check": "6",
	"matrix_star":  "6",
	"select":       "7",
	"dropdown":     "7",
	"description":  "0",
}

var qqSupportedProviderTypes = map[string]bool{
	"radio":        true,
	"checkbox":     true,
	"text":         true,
	"textarea":     true,
	"nps":          true,
	"star":         true,
	"matrix_radio": true,
	"matrix_check": true,
	"matrix_star":  true,
	"select":       true,
	"dropdown":     true,
	"description":  true,
}

// TencentAnswerAction represents a Tencent survey answer.
type TencentAnswerAction struct {
	QuestionID      string
	QuestionType    string
	SelectedIDs     []string
	SelectedTexts   []string
	SelectedIndices []int
	TextValue       string
	MatrixAnswers   []TencentMatrixRow
	MatrixIndices   []int
}

type TencentMatrixRow struct {
	RowID     string
	OptionIDs []string
}
