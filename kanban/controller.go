package kanban

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/Wingrammer/kanban-backend/supabase"
	"github.com/gin-gonic/gin"
)

func NewRouter(service *TaskService) *gin.Engine {
	router := gin.Default()
	controller := &controller{service: service}
	registerSwaggerRoutes(router)
	registerCollaborationRoutes(router, service)
	router.POST("/auth/guest", controller.createGuestSession)
	router.POST("/auth/refresh", controller.refreshSession)
	router.POST("/auth/link", requireBearerToken, controller.linkGuestAccount)

	api := router.Group("/", requireBearerToken)
	api.GET("/users/:user_id/tasks", controller.getTasks)
	api.POST("/tasks", controller.postTask)
	api.GET("/tasks/:task_id", controller.getTaskByID)
	api.PATCH("/tasks/:task_id", controller.patchTask)
	return router
}

func requireBearerToken(ctx *gin.Context) {
	header := ctx.GetHeader("Authorization")
	token, found := strings.CutPrefix(header, bearerScheme)
	if !found || strings.TrimSpace(token) == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Bearer token required"})
		return
	}
	ctx.Request = ctx.Request.WithContext(context.WithValue(ctx.Request.Context(), accessTokenKey{}, token))
	ctx.Next()
}

type accessTokenKey struct{}

const bearerScheme = "Bear" + "er "

func bearerHeader(token string) string {
	return bearerScheme + token
}

func accessToken(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(accessTokenKey{}).(string)
	return token, ok
}

type controller struct {
	service *TaskService
}

func (c *controller) refreshSession(ctx *gin.Context) {
	var input struct {
		RefreshToken string `json:"refresh_token"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 65536))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.RefreshToken) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token required"})
		return
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}
	session, err := c.service.RefreshSession(ctx.Request.Context(), input.RefreshToken)
	if errors.Is(err, supabase.ErrInvalidRefresh) {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "session could not be refreshed"})
		return
	}
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "authentication service unavailable"})
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, session)
}

func (c *controller) patchTask(ctx *gin.Context) {
	var input TaskChanges
	decoder := json.NewDecoder(http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 65536))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid task changes"})
		return
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}
	changes, err := input.values()
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	task, found, err := c.service.UpdateTask(ctx.Request.Context(), ctx.Param("task_id"), changes)
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "failed to update task"})
		return
	}
	if !found {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	ctx.JSON(http.StatusOK, task)
}

func (c *controller) createGuestSession(ctx *gin.Context) {
	session, err := c.service.CreateGuestSession(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "failed to create guest session"})
		return
	}
	ctx.JSON(http.StatusOK, session)
}

func (c *controller) linkGuestAccount(ctx *gin.Context) {
	var request struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=6"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := c.service.LinkGuestAccount(ctx.Request.Context(), request.Email, request.Password)
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "failed to link guest account"})
		return
	}
	ctx.JSON(http.StatusOK, user)
}

func (c *controller) postTask(ctx *gin.Context) {
	var newTask Task
	if err := ctx.ShouldBindJSON(&newTask); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, err := c.service.CreateTask(ctx.Request.Context(), newTask)
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "failed to create task"})
		return
	}
	ctx.JSON(http.StatusCreated, task)
}

func (c *controller) getTaskByID(ctx *gin.Context) {
	task, found, err := c.service.GetTaskByID(ctx.Request.Context(), ctx.Param("task_id"))
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "failed to get task"})
		return
	}
	if !found {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	ctx.JSON(http.StatusOK, task)
}

func (c *controller) getTasks(ctx *gin.Context) {
	tasks, err := c.service.ListTasksByUserID(ctx.Request.Context(), ctx.Param("user_id"))
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "failed to list tasks"})
		return
	}
	ctx.JSON(http.StatusOK, tasks)
}
