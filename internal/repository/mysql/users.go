package mysql

import (
	"context"
	"database/sql"

	"task-service/internal/domain"
)

func (r *Repository) CreateUser(ctx context.Context, user domain.User) (domain.User, error) {
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO users (email, password_hash, name, created_at)
		VALUES (?, ?, ?, ?)`,
		user.Email, user.PasswordHash, user.Name, user.CreatedAt.UTC())
	if err != nil {
		return domain.User{}, mapDBErr(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return domain.User{}, err
	}
	user.ID = id
	return user, nil
}

func (r *Repository) UserByEmail(ctx context.Context, email string) (domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, userSelect+` WHERE email = ?`, email))
}

func (r *Repository) UserByID(ctx context.Context, id int64) (domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, userSelect+` WHERE id = ?`, id))
}

const userSelect = `SELECT id, email, password_hash, name, created_at FROM users`

func scanUser(row *sql.Row) (domain.User, error) {
	var user domain.User
	err := row.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.CreatedAt)
	if err != nil {
		return domain.User{}, mapDBErr(err)
	}
	user.CreatedAt = user.CreatedAt.UTC()
	return user, nil
}
