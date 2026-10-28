package cache

import (
	"context"
	"testing"
	"time"

	"task-service/internal/domain"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestListCache(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	store := NewRedis(client)
	ctx := context.Background()
	assignee := int64(5)

	first := domain.TaskFilter{TeamID: 1, Status: domain.StatusTodo, AssigneeID: &assignee, Limit: 20, Offset: 0}
	second := first
	second.Offset = 20
	if ListKey(first) == ListKey(second) {
		t.Fatal("filters must produce different keys")
	}
	tasks := []domain.Task{{ID: 1, TeamID: 1, Title: "a", Status: domain.StatusTodo}}
	if err := store.SetList(ctx, 1, ListKey(first), tasks, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := store.SetList(ctx, 1, ListKey(second), tasks, time.Minute); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.GetList(ctx, ListKey(first))
	if err != nil || !ok || len(got) != 1 || got[0].Title != "a" {
		t.Fatalf("got = %+v, ok = %v, err = %v", got, ok, err)
	}
	if _, ok, err := store.GetList(ctx, "missing"); err != nil || ok {
		t.Fatalf("missing ok = %v, err = %v", ok, err)
	}
	if err := store.Invalidate(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.GetList(ctx, ListKey(first)); err != nil || ok {
		t.Fatalf("after invalidate ok = %v, err = %v", ok, err)
	}
	if _, ok, err := store.GetList(ctx, ListKey(second)); err != nil || ok {
		t.Fatalf("second key ok = %v, err = %v", ok, err)
	}
	if err := server.Set("broken", "{"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.GetList(ctx, "broken"); err == nil {
		t.Fatal("expected broken cache payload")
	}
}

func TestPing(t *testing.T) {
	server := miniredis.RunT(t)
	store := Open(server.Addr(), "")
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}
