package kanban

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wingrammer/kanban-backend/supabase"
)

func TestPatchTaskValidation(t *testing.T) {
	for _, test := range []struct {
		body   string
		token  bool
		status int
	}{
		{`{"status":"in_progress"}`, true, 200},
		{`{"title":"Updated title"}`, true, 200},
		{`{"status":"invalid"}`, true, 400},
		{`{"user_id":"another-user"}`, true, 400},
		{`{"title":" "}`, true, 400},
		{`{}`, true, 400},
		{`{"status":"review"} {}`, true, 400},
		{`{"status":"review"}`, false, 401},
	} {
		t.Run(test.body, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPatch, "/tasks/1", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			if test.token {
				request.Header.Set("Authorization", bearerHeader("user-token"))
			}
			testRouter().ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("expected %d, got %d: %s", test.status, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestUpdateUsesCallerJWTAndTaskFilter(t *testing.T) {
	for _, exists := range []bool{true, false} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPatch || r.URL.Path != "/rest/v1/tasks" {
				t.Errorf("unexpected update endpoint")
			}
			if r.Header.Get("Authorization") != bearerHeader("owner-token") {
				t.Errorf("caller JWT not forwarded")
			}
			if r.URL.Query().Get("id") != "eq.task-1" {
				t.Errorf("task filter missing")
			}
			var changes map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&changes); err != nil {
				t.Error(err)
			}
			if changes["status"] != "review" || len(changes) != 1 {
				t.Errorf("unexpected update body")
			}
			w.Header().Set("Content-Type", "application/json")
			if exists {
				_, _ = w.Write([]byte(`[{"id":"task-1","title":"Task","status":"review"}]`))
			} else {
				_, _ = w.Write([]byte(`[]`))
			}
		}))
		client, err := supabase.NewClient(server.URL, "publishable-key", server.Client())
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.WithValue(context.Background(), accessTokenKey{}, "owner-token")
		_, found, err := NewSupabaseRepository(client).Update(ctx, "task-1", map[string]interface{}{"status": "review"})
		server.Close()
		if err != nil || found != exists {
			t.Fatalf("unexpected update result: found=%v err=%v", found, err)
		}
	}
}

func TestRefreshRoute(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/v1/token" || r.URL.Query().Get("grant_type") != "refresh_token" {
			t.Errorf("wrong refresh endpoint")
		}
		if r.Header.Get("apikey") != "publishable-key" {
			t.Errorf("API key missing")
		}
		var input struct {
			RefreshToken string `json:"refresh_token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		w.Header().Set("Content-Type", "application/json")
		if input.RefreshToken == "invalid" {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":"invalid token"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600,"user":{"id":"00000000-0000-0000-0000-000000000001"}}`))
	}))
	defer provider.Close()
	client, _ := supabase.NewClient(provider.URL, "publishable-key", provider.Client())
	service := NewTaskService(&testRepository{})
	service.SetAuthService(client)
	router := NewRouter(service)
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{"refresh_token":"old-refresh"}`, 200},
		{`{"refresh_token":"invalid"}`, 401},
		{`{}`, 400},
		{`{"refresh_token":"old-refresh"} {}`, 400},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != test.status {
			t.Fatalf("expected %d, got %d", test.status, recorder.Code)
		}
		if test.status == 200 && recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("session response may be cached")
		}
	}
}
