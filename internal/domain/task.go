package domain

import (
	"strings"
	"time"
)

type AssigneeChange struct {
	ID *int64
}

type TaskPatch struct {
	Title       *string
	Description *string
	Status      *string
	Assignee    *AssigneeChange
	Version     int64
}

func PrepareNewTask(task Task, now time.Time) (Task, error) {
	task.Title = strings.TrimSpace(task.Title)
	task.Description = strings.TrimSpace(task.Description)
	if task.Title == "" {
		return Task{}, Invalid("название задачи обязательно")
	}
	if len(task.Title) > 500 {
		return Task{}, Invalid("название задачи слишком длинное")
	}
	if len(task.Description) > 10000 {
		return Task{}, Invalid("описание задачи слишком длинное")
	}
	if task.Status == "" {
		task.Status = StatusTodo
	}
	if !ValidStatus(task.Status) {
		return Task{}, Invalid("неизвестный статус")
	}
	now = now.UTC()
	task.CreatedAt = now
	task.UpdatedAt = now
	task.Version = 1
	task.ClosedAt = nil
	if task.Status == StatusDone {
		closed := now
		task.ClosedAt = &closed
	}
	return task, nil
}

func CreationChanges(task Task) Changes {
	changes := Changes{
		"title":       {Old: nil, New: task.Title},
		"description": {Old: nil, New: task.Description},
		"status":      {Old: nil, New: task.Status},
		"assignee_id": {Old: nil, New: assigneeJSON(task.AssigneeID)},
	}
	if task.ClosedAt != nil {
		changes["closed_at"] = FieldChange{Old: nil, New: task.ClosedAt.UTC().Format(time.RFC3339)}
	}
	return changes
}

func ApplyPatch(current Task, patch TaskPatch, actor Member, now time.Time) (Task, Changes, error) {
	if actor.TeamID != current.TeamID || actor.UserID == 0 {
		return Task{}, nil, ErrForbidden
	}
	fullAccess := actor.Role == RoleOwner || actor.Role == RoleAdmin
	isCreator := current.CreatedBy == actor.UserID
	isAssignee := current.AssigneeID != nil && *current.AssigneeID == actor.UserID
	if !fullAccess && !isCreator && !isAssignee {
		return Task{}, nil, ErrForbidden
	}
	if !fullAccess && !isCreator {
		if patch.Title != nil || patch.Description != nil || patch.Assignee != nil {
			return Task{}, nil, ErrForbidden
		}
	}

	next := current
	changes := Changes{}

	if patch.Title != nil {
		title := strings.TrimSpace(*patch.Title)
		if title == "" {
			return Task{}, nil, Invalid("название задачи обязательно")
		}
		if len(title) > 500 {
			return Task{}, nil, Invalid("название задачи слишком длинное")
		}
		if title != current.Title {
			changes["title"] = FieldChange{Old: current.Title, New: title}
			next.Title = title
		}
	}
	if patch.Description != nil {
		description := strings.TrimSpace(*patch.Description)
		if len(description) > 10000 {
			return Task{}, nil, Invalid("описание задачи слишком длинное")
		}
		if description != current.Description {
			changes["description"] = FieldChange{Old: current.Description, New: description}
			next.Description = description
		}
	}
	if patch.Status != nil {
		if !ValidStatus(*patch.Status) {
			return Task{}, nil, Invalid("неизвестный статус")
		}
		if *patch.Status != current.Status {
			changes["status"] = FieldChange{Old: current.Status, New: *patch.Status}
			next.Status = *patch.Status
			var closed *time.Time
			if next.Status == StatusDone {
				stamp := now.UTC()
				closed = &stamp
			}
			changes["closed_at"] = FieldChange{Old: timeJSON(current.ClosedAt), New: timeJSON(closed)}
			next.ClosedAt = closed
		}
	}
	if patch.Assignee != nil && !sameAssignee(current.AssigneeID, patch.Assignee.ID) {
		changes["assignee_id"] = FieldChange{
			Old: assigneeJSON(current.AssigneeID),
			New: assigneeJSON(patch.Assignee.ID),
		}
		next.AssigneeID = cloneID(patch.Assignee.ID)
	}

	return next, changes, nil
}

func sameAssignee(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func cloneID(id *int64) *int64 {
	if id == nil {
		return nil
	}
	value := *id
	return &value
}

func assigneeJSON(id *int64) any {
	if id == nil {
		return nil
	}
	return *id
}

func timeJSON(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339)
}
