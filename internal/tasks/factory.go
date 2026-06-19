package tasks

import (
	"github.com/SurveyController/SurveyCore/internal/execution"
	"github.com/SurveyController/SurveyCore/internal/providers"
)

func DefaultTaskManager() (*TaskManager, error) {
	return DefaultTaskManagerWithStore("data/surveycore.db")
}

func DefaultTaskManagerWithStore(dbPath string) (*TaskManager, error) {
	store := NewStore(dbPath)
	if err := store.Init(); err != nil {
		return nil, err
	}
	manager := NewTaskManager(store, providers.Default())
	return manager, nil
}

func DefaultTaskManagerWithStoreAndExecutionDefaults(dbPath string, executionDefaults func(*execution.ExecutionConfig)) (*TaskManager, error) {
	store := NewStore(dbPath)
	if err := store.Init(); err != nil {
		return nil, err
	}
	manager := NewTaskManagerWithExecutionDefaults(store, providers.Default(), executionDefaults)
	return manager, nil
}
