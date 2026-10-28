package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"task-service/internal/domain"
)

func (r *Repository) CreateTask(ctx context.Context, task domain.Task, changes domain.Changes) (domain.Task, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Task{}, err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO tasks (
			team_id, title, description, status, created_by, assignee_id,
			created_at, updated_at, closed_at, version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.TeamID, task.Title, task.Description, task.Status, task.CreatedBy, task.AssigneeID,
		task.CreatedAt.UTC(), task.UpdatedAt.UTC(), utcPtr(task.ClosedAt), task.Version)
	if err != nil {
		return domain.Task{}, mapDBErr(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return domain.Task{}, err
	}
	task.ID = id
	if err := insertHistory(ctx, tx, task.ID, task.CreatedBy, changes, task.CreatedAt); err != nil {
		return domain.Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

func (r *Repository) ListTasks(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, error) {
	query := `
		SELECT id, team_id, title, description, status, created_by, assignee_id,
		       created_at, updated_at, closed_at, version
		FROM tasks
		WHERE team_id = ?`
	args := []any{filter.TeamID}
	if filter.Status != "" {
		query += ` AND status = ?`
		args = append(args, filter.Status)
	}
	if filter.AssigneeID != nil {
		query += ` AND assignee_id = ?`
		args = append(args, *filter.AssigneeID)
	}
	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]domain.Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (r *Repository) TaskByID(ctx context.Context, id int64) (domain.Task, error) {
	row := r.db.QueryRowContext(ctx, taskSelect+` WHERE id = ?`, id)
	task, err := scanTask(row)
	if err != nil {
		return domain.Task{}, mapDBErr(err)
	}
	return task, nil
}

func (r *Repository) UpdateTask(ctx context.Context, id, actorID, expectedVersion int64, now time.Time, apply func(domain.Task) (domain.Task, domain.Changes, error)) (domain.Task, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Task{}, err
	}
	defer func() { _ = tx.Rollback() }()

	current, err := scanTask(tx.QueryRowContext(ctx, taskSelect+` WHERE id = ? FOR UPDATE`, id))
	if err != nil {
		return domain.Task{}, mapDBErr(err)
	}
	if current.Version != expectedVersion {
		return domain.Task{}, domain.ErrConflict
	}
	next, changes, err := apply(current)
	if err != nil {
		return domain.Task{}, err
	}
	if len(changes) == 0 {
		return current, nil
	}
	next.ID = current.ID
	next.TeamID = current.TeamID
	next.CreatedBy = current.CreatedBy
	next.CreatedAt = current.CreatedAt
	next.Version = expectedVersion + 1
	next.UpdatedAt = now.UTC()

	result, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET title = ?, description = ?, status = ?, assignee_id = ?,
		    updated_at = ?, closed_at = ?, version = ?
		WHERE id = ? AND version = ?`,
		next.Title, next.Description, next.Status, next.AssigneeID,
		next.UpdatedAt, utcPtr(next.ClosedAt), next.Version,
		current.ID, expectedVersion)
	if err != nil {
		return domain.Task{}, mapDBErr(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.Task{}, err
	}
	if affected == 0 {
		return domain.Task{}, domain.ErrConflict
	}
	if err := insertHistory(ctx, tx, next.ID, actorID, changes, next.UpdatedAt); err != nil {
		return domain.Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Task{}, err
	}
	return next, nil
}

func (r *Repository) Comments(ctx context.Context, taskID int64) ([]domain.Comment, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, user_id, content, created_at
		FROM task_comments
		WHERE task_id = ?
		ORDER BY id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	comments := make([]domain.Comment, 0)
	for rows.Next() {
		var comment domain.Comment
		if err := rows.Scan(&comment.ID, &comment.TaskID, &comment.UserID, &comment.Content, &comment.CreatedAt); err != nil {
			return nil, err
		}
		comment.CreatedAt = comment.CreatedAt.UTC()
		comments = append(comments, comment)
	}
	return comments, rows.Err()
}

func (r *Repository) AddComment(ctx context.Context, comment domain.Comment) (domain.Comment, error) {
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO task_comments (task_id, user_id, content, created_at) VALUES (?, ?, ?, ?)`,
		comment.TaskID, comment.UserID, comment.Content, comment.CreatedAt.UTC())
	if err != nil {
		return domain.Comment{}, mapDBErr(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return domain.Comment{}, err
	}
	comment.ID = id
	comment.CreatedAt = comment.CreatedAt.UTC()
	return comment, nil
}

func (r *Repository) History(ctx context.Context, taskID int64) ([]domain.HistoryEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_id, changed_by, changes, created_at
		FROM task_history
		WHERE task_id = ?
		ORDER BY id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]domain.HistoryEntry, 0)
	for rows.Next() {
		var (
			entry domain.HistoryEntry
			raw   []byte
		)
		if err := rows.Scan(&entry.ID, &entry.TaskID, &entry.ChangedBy, &raw, &entry.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &entry.Changes); err != nil {
			return nil, err
		}
		entry.CreatedAt = entry.CreatedAt.UTC()
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

const taskSelect = `
	SELECT id, team_id, title, description, status, created_by, assignee_id,
	       created_at, updated_at, closed_at, version
	FROM tasks`

type scanner interface {
	Scan(dest ...any) error
}

func scanTask(row scanner) (domain.Task, error) {
	var (
		task     domain.Task
		assignee sql.NullInt64
		closed   sql.NullTime
	)
	err := row.Scan(
		&task.ID, &task.TeamID, &task.Title, &task.Description, &task.Status, &task.CreatedBy,
		&assignee, &task.CreatedAt, &task.UpdatedAt, &closed, &task.Version,
	)
	if err != nil {
		return domain.Task{}, err
	}
	task.CreatedAt = task.CreatedAt.UTC()
	task.UpdatedAt = task.UpdatedAt.UTC()
	if assignee.Valid {
		value := assignee.Int64
		task.AssigneeID = &value
	}
	if closed.Valid {
		value := closed.Time.UTC()
		task.ClosedAt = &value
	}
	return task, nil
}

func insertHistory(ctx context.Context, tx *sql.Tx, taskID, actorID int64, changes domain.Changes, at time.Time) error {
	raw, err := json.Marshal(changes)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO task_history (task_id, changed_by, changes, created_at) VALUES (?, ?, ?, ?)`,
		taskID, actorID, raw, at.UTC())
	return err
}

func utcPtr(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}
