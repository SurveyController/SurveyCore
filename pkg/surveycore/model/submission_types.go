package model

// SubmissionResult describes one submission attempt, matching surveycore-py.
// Standalone Submit calls use attempt 0. Error is null on success.
type SubmissionResult struct {
	Attempt        int     `json:"attempt"`
	Provider       string  `json:"provider"`
	Success        bool    `json:"success"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	Error          *string `json:"error"`
}
