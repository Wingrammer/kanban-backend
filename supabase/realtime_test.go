package supabase

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRealtimeUsesCallerJWTAndBoardFilter(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/realtime/v1/websocket" || r.URL.Query().Get("apikey") != "public-key" {
			t.Error("wrong realtime endpoint")
		}
		socket, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer socket.Close()
		var join struct {
			Event   string `json:"event"`
			Payload struct {
				Token  string `json:"access_token"`
				Config struct {
					Changes []map[string]string `json:"postgres_changes"`
				} `json:"config"`
			} `json:"payload"`
		}
		if err := socket.ReadJSON(&join); err != nil {
			t.Error(err)
			return
		}
		if join.Event != "phx_join" || join.Payload.Token != "caller-jwt" || len(join.Payload.Config.Changes) != 1 || join.Payload.Config.Changes[0]["filter"] != "id=eq.board-id" {
			t.Error("unsafe subscription")
		}
		for _, event := range []string{"system", "postgres_changes"} {
			_ = socket.WriteJSON(map[string]interface{}{"event": event, "payload": map[string]string{"status": "ok"}})
		}
		var message json.RawMessage
		_ = socket.ReadJSON(&message)
	}))
	defer provider.Close()
	client, _ := NewClient(provider.URL, "public-key", provider.Client())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{}, 1)
	changed := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() { done <- client.SubscribeBoard(ctx, "caller-jwt", "board-id", ready, changed) }()
	for _, event := range []chan struct{}{ready, changed} {
		select {
		case <-event:
		case <-time.After(3 * time.Second):
			t.Fatal("provider event not delivered")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("subscription did not stop")
	}
}
