package kanban

import (
	"context"
	"errors"
	"time"

	"github.com/supabase-community/gotrue-go/types"
)

type TaskService struct {
	repository TaskRepository
	auth       AuthService
}

func NewTaskService(repository TaskRepository) *TaskService {
	return &TaskService{repository: repository}
}

type AuthService interface {
	CreateAnonymousSession(context.Context) (types.Session, error)
	RefreshSession(context.Context, string) (types.Session, error)
	LinkAnonymousAccount(context.Context, string, string, string) (types.User, error)
}

var ErrAuthUnavailable = errors.New("authentication service is not configured")

func (s *TaskService) RefreshSession(ctx context.Context, token string) (types.Session, error) {
	if s.auth == nil {
		return types.Session{}, ErrAuthUnavailable
	}
	return s.auth.RefreshSession(ctx, token)
}

func (s *TaskService) SetAuthService(auth AuthService) {
	s.auth = auth
}

func (s *TaskService) CreateGuestSession(ctx context.Context) (types.Session, error) {
	if s.auth == nil {
		return types.Session{}, ErrAuthUnavailable
	}
	return s.auth.CreateAnonymousSession(ctx)
}

func (s *TaskService) LinkGuestAccount(ctx context.Context, email, password string) (types.User, error) {
	if s.auth == nil {
		return types.User{}, ErrAuthUnavailable
	}
	token, found := accessToken(ctx)
	if !found {
		return types.User{}, ErrUnauthorized
	}
	return s.auth.LinkAnonymousAccount(ctx, token, email, password)
}

func (s *TaskService) ListTasksByUserID(ctx context.Context, userID string) ([]Task, error) {
	return s.repository.ListByUserID(ctx, userID)
}

func (s *TaskService) GetTaskByID(ctx context.Context, id string) (Task, bool, error) {
	return s.repository.GetByID(ctx, id)
}

func (s *TaskService) CreateTask(ctx context.Context, task Task) (Task, error) {
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	return s.repository.Create(ctx, task)
}

func (s *TaskService) UpdateTask(ctx context.Context, id string, changes map[string]interface{}) (Task, bool, error) {
	return s.repository.Update(ctx, id, changes)
}
