package kanban

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func NewRouter(service *TaskService) *gin.Engine {
	router := gin.Default()
	controller := &controller{service: service}

	api := router.Group("/", requireBearerToken)
	api.GET("/users/:user_id/tasks", controller.getTasks)
	api.POST("/tasks", controller.postTask)
	api.GET("/tasks/:task_id", controller.getTaskByID)
	return router
}

func requireBearerToken(ctx *gin.Context) {
	header := ctx.GetHeader("Authorization")
	token, found := strings.CutPrefix(header, "Bearer ")
	if !found || strings.TrimSpace(token) == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Bearer token required"})
		return
	}
	ctx.Request = ctx.Request.WithContext(context.WithValue(ctx.Request.Context(), accessTokenKey{}, token))
	ctx.Next()
}

type accessTokenKey struct{}

func accessToken(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(accessTokenKey{}).(string)
	return token, ok
}

type controller struct {
	service *TaskService
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
