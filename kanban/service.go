package kanban

import (
	"context"
	"time"
)

type TaskService struct {
	repository TaskRepository
}

func NewTaskService(repository TaskRepository) *TaskService {
	return &TaskService{repository: repository}
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
