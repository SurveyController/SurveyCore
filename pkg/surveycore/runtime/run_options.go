package runtime

import "time"

const defaultFailStopThreshold = 5

func ExecutionOptionsFromConfig(cfg *RunRequest) ExecutionOptions {
	if cfg == nil {
		return ExecutionOptions{}
	}
	target := cfg.ExecutionPlan.Target
	if target <= 0 {
		target = 1
	}
	threads := cfg.ExecutionPlan.Threads
	if cfg.ReverseFillPlan.Enabled && cfg.ReverseFillPlan.Threads > 0 {
		threads = cfg.ReverseFillPlan.Threads
	}
	if threads <= 0 {
		threads = 1
	}
	options := ExecutionOptions{
		Target:                target,
		Threads:               threads,
		ContinueOnFailure:     true,
		IntervalBeforeAttempt: true,
		FailStop:              cfg.FailStop,
		FailStopThreshold:     defaultFailStopThreshold,
		CooldownOnError:       30 * time.Second,
	}
	if cfg.UserAgent != nil {
		options.UserAgent = *cfg.UserAgent
		options.UserAgent.Ratios = make(map[string]int, len(cfg.UserAgent.Ratios))
		for key, value := range cfg.UserAgent.Ratios {
			options.UserAgent.Ratios[key] = value
		}
	}
	return options
}

func failStopThreshold(options ExecutionOptions) int {
	if options.FailStopThreshold > 0 {
		return options.FailStopThreshold
	}
	return defaultFailStopThreshold
}
