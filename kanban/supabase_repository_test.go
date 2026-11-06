package kanban

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wingrammer/kanban-backend/supabase"
)

func TestSupabaseRepositoryUsesCallerTokenAndRLSFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/rest/v1/tasks" {
			t.Errorf("expected tasks endpoint, got %s", request.URL.Path)
		}
		if request.Header.Get("apikey") != "publishable-key" {
			t.Errorf("expected publishable key header")
		}
		if request.Header.Get("Authorization") != "Bearer user-token" {
			t.Errorf("expected caller token authorization")
		}
		if request.URL.Query().Get("user_id") != "eq.user-1" {
			t.Errorf("expected user filter, got %q", request.URL.Query().Get("user_id"))
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`[{"id":"task-1","title":"Task","status":"pending","user_id":"user-1","created_at":"2026-01-01T00:00:00Z"}]`))
	}))
	defer server.Close()

	client, err := supabase.NewClient(server.URL, "publishable-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	repository := NewSupabaseRepository(client)
	ctx := context.WithValue(context.Background(), accessTokenKey{}, "user-token")

	tasks, err := repository.ListByUserID(ctx, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != "task-1" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}
}
