package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	UserID      string    `json:"user_id"`
	CreatedAt   time.Time `json:"created_at"`
	Description string    `json:"description,omitempty"`
	Priority    string    `json:"priority,omitempty"`
	DueDate     time.Time `json:"due_date,omitempty"`
	AssigneeID  string    `json:"assignee_id,omitempty"`
}

func getTasksByUserID(userID string) []task {
	var userTasks []task
	for _, t := range tasks {
		if t.UserID == userID {
			userTasks = append(userTasks, t)
		}
	}
	return userTasks
}

func postTask(c *gin.Context) {
	var newTask task
	if err := c.ShouldBindJSON(&newTask); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tasks = append(tasks, newTask)
	c.JSON(http.StatusCreated, newTask)
}

func getTaskByID(c *gin.Context) {
	taskID := c.Param("task_id")
	for _, t := range tasks {
		if t.ID == taskID {
			c.JSON(http.StatusOK, t)
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
}

func getTasks(c *gin.Context) {
	userID := c.Param("user_id")
	userTasks := getTasksByUserID(userID)
	c.JSON(http.StatusOK, userTasks)
}

var tasks = []task{
	{
		ID:        "1",
		Title:     "Task 1",
		Status:    "pending",
		UserID:    "user1",
		CreatedAt: time.Now(),
	},
	{
		ID:        "2",
		Title:     "Task 2",
		Status:    "pending",
		UserID:    "user1",
		CreatedAt: time.Now(),
	},
	{
		ID:        "3",
		Title:     "Task 3",
		Status:    "pending",
		UserID:    "user1",
		CreatedAt: time.Now(),
	},
}

func main() {
	router := gin.Default()
	router.GET("/tasks/:user_id", getTasks)
	router.POST("/tasks", postTask)
	router.GET("/tasks/:task_id", getTaskByID)
	router.Run(":8080")
}
