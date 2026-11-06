package kanban

import (
	"github.com/Wingrammer/kanban-backend/supabase"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccountReadsFreshAuthUserAndGuardsPassword(t *testing.T) {
	for _, confirmed := range []bool{false, true} {
		t.Run(map[bool]string{false: "guest", true: "confirmed"}[confirmed], func(t *testing.T) {
			writes := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer old-jwt" {
					t.Error("caller identity lost")
				}
				if r.Method == "PUT" {
					writes++
				}
				w.Header().Set("Content-Type", "application/json")
				if confirmed {
					_, _ = w.Write([]byte(`{"id":"same-user","email":"person@example.com","email_confirmed_at":"2026-11-06T00:00:00Z","is_anonymous":false,"user_metadata":{"password_configured":true}}`))
				} else {
					_, _ = w.Write([]byte(`{"id":"same-user","is_anonymous":true}`))
				}
			}))
			defer provider.Close()
			client, _ := supabase.NewClient(provider.URL, "public-key", provider.Client())
			service := NewTaskService(&testRepository{})
			service.SetCollaboration(NewCollaboration(client))
			router := NewRouter(service)
			response := httptest.NewRecorder()
			request := httptest.NewRequest("GET", "/auth/account", nil)
			request.Header.Set("Authorization", "Bearer old-jwt")
			router.ServeHTTP(response, request)
			if response.Code != 200 || !strings.Contains(response.Body.String(), `"confirmed":`+map[bool]string{false: "false", true: "true"}[confirmed]) {
				t.Fatal("account did not use current Auth user")
			}
			response = httptest.NewRecorder()
			request = httptest.NewRequest("POST", "/auth/password", strings.NewReader(`{"password":"A-strong-test-password"}`))
			request.Header.Set("Authorization", "Bearer old-jwt")
			router.ServeHTTP(response, request)
			expected := 400
			if confirmed {
				expected = 200
			}
			if response.Code != expected {
				t.Fatalf("got %d want %d", response.Code, expected)
			}
			if !confirmed && writes != 0 {
				t.Fatal("password changed before verification")
			}
		})
	}
}
