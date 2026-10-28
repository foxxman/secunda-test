package mysql

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"task-service/internal/domain"

	mysqldriver "github.com/go-sql-driver/mysql"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Repository struct {
	db *sql.DB
}

func Open(ctx context.Context, dsn string) (*Repository, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var last error
	for {
		last = db.PingContext(pingCtx)
		if last == nil {
			break
		}
		if pingCtx.Err() != nil {
			_ = db.Close()
			return nil, fmt.Errorf("mysql ping: %w", last)
		}
		select {
		case <-pingCtx.Done():
			_ = db.Close()
			return nil, fmt.Errorf("mysql ping: %w", last)
		case <-time.After(time.Second):
		}
	}
	repo := &Repository{db: db}
	if err := repo.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return repo, nil
}

func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Close() error {
	return r.db.Close()
}

func (r *Repository) migrate(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(64) NOT NULL PRIMARY KEY,
			applied_at DATETIME(6) NOT NULL
		) ENGINE=InnoDB`)
	if err != nil {
		return fmt.Errorf("schema_migrations: %w", err)
	}
	const version = "001_init"
	var found string
	err = r.db.QueryRowContext(ctx, `SELECT version FROM schema_migrations WHERE version = ?`, version).Scan(&found)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	body, err := migrationFiles.ReadFile("migrations/001_init.sql")
	if err != nil {
		return err
	}
	for _, statement := range splitSQL(string(body)) {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migration %s: %w", version, err)
		}
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, UTC_TIMESTAMP(6))`, version)
	return err
}

func splitSQL(body string) []string {
	parts := strings.Split(body, ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		statement := strings.TrimSpace(part)
		if statement != "" {
			statements = append(statements, statement)
		}
	}
	return statements
}

func mapDBErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	var mysqlErr *mysqldriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return domain.ErrConflict
	}
	return err
}
