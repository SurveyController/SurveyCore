package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/internal/answerplan"
	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

func runExecutionWorker(
	ctx context.Context,
	config *RunRequest,
	submit SubmitFunc,
	options ExecutionOptions,
	state *executionState,
	workerIndex int,
	workerName string,
	jobs <-chan int,
	recordError func(error),
	cancel context.CancelFunc,
) {
	state.setProgress(workerIndex, workerName, "等待任务", true)
	defer state.setProgress(workerIndex, workerName, "空闲", false)
	hasSubmitted := false
	for jobIndex := range jobs {
		if waitIfExecutionPaused(ctx, options, state, workerIndex, workerName) {
			return
		}
		if options.IntervalBeforeAttempt && jobIndex > 0 {
			if err := waitAttemptInterval(ctx, config.ExecutionPlan.SubmitInterval, state, workerIndex, workerName, options.WaitInterval); err != nil {
				recordError(err)
				cancel()
				return
			}
		} else if !options.IntervalBeforeAttempt && hasSubmitted {
			waitSubmitInterval(ctx, config.ExecutionPlan.SubmitInterval, state, workerIndex, workerName)
		}
		if waitIfExecutionPaused(ctx, options, state, workerIndex, workerName) || ctx.Err() != nil {
			return
		}
		started := time.Now()
		consecutiveFailures, err := runOneJob(ctx, config, submit, options, state, workerIndex, workerName, jobIndex)
		// Retain completed results if cancellation arrives just after the attempt.
		contextErr := ctx.Err()
		if contextErr == nil || !errors.Is(err, contextErr) {
			result := model.SubmissionResult{Attempt: jobIndex + 1, Provider: config.SurveySource.Provider, Success: err == nil, ElapsedSeconds: time.Since(started).Seconds()}
			if err != nil {
				message := err.Error()
				result.Error = &message
			}
			state.emitResult(workerName, result)
		}
		if err != nil {
			recordError(err)
			if (!options.ContinueOnFailure && !isRetryableRunError(err)) || (options.FailStop && consecutiveFailures >= failStopThreshold(options)) {
				cancel()
			}
		} else {
			hasSubmitted = true
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func runOneJob(ctx context.Context, cfg *RunRequest, submit SubmitFunc, options ExecutionOptions, state *executionState, workerIndex int, workerName string, jobIndex int) (int, error) {
	attempts := options.MaxRetries + 1
	if attempts <= 0 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			state.setProgress(workerIndex, workerName, "已停止", false)
			return 0, err
		}
		owner := fmt.Sprintf("%s-%d-%d", workerName, jobIndex+1, attempt)
		lease, leased, leaseErr := acquireExecutionLease(ctx, options, state, workerIndex, workerName, owner)
		if leaseErr != nil {
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			if shouldRetry(leaseErr, attempt, attempts) {
				state.emit(workerName, "代理不可用，准备重试", false, true)
				sleepRetry(ctx, options.RetryDelay)
				continue
			}
			return state.addFail(workerIndex, workerName, "代理不可用"), leaseErr
		}

		job := JobRequest{
			Answers: cloneAnswerPlan(cfg.AnswerPlan),
			Submission: model.SubmissionRequest{
				Source:               cfg.SurveySource,
				Definition:           cloneSurveyDefinition(cfg.SurveyDefinition),
				AnswerDuration:       cfg.AnswerDuration,
				AnswerDatetimeWindow: cfg.AnswerDatetimeWindow,
				Context: model.SubmissionContext{
					Runtime:   options.AnswerRuntime,
					AIProfile: options.AIProfile,
					UserAgent: model.RuntimeUserAgent(options.UserAgent),
				},
			},
		}
		if leased {
			job.Submission.Context.ProxyAddress = lease.Address
		}
		job.Submission.Context.RuntimeOwner = owner
		resetPendingAnswerRuntime(job.Submission.Context.Runtime, owner)
		if options.ConfigureJob != nil {
			if err := options.ConfigureJob(ctx, jobIndex, attempt, &job); err != nil {
				releaseExecutionLease(options, owner, lease, leased, err)
				resetPendingAnswerRuntime(job.Submission.Context.Runtime, owner)
				if ctx.Err() != nil {
					return 0, ctx.Err()
				}
				return state.addFail(workerIndex, workerName, "配置失败"), err
			}
		}
		actions, err := answerplan.BuildActionsWithLogic(
			job.Submission.Definition.Questions,
			job.Answers.Strategies,
			answerplan.OptionsFromAnswerPlan(job.Answers, job.Submission.Context),
		)
		if err != nil {
			releaseExecutionLease(options, owner, lease, leased, err)
			resetPendingAnswerRuntime(job.Submission.Context.Runtime, owner)
			return state.addFail(workerIndex, workerName, "生成答案失败"), err
		}
		job.Submission.Context.Actions = convertActions(actions)
		state.setProgress(workerIndex, workerName, "提交中", true)
		result, err := submit(ctx, &job.Submission, func(event Event) {
			state.forward(workerIndex, workerName, event)
		})
		releaseExecutionLease(options, owner, lease, leased, err)
		finalizeAnswerRuntime(job.Submission.Context.Runtime, owner, err == nil)
		if err != nil && ctx.Err() != nil {
			state.setProgress(workerIndex, workerName, "已停止", false)
			return 0, ctx.Err()
		}

		if err == nil {
			state.addSuccess(workerIndex, workerName, "提交成功")
			return 0, nil
		}
		statusText := statusFromRunResult(result, err)
		if shouldRetry(err, attempt, attempts) {
			state.setProgress(workerIndex, workerName, "准备重试", true)
			state.emit(workerName, "提交失败，准备重试", false, true)
			sleepRetry(ctx, options.RetryDelay)
			continue
		}
		return state.addFail(workerIndex, workerName, statusText), err
	}
	return state.addFail(workerIndex, workerName, "failed"), ErrRunFailed
}
