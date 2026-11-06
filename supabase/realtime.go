package supabase

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"
)

// SubscribeBoard consumes Supabase Postgres Changes inside Gin. A revision change
// invalidates the UI snapshot; no task contents or pointer positions are relayed.
// One reader and one writer own the socket, as required by gorilla/websocket.
func (c *Client) SubscribeBoard(ctx context.Context, token, board string, ready, changed chan<- struct{}) error {
	endpoint, err := url.Parse(c.url)
	if err != nil {
		return err
	}
	if endpoint.Scheme == "https" {
		endpoint.Scheme = "wss"
	} else {
		endpoint.Scheme = "ws"
	}
	endpoint.Path = "/realtime/v1/websocket"
	query := endpoint.Query()
	query.Set("apikey", c.key)
	query.Set("vsn", "1.0.0")
	endpoint.RawQuery = query.Encode()
	socket, _, err := websocket.DefaultDialer.DialContext(ctx, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("realtime connection unavailable")
	}
	defer socket.Close()
	socket.SetReadLimit(1 << 20)
	topic := "realtime:board:" + board
	write := func(event, topic, ref string, payload interface{}) error {
		_ = socket.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return socket.WriteJSON(map[string]interface{}{"topic": topic, "event": event, "ref": ref, "join_ref": "1", "payload": payload})
	}
	err = write("phx_join", topic, "1", map[string]interface{}{"access_token": token, "config": map[string]interface{}{"broadcast": map[string]bool{"self": false}, "presence": map[string]bool{"enabled": false}, "postgres_changes": []map[string]string{{"event": "UPDATE", "schema": "public", "table": "boards", "filter": "id=eq." + board}}}})
	if err != nil {
		return err
	}
	readErrors := make(chan error, 1)
	var connected atomic.Bool
	go func() {
		for {
			_ = socket.SetReadDeadline(time.Now().Add(60 * time.Second))
			var message struct {
				Event   string          `json:"event"`
				Ref     string          `json:"ref"`
				Payload json.RawMessage `json:"payload"`
			}
			if err := socket.ReadJSON(&message); err != nil {
				readErrors <- err
				return
			}
			switch message.Event {
			case "phx_reply":
				if message.Ref == "1" {
					var payload struct {
						Status string `json:"status"`
					}
					_ = json.Unmarshal(message.Payload, &payload)
					if payload.Status != "ok" {
						readErrors <- fmt.Errorf("realtime subscription rejected")
						return
					}
				}
			case "system":
				var payload struct {
					Status string `json:"status"`
				}
				_ = json.Unmarshal(message.Payload, &payload)
				if payload.Status != "ok" {
					readErrors <- fmt.Errorf("realtime replication unavailable")
					return
				}
				connected.Store(true)
				select {
				case ready <- struct{}{}:
				default:
				}
			case "postgres_changes":
				select {
				case changed <- struct{}{}:
				default:
				}
			case "phx_error", "phx_close":
				readErrors <- fmt.Errorf("realtime channel closed")
				return
			}
		}
	}()
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	startup := time.NewTimer(15 * time.Second)
	defer startup.Stop()
	// The replication-ready event is surfaced separately to the SSE controller.
	for sequence := 2; ; sequence++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErrors:
			return err
		case <-startup.C: // A successful join alone does not prove replication is active.
			if !connected.Load() {
				return fmt.Errorf("realtime replication did not become ready")
			}
		case <-heartbeat.C:
			if err := write("heartbeat", "phoenix", strconv.Itoa(sequence), map[string]interface{}{}); err != nil {
				return err
			}
		}
	}
}
