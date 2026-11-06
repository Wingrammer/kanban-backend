package kanban

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// TaskChanges is the allowlist of fields editable by a task owner.
type TaskChanges struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
	Priority    *string `json:"priority"`
}

func (c TaskChanges) values() (map[string]interface{}, error) {
	values := map[string]interface{}{}
	if c.Title != nil {
		title := strings.TrimSpace(*c.Title)
		if title == "" || utf8.RuneCountInString(title) > 160 {
			return nil, fmt.Errorf("title must be 1-160 characters")
		}
		values["title"] = title
	}
	if c.Description != nil {
		if utf8.RuneCountInString(*c.Description) > 4000 {
			return nil, fmt.Errorf("description must be at most 4000 characters")
		}
		values["description"] = *c.Description
	}
	if c.Status != nil {
		switch *c.Status {
		case "pending", "in_progress", "review", "completed":
		default:
			return nil, fmt.Errorf("invalid task status")
		}
		values["status"] = *c.Status
	}
	if c.Priority != nil {
		switch *c.Priority {
		case "low", "medium", "high":
		default:
			return nil, fmt.Errorf("invalid task priority")
		}
		values["priority"] = *c.Priority
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("no task changes supplied")
	}
	return values, nil
}

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
