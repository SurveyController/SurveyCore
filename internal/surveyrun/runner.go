package surveyrun

import (
	"context"
	"errors"
	"fmt"

	"github.com/SurveyController/SurveyCore/internal/config"
	"github.com/SurveyController/SurveyCore/internal/engine"
	"github.com/SurveyController/SurveyCore/internal/execution"
	"github.com/SurveyController/SurveyCore/internal/models"
	runstate "github.com/SurveyController/SurveyCore/internal/runtime"
	"github.com/SurveyController/SurveyCore/internal/surveyparse"
)

type EventHandler func(engine.StatusEvent)

type Hooks struct {
	OnParsed   func(*models.SurveyDefinition)
	OnPrepared func()
	OnEvent    EventHandler
}

type Runner struct {
	registry          engine.ProviderRegistry
	executionDefaults func(*execution.ExecutionConfig)
	parser            *surveyparse.Service
}

func New(registry engine.ProviderRegistry, executionDefaults func(*execution.ExecutionConfig)) *Runner {
	return &Runner{
		registry:          registry,
		executionDefaults: executionDefaults,
		parser:            surveyparse.New(registry),
	}
}

func (r *Runner) Run(ctx context.Context, cfg *models.RuntimeConfig, handler EventHandler) (*runstate.ExecutionState, error) {
	state := runstate.NewExecutionState()
	return r.RunWithHooks(ctx, cfg, state, Hooks{OnEvent: handler})
}

func (r *Runner) RunIntoState(ctx context.Context, cfg *models.RuntimeConfig, state *runstate.ExecutionState, handler EventHandler) (*runstate.ExecutionState, error) {
	return r.RunWithHooks(ctx, cfg, state, Hooks{OnEvent: handler})
}

func (r *Runner) RunWithHooks(ctx context.Context, cfg *models.RuntimeConfig, state *runstate.ExecutionState, hooks Hooks) (*runstate.ExecutionState, error) {
	if cfg == nil {
		return state, errors.New("配置为空")
	}
	if state == nil {
		state = runstate.NewExecutionState()
	}
	config.MergeDefaults(cfg)
	if cfg.URL == "" {
		return state, errors.New("必须提供问卷链接")
	}

	def, err := r.parser.Parse(ctx, cfg.URL)
	if err != nil {
		return state, fmt.Errorf("解析问卷失败: %w", err)
	}
	cfg.SurveyTitle = def.Title
	cfg.SurveyProvider = def.Provider
	if hooks.OnParsed != nil {
		hooks.OnParsed(def)
	}

	execCfg, err := config.BuildExecutionConfigWithError(cfg, def.Questions)
	if err != nil {
		return state, fmt.Errorf("准备执行配置失败: %w", err)
	}
	if r.executionDefaults != nil {
		r.executionDefaults(execCfg)
	}
	state.Config = execCfg
	if hooks.OnPrepared != nil {
		hooks.OnPrepared()
	}

	runner := engine.NewEngine(r.registry, func(event engine.StatusEvent) {
		if hooks.OnEvent != nil {
			hooks.OnEvent(event)
		}
	})
	if err := runner.Run(ctx, execCfg, state); err != nil {
		return state, err
	}
	return state, nil
}
