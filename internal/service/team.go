package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"task-service/internal/domain"
	"task-service/internal/repository"
)

type Team struct {
	store repository.Store
	now   func() time.Time
}

func NewTeam(store repository.Store) *Team {
	return &Team{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func (t *Team) Create(ctx context.Context, actorID int64, name string) (domain.Team, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Team{}, domain.Invalid("название команды обязательно")
	}
	if len(name) > 255 {
		return domain.Team{}, domain.Invalid("название команды слишком длинное")
	}
	if _, err := t.store.UserByID(ctx, actorID); err != nil {
		return domain.Team{}, err
	}
	return t.store.CreateTeam(ctx, name, actorID, t.now())
}

func (t *Team) List(ctx context.Context, actorID int64) ([]domain.Team, error) {
	return t.store.ListTeams(ctx, actorID)
}

type Invite struct {
	UserID *int64
	Email  string
	Role   string
}

func (t *Team) Invite(ctx context.Context, actorID, teamID int64, invite Invite) (domain.Member, error) {
	if !domain.AssignableRole(invite.Role) {
		return domain.Member{}, domain.Invalid("приглашением можно выдать только роль admin или member")
	}
	team, err := t.store.TeamByID(ctx, teamID)
	if err != nil {
		return domain.Member{}, err
	}
	actor, err := t.requireMember(ctx, teamID, actorID)
	if err != nil {
		return domain.Member{}, err
	}
	if actor.Role != domain.RoleOwner && actor.Role != domain.RoleAdmin {
		return domain.Member{}, domain.ErrForbidden
	}
	user, err := t.findInvitee(ctx, invite)
	if err != nil {
		return domain.Member{}, err
	}
	if user.ID == team.CreatedBy {
		return domain.Member{}, domain.ErrForbidden
	}

	existing, err := t.store.TeamMember(ctx, teamID, user.ID)
	if err == nil {
		if existing.Role == invite.Role {
			return existing, nil
		}
		if actor.Role != domain.RoleOwner {
			return domain.Member{}, domain.ErrConflict
		}
		if err := t.store.UpdateMemberRole(ctx, teamID, user.ID, invite.Role); err != nil {
			return domain.Member{}, err
		}
		existing.Role = invite.Role
		return existing, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.Member{}, err
	}

	member := domain.Member{
		TeamID:    teamID,
		UserID:    user.ID,
		Role:      invite.Role,
		CreatedAt: t.now(),
	}
	if err := t.store.AddMember(ctx, member); err != nil {
		return domain.Member{}, err
	}
	return member, nil
}

func (t *Team) Stats(ctx context.Context, actorID, teamID int64) (domain.Stats, error) {
	if _, err := t.store.TeamByID(ctx, teamID); err != nil {
		return domain.Stats{}, err
	}
	actor, err := t.requireMember(ctx, teamID, actorID)
	if err != nil {
		return domain.Stats{}, err
	}
	if actor.Role != domain.RoleOwner && actor.Role != domain.RoleAdmin {
		return domain.Stats{}, domain.ErrForbidden
	}
	since := t.now().Add(-30 * 24 * time.Hour)
	return t.store.Stats(ctx, teamID, since)
}

func (t *Team) findInvitee(ctx context.Context, invite Invite) (domain.User, error) {
	email := strings.TrimSpace(invite.Email)
	if invite.UserID == nil && email == "" {
		return domain.User{}, domain.Invalid("укажите user_id или email")
	}
	if invite.UserID != nil {
		if *invite.UserID <= 0 {
			return domain.User{}, domain.Invalid("некорректный user_id")
		}
		user, err := t.store.UserByID(ctx, *invite.UserID)
		if err != nil {
			return domain.User{}, err
		}
		if email != "" {
			normalized, err := normalizeEmail(email)
			if err != nil {
				return domain.User{}, err
			}
			if user.Email != normalized {
				return domain.User{}, domain.Invalid("user_id и email указывают на разных пользователей")
			}
		}
		return user, nil
	}
	normalized, err := normalizeEmail(email)
	if err != nil {
		return domain.User{}, err
	}
	return t.store.UserByEmail(ctx, normalized)
}

func (t *Team) requireMember(ctx context.Context, teamID, userID int64) (domain.Member, error) {
	member, err := t.store.TeamMember(ctx, teamID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Member{}, domain.ErrForbidden
		}
		return domain.Member{}, err
	}
	return member, nil
}
