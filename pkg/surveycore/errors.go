package surveycore

import surveyRuntime "github.com/SurveyController/SurveyCore/pkg/surveycore/runtime"

var (
	ErrInvalidConfig        = surveyRuntime.ErrInvalidConfig
	ErrParseFailed          = surveyRuntime.ErrParseFailed
	ErrPrepareConfigFailed  = surveyRuntime.ErrPrepareConfigFailed
	ErrRunFailed            = surveyRuntime.ErrRunFailed
	ErrUnsupportedOperation = surveyRuntime.ErrUnsupportedOperation
)

// ErrorKind classifies a SurveyCore failure without parsing its message.
type ErrorKind string

const (
	ErrorKindCanceled    ErrorKind = "canceled"
	ErrorKindParse       ErrorKind = "parse"
	ErrorKindConfig      ErrorKind = "config"
	ErrorKindUnsupported ErrorKind = "unsupported"
	ErrorKindRun         ErrorKind = "run"
)

// ClassifyRunError returns the stable category for err.
func ClassifyRunError(err error) ErrorKind {
	return ErrorKind(surveyRuntime.ClassifyRunError(err))
}
