-- name: InitTaskCounter :exec
INSERT OR IGNORE INTO counters (name, value) VALUES ('task_number', 0);

-- name: GetTaskCounter :one
SELECT value FROM counters WHERE name = 'task_number';

-- name: IncrementTaskCounter :one
UPDATE counters
SET value = value + 1
WHERE name = 'task_number'
RETURNING value;

-- name: CreateTask :one
INSERT INTO tasks (
    task_number,
    task_code,
    title,
    description,
    status,
    priority,
    category,
    due_date,
    source_file,
    source_line,
    source_snippet
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = ?;

-- name: GetTaskByCode :one
SELECT * FROM tasks WHERE task_code = ?;

-- name: ListTasks :many
SELECT * FROM tasks
ORDER BY
    CASE status
        WHEN 'in_progress' THEN 1
        WHEN 'todo' THEN 2
        WHEN 'done' THEN 3
    END,
    CASE priority
        WHEN 'urgent' THEN 1
        WHEN 'high' THEN 2
        WHEN 'medium' THEN 3
        WHEN 'low' THEN 4
    END,
    id DESC;

-- name: UpdateTask :one
UPDATE tasks
SET
    title = sqlc.arg(title),
    description = sqlc.arg(description),
    status = sqlc.arg(status),
    priority = sqlc.arg(priority),
    category = sqlc.arg(category),
    due_date = sqlc.arg(due_date),
    updated_at = CURRENT_TIMESTAMP,
    completed_at = CASE WHEN sqlc.arg(status) = 'done' AND completed_at IS NULL THEN CURRENT_TIMESTAMP
                        WHEN sqlc.arg(status) != 'done' THEN NULL
                        ELSE completed_at END
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateTaskStatus :one
UPDATE tasks
SET
    status = sqlc.arg(status),
    updated_at = CURRENT_TIMESTAMP,
    completed_at = CASE WHEN sqlc.arg(status) = 'done' THEN CURRENT_TIMESTAMP ELSE NULL END
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteTask :exec
DELETE FROM tasks WHERE id = ?;

-- name: DeleteCompletedTasks :exec
DELETE FROM tasks WHERE status = 'done';

-- name: ListCategories :many
SELECT DISTINCT category FROM tasks
WHERE category != ''
ORDER BY category ASC;

-- name: GetTaskStats :one
SELECT
    COUNT(*) AS total,
    CAST(COALESCE(SUM(CASE WHEN status = 'todo' THEN 1 ELSE 0 END), 0) AS INTEGER) AS todo_count,
    CAST(COALESCE(SUM(CASE WHEN status = 'in_progress' THEN 1 ELSE 0 END), 0) AS INTEGER) AS in_progress_count,
    CAST(COALESCE(SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END), 0) AS INTEGER) AS done_count,
    CAST(COALESCE(SUM(CASE WHEN source_file != '' THEN 1 ELSE 0 END), 0) AS INTEGER) AS scanned_count
FROM tasks;

-- name: CreateSubtask :one
INSERT INTO subtasks (task_id, title)
VALUES (?, ?)
RETURNING *;

-- name: ListSubtasksByTask :many
SELECT * FROM subtasks
WHERE task_id = ?
ORDER BY id ASC;

-- name: ToggleSubtask :one
UPDATE subtasks
SET completed = CASE WHEN completed = 0 THEN 1 ELSE 0 END
WHERE id = ?
RETURNING *;

-- name: DeleteSubtask :exec
DELETE FROM subtasks WHERE id = ?;

-- name: CreateScanHistory :one
INSERT INTO scan_history (root_dir, files_scanned, todos_found, todos_replaced)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: ListScanHistory :many
SELECT * FROM scan_history
ORDER BY id DESC
LIMIT 20;
