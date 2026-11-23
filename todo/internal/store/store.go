package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	dbschema "todo/db"
	"todo/internal/db"

	_ "modernc.org/sqlite"
)

type Store struct {
	sqlDB   *sql.DB
	queries *db.Queries
	dbPath  string
}

type TaskDTO struct {
	ID            int64        `json:"id"`
	TaskNumber    int64        `json:"task_number"`
	TaskCode      string       `json:"task_code"`
	Title         string       `json:"title"`
	Description   string       `json:"description"`
	Status        string       `json:"status"`
	Priority      string       `json:"priority"`
	Category      string       `json:"category"`
	DueDate       string       `json:"due_date"`
	SourceFile    string       `json:"source_file"`
	SourceLine    int64        `json:"source_line"`
	SourceSnippet string       `json:"source_snippet"`
	CreatedAt     string       `json:"created_at"`
	UpdatedAt     string       `json:"updated_at"`
	CompletedAt   string       `json:"completed_at"`
	Subtasks      []SubtaskDTO `json:"subtasks"`
}

type SubtaskDTO struct {
	ID        int64  `json:"id"`
	TaskID    int64  `json:"task_id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
	CreatedAt string `json:"created_at"`
}

type CreateTaskInput struct {
	Title         string `json:"title"`
	Description   string `json:"description"`
	Status        string `json:"status"`
	Priority      string `json:"priority"`
	Category      string `json:"category"`
	DueDate       string `json:"due_date"`
	SourceFile    string `json:"source_file"`
	SourceLine    int64  `json:"source_line"`
	SourceSnippet string `json:"source_snippet"`
}

type UpdateTaskInput struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	Category    string `json:"category"`
	DueDate     string `json:"due_date"`
}

type StatsDTO struct {
	Total           int64 `json:"total"`
	TodoCount       int64 `json:"todo_count"`
	InProgressCount int64 `json:"in_progress_count"`
	DoneCount       int64 `json:"done_count"`
	ScannedCount    int64 `json:"scanned_count"`
}

type ScanHistoryDTO struct {
	ID            int64  `json:"id"`
	RootDir       string `json:"root_dir"`
	FilesScanned  int64  `json:"files_scanned"`
	TodosFound    int64  `json:"todos_found"`
	TodosReplaced int64  `json:"todos_replaced"`
	CreatedAt     string `json:"created_at"`
}

func FormatTaskCode(num int64) string {
	return fmt.Sprintf("TASK (%03d)", num)
}

func DefaultDBPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "todo.db", nil
	}
	dir := filepath.Join(cfgDir, "todo-code-tracker")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	return filepath.Join(dir, "todo.db"), nil
}

func Open(dbPath string) (*Store, error) {
	if dbPath == "" {
		var err error
		dbPath, err = DefaultDBPath()
		if err != nil {
			return nil, err
		}
	}
	if dbPath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	dsn := dbPath
	if dbPath != ":memory:" {
		dsn = fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", filepath.ToSlash(dbPath))
	} else {
		dsn = "file::memory:?_pragma=foreign_keys(ON)"
	}

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if _, err := sqlDB.Exec(dbschema.SchemaSQL); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	q := db.New(sqlDB)
	if err := q.InitTaskCounter(context.Background()); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("init task counter: %w", err)
	}

	return &Store{
		sqlDB:   sqlDB,
		queries: q,
		dbPath:  dbPath,
	}, nil
}

func (s *Store) Close() error {
	if s.sqlDB != nil {
		return s.sqlDB.Close()
	}
	return nil
}

func (s *Store) DBPath() string {
	return s.dbPath
}

func (s *Store) CurrentTaskNumber(ctx context.Context) (int64, error) {
	return s.queries.GetTaskCounter(ctx)
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "in_progress", "in-progress", "doing":
		return "in_progress"
	case "done", "completed":
		return "done"
	default:
		return "todo"
	}
}

func normalizePriority(priority string) string {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "low":
		return "low"
	case "high":
		return "high"
	case "urgent":
		return "urgent"
	default:
		return "medium"
	}
}

func (s *Store) CreateTask(ctx context.Context, in CreateTaskInput) (TaskDTO, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return TaskDTO{}, fmt.Errorf("task title is required")
	}

	category := strings.TrimSpace(in.Category)
	if category == "" {
		if in.SourceFile != "" {
			category = "Code"
		} else {
			category = "General"
		}
	}

	tx, err := s.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return TaskDTO{}, err
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)
	nextNum, err := qtx.IncrementTaskCounter(ctx)
	if err != nil {
		return TaskDTO{}, fmt.Errorf("increment task counter: %w", err)
	}

	created, err := qtx.CreateTask(ctx, db.CreateTaskParams{
		TaskNumber:    nextNum,
		TaskCode:      FormatTaskCode(nextNum),
		Title:         title,
		Description:   strings.TrimSpace(in.Description),
		Status:        normalizeStatus(in.Status),
		Priority:      normalizePriority(in.Priority),
		Category:      category,
		DueDate:       strings.TrimSpace(in.DueDate),
		SourceFile:    strings.TrimSpace(in.SourceFile),
		SourceLine:    in.SourceLine,
		SourceSnippet: strings.TrimSpace(in.SourceSnippet),
	})
	if err != nil {
		return TaskDTO{}, fmt.Errorf("insert task: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return TaskDTO{}, err
	}

	return toTaskDTO(created, []SubtaskDTO{}), nil
}

func (s *Store) ListTasks(ctx context.Context) ([]TaskDTO, error) {
	tasks, err := s.queries.ListTasks(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]TaskDTO, 0, len(tasks))
	for _, t := range tasks {
		subs, err := s.queries.ListSubtasksByTask(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, toTaskDTO(t, toSubtaskDTOs(subs)))
	}
	return result, nil
}

func (s *Store) GetTask(ctx context.Context, id int64) (TaskDTO, error) {
	t, err := s.queries.GetTask(ctx, id)
	if err != nil {
		return TaskDTO{}, err
	}
	subs, err := s.queries.ListSubtasksByTask(ctx, t.ID)
	if err != nil {
		return TaskDTO{}, err
	}
	return toTaskDTO(t, toSubtaskDTOs(subs)), nil
}

func (s *Store) UpdateTask(ctx context.Context, in UpdateTaskInput) (TaskDTO, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return TaskDTO{}, fmt.Errorf("task title is required")
	}
	category := strings.TrimSpace(in.Category)
	if category == "" {
		category = "General"
	}

	updated, err := s.queries.UpdateTask(ctx, db.UpdateTaskParams{
		Title:       title,
		Description: strings.TrimSpace(in.Description),
		Status:      normalizeStatus(in.Status),
		Priority:    normalizePriority(in.Priority),
		Category:    category,
		DueDate:     strings.TrimSpace(in.DueDate),
		ID:          in.ID,
	})
	if err != nil {
		return TaskDTO{}, err
	}
	subs, err := s.queries.ListSubtasksByTask(ctx, updated.ID)
	if err != nil {
		return TaskDTO{}, err
	}
	return toTaskDTO(updated, toSubtaskDTOs(subs)), nil
}

func (s *Store) UpdateTaskStatus(ctx context.Context, id int64, status string) (TaskDTO, error) {
	updated, err := s.queries.UpdateTaskStatus(ctx, db.UpdateTaskStatusParams{
		Status: normalizeStatus(status),
		ID:     id,
	})
	if err != nil {
		return TaskDTO{}, err
	}
	subs, err := s.queries.ListSubtasksByTask(ctx, updated.ID)
	if err != nil {
		return TaskDTO{}, err
	}
	return toTaskDTO(updated, toSubtaskDTOs(subs)), nil
}

func (s *Store) DeleteTask(ctx context.Context, id int64) error {
	return s.queries.DeleteTask(ctx, id)
}

func (s *Store) DeleteCompletedTasks(ctx context.Context) error {
	return s.queries.DeleteCompletedTasks(ctx)
}

func (s *Store) GetStats(ctx context.Context) (StatsDTO, error) {
	st, err := s.queries.GetTaskStats(ctx)
	if err != nil {
		return StatsDTO{}, err
	}
	return StatsDTO{
		Total:           st.Total,
		TodoCount:       st.TodoCount,
		InProgressCount: st.InProgressCount,
		DoneCount:       st.DoneCount,
		ScannedCount:    st.ScannedCount,
	}, nil
}

func (s *Store) ListCategories(ctx context.Context) ([]string, error) {
	return s.queries.ListCategories(ctx)
}

func (s *Store) CreateSubtask(ctx context.Context, taskID int64, title string) (SubtaskDTO, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return SubtaskDTO{}, fmt.Errorf("subtask title is required")
	}
	sub, err := s.queries.CreateSubtask(ctx, db.CreateSubtaskParams{
		TaskID: taskID,
		Title:  title,
	})
	if err != nil {
		return SubtaskDTO{}, err
	}
	return toSubtaskDTO(sub), nil
}

func (s *Store) ToggleSubtask(ctx context.Context, id int64) (SubtaskDTO, error) {
	sub, err := s.queries.ToggleSubtask(ctx, id)
	if err != nil {
		return SubtaskDTO{}, err
	}
	return toSubtaskDTO(sub), nil
}

func (s *Store) DeleteSubtask(ctx context.Context, id int64) error {
	return s.queries.DeleteSubtask(ctx, id)
}

func (s *Store) RecordScanHistory(ctx context.Context, rootDir string, filesScanned, todosFound, todosReplaced int64) (ScanHistoryDTO, error) {
	rec, err := s.queries.CreateScanHistory(ctx, db.CreateScanHistoryParams{
		RootDir:       rootDir,
		FilesScanned:  filesScanned,
		TodosFound:    todosFound,
		TodosReplaced: todosReplaced,
	})
	if err != nil {
		return ScanHistoryDTO{}, err
	}
	return toScanHistoryDTO(rec), nil
}

func (s *Store) ListScanHistory(ctx context.Context) ([]ScanHistoryDTO, error) {
	rows, err := s.queries.ListScanHistory(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ScanHistoryDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toScanHistoryDTO(r))
	}
	return out, nil
}

func toTaskDTO(t db.Task, subtasks []SubtaskDTO) TaskDTO {
	completedAt := ""
	if t.CompletedAt.Valid {
		completedAt = t.CompletedAt.Time.Format(time.RFC3339)
	}
	return TaskDTO{
		ID:            t.ID,
		TaskNumber:    t.TaskNumber,
		TaskCode:      t.TaskCode,
		Title:         t.Title,
		Description:   t.Description,
		Status:        t.Status,
		Priority:      t.Priority,
		Category:      t.Category,
		DueDate:       t.DueDate,
		SourceFile:    t.SourceFile,
		SourceLine:    t.SourceLine,
		SourceSnippet: t.SourceSnippet,
		CreatedAt:     t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     t.UpdatedAt.Format(time.RFC3339),
		CompletedAt:   completedAt,
		Subtasks:      subtasks,
	}
}

func toSubtaskDTO(sub db.Subtask) SubtaskDTO {
	return SubtaskDTO{
		ID:        sub.ID,
		TaskID:    sub.TaskID,
		Title:     sub.Title,
		Completed: sub.Completed != 0,
		CreatedAt: sub.CreatedAt.Format(time.RFC3339),
	}
}

func toSubtaskDTOs(subs []db.Subtask) []SubtaskDTO {
	out := make([]SubtaskDTO, 0, len(subs))
	for _, sub := range subs {
		out = append(out, toSubtaskDTO(sub))
	}
	return out
}

func toScanHistoryDTO(r db.ScanHistory) ScanHistoryDTO {
	return ScanHistoryDTO{
		ID:            r.ID,
		RootDir:       r.RootDir,
		FilesScanned:  r.FilesScanned,
		TodosFound:    r.TodosFound,
		TodosReplaced: r.TodosReplaced,
		CreatedAt:     r.CreatedAt.Format(time.RFC3339),
	}
}
