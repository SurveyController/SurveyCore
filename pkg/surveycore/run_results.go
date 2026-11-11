package surveycore

import (
	"context"
	"iter"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

// RunResults yields attempts in completion order with one-based attempt numbers.
// Platform failures are unsuccessful results with a nil iterator error. Setup and
// context errors terminate iteration with an error. Breaking the loop cancels
// outstanding requests and waits for workers before returning.
func (client *Client) RunResults(ctx context.Context, config *model.RunRequest) iter.Seq2[SubmissionResult, error] {
	return func(yield func(SubmissionResult, error) bool) {
		runCtx, cancel := context.WithCancel(ctx)
		results := make(chan SubmissionResult)
		done := make(chan struct{})
		var aggregate *RunResult
		var runErr error
		go func() {
			defer close(done)
			defer close(results)
			aggregate, runErr = client.RunWithEvents(runCtx, config, func(event Event) {
				if event.Result == nil {
					return
				}
				select {
				case results <- *event.Result:
				case <-runCtx.Done():
				}
			})
		}()
		defer func() {
			cancel()
			<-done
		}()
		for result := range results {
			if !yield(result, nil) {
				return
			}
		}
		<-done
		if ctx.Err() != nil {
			yield(SubmissionResult{}, ctx.Err())
		} else if aggregate == nil && runErr != nil {
			yield(SubmissionResult{}, runErr)
		}
	}
}

// RunResults uses a default Client to yield results from a run.
func RunResults(ctx context.Context, config *model.RunRequest) iter.Seq2[SubmissionResult, error] {
	return New().RunResults(ctx, config)
}
