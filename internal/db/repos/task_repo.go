package repos

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saterdoe/oberth/internal/db"
)

type Task struct {
	ID           uuid.UUID       `json:"id"`
	RepositoryID *uuid.UUID      `json:"repository_id,omitempty"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	TaskType     string          `json:"task_type"`
	Risk         string          `json:"risk"`
	Strategy     string          `json:"strategy"`
	Constraints  json.RawMessage `json:"constraints"`
	Status       string          `json:"status"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type TaskFilter struct {
	Status        string
	Offset, Limit int
}
type TaskRepo struct{ pool *pgxpool.Pool }

func NewTaskRepo(pool *pgxpool.Pool) *TaskRepo { return &TaskRepo{pool: pool} }

func (r *TaskRepo) Create(ctx context.Context, t *Task) error {
	if t.Risk == "" {
		t.Risk = "medium"
	}
	if t.Strategy == "" {
		t.Strategy = "guided"
	}
	return r.pool.QueryRow(ctx, `INSERT INTO tasks (repository_id,title,description,task_type,risk,strategy,constraints,status)
		VALUES (@repository_id,@title,@description,@task_type,@risk,@strategy,@constraints,@status)
		RETURNING id,created_at,updated_at`, pgx.NamedArgs{
		"repository_id": t.RepositoryID, "title": t.Title, "description": t.Description,
		"task_type": t.TaskType, "risk": t.Risk, "strategy": t.Strategy, "constraints": nullJSON(t.Constraints), "status": t.Status,
	}).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
}

func (r *TaskRepo) GetByID(ctx context.Context, id uuid.UUID) (*Task, error) {
	var t Task
	err := r.pool.QueryRow(ctx, `SELECT id,repository_id,title,description,task_type,risk,strategy,constraints,status,created_at,updated_at FROM tasks WHERE id=$1`, id).
		Scan(&t.ID, &t.RepositoryID, &t.Title, &t.Description, &t.TaskType, &t.Risk, &t.Strategy, &t.Constraints, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, db.ErrNotFound
	}
	return &t, err
}

func (r *TaskRepo) List(ctx context.Context, f TaskFilter) ([]Task, error) {
	q := `SELECT id,repository_id,title,description,task_type,risk,strategy,constraints,status,created_at,updated_at FROM tasks`
	args := pgx.NamedArgs{}
	if f.Status != "" {
		q += ` WHERE status=@status`
		args["status"] = f.Status
	}
	q += ` ORDER BY created_at DESC`
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 100
	}
	q += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, max(f.Offset, 0))
	rows, err := r.pool.Query(ctx, q, args)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Task{}
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.RepositoryID, &t.Title, &t.Description, &t.TaskType, &t.Risk, &t.Strategy, &t.Constraints, &t.Status, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

func (r *TaskRepo) Update(ctx context.Context, t *Task) error {
	if strings.TrimSpace(t.Title) == "" {
		return fmt.Errorf("task title is required")
	}
	tag, err := r.pool.Exec(ctx, `UPDATE tasks SET repository_id=$2,title=$3,description=$4,task_type=$5,risk=$6,strategy=$7,constraints=$8,status=$9,updated_at=NOW() WHERE id=$1`,
		t.ID, t.RepositoryID, t.Title, t.Description, t.TaskType, t.Risk, t.Strategy, nullJSON(t.Constraints), t.Status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

func (r *TaskRepo) SetStatus(ctx context.Context, id uuid.UUID, from []string, to string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE tasks SET status=$2,updated_at=NOW() WHERE id=$1 AND status=ANY($3)`, id, to, from)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return db.ErrConflict
	}
	return nil
}

func (r *TaskRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM tasks WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}
