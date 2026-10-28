package domain

import (
	"testing"
	"time"
)

func TestPrepareNewTask(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	assignee := int64(7)
	task, err := PrepareNewTask(Task{
		TeamID:      1,
		Title:       "  Купить молоко ",
		Description: "  два литра ",
		Status:      StatusDone,
		CreatedBy:   3,
		AssigneeID:  &assignee,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != "Купить молоко" || task.Description != "два литра" || task.Version != 1 {
		t.Fatalf("task = %+v", task)
	}
	if task.ClosedAt == nil || !task.ClosedAt.Equal(now) {
		t.Fatalf("closed_at = %v", task.ClosedAt)
	}
	changes := CreationChanges(task)
	if changes["status"].New != StatusDone || changes["assignee_id"].New != assignee || changes["title"].Old != nil {
		t.Fatalf("changes = %+v", changes)
	}

	if _, err := PrepareNewTask(Task{Title: "  "}, now); err == nil {
		t.Fatal("expected empty title error")
	}
	if _, err := PrepareNewTask(Task{Title: "x", Status: "later"}, now); err == nil {
		t.Fatal("expected status error")
	}
	open, err := PrepareNewTask(Task{Title: "a"}, now)
	if err != nil || open.Status != StatusTodo || open.ClosedAt != nil {
		t.Fatalf("open = %+v, err = %v", open, err)
	}
}

func TestApplyPatchPermissions(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	assignee := int64(5)
	current := Task{
		ID: 1, TeamID: 10, Title: "old", Description: "desc", Status: StatusTodo,
		CreatedBy: 4, AssigneeID: &assignee, Version: 2, CreatedAt: now, UpdatedAt: now,
	}
	title := "new"
	status := StatusDone
	other := int64(8)

	owner := Member{TeamID: 10, UserID: 1, Role: RoleOwner}
	updated, changes, err := ApplyPatch(current, TaskPatch{Title: &title, Status: &status, Assignee: &AssigneeChange{ID: &other}}, owner, now)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != title || updated.Status != StatusDone || updated.ClosedAt == nil || *updated.AssigneeID != other {
		t.Fatalf("updated = %+v", updated)
	}
	if changes["title"].Old != "old" || changes["status"].New != StatusDone || changes["closed_at"].Old != nil {
		t.Fatalf("changes = %+v", changes)
	}

	creator := Member{TeamID: 10, UserID: 4, Role: RoleMember}
	if _, _, err := ApplyPatch(current, TaskPatch{Title: &title}, creator, now); err != nil {
		t.Fatal(err)
	}
	assigneeActor := Member{TeamID: 10, UserID: 5, Role: RoleMember}
	if _, _, err := ApplyPatch(current, TaskPatch{Title: &title}, assigneeActor, now); err != ErrForbidden {
		t.Fatalf("assignee title err = %v", err)
	}
	if _, _, err := ApplyPatch(current, TaskPatch{Assignee: &AssigneeChange{ID: &other}}, assigneeActor, now); err != ErrForbidden {
		t.Fatalf("assignee reassignment err = %v", err)
	}
	next, changes, err := ApplyPatch(current, TaskPatch{Status: &status}, assigneeActor, now)
	if err != nil || next.Status != StatusDone || changes["status"].Old != StatusTodo {
		t.Fatalf("status update = %+v, %+v, %v", next, changes, err)
	}

	stranger := Member{TeamID: 10, UserID: 9, Role: RoleMember}
	if _, _, err := ApplyPatch(current, TaskPatch{Status: &status}, stranger, now); err != ErrForbidden {
		t.Fatalf("stranger err = %v", err)
	}
	wrongTeam := Member{TeamID: 11, UserID: 1, Role: RoleOwner}
	if _, _, err := ApplyPatch(current, TaskPatch{Title: &title}, wrongTeam, now); err != ErrForbidden {
		t.Fatalf("team err = %v", err)
	}

	same, changes, err := ApplyPatch(current, TaskPatch{Title: strPtr("old")}, owner, now)
	if err != nil || len(changes) != 0 || same.Version != current.Version {
		t.Fatalf("noop = %+v, %+v, %v", same, changes, err)
	}
	reopen := StatusTodo
	done := current
	closed := now
	done.Status = StatusDone
	done.ClosedAt = &closed
	reopened, changes, err := ApplyPatch(done, TaskPatch{Status: &reopen}, owner, now.Add(time.Hour))
	if err != nil || reopened.ClosedAt != nil || changes["closed_at"].New != nil {
		t.Fatalf("reopen = %+v, %+v, %v", reopened, changes, err)
	}
	cleared, changes, err := ApplyPatch(current, TaskPatch{Assignee: &AssigneeChange{}}, owner, now)
	if err != nil || cleared.AssigneeID != nil || changes["assignee_id"].New != nil {
		t.Fatalf("clear assignee = %+v, %+v, %v", cleared, changes, err)
	}
}

func strPtr(value string) *string {
	return &value
}
