package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"task-service/internal/cache"
	"task-service/internal/domain"
	"task-service/internal/repository"
)

type Task struct {
	store repository.Store
	cache cache.TaskList
	ttl   time.Duration
	now   func() time.Time
	log   *slog.Logger
}

func NewTask(store repository.Store, lists cache.TaskList, ttl time.Duration, logger *slog.Logger) *Task {
	if logger == nil {
		logger = slog.Default()
	}
	return &Task{
		store: store,
		cache: lists,
		ttl:   ttl,
		now:   func() time.Time { return time.Now().UTC() },
		log:   logger,
	}
}

type TaskInput struct {
	TeamID      int64
	Title       string
	Description string
	Status      string
	AssigneeID  *int64
}

func (t *Task) Create(ctx context.Context, actorID int64, input TaskInput) (domain.Task, error) {
	if _, err := t.requireMember(ctx, input.TeamID, actorID); err != nil {
		return domain.Task{}, err
	}
	if err := t.ensureAssignee(ctx, input.TeamID, input.AssigneeID); err != nil {
		return domain.Task{}, err
	}
	prepared, err := domain.PrepareNewTask(domain.Task{
		TeamID:      input.TeamID,
		Title:       input.Title,
		Description: input.Description,
		Status:      input.Status,
		CreatedBy:   actorID,
		AssigneeID:  input.AssigneeID,
	}, t.now())
	if err != nil {
		return domain.Task{}, err
	}
	created, err := t.store.CreateTask(ctx, prepared, domain.CreationChanges(prepared))
	if err != nil {
		return domain.Task{}, err
	}
	t.invalidate(ctx, input.TeamID)
	return created, nil
}

func (t *Task) List(ctx context.Context, actorID int64, filter domain.TaskFilter) ([]domain.Task, error) {
	if filter.TeamID <= 0 {
		return nil, domain.Invalid("team_id обязателен")
	}
	if _, err := t.requireMember(ctx, filter.TeamID, actorID); err != nil {
		return nil, err
	}
	if filter.Status != "" && !domain.ValidStatus(filter.Status) {
		return nil, domain.Invalid("неизвестный статус")
	}
	if filter.AssigneeID != nil && *filter.AssigneeID <= 0 {
		return nil, domain.Invalid("некорректный assignee_id")
	}
	if filter.Limit < 0 {
		return nil, domain.Invalid("limit не может быть отрицательным")
	}
	if filter.Limit == 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		return nil, domain.Invalid("limit не может быть больше 100")
	}
	if filter.Offset < 0 {
		return nil, domain.Invalid("offset не может быть отрицательным")
	}

	key := cache.ListKey(filter)
	if tasks, ok, err := t.cache.GetList(ctx, key); err != nil {
		t.log.Warn("task cache read failed", "team_id", filter.TeamID, "err", err)
	} else if ok {
		return tasks, nil
	}

	tasks, err := t.store.ListTasks(ctx, filter)
	if err != nil {
		return nil, err
	}
	if err := t.cache.SetList(ctx, filter.TeamID, key, tasks, t.ttl); err != nil {
		t.log.Warn("task cache write failed", "team_id", filter.TeamID, "err", err)
	}
	return tasks, nil
}

func (t *Task) Update(ctx context.Context, actorID, taskID int64, patch domain.TaskPatch) (domain.Task, error) {
	if patch.Version <= 0 {
		return domain.Task{}, domain.Invalid("version обязательна")
	}
	current, err := t.store.TaskByID(ctx, taskID)
	if err != nil {
		return domain.Task{}, err
	}
	actor, err := t.requireMember(ctx, current.TeamID, actorID)
	if err != nil {
		if errors.Is(err, domain.ErrForbidden) {
			return domain.Task{}, domain.ErrNotFound
		}
		return domain.Task{}, err
	}
	if patch.Assignee != nil && patch.Assignee.ID != nil {
		if err := t.ensureAssignee(ctx, current.TeamID, patch.Assignee.ID); err != nil {
			return domain.Task{}, err
		}
	}
	updated, err := t.store.UpdateTask(ctx, taskID, actorID, patch.Version, t.now(), func(locked domain.Task) (domain.Task, domain.Changes, error) {
		return domain.ApplyPatch(locked, patch, actor, t.now())
	})
	if err != nil {
		return domain.Task{}, err
	}
	t.invalidate(ctx, current.TeamID)
	return updated, nil
}

func (t *Task) History(ctx context.Context, actorID, taskID int64) ([]domain.HistoryEntry, error) {
	if _, err := t.visibleTask(ctx, actorID, taskID); err != nil {
		return nil, err
	}
	return t.store.History(ctx, taskID)
}

func (t *Task) Comments(ctx context.Context, actorID, taskID int64) ([]domain.Comment, error) {
	if _, err := t.visibleTask(ctx, actorID, taskID); err != nil {
		return nil, err
	}
	return t.store.Comments(ctx, taskID)
}

func (t *Task) AddComment(ctx context.Context, actorID, taskID int64, content string) (domain.Comment, error) {
	if _, err := t.visibleTask(ctx, actorID, taskID); err != nil {
		return domain.Comment{}, err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return domain.Comment{}, domain.Invalid("комментарий не может быть пустым")
	}
	if len(content) > 5000 {
		return domain.Comment{}, domain.Invalid("комментарий слишком длинный")
	}
	return t.store.AddComment(ctx, domain.Comment{
		TaskID:    taskID,
		UserID:    actorID,
		Content:   content,
		CreatedAt: t.now(),
	})
}

func (t *Task) visibleTask(ctx context.Context, actorID, taskID int64) (domain.Task, error) {
	task, err := t.store.TaskByID(ctx, taskID)
	if err != nil {
		return domain.Task{}, err
	}
	if _, err := t.requireMember(ctx, task.TeamID, actorID); err != nil {
		if errors.Is(err, domain.ErrForbidden) {
			return domain.Task{}, domain.ErrNotFound
		}
		return domain.Task{}, err
	}
	return task, nil
}

func (t *Task) requireMember(ctx context.Context, teamID, userID int64) (domain.Member, error) {
	if teamID <= 0 {
		return domain.Member{}, domain.Invalid("некорректная команда")
	}
	if _, err := t.store.TeamByID(ctx, teamID); err != nil {
		return domain.Member{}, err
	}
	member, err := t.store.TeamMember(ctx, teamID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Member{}, domain.ErrForbidden
		}
		return domain.Member{}, err
	}
	return member, nil
}

func (t *Task) ensureAssignee(ctx context.Context, teamID int64, assigneeID *int64) error {
	if assigneeID == nil {
		return nil
	}
	if *assigneeID <= 0 {
		return domain.Invalid("некорректный исполнитель")
	}
	_, err := t.store.TeamMember(ctx, teamID, *assigneeID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Invalid("исполнитель не состоит в команде")
		}
		return err
	}
	return nil
}

func (t *Task) invalidate(ctx context.Context, teamID int64) {
	if err := t.cache.Invalidate(ctx, teamID); err != nil {
		t.log.Warn("task cache invalidate failed", "team_id", teamID, "err", err)
	}
}
