package tasks

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	defaultLogPageSize = 200
	maxLogPageSize     = 1000
)

// Store persists tasks and task logs in one SQLite database.
type Store struct {
	path string
	mu   sync.Mutex
	db   *sql.DB
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Init() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", sqliteDSN(s.path))
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return err
	}
	if err := createSchema(db); err != nil {
		_ = db.Close()
		return err
	}
	s.db = db
	return nil
}

func sqliteDSN(path string) string {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		absolutePath = path
	}
	uriPath := filepath.ToSlash(absolutePath)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	dsn := url.URL{Scheme: "file", Path: uriPath}
	query := dsn.Query()
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "foreign_keys(1)")
	dsn.RawQuery = query.Encode()
	return dsn.String()
}

func createSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			record_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS task_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT NOT NULL,
			timestamp TEXT NOT NULL,
			level TEXT NOT NULL,
			message TEXT NOT NULL,
			fields_json TEXT,
			event_json TEXT,
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_task_logs_task_id_id
			ON task_logs(task_id, id);

		INSERT OR IGNORE INTO schema_migrations(version, applied_at)
			VALUES (1, CURRENT_TIMESTAMP);
	`)
	return err
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

func (s *Store) SaveTask(task *TaskRecord) error {
	if task == nil || task.ID == "" {
		return errors.New("任务为空")
	}
	syncTaskDerivedFields(task)
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339Nano)
	_, err = s.database().Exec(`
		INSERT INTO tasks(id, status, record_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			record_json = excluded.record_json,
			updated_at = excluded.updated_at
	`, task.ID, task.Status, string(data), task.CreatedAt.Format(time.RFC3339Nano), now)
	return err
}

func (s *Store) AppendLog(taskID string, entry TaskLog) error {
	if taskID == "" {
		return errors.New("任务 ID 为空")
	}
	fieldsJSON, err := optionalJSON(entry.Fields)
	if err != nil {
		return err
	}
	eventJSON, err := optionalJSON(entry.Event)
	if err != nil {
		return err
	}
	_, err = s.database().Exec(`
		INSERT INTO task_logs(task_id, timestamp, level, message, fields_json, event_json)
		VALUES (?, ?, ?, ?, ?, ?)
	`, taskID, entry.Timestamp.Format(time.RFC3339Nano), entry.Level, entry.Message, fieldsJSON, eventJSON)
	return err
}

func optionalJSON(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if string(data) == "null" {
		return nil, nil
	}
	return string(data), nil
}

func (s *Store) LoadTasks() ([]*TaskRecord, []error) {
	rows, err := s.database().Query(`SELECT record_json FROM tasks ORDER BY created_at DESC`)
	if err != nil {
		return nil, []error{err}
	}
	defer rows.Close()

	tasks := make([]*TaskRecord, 0)
	errs := make([]error, 0)
	for rows.Next() {
		var recordJSON string
		if err := rows.Scan(&recordJSON); err != nil {
			errs = append(errs, err)
			continue
		}
		var task TaskRecord
		if err := json.Unmarshal([]byte(recordJSON), &task); err != nil {
			errs = append(errs, fmt.Errorf("解析任务记录失败: %w", err))
			continue
		}
		tasks = append(tasks, &task)
	}
	if err := rows.Err(); err != nil {
		errs = append(errs, err)
	}
	return tasks, errs
}

func (s *Store) LoadLogs(taskID string, afterID int64, limit int) (*TaskLogPage, error) {
	if afterID < 0 {
		return nil, errors.New("日志游标不能小于 0")
	}
	limit = normalizeLogLimit(limit)
	rows, err := s.database().Query(`
		SELECT id, timestamp, level, message, fields_json, event_json
		FROM task_logs
		WHERE task_id = ? AND id > ?
		ORDER BY id ASC
		LIMIT ?
	`, taskID, afterID, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]TaskLog, 0, limit)
	var nextCursor int64
	for rows.Next() {
		var (
			entry      TaskLog
			timestamp  string
			fieldsJSON sql.NullString
			eventJSON  sql.NullString
		)
		if err := rows.Scan(&entry.ID, &timestamp, &entry.Level, &entry.Message, &fieldsJSON, &eventJSON); err != nil {
			return nil, err
		}
		if len(logs) == limit {
			return &TaskLogPage{Logs: logs, NextCursor: nextCursor, HasMore: true}, nil
		}
		entry.Timestamp, err = time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return nil, fmt.Errorf("解析日志时间失败: %w", err)
		}
		if fieldsJSON.Valid {
			if err := json.Unmarshal([]byte(fieldsJSON.String), &entry.Fields); err != nil {
				return nil, fmt.Errorf("解析日志字段失败: %w", err)
			}
		}
		if eventJSON.Valid {
			if err := json.Unmarshal([]byte(eventJSON.String), &entry.Event); err != nil {
				return nil, fmt.Errorf("解析日志事件失败: %w", err)
			}
		}
		logs = append(logs, entry)
		nextCursor = entry.ID
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &TaskLogPage{Logs: logs, NextCursor: nextCursor}, nil
}

func normalizeLogLimit(limit int) int {
	if limit <= 0 {
		return defaultLogPageSize
	}
	if limit > maxLogPageSize {
		return maxLogPageSize
	}
	return limit
}

func (s *Store) database() *sql.DB {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		panic("tasks.Store.Init 必须在读写前调用")
	}
	return s.db
}

func ParseLogCursor(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	cursor, err := strconv.ParseInt(value, 10, 64)
	if err != nil || cursor < 0 {
		return 0, errors.New("日志游标必须是非负整数")
	}
	return cursor, nil
}

func ParseLogLimit(value string) (int, error) {
	if value == "" {
		return defaultLogPageSize, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > maxLogPageSize {
		return 0, fmt.Errorf("日志条数必须是 1 到 %d 之间的整数", maxLogPageSize)
	}
	return limit, nil
}
