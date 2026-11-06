package kanban

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"time"

	"github.com/Wingrammer/kanban-backend/supabase"
	"github.com/gin-gonic/gin"
)

// Collaboration keeps provider access and board-scoped realtime inside Gin.
type Collaboration struct {
	client *supabase.Client
}

func NewCollaboration(client *supabase.Client) *Collaboration {
	return &Collaboration{client: client}
}
func (s *TaskService) SetCollaboration(value *Collaboration) { s.collaboration = value }

func registerCollaborationRoutes(router *gin.Engine, service *TaskService) {
	api := router.Group("/", requireBearerToken)
	handler := func(name string) gin.HandlerFunc {
		return func(ctx *gin.Context) {
			if service == nil || service.collaboration == nil {
				ctx.JSON(503, gin.H{"error": "Board service is unavailable"})
				return
			}
			service.collaboration.handle(ctx, name)
		}
	}
	api.GET("/workspace", handler("workspace"))
	api.GET("/auth/account", handler("account"))
	api.POST("/auth/password", handler("password"))
	router.POST("/auth/session", handler("session"))
	router.POST("/auth/login", handler("login"))
	api.GET("/boards/:board_id/events", handler("events"))
	api.POST("/boards/:board_id/members", handler("members"))
	api.POST("/invitations/accept", handler("accept"))
	api.GET("/invitations", handler("invitations"))
	api.POST("/invitations/respond", handler("respond"))
	api.POST("/boards/:board_id/tasks", handler("create"))
	api.PATCH("/boards/:board_id/tasks/:task_id", handler("update"))
	api.GET("/boards/:board_id/tasks/:task_id", handler("details"))
	api.POST("/boards/:board_id/tasks/:task_id/comments", handler("comment"))
	api.POST("/boards/:board_id/labels", handler("label"))
	api.DELETE("/boards/:board_id", handler("delete"))
}

func readBoardInput(ctx *gin.Context) (map[string]interface{}, error) {
	var input map[string]interface{}
	decoder := json.NewDecoder(http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 65536))
	if err := decoder.Decode(&input); err != nil || input == nil {
		return nil, fmt.Errorf("expected a JSON object")
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("expected one JSON object")
	}
	return input, nil
}
func boardError(ctx *gin.Context, err error) {
	var failure *supabase.RPCError
	if errors.As(err, &failure) {
		ctx.JSON(failure.Status, gin.H{"error": failure.Message})
		return
	}
	ctx.JSON(502, gin.H{"error": "The board service is unavailable. Please try again."})
}
func (c *Collaboration) handle(ctx *gin.Context, action string) {
	if action == "account" || action == "password" || action == "session" || action == "login" {
		c.account(ctx, action)
		return
	}
	token, _ := accessToken(ctx.Request.Context())
	board := ctx.Param("board_id")
	if action == "events" {
		c.stream(ctx, token, board)
		return
	}
	args := map[string]interface{}{}
	rpc := "workspace_snapshot"
	if action == "workspace" {
		if id := ctx.Query("board"); id != "" {
			args["p_board"] = id
		}
	} else if action == "invitations" {
		rpc = "my_board_invitations"
	} else if action == "details" {
		rpc = "task_details"
		args = map[string]interface{}{"p_board": board, "p_task": ctx.Param("task_id")}
	} else {
		input, err := readBoardInput(ctx)
		if err != nil {
			ctx.JSON(400, gin.H{"error": err.Error()})
			return
		}
		switch action {
		case "respond":
			rpc = "respond_board_invitation"
			args = map[string]interface{}{"p_invitation": input["invitation_id"], "p_accept": input["accept"]}
		case "delete":
			rpc = "delete_owned_board"
			args = map[string]interface{}{"p_board": board}
		case "label":
			rpc = "create_board_label"
			args = map[string]interface{}{"p_board": board, "p_name": input["name"]}
			if color, ok := input["color"]; ok {
				args["p_color"] = color
			}
		case "comment":
			rpc = "add_task_comment"
			args = map[string]interface{}{"p_board": board, "p_task": ctx.Param("task_id"), "p_body": input["body"]}
		case "members":
			rpc = "invite_board_member"
			args = map[string]interface{}{"p_board": board, "p_name": input["name"]}
			args["p_email"] = input["email"]
			if color, ok := input["color"]; ok {
				args["p_color"] = color
			}
			if member, ok := input["member_id"]; ok {
				args["p_member"] = member
			}
		case "accept":
			rpc = "accept_board_invitation"
			args = map[string]interface{}{"p_token": input["token"]}
		case "create", "update":
			rpc = "save_board_task"
			args = map[string]interface{}{"p_board": board, "p_task": nil, "p_changes": input}
			if action == "update" {
				args["p_task"] = ctx.Param("task_id")
			}
		}
	}
	var result json.RawMessage
	if err := c.client.RPC(ctx.Request.Context(), token, rpc, args, &result); err != nil {
		boardError(ctx, err)
		return
	}

	ctx.Header("Cache-Control", "no-store")
	ctx.Data(200, "application/json", result)
}

func (c *Collaboration) stream(ctx *gin.Context, token, board string) {
	// Authenticate membership before registering a listener or sending stream headers.
	var revision int64
	if err := c.client.RPC(ctx.Request.Context(), token, "board_revision", map[string]string{"p_board": board}, &revision); err != nil {
		boardError(ctx, err)
		return
	}

	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("X-Accel-Buffering", "no")
	send := func(event string) {
		_, _ = fmt.Fprintf(ctx.Writer, "event: %s\ndata: {}\n\n", event)
		ctx.Writer.Flush()
	}
	// Supabase owns change detection. SSE is only Gin's transport to the browser.
	streamContext, cancel := context.WithCancel(ctx.Request.Context())
	defer cancel()
	ready := make(chan struct{}, 1)
	changed := make(chan struct{}, 1)
	failed := make(chan error, 1)
	go func() { failed <- c.client.SubscribeBoard(streamContext, token, board, ready, changed) }()
	poll := time.NewTicker(30 * time.Second)
	defer poll.Stop()
	// A verified JWT's expiry only controls stream lifetime; the database decides access.
	expires := time.Now().Add(time.Hour)
	pieces := strings.Split(token, ".")
	if len(pieces) == 3 {
		if payload, err := base64.RawURLEncoding.DecodeString(pieces[1]); err == nil {
			var claims struct {
				Exp int64 `json:"exp"`
			}
			if json.Unmarshal(payload, &claims) == nil && claims.Exp > 0 {
				expires = time.Unix(claims.Exp, 0)
			}
		}
	}
	deadline := time.NewTimer(max(time.Until(expires)-15*time.Second, time.Second))
	defer deadline.Stop()
	// End cleanly before Vercel's 300-second function budget. Reconnection
	// reloads a snapshot and opens a fresh provider subscription.
	lifetime := time.NewTimer(240 * time.Second)
	defer lifetime.Stop()
	for {
		select {
		case <-ctx.Request.Context().Done():
			return
		case <-deadline.C:
			send("session_expired")
			return
		case <-lifetime.C:
			send("reconnect")
			return
		case <-ready:
			send("ready")
		case <-changed:
			send("board_changed")
		case <-failed:
			send("stream_error")
			return
		case <-poll.C:
			var next int64
			if err := c.client.RPC(ctx.Request.Context(), token, "board_revision", map[string]string{"p_board": board}, &next); err != nil {
				var failure *supabase.RPCError
				if errors.As(err, &failure) && failure.Status == 401 {
					send("session_expired")
				} else if errors.As(err, &failure) && failure.Status == 403 {
					send("access_changed")
				} else {
					send("stream_error")
				}
				return
			}
			if next != revision {
				revision = next
				send("board_changed")
			} else {
				_, _ = fmt.Fprint(ctx.Writer, ": keepalive\n\n")
				ctx.Writer.Flush()
			}
		}
	}
}
