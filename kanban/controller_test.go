package kanban

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type testRepository struct {
	tasks []Task
}

func (r *testRepository) ListByUserID(_ context.Context, userID string) ([]Task, error) {
	var result []Task
	for _, task := range r.tasks {
		if task.UserID == userID {
			result = append(result, task)
		}
	}
	return result, nil
}

func (r *testRepository) GetByID(_ context.Context, id string) (Task, bool, error) {
	for _, task := range r.tasks {
		if task.ID == id {
			return task, true, nil
		}
	}
	return Task{}, false, nil
}

func (r *testRepository) Create(_ context.Context, task Task) (Task, error) {
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	r.tasks = append(r.tasks, task)
	return task, nil
}

func (r *testRepository) Update(_ context.Context, id string, changes map[string]interface{}) (Task, bool, error) {
	for i := range r.tasks {
		if r.tasks[i].ID != id {
			continue
		}
		if v, ok := changes["status"].(string); ok {
			r.tasks[i].Status = v
		}
		if v, ok := changes["title"].(string); ok {
			r.tasks[i].Title = v
		}
		return r.tasks[i], true, nil
	}
	return Task{}, false, nil
}

func testRouter() *gin.Engine {
	repository := &testRepository{tasks: []Task{
		{ID: "1", Title: "Task 1", Status: "pending", UserID: "user1"},
		{ID: "2", Title: "Task 2", Status: "done", UserID: "user2"},
	}}
	return NewRouter(NewTaskService(repository))
}

func TestGetTasksByUser(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/users/user1/tasks", nil)
	request.Header.Set("Authorization", bearerHeader("test-token"))

	testRouter().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"id":"1"`) {
		t.Fatalf("expected user task in response, got %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), `"id":"2"`) {
		t.Fatalf("did not expect another user's task, got %s", recorder.Body.String())
	}
}

func TestGetTaskByIDNotFound(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/tasks/missing", nil)
	request.Header.Set("Authorization", bearerHeader("test-token"))

	testRouter().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, recorder.Code)
	}
}

func TestPostTaskSetsCreatedAt(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{"id":"3","title":"New task","user_id":"user1"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", bearerHeader("test-token"))

	testRouter().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"created_at"`) {
		t.Fatalf("expected created_at in response, got %s", recorder.Body.String())
	}
}
