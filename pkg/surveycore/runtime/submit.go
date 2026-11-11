package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

// Submit executes exactly one submission without task-level retries or intervals.
// A valid request returns a result even when execution fails; err retains the
// typed cause for errors.Is and ClassifyRunError.
func (c *Client) Submit(ctx context.Context, config *RunRequest) (*model.SubmissionResult, error) {
	started := time.Now()
	provider, err := submissionProvider(config)
	if err != nil {
		return nil, err
	}
	cfg := cloneRunRequest(config)
	cfg.SurveySource.Provider = provider
	cfg.ExecutionPlan.Target = 1
	cfg.ExecutionPlan.Threads = 1
	cfg.ExecutionPlan.SubmitInterval = [2]int{}
	cfg.ExecutionPlan.FailStop = false
	cfg.ExecutionPlan.PauseOnAliyunCaptcha = false
	cfg.ReverseFillPlan.Threads = 1
	options := ExecutionOptionsFromConfig(&cfg)
	options.MaxRetries = 0
	var runResult *RunResult
	if err = ctx.Err(); err == nil {
		if len(cfg.AnswerPlan.Strategies) == 0 {
			if len(cfg.SurveyDefinition.Questions) == 0 {
				var definition *SurveyDefinition
				definition, err = c.Parse(ctx, cfg.SurveySource.URL)
				if err == nil {
					cfg.SurveyDefinition = *definition
				}
			}
			if err == nil {
				ensureQuestionStrategies(&cfg)
			}
		}
		if err == nil {
			runResult, err = c.RunWithExecutionOptions(ctx, &cfg, nil, options)
		}
	}
	if err != nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	result := &model.SubmissionResult{
		Provider:       provider,
		Success:        err == nil && runResult != nil && runResult.Success == 1 && runResult.Fail == 0 && !runResult.Stopped,
		ElapsedSeconds: time.Since(started).Seconds(),
	}
	if !result.Success {
		if err == nil {
			err = fmt.Errorf("%w: 平台未确认提交成功", ErrRunFailed)
		}
		message := err.Error()
		result.Error = &message
	}
	return result, err
}

// Submit uses a default client for one submission.
func Submit(ctx context.Context, config *RunRequest) (*model.SubmissionResult, error) {
	return New().Submit(ctx, config)
}

func submissionProvider(config *RunRequest) (string, error) {
	if config == nil || strings.TrimSpace(config.SurveySource.URL) == "" {
		return "", fmt.Errorf("%w: 必须提供问卷配置和链接", ErrInvalidConfig)
	}
	detected := detectProvider(config.SurveySource.URL)
	provider := config.SurveySource.Provider
	if provider == "" {
		provider = detected
	}
	if provider != model.ProviderWJX && provider != model.ProviderQQ && provider != model.ProviderCredamo {
		return "", fmt.Errorf("%w: 不支持的问卷平台：%s", ErrUnsupportedOperation, provider)
	}
	if detected != "" && provider != detected {
		return "", fmt.Errorf("%w: 问卷链接平台与配置不一致：%s != %s", ErrUnsupportedOperation, provider, detected)
	}
	if config.SurveyDefinition.Provider != "" && config.SurveyDefinition.Provider != provider {
		return "", fmt.Errorf("%w: 问卷快照平台与配置不一致", ErrInvalidConfig)
	}
	return provider, nil
}
