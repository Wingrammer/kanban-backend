package kanban

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wingrammer/kanban-backend/supabase"
)

// TaskRepository stores and retrieves tasks.
type TaskRepository interface {
	ListByUserID(ctx context.Context, userID string) ([]Task, error)
	GetByID(ctx context.Context, id string) (Task, bool, error)
	Create(ctx context.Context, task Task) (Task, error)
}

var ErrUnauthorized = errors.New("missing Supabase access token")

type SupabaseRepository struct {
	client *supabase.Client
}

func NewSupabaseRepository(client *supabase.Client) *SupabaseRepository {
	return &SupabaseRepository{client: client}
}

func (r *SupabaseRepository) ListByUserID(ctx context.Context, userID string) ([]Task, error) {
	query := url.Values{
		"select":  []string{"*"},
		"user_id": []string{"eq." + userID},
		"order":   []string{"created_at.asc"},
	}
	var tasks []Task
	if err := r.do(ctx, http.MethodGet, query, nil, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *SupabaseRepository) GetByID(ctx context.Context, id string) (Task, bool, error) {
	query := url.Values{"select": []string{"*"}, "id": []string{"eq." + id}, "limit": []string{"1"}}
	var tasks []Task
	if err := r.do(ctx, http.MethodGet, query, nil, &tasks); err != nil {
		return Task{}, false, err
	}
	if len(tasks) == 0 {
		return Task{}, false, nil
	}
	return tasks[0], true, nil
}

func (r *SupabaseRepository) Create(ctx context.Context, task Task) (Task, error) {
	body, err := json.Marshal(task)
	if err != nil {
		return Task{}, err
	}
	var tasks []Task
	if err := r.do(ctx, http.MethodPost, url.Values{"select": []string{"*"}}, bytes.NewReader(body), &tasks); err != nil {
		return Task{}, err
	}
	if len(tasks) == 0 {
		return Task{}, fmt.Errorf("Supabase returned no created task")
	}
	return tasks[0], nil
}

func (r *SupabaseRepository) do(ctx context.Context, method string, query url.Values, body io.Reader, result interface{}) error {
	token, found := accessToken(ctx)
	if !found {
		return ErrUnauthorized
	}
	request, err := r.client.NewRequest(ctx, method, query, body, token)
	if err != nil {
		return err
	}
	response, err := r.client.HTTPClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Supabase request failed with status %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	return json.NewDecoder(response.Body).Decode(result)
}
