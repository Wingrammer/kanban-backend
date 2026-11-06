package kanban

import (
	"context"
	"errors"
	"fmt"

	"github.com/Wingrammer/kanban-backend/supabase"
)

// TaskRepository stores and retrieves tasks.
type TaskRepository interface {
	ListByUserID(ctx context.Context, userID string) ([]Task, error)
	GetByID(ctx context.Context, id string) (Task, bool, error)
	Create(ctx context.Context, task Task) (Task, error)
	Update(ctx context.Context, id string, changes map[string]interface{}) (Task, bool, error)
}

var ErrUnauthorized = errors.New("missing Supabase access token")

type SupabaseRepository struct {
	client *supabase.Client
}

func NewSupabaseRepository(client *supabase.Client) *SupabaseRepository {
	return &SupabaseRepository{client: client}
}

func (r *SupabaseRepository) ListByUserID(ctx context.Context, userID string) ([]Task, error) {
	token, err := accessTokenFromContext(ctx)
	if err != nil {
		return nil, err
	}
	query, err := r.client.From("tasks", token)
	if err != nil {
		return nil, err
	}
	var tasks []Task
	if _, err := query.Select("*", "", false).
		Eq("user_id", userID).
		Order("created_at", nil).
		ExecuteTo(&tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *SupabaseRepository) GetByID(ctx context.Context, id string) (Task, bool, error) {
	token, err := accessTokenFromContext(ctx)
	if err != nil {
		return Task{}, false, err
	}
	query, err := r.client.From("tasks", token)
	if err != nil {
		return Task{}, false, err
	}
	var tasks []Task
	if _, err := query.Select("*", "", false).
		Eq("id", id).
		Limit(1, "").
		ExecuteTo(&tasks); err != nil {
		return Task{}, false, err
	}
	if len(tasks) == 0 {
		return Task{}, false, nil
	}
	return tasks[0], true, nil
}

func (r *SupabaseRepository) Create(ctx context.Context, task Task) (Task, error) {
	token, err := accessTokenFromContext(ctx)
	if err != nil {
		return Task{}, err
	}
	query, err := r.client.From("tasks", token)
	if err != nil {
		return Task{}, err
	}
	var tasks []Task
	if _, err := query.Insert(task, false, "", "representation", "").
		ExecuteTo(&tasks); err != nil {
		return Task{}, err
	}
	if len(tasks) == 0 {
		return Task{}, fmt.Errorf("Supabase returned no created task")
	}
	return tasks[0], nil
}

func accessTokenFromContext(ctx context.Context) (string, error) {
	token, found := accessToken(ctx)
	if !found {
		return "", ErrUnauthorized
	}
	return token, nil
}

func (r *SupabaseRepository) Update(ctx context.Context, id string, changes map[string]interface{}) (Task, bool, error) {
	token, err := accessTokenFromContext(ctx)
	if err != nil {
		return Task{}, false, err
	}
	query, err := r.client.From("tasks", token)
	if err != nil {
		return Task{}, false, err
	}
	var tasks []Task
	// Supabase evaluates the owner-only update policy using this user's JWT.
	if _, err := query.Update(changes, "representation", "").Eq("id", id).ExecuteTo(&tasks); err != nil {
		return Task{}, false, err
	}
	if len(tasks) == 0 {
		return Task{}, false, nil
	}
	return tasks[0], true, nil
}
