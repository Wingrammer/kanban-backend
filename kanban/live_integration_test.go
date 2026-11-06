package kanban

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Wingrammer/kanban-backend/supabase"
	"github.com/joho/godotenv"
	"github.com/supabase-community/gotrue-go/types"
)

// Explicit opt-in: creates one temporary task and removes only that task.
func TestLiveGuestTaskLifecycle(t *testing.T) {
	if os.Getenv("KANBAN_LIVE_TEST") != "1" {
		t.Skip("set KANBAN_LIVE_TEST=1 to test the configured Supabase project")
	}
	_ = godotenv.Load("../.env")
	client, err := supabase.NewClient(os.Getenv("SUPABASE_URL"), os.Getenv("SUPABASE_PUBLISHABLE_KEY"), nil)
	if err != nil {
		t.Fatal(err)
	}
	service := NewTaskService(NewSupabaseRepository(client))
	service.SetAuthService(client)
	router := NewRouter(service)
	call := func(method, path, token string, body interface{}) *httptest.ResponseRecorder {
		t.Helper()
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(method, path, bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", bearerHeader(token))
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}
	guest := func() types.Session {
		t.Helper()
		response := call(http.MethodPost, "/auth/guest", "", nil)
		if response.Code != 200 {
			t.Fatalf("guest failed: HTTP %d", response.Code)
		}
		var session types.Session
		if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil {
			t.Fatal(err)
		}
		return session
	}
	owner := guest()
	id := fmt.Sprintf("integration-%d", time.Now().UnixNano())
	created := call(http.MethodPost, "/tasks", owner.AccessToken, map[string]interface{}{"id": id, "title": "Temporary backend integration task", "status": "pending", "user_id": owner.User.ID})
	if created.Code != 201 {
		t.Fatalf("create failed: HTTP %d", created.Code)
	}
	defer func() {
		query, err := client.From("tasks", owner.AccessToken)
		if err != nil {
			t.Error(err)
			return
		}
		var deleted []Task
		if _, err := query.Delete("representation", "").Eq("id", id).ExecuteTo(&deleted); err != nil {
			t.Errorf("temporary task cleanup failed: %v", err)
		}
	}()
	if response := call(http.MethodPatch, "/tasks/"+id, owner.AccessToken, map[string]string{"status": "in_progress"}); response.Code != 200 {
		t.Fatalf("update failed: HTTP %d", response.Code)
	}
	response := call(http.MethodGet, "/tasks/"+id, owner.AccessToken, nil)
	var task Task
	if response.Code != 200 {
		t.Fatalf("reload failed: HTTP %d", response.Code)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &task); err != nil || task.Status != "in_progress" {
		t.Fatal("status did not persist")
	}
	other := guest()
	if response := call(http.MethodGet, "/tasks/"+id, other.AccessToken, nil); response.Code != 404 {
		t.Fatal("task visible to another guest")
	}
	if response := call(http.MethodPatch, "/tasks/"+id, other.AccessToken, map[string]string{"status": "completed"}); response.Code != 404 {
		t.Fatal("another guest could update task")
	}
	refreshed := call(http.MethodPost, "/auth/refresh", "", map[string]string{"refresh_token": owner.RefreshToken})
	if refreshed.Code != 200 {
		t.Fatalf("refresh failed: HTTP %d", refreshed.Code)
	}
	var next types.Session
	if err := json.Unmarshal(refreshed.Body.Bytes(), &next); err != nil || next.User.ID != owner.User.ID || next.ExpiresAt <= time.Now().Unix() {
		t.Fatal("refresh did not preserve identity and expiry")
	}
	owner = next
	if response := call(http.MethodGet, "/tasks/"+id, owner.AccessToken, nil); response.Code != 200 {
		t.Fatal("refreshed session lost task access")
	}
}
