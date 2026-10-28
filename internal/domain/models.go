package domain

import "time"

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"

	StatusTodo       = "todo"
	StatusInProgress = "in_progress"
	StatusDone       = "done"
)

type User struct {
	ID           int64
	Email        string
	PasswordHash string
	Name         string
	CreatedAt    time.Time
}

type Team struct {
	ID        int64
	Name      string
	CreatedBy int64
	Role      string
	CreatedAt time.Time
}

type Member struct {
	TeamID    int64
	UserID    int64
	Role      string
	CreatedAt time.Time
}

type Task struct {
	ID          int64
	TeamID      int64
	Title       string
	Description string
	Status      string
	CreatedBy   int64
	AssigneeID  *int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ClosedAt    *time.Time
	Version     int64
}

type FieldChange struct {
	Old any `json:"old"`
	New any `json:"new"`
}

type Changes map[string]FieldChange

type HistoryEntry struct {
	ID        int64
	TaskID    int64
	ChangedBy int64
	Changes   Changes
	CreatedAt time.Time
}

type Comment struct {
	ID        int64
	TaskID    int64
	UserID    int64
	Content   string
	CreatedAt time.Time
}

type TaskFilter struct {
	TeamID     int64
	Status     string
	AssigneeID *int64
	Limit      int
	Offset     int
}

type TopAssignee struct {
	AssigneeID  int64 `json:"assignee_id"`
	ClosedCount int   `json:"closed_count"`
}

type Stats struct {
	TasksByStatus   map[string]int `json:"tasks_by_status"`
	TopAssignees    []TopAssignee  `json:"top_assignees"`
	AvgCloseSeconds *float64       `json:"avg_close_seconds"`
	CommentCount    int            `json:"comment_count"`
}

func ValidRole(role string) bool {
	switch role {
	case RoleOwner, RoleAdmin, RoleMember:
		return true
	default:
		return false
	}
}

func AssignableRole(role string) bool {
	return role == RoleAdmin || role == RoleMember
}

func ValidStatus(status string) bool {
	switch status {
	case StatusTodo, StatusInProgress, StatusDone:
		return true
	default:
		return false
	}
}

func EmptyStats() Stats {
	return Stats{
		TasksByStatus: map[string]int{
			StatusTodo:       0,
			StatusInProgress: 0,
			StatusDone:       0,
		},
		TopAssignees: []TopAssignee{},
	}
}
