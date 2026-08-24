package defaults

import "github.com/SurveyController/SurveyCore/pkg/surveycore/model"

func QuestionType(question model.QuestionMeta) string {
	switch question.ProviderType {
	case "single", "multiple", "dropdown", "scale", "score", "matrix", "order", "slider", "text":
		return question.ProviderType
	case "multi_text":
		return "text"
	case "radio":
		return "single"
	case "checkbox":
		return "multiple"
	case "select":
		return "dropdown"
	case "matrix_radio":
		return "matrix"
	case "textarea":
		return "text"
	}
	switch question.TypeCode {
	case "3":
		return "single"
	case "4":
		return "multiple"
	case "5":
		if question.IsRating {
			return "score"
		}
		return "scale"
	case "6":
		return "matrix"
	case "7":
		return "dropdown"
	case "8":
		return "slider"
	case "11":
		return "order"
	default:
		if question.IsTextLike || question.TextInputs > 0 {
			return "text"
		}
		return ""
	}
}

func QuestionProbabilities(question model.QuestionMeta) []float64 {
	if question.ForcedOptionIdx != nil && question.Options > 0 {
		values := make([]float64, question.Options)
		if *question.ForcedOptionIdx >= 0 && *question.ForcedOptionIdx < len(values) {
			values[*question.ForcedOptionIdx] = 1
			return values
		}
	}
	kind := QuestionType(question)
	count := maxInt(1, question.Options)
	values := make([]float64, count)
	for i := range values {
		if kind == "multiple" {
			values[i] = 50
		} else if kind == "score" {
			values[i] = scoreMiddleWeight(i, len(values))
		} else {
			values[i] = 1
		}
	}
	return values
}

func scoreMiddleWeight(index int, count int) float64 {
	if count <= 1 {
		return 1
	}
	center := float64(count-1) / 2
	distance := absFloat(float64(index) - center)
	return float64(count) - 2*distance
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
