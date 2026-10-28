package repository

import (
	"context"
	"time"

	"task-service/internal/domain"
)

type Store interface {
	CreateUser(ctx context.Context, user domain.User) (domain.User, error)
	UserByEmail(ctx context.Context, email string) (domain.User, error)
	UserByID(ctx context.Context, id int64) (domain.User, error)

	CreateTeam(ctx context.Context, name string, ownerID int64, now time.Time) (domain.Team, error)
	ListTeams(ctx context.Context, userID int64) ([]domain.Team, error)
	TeamByID(ctx context.Context, id int64) (domain.Team, error)
	TeamMember(ctx context.Context, teamID, userID int64) (domain.Member, error)
	AddMember(ctx context.Context, member domain.Member) error
	UpdateMemberRole(ctx context.Context, teamID, userID int64, role string) error

	CreateTask(ctx context.Context, task domain.Task, changes domain.Changes) (domain.Task, error)
	ListTasks(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, error)
	TaskByID(ctx context.Context, id int64) (domain.Task, error)
	UpdateTask(ctx context.Context, id, actorID, expectedVersion int64, now time.Time, apply func(domain.Task) (domain.Task, domain.Changes, error)) (domain.Task, error)
	Comments(ctx context.Context, taskID int64) ([]domain.Comment, error)
	AddComment(ctx context.Context, comment domain.Comment) (domain.Comment, error)
	History(ctx context.Context, taskID int64) ([]domain.HistoryEntry, error)
	Stats(ctx context.Context, teamID int64, since time.Time) (domain.Stats, error)
}
