package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Store persists tasks and task logs.
type Store struct {
	dir string
	mu  sync.Mutex
}

func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

func (s *Store) Init() error {
	return os.MkdirAll(s.dir, 0755)
}

func (s *Store) TaskPath(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func (s *Store) LogPath(id string) string {
	return filepath.Join(s.dir, id+".logs.jsonl")
}

func (s *Store) SaveTask(task *TaskRecord) error {
	if task == nil || task.ID == "" {
		return errors.New("任务为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return err
	}
	path := s.TaskPath(task.ID)
	tmp := path + ".tmp"
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) AppendLog(taskID string, entry TaskLog) error {
	if strings.TrimSpace(taskID) == "" {
		return errors.New("任务 ID 为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(s.LogPath(taskID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *Store) LoadTasks() ([]*TaskRecord, []error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []error{err}
	}
	tasks := make([]*TaskRecord, 0)
	errs := make([]error, 0)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".logs.json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, name))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		var task TaskRecord
		if err := json.Unmarshal(data, &task); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		if task.ID == "" {
			task.ID = strings.TrimSuffix(name, ".json")
		}
		tasks = append(tasks, &task)
	}
	return tasks, errs
}

func (s *Store) LoadLogs(taskID string) ([]TaskLog, error) {
	file, err := os.Open(s.LogPath(taskID))
	if err != nil {
		if os.IsNotExist(err) {
			return []TaskLog{}, nil
		}
		return nil, err
	}
	defer file.Close()

	logs := make([]TaskLog, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry TaskLog
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		logs = append(logs, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}
