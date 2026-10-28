package mysql

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"testing"
	"time"

	"task-service/internal/domain"

	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

func TestStatsReportSQL(t *testing.T) {
	repo := openMySQL(t)
	ctx := context.Background()
	since := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour

	owner := mustUser(t, repo, "owner@example.com")
	assigneeA := mustUser(t, repo, "a@example.com")
	assigneeB := mustUser(t, repo, "b@example.com")
	assigneeC := mustUser(t, repo, "c@example.com")
	assigneeD := mustUser(t, repo, "d@example.com")
	team, err := repo.CreateTeam(ctx, "Команда", owner.ID, since)
	if err != nil {
		t.Fatal(err)
	}
	member, err := repo.TeamMember(ctx, team.ID, owner.ID)
	if err != nil || member.Role != domain.RoleOwner {
		t.Fatalf("owner membership = %+v, err = %v", member, err)
	}
	other, err := repo.CreateTeam(ctx, "Чужая", owner.ID, since)
	if err != nil {
		t.Fatal(err)
	}

	task := func(teamID int64, title, status string, assignee *int64, created, closed *time.Time) domain.Task {
		t.Helper()
		item := domain.Task{
			TeamID: teamID, Title: title, Description: "", Status: status,
			CreatedBy: owner.ID, AssigneeID: assignee, CreatedAt: *created, UpdatedAt: *created, Version: 1,
		}
		if closed != nil {
			item.ClosedAt = closed
		}
		createdTask, err := repo.CreateTask(ctx, item, domain.CreationChanges(item))
		if err != nil {
			t.Fatal(err)
		}
		return createdTask
	}
	at := func(value time.Time) *time.Time { return &value }

	todo := task(team.ID, "todo", domain.StatusTodo, nil, at(since), nil)
	task(team.ID, "progress", domain.StatusInProgress, &assigneeA.ID, at(since), nil)
	recentA := task(team.ID, "recent-a", domain.StatusDone, &assigneeA.ID, at(since.Add(-12*day)), at(since.Add(-10*day)))
	bOne := task(team.ID, "b-one", domain.StatusDone, &assigneeB.ID, at(since.Add(-6*day)), at(since.Add(-5*day)))
	task(team.ID, "old-a", domain.StatusDone, &assigneeA.ID, at(since.Add(-50*day)), at(since.Add(-40*day)))
	task(team.ID, "c", domain.StatusDone, &assigneeC.ID, at(since.Add(-2*day)), at(since.Add(-1*day)))
	task(team.ID, "b-two", domain.StatusDone, &assigneeB.ID, at(since.Add(-4*day)), at(since.Add(-2*day)))
	task(team.ID, "d", domain.StatusDone, &assigneeD.ID, at(since.Add(-3*day)), at(since.Add(-1*day)))
	foreign := task(other.ID, "foreign", domain.StatusDone, &assigneeD.ID, at(since.Add(-2*day)), at(since.Add(-1*day)))

	for _, id := range []int64{todo.ID, todo.ID, todo.ID, recentA.ID, recentA.ID, bOne.ID} {
		if _, err := repo.AddComment(ctx, domain.Comment{TaskID: id, UserID: owner.ID, Content: "заметка", CreatedAt: since}); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if _, err := repo.AddComment(ctx, domain.Comment{TaskID: foreign.ID, UserID: owner.ID, Content: "чужое", CreatedAt: since}); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := repo.Stats(ctx, team.ID, since.Add(-30*day))
	if err != nil {
		t.Fatal(err)
	}
	if stats.TasksByStatus[domain.StatusTodo] != 1 || stats.TasksByStatus[domain.StatusInProgress] != 1 || stats.TasksByStatus[domain.StatusDone] != 6 {
		t.Fatalf("statuses = %+v", stats.TasksByStatus)
	}
	if stats.CommentCount != 6 {
		t.Fatalf("comments = %d", stats.CommentCount)
	}
	if stats.AvgCloseSeconds == nil || *stats.AvgCloseSeconds != 259200 {
		t.Fatalf("avg = %v", stats.AvgCloseSeconds)
	}
	if len(stats.TopAssignees) != 3 ||
		stats.TopAssignees[0] != (domain.TopAssignee{AssigneeID: assigneeB.ID, ClosedCount: 2}) ||
		stats.TopAssignees[1] != (domain.TopAssignee{AssigneeID: assigneeA.ID, ClosedCount: 1}) ||
		stats.TopAssignees[2] != (domain.TopAssignee{AssigneeID: assigneeC.ID, ClosedCount: 1}) {
		t.Fatalf("top = %+v", stats.TopAssignees)
	}

	current, err := repo.TaskByID(ctx, todo.ID)
	if err != nil {
		t.Fatal(err)
	}
	actor := domain.Member{TeamID: team.ID, UserID: owner.ID, Role: domain.RoleOwner}
	var wg sync.WaitGroup
	var mu sync.Mutex
	succeeded, conflicts := 0, 0
	for _, title := range []string{"A", "B"} {
		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			_, updateErr := repo.UpdateTask(ctx, current.ID, owner.ID, current.Version, since.Add(time.Hour), func(locked domain.Task) (domain.Task, domain.Changes, error) {
				return domain.ApplyPatch(locked, domain.TaskPatch{Title: &value}, actor, since.Add(time.Hour))
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case updateErr == nil:
				succeeded++
			case errors.Is(updateErr, domain.ErrConflict):
				conflicts++
			default:
				t.Errorf("update %s: %v", value, updateErr)
			}
		}(title)
	}
	wg.Wait()
	if succeeded != 1 || conflicts != 1 {
		t.Fatalf("concurrent results: ok=%d conflict=%d", succeeded, conflicts)
	}
	history, err := repo.History(ctx, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[1].Changes["title"].Old != "todo" {
		t.Fatalf("history = %+v", history)
	}
	final, err := repo.TaskByID(ctx, current.ID)
	if err != nil || final.Version != current.Version+1 || final.Title != history[1].Changes["title"].New {
		t.Fatalf("final = %+v, history = %+v, err = %v", final, history[1].Changes["title"], err)
	}
}

func mustUser(t *testing.T, repo *Repository, email string) domain.User {
	t.Helper()
	user, err := repo.CreateUser(context.Background(), domain.User{
		Email: email, PasswordHash: "hash", Name: email, CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func openMySQL(t *testing.T) *Repository {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon not available")
	}
	ctx := context.Background()
	container, err := tcmysql.Run(ctx, "mysql:8.4",
		tcmysql.WithDatabase("tasks"),
		tcmysql.WithUsername("tasks"),
		tcmysql.WithPassword("tasks"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})
	dsn, err := container.ConnectionString(ctx, "parseTime=true", "loc=UTC")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}
