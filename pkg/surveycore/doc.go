// Package surveycore provides the stable, high-level SurveyCore SDK.
//
// Parse, DefaultConfig, Submit, Run, and RunWithEvents honor context cancellation.
// Cancellation stops scheduling new submissions and waits for active provider
// calls to return. Advanced controls live in the experimental runtime, proxy,
// and config subpackages.
package surveycore
