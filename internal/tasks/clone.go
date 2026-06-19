package tasks

import "github.com/SurveyController/SurveyCore/internal/models"

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

func cloneTask(task *TaskRecord) *TaskRecord {
	if task == nil {
		return nil
	}
	copy := *task
	copy.Config = cloneRuntimeConfig(task.Config)
	copy.State = task.State.Snapshot()
	return &copy
}
