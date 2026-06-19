package surveycore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SurveyController/SurveyCore/internal/engine"
	"github.com/SurveyController/SurveyCore/internal/models"
	runstate "github.com/SurveyController/SurveyCore/internal/runtime"
	"github.com/SurveyController/SurveyCore/internal/surveyrun"
)

func (c *Client) Run(ctx context.Context, cfg *RuntimeConfig) (*RunResult, error) {
	return c.RunWithEvents(ctx, cfg, nil)
}

func (c *Client) RunWithEvents(ctx context.Context, cfg *RuntimeConfig, handler EventHandler) (*RunResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: 配置为空", ErrInvalidConfig)
	}
	runtimeCfg := cloneRuntimeConfig(cfg)
	statusHandler := func(event engine.StatusEvent) {
		if handler != nil {
			handler(mapEvent(event))
		}
	}
	state, err := surveyrun.New(c.registry, c.applyExecutionDefaults).Run(ctx, runtimeCfg, statusHandler)
	if err != nil {
		return resultFromState(state), wrapRunError(err)
	}
	return resultFromState(state), nil
}

func wrapRunError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	switch {
	case message == "配置为空", message == "必须提供问卷链接":
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	case strings.HasPrefix(message, "解析问卷失败"):
		return fmt.Errorf("%w: %v", ErrParseFailed, err)
	case strings.HasPrefix(message, "准备执行配置失败"):
		return fmt.Errorf("%w: %v", ErrPrepareConfigFailed, err)
	default:
		return fmt.Errorf("%w: %v", ErrRunFailed, err)
	}
}

func resultFromState(state *runstate.ExecutionState) *RunResult {
	if state == nil {
		return &RunResult{}
	}
	stopped := state.IsStopped()
	snapshot := state.Snapshot()
	category, reason, message := snapshot.GetTerminalStopSnapshot()
	return &RunResult{
		Success:               snapshot.GetCurNum(),
		Fail:                  snapshot.GetCurFail(),
		Stopped:               stopped || category != "",
		TerminalStopCategory:  category,
		TerminalFailureReason: reason,
		TerminalStopMessage:   message,
		ThreadProgress:        mapThreadProgress(snapshot),
	}
}

func mapThreadProgress(state *runstate.ExecutionState) []ThreadProgress {
	if state == nil {
		return nil
	}
	raw := state.SnapshotThreadProgress()
	progress := make([]ThreadProgress, 0, len(raw))
	for _, item := range raw {
		progress = append(progress, ThreadProgress{
			ThreadName:   stringValue(item["thread_name"]),
			ThreadIndex:  intValue(item["thread_index"]),
			SuccessCount: intValue(item["success_count"]),
			FailCount:    intValue(item["fail_count"]),
			StepCurrent:  intValue(item["step_current"]),
			StepTotal:    intValue(item["step_total"]),
			StatusText:   stringValue(item["status_text"]),
			Running:      boolValue(item["running"]),
			LastUpdate:   unixTime(floatValue(item["last_update"])),
		})
	}
	return progress
}

func cloneRuntimeConfig(cfg *models.RuntimeConfig) *models.RuntimeConfig {
	if cfg == nil {
		return nil
	}
	data, err := models.SerializeRuntimeConfig(cfg)
	if err != nil {
		copy := *cfg
		return &copy
	}
	cloned, err := models.DeserializeRuntimeConfig(data)
	if err != nil {
		copy := *cfg
		return &copy
	}
	return cloned
}

func stringValue(value any) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

func intValue(value any) int {
	if typed, ok := value.(int); ok {
		return typed
	}
	return 0
}

func boolValue(value any) bool {
	if typed, ok := value.(bool); ok {
		return typed
	}
	return false
}

func floatValue(value any) float64 {
	if typed, ok := value.(float64); ok {
		return typed
	}
	return 0
}

func unixTime(value float64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.Unix(int64(value), 0)
}
