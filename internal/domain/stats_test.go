package domain

import (
	"testing"
	"time"
)

func TestComputeStats(t *testing.T) {
	since := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	id := func(v int64) *int64 { return &v }
	tasks := []Task{
		{ID: 1, TeamID: 1, Status: StatusTodo, CreatedAt: since},
		{ID: 2, TeamID: 1, Status: StatusInProgress, AssigneeID: id(2), CreatedAt: since},
		{ID: 3, TeamID: 1, Status: StatusDone, AssigneeID: id(2), CreatedAt: since.Add(-12 * day), ClosedAt: timePtr(since.Add(-10 * day))},
		{ID: 4, TeamID: 1, Status: StatusDone, AssigneeID: id(3), CreatedAt: since.Add(-6 * day), ClosedAt: timePtr(since.Add(-5 * day))},
		{ID: 5, TeamID: 1, Status: StatusDone, AssigneeID: id(2), CreatedAt: since.Add(-50 * day), ClosedAt: timePtr(since.Add(-40 * day))},
		{ID: 6, TeamID: 1, Status: StatusDone, AssigneeID: id(4), CreatedAt: since.Add(-2 * day), ClosedAt: timePtr(since.Add(-1 * day))},
		{ID: 7, TeamID: 1, Status: StatusDone, AssigneeID: id(3), CreatedAt: since.Add(-4 * day), ClosedAt: timePtr(since.Add(-2 * day))},
		{ID: 8, TeamID: 1, Status: StatusDone, AssigneeID: id(5), CreatedAt: since.Add(-3 * day), ClosedAt: timePtr(since.Add(-1 * day))},
		{ID: 9, TeamID: 2, Status: StatusDone, AssigneeID: id(9), CreatedAt: since.Add(-2 * day), ClosedAt: timePtr(since.Add(-1 * day))},
	}
	comments := []Comment{{TaskID: 1}, {TaskID: 1}, {TaskID: 1}, {TaskID: 3}, {TaskID: 3}, {TaskID: 4}, {TaskID: 9}, {TaskID: 9}}

	stats := ComputeStats(tasks, comments, 1, since.Add(-30*day))
	if stats.TasksByStatus[StatusTodo] != 1 || stats.TasksByStatus[StatusInProgress] != 1 || stats.TasksByStatus[StatusDone] != 6 {
		t.Fatalf("statuses = %+v", stats.TasksByStatus)
	}
	if stats.CommentCount != 6 {
		t.Fatalf("comments = %d", stats.CommentCount)
	}
	if stats.AvgCloseSeconds == nil || *stats.AvgCloseSeconds != 259200 {
		t.Fatalf("avg = %v", stats.AvgCloseSeconds)
	}
	if len(stats.TopAssignees) != 3 ||
		stats.TopAssignees[0] != (TopAssignee{AssigneeID: 3, ClosedCount: 2}) ||
		stats.TopAssignees[1] != (TopAssignee{AssigneeID: 2, ClosedCount: 1}) ||
		stats.TopAssignees[2] != (TopAssignee{AssigneeID: 4, ClosedCount: 1}) {
		t.Fatalf("top = %+v", stats.TopAssignees)
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}
