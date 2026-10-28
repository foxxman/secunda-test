package httpserver

import (
	"encoding/json"
	"strconv"
	"time"

	"task-service/internal/domain"

	"net/http"

	"github.com/go-chi/chi/v5"
)

type userResponse struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type tokenResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type teamResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedBy int64     `json:"created_by"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type memberResponse struct {
	TeamID int64  `json:"team_id"`
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
}

type taskResponse struct {
	ID          int64      `json:"id"`
	TeamID      int64      `json:"team_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	CreatedBy   int64      `json:"created_by"`
	AssigneeID  *int64     `json:"assignee_id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at"`
	Version     int64      `json:"version"`
}

type taskListResponse struct {
	Tasks  []taskResponse `json:"tasks"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

type historyResponse struct {
	ID        int64          `json:"id"`
	TaskID    int64          `json:"task_id"`
	ChangedBy int64          `json:"changed_by"`
	Changes   domain.Changes `json:"changes"`
	CreatedAt time.Time      `json:"created_at"`
}

type commentResponse struct {
	ID        int64     `json:"id"`
	TaskID    int64     `json:"task_id"`
	UserID    int64     `json:"user_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type statsResponse struct {
	TeamID          int64                `json:"team_id"`
	TasksByStatus   map[string]int       `json:"tasks_by_status"`
	TopAssignees    []domain.TopAssignee `json:"top_assignees"`
	AvgCloseSeconds *float64             `json:"avg_close_seconds"`
	CommentCount    int                  `json:"comment_count"`
}

func newUserResponse(id int64, email, name string, createdAt time.Time) userResponse {
	return userResponse{ID: id, Email: email, Name: name, CreatedAt: createdAt}
}

func newTeamResponse(team domain.Team) teamResponse {
	return teamResponse{
		ID: team.ID, Name: team.Name, CreatedBy: team.CreatedBy, Role: team.Role, CreatedAt: team.CreatedAt,
	}
}

func newTaskResponse(task domain.Task) taskResponse {
	return taskResponse{
		ID: task.ID, TeamID: task.TeamID, Title: task.Title, Description: task.Description,
		Status: task.Status, CreatedBy: task.CreatedBy, AssigneeID: task.AssigneeID,
		CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt, ClosedAt: task.ClosedAt, Version: task.Version,
	}
}

func newTaskList(tasks []domain.Task, limit, offset int) taskListResponse {
	items := make([]taskResponse, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, newTaskResponse(task))
	}
	return taskListResponse{Tasks: items, Limit: limit, Offset: offset}
}

func pathID(r *http.Request, name string) (int64, error) {
	value, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || value <= 0 {
		return 0, domain.Invalid("некорректный идентификатор")
	}
	return value, nil
}

func queryInt(r *http.Request, name string) (int, bool, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, false, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, true, domain.Invalid("некорректный параметр " + name)
	}
	return value, true, nil
}

func queryInt64(r *http.Request, name string) (*int64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return nil, domain.Invalid("некорректный параметр " + name)
	}
	return &value, nil
}

type updateTaskBody struct {
	Title       json.RawMessage `json:"title"`
	Description json.RawMessage `json:"description"`
	Status      json.RawMessage `json:"status"`
	AssigneeID  json.RawMessage `json:"assignee_id"`
	Version     json.RawMessage `json:"version"`
}

func taskPatch(body updateTaskBody) (domain.TaskPatch, error) {
	var patch domain.TaskPatch
	if len(body.Title) > 0 {
		var title string
		if err := json.Unmarshal(body.Title, &title); err != nil {
			return domain.TaskPatch{}, domain.Invalid("некорректное название")
		}
		patch.Title = &title
	}
	if len(body.Description) > 0 {
		var description string
		if err := json.Unmarshal(body.Description, &description); err != nil {
			return domain.TaskPatch{}, domain.Invalid("некорректное описание")
		}
		patch.Description = &description
	}
	if len(body.Status) > 0 {
		var status string
		if err := json.Unmarshal(body.Status, &status); err != nil {
			return domain.TaskPatch{}, domain.Invalid("некорректный статус")
		}
		patch.Status = &status
	}
	if len(body.AssigneeID) > 0 {
		if string(body.AssigneeID) == "null" {
			patch.Assignee = &domain.AssigneeChange{}
		} else {
			var id int64
			if err := json.Unmarshal(body.AssigneeID, &id); err != nil || id <= 0 {
				return domain.TaskPatch{}, domain.Invalid("некорректный исполнитель")
			}
			patch.Assignee = &domain.AssigneeChange{ID: &id}
		}
	}
	if len(body.Version) == 0 {
		return domain.TaskPatch{}, domain.Invalid("version обязательна")
	}
	if err := json.Unmarshal(body.Version, &patch.Version); err != nil || patch.Version <= 0 {
		return domain.TaskPatch{}, domain.Invalid("некорректная version")
	}
	return patch, nil
}
