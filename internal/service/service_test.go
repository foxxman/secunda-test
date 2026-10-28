package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"task-service/internal/auth"
	"task-service/internal/cache"
	"task-service/internal/domain"
	"task-service/internal/memstore"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestAuthAndTeams(t *testing.T) {
	ctx := context.Background()
	store := memstore.New()
	now := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	accounts := NewAuth(store, auth.NewManager("test-secret-value", time.Hour))
	accounts.now = func() time.Time { return now }
	teams := NewTeam(store)
	teams.now = func() time.Time { return now }

	owner, err := accounts.Register(ctx, "Owner@Example.com", "password123", " Анна ")
	if err != nil || owner.Email != "owner@example.com" || owner.Name != "Анна" {
		t.Fatalf("owner = %+v, err = %v", owner, err)
	}
	if _, err := accounts.Register(ctx, "owner@example.com", "password123", "Другой"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate err = %v", err)
	}
	if _, err := accounts.Login(ctx, "owner@example.com", "wrong-pass"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("login err = %v", err)
	}
	if _, err := accounts.Login(ctx, "missing@example.com", "password123"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("missing err = %v", err)
	}
	session, err := accounts.Login(ctx, "owner@example.com", "password123")
	if err != nil || session.AccessToken == "" {
		t.Fatal(err)
	}

	member, err := accounts.Register(ctx, "member@example.com", "password123", "Борис")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := accounts.Register(ctx, "admin@example.com", "password123", "Вера")
	if err != nil {
		t.Fatal(err)
	}

	team, err := teams.Create(ctx, owner.ID, "  Платформа ")
	if err != nil || team.Role != domain.RoleOwner || team.CreatedBy != owner.ID {
		t.Fatalf("team = %+v, err = %v", team, err)
	}
	listed, err := teams.List(ctx, member.ID)
	if err != nil || len(listed) != 0 {
		t.Fatalf("listed = %+v, err = %v", listed, err)
	}
	if _, err := teams.Invite(ctx, member.ID, team.ID, Invite{Email: admin.Email, Role: domain.RoleAdmin}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("member invite err = %v", err)
	}
	if _, err := teams.Invite(ctx, owner.ID, team.ID, Invite{Email: admin.Email, Role: domain.RoleOwner}); err == nil {
		t.Fatal("owner role must be rejected")
	}
	invited, err := teams.Invite(ctx, owner.ID, team.ID, Invite{UserID: &member.ID, Email: member.Email, Role: domain.RoleMember})
	if err != nil || invited.Role != domain.RoleMember {
		t.Fatalf("invite = %+v, err = %v", invited, err)
	}
	if _, err := teams.Invite(ctx, owner.ID, team.ID, Invite{UserID: &admin.ID, Email: member.Email, Role: domain.RoleAdmin}); err == nil {
		t.Fatal("mismatched identity must be rejected")
	}
	if _, err := teams.Invite(ctx, owner.ID, team.ID, Invite{Role: domain.RoleMember}); err == nil {
		t.Fatal("missing identity must be rejected")
	}
	adminMember, err := teams.Invite(ctx, owner.ID, team.ID, Invite{Email: "ADMIN@example.com", Role: domain.RoleMember})
	if err != nil || adminMember.UserID != admin.ID {
		t.Fatalf("admin invite = %+v, err = %v", adminMember, err)
	}
	promoted, err := teams.Invite(ctx, owner.ID, team.ID, Invite{UserID: &admin.ID, Role: domain.RoleAdmin})
	if err != nil || promoted.Role != domain.RoleAdmin {
		t.Fatalf("promote = %+v, err = %v", promoted, err)
	}
	if _, err := teams.Invite(ctx, admin.ID, team.ID, Invite{UserID: &member.ID, Role: domain.RoleAdmin}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("admin role change err = %v", err)
	}
	if _, err := teams.Invite(ctx, admin.ID, team.ID, Invite{UserID: &owner.ID, Role: domain.RoleMember}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("owner role change err = %v", err)
	}

	if _, err := teams.Stats(ctx, member.ID, team.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("member stats err = %v", err)
	}
	stats, err := teams.Stats(ctx, admin.ID, team.ID)
	if err != nil || stats.CommentCount != 0 || stats.TasksByStatus[domain.StatusTodo] != 0 {
		t.Fatalf("stats = %+v, err = %v", stats, err)
	}
	if _, err := teams.Stats(ctx, owner.ID, 999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing team err = %v", err)
	}
}

func TestTaskRulesAndCache(t *testing.T) {
	ctx := context.Background()
	store := memstore.New()
	now := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	accounts := NewAuth(store, auth.NewManager("test-secret-value", time.Hour))
	accounts.now = func() time.Time { return now }
	teams := NewTeam(store)
	teams.now = accounts.now
	redisServer := miniredis.RunT(t)
	lists := cache.NewRedis(redis.NewClient(&redis.Options{Addr: redisServer.Addr()}))
	tasks := NewTask(store, lists, time.Minute, slog.New(slog.NewTextHandler(io.Discard, nil)))
	tasks.now = accounts.now

	owner, _ := accounts.Register(ctx, "owner@example.com", "password123", "Анна")
	worker, _ := accounts.Register(ctx, "worker@example.com", "password123", "Борис")
	stranger, _ := accounts.Register(ctx, "stranger@example.com", "password123", "Глеб")
	team, err := teams.Create(ctx, owner.ID, "Команда")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := teams.Invite(ctx, owner.ID, team.ID, Invite{UserID: &worker.ID, Role: domain.RoleMember}); err != nil {
		t.Fatal(err)
	}

	outside := stranger.ID
	if _, err := tasks.Create(ctx, owner.ID, TaskInput{TeamID: team.ID, Title: "x", AssigneeID: &outside}); err == nil {
		t.Fatal("foreign assignee must be rejected")
	}
	created, err := tasks.Create(ctx, owner.ID, TaskInput{TeamID: team.ID, Title: "Отчет", Description: "черновик", AssigneeID: &worker.ID})
	if err != nil || created.CreatedBy != owner.ID || created.Status != domain.StatusTodo || created.Version != 1 {
		t.Fatalf("created = %+v, err = %v", created, err)
	}
	if _, err := tasks.List(ctx, stranger.ID, domain.TaskFilter{TeamID: team.ID}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("list err = %v", err)
	}
	if _, err := tasks.List(ctx, owner.ID, domain.TaskFilter{TeamID: team.ID, Status: "later"}); err == nil {
		t.Fatal("expected bad status")
	}
	first, err := tasks.List(ctx, owner.ID, domain.TaskFilter{TeamID: team.ID, Status: domain.StatusTodo, Limit: 20})
	if err != nil || len(first) != 1 {
		t.Fatalf("first = %+v, err = %v", first, err)
	}
	hidden := created
	hidden.Title = "из базы"
	store.SetTask(hidden)
	cached, err := tasks.List(ctx, owner.ID, domain.TaskFilter{TeamID: team.ID, Status: domain.StatusTodo, Limit: 20})
	if err != nil || cached[0].Title != "Отчет" {
		t.Fatalf("cache = %+v, err = %v", cached, err)
	}

	title := "Отчет v2"
	if _, err := tasks.Update(ctx, stranger.ID, created.ID, domain.TaskPatch{Title: &title, Version: created.Version}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("stranger update err = %v", err)
	}
	if _, err := tasks.Update(ctx, owner.ID, created.ID, domain.TaskPatch{Title: &title}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("missing version err = %v", err)
	}
	other := owner.ID
	if _, err := tasks.Update(ctx, worker.ID, created.ID, domain.TaskPatch{Assignee: &domain.AssigneeChange{ID: &other}, Version: created.Version}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("reassign err = %v", err)
	}
	done := domain.StatusDone
	updated, err := tasks.Update(ctx, worker.ID, created.ID, domain.TaskPatch{Status: &done, Version: created.Version})
	if err != nil || updated.Status != domain.StatusDone || updated.ClosedAt == nil || updated.Version != 2 {
		t.Fatalf("updated = %+v, err = %v", updated, err)
	}
	renamed, err := tasks.Update(ctx, owner.ID, created.ID, domain.TaskPatch{Title: &title, Version: updated.Version})
	if err != nil || renamed.Title != title {
		t.Fatalf("renamed = %+v, err = %v", renamed, err)
	}
	after, err := tasks.List(ctx, owner.ID, domain.TaskFilter{TeamID: team.ID, Status: domain.StatusTodo, Limit: 20})
	if err != nil || len(after) != 0 {
		t.Fatalf("invalidated list = %+v, err = %v", after, err)
	}

	history, err := tasks.History(ctx, owner.ID, created.ID)
	if err != nil || len(history) != 3 {
		t.Fatalf("history = %+v, err = %v", history, err)
	}
	if history[1].Changes["status"].Old != domain.StatusTodo || history[2].Changes["title"].New != title {
		t.Fatalf("history changes = %+v / %+v", history[1].Changes, history[2].Changes)
	}
	if _, err := tasks.History(ctx, stranger.ID, created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("hidden history err = %v", err)
	}
	if _, err := tasks.AddComment(ctx, worker.ID, created.ID, "   "); err == nil {
		t.Fatal("empty comment")
	}
	comment, err := tasks.AddComment(ctx, owner.ID, created.ID, " готово ")
	if err != nil || comment.Content != "готово" {
		t.Fatalf("comment = %+v, err = %v", comment, err)
	}
	comments, err := tasks.Comments(ctx, worker.ID, created.ID)
	if err != nil || len(comments) != 1 {
		t.Fatalf("comments = %+v, err = %v", comments, err)
	}

	staleTitle := "устаревшее"
	if _, err := tasks.Update(ctx, owner.ID, created.ID, domain.TaskPatch{Title: &staleTitle, Version: created.Version}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale version err = %v", err)
	}
	kept, err := store.TaskByID(ctx, created.ID)
	if err != nil || kept.Title != title || kept.Version != renamed.Version {
		t.Fatalf("kept = %+v, err = %v", kept, err)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	succeeded, conflicts := 0, 0
	for _, nextTitle := range []string{"A", "B"} {
		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			_, updateErr := tasks.Update(ctx, owner.ID, created.ID, domain.TaskPatch{Title: &value, Version: renamed.Version})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case updateErr == nil:
				succeeded++
			case errors.Is(updateErr, domain.ErrConflict):
				conflicts++
			default:
				t.Errorf("concurrent update: %v", updateErr)
			}
		}(nextTitle)
	}
	wg.Wait()
	if succeeded != 1 || conflicts != 1 {
		t.Fatalf("concurrent results: ok=%d conflict=%d", succeeded, conflicts)
	}
	finalHistory, err := tasks.History(ctx, owner.ID, created.ID)
	if err != nil || len(finalHistory) != 4 {
		t.Fatalf("concurrent history = %d, err = %v", len(finalHistory), err)
	}
	finalTask, err := store.TaskByID(ctx, created.ID)
	if err != nil || finalTask.Version != renamed.Version+1 || finalHistory[3].Changes["title"].New != finalTask.Title {
		t.Fatalf("final = %+v, last = %+v, err = %v", finalTask, finalHistory[3].Changes["title"], err)
	}
}
