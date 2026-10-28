package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"task-service/internal/domain"

	"github.com/redis/go-redis/v9"
)

type TaskList interface {
	GetList(ctx context.Context, key string) ([]domain.Task, bool, error)
	SetList(ctx context.Context, teamID int64, key string, tasks []domain.Task, ttl time.Duration) error
	Invalidate(ctx context.Context, teamID int64) error
}

type Redis struct {
	client *redis.Client
}

func Open(addr, password string) *Redis {
	return &Redis{client: redis.NewClient(&redis.Options{Addr: addr, Password: password})}
}

func NewRedis(client *redis.Client) *Redis {
	return &Redis{client: client}
}

func (r *Redis) Ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var last error
	for {
		last = r.client.Ping(pingCtx).Err()
		if last == nil {
			return nil
		}
		if pingCtx.Err() != nil {
			return last
		}
		select {
		case <-pingCtx.Done():
			return last
		case <-time.After(time.Second):
		}
	}
}

func (r *Redis) Close() error {
	return r.client.Close()
}

func (r *Redis) GetList(ctx context.Context, key string) ([]domain.Task, bool, error) {
	raw, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var tasks []domain.Task
	if err := json.Unmarshal(raw, &tasks); err != nil {
		return nil, false, err
	}
	return tasks, true, nil
}

func (r *Redis) SetList(ctx context.Context, teamID int64, key string, tasks []domain.Task, ttl time.Duration) error {
	raw, err := json.Marshal(tasks)
	if err != nil {
		return err
	}
	index := teamIndex(teamID)
	_, err = r.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, key, raw, ttl)
		pipe.SAdd(ctx, index, key)
		pipe.Expire(ctx, index, ttl)
		return nil
	})
	return err
}

func (r *Redis) Invalidate(ctx context.Context, teamID int64) error {
	index := teamIndex(teamID)
	keys, err := r.client.SMembers(ctx, index).Result()
	if err != nil {
		return err
	}
	keys = append(keys, index)
	return r.client.Del(ctx, keys...).Err()
}

func ListKey(filter domain.TaskFilter) string {
	status := "-"
	if filter.Status != "" {
		status = filter.Status
	}
	assignee := "-"
	if filter.AssigneeID != nil {
		assignee = strconv.FormatInt(*filter.AssigneeID, 10)
	}
	return fmt.Sprintf("tasks:%d:%s:%s:%d:%d", filter.TeamID, status, assignee, filter.Limit, filter.Offset)
}

func teamIndex(teamID int64) string {
	return fmt.Sprintf("tasks:team:%d:keys", teamID)
}
