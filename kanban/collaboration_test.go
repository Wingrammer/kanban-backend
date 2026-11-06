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

func TestBoardHubScopesSubscribersAndCoalescesEvents(t *testing.T) {
	hub := &boardHub{boards: make(map[string]map[chan struct{}]bool)}
	first, unsubscribe := hub.subscribe("first")
	defer unsubscribe()
	other, unsubscribeOther := hub.subscribe("other")
	defer unsubscribeOther()
	hub.publish("first")
	hub.publish("first")
	if len(first) != 1 {
		t.Fatal("events should coalesce for a slow subscriber")
	}
	if len(other) != 0 {
		t.Fatal("event leaked into another board")
	}
}

func TestCollaborationUsesCallerTokenAndTypedRPCArguments(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != bearerHeader("caller-jwt") {
			t.Error("caller JWT missing")
		}
		if r.Header.Get("apikey") != "publishable-key" {
			t.Error("project key missing")
		}
		var input map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if r.URL.Path != "/rest/v1/rpc/save_board_task" || input["p_board"] != "board-one" || input["p_task"] != "task-one" {
			t.Error("unexpected RPC routing")
		}
		changes := input["p_changes"].(map[string]interface{})
		if changes["status"] != "review" {
			t.Error("task changes were lost")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"task-one","status":"review"}`))
	}))
	defer upstream.Close()
	client, _ := supabase.NewClient(upstream.URL, "publishable-key", upstream.Client())
	service := NewTaskService(&testRepository{})
	service.SetCollaboration(NewCollaboration(client))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/boards/board-one/tasks/task-one", strings.NewReader(`{"status":"review"}`))
	request.Header.Set("Authorization", bearerHeader("caller-jwt"))
	NewRouter(service).ServeHTTP(recorder, request)
	if recorder.Code != 200 || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected response: %d", recorder.Code)
	}
}

func TestStreamRejectsUnauthorizedBoardBeforeSendingEvents(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"code":"42501","message":"Board access denied"}`))
	}))
	defer upstream.Close()
	client, _ := supabase.NewClient(upstream.URL, "publishable-key", upstream.Client())
	service := NewTaskService(&testRepository{})
	service.SetCollaboration(NewCollaboration(client))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/boards/private/events", nil).WithContext(context.Background())
	request.Header.Set("Authorization", bearerHeader("caller-jwt"))
	NewRouter(service).ServeHTTP(recorder, request)
	if recorder.Code != 403 || strings.Contains(recorder.Body.String(), "event:") {
		t.Fatal("unauthorized SSE subscription was accepted")
	}
	if len(service.collaboration.hub.boards) != 0 {
		t.Fatal("unauthorized listener was registered")
	}
}
