package kanban

import "time"

// Task represents a kanban task.
type Task struct {
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
