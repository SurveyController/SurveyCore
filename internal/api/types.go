package api

type createConfigRequest struct {
	URL string `json:"url"`
}

type parseSurveyRequest struct {
	URL string `json:"url"`
}
