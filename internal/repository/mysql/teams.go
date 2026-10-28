package mysql

import (
	"context"
	"time"

	"task-service/internal/domain"
)

func (r *Repository) CreateTeam(ctx context.Context, name string, ownerID int64, now time.Time) (domain.Team, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Team{}, err
	}
	defer func() { _ = tx.Rollback() }()

	now = now.UTC()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO teams (name, created_by, created_at) VALUES (?, ?, ?)`,
		name, ownerID, now)
	if err != nil {
		return domain.Team{}, mapDBErr(err)
	}
	teamID, err := result.LastInsertId()
	if err != nil {
		return domain.Team{}, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO team_members (team_id, user_id, role, created_at) VALUES (?, ?, ?, ?)`,
		teamID, ownerID, domain.RoleOwner, now)
	if err != nil {
		return domain.Team{}, mapDBErr(err)
	}
	if err := tx.Commit(); err != nil {
		return domain.Team{}, err
	}
	return domain.Team{
		ID:        teamID,
		Name:      name,
		CreatedBy: ownerID,
		Role:      domain.RoleOwner,
		CreatedAt: now,
	}, nil
}

func (r *Repository) ListTeams(ctx context.Context, userID int64) ([]domain.Team, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, t.name, t.created_by, COALESCE(m.role, ?), t.created_at
		FROM teams t
		LEFT JOIN team_members m ON m.team_id = t.id AND m.user_id = ?
		WHERE m.user_id IS NOT NULL OR t.created_by = ?
		ORDER BY t.id`,
		domain.RoleOwner, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	teams := make([]domain.Team, 0)
	for rows.Next() {
		var team domain.Team
		if err := rows.Scan(&team.ID, &team.Name, &team.CreatedBy, &team.Role, &team.CreatedAt); err != nil {
			return nil, err
		}
		team.CreatedAt = team.CreatedAt.UTC()
		teams = append(teams, team)
	}
	return teams, rows.Err()
}

func (r *Repository) TeamByID(ctx context.Context, id int64) (domain.Team, error) {
	var team domain.Team
	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, created_by, created_at FROM teams WHERE id = ?`, id).
		Scan(&team.ID, &team.Name, &team.CreatedBy, &team.CreatedAt)
	if err != nil {
		return domain.Team{}, mapDBErr(err)
	}
	team.CreatedAt = team.CreatedAt.UTC()
	return team, nil
}

func (r *Repository) TeamMember(ctx context.Context, teamID, userID int64) (domain.Member, error) {
	var member domain.Member
	err := r.db.QueryRowContext(ctx, `
		SELECT team_id, user_id, role, created_at
		FROM team_members
		WHERE team_id = ? AND user_id = ?`, teamID, userID).
		Scan(&member.TeamID, &member.UserID, &member.Role, &member.CreatedAt)
	if err != nil {
		return domain.Member{}, mapDBErr(err)
	}
	member.CreatedAt = member.CreatedAt.UTC()
	return member, nil
}

func (r *Repository) AddMember(ctx context.Context, member domain.Member) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO team_members (team_id, user_id, role, created_at) VALUES (?, ?, ?, ?)`,
		member.TeamID, member.UserID, member.Role, member.CreatedAt.UTC())
	return mapDBErr(err)
}

func (r *Repository) UpdateMemberRole(ctx context.Context, teamID, userID int64, role string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE team_members SET role = ? WHERE team_id = ? AND user_id = ? AND role <> ?`,
		role, teamID, userID, domain.RoleOwner)
	if err != nil {
		return mapDBErr(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
