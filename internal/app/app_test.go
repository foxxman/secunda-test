package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func TestRunRequiresConfig(t *testing.T) {
	t.Setenv("MYSQL_DSN", "")
	t.Setenv("JWT_SECRET", "")
	err := Run(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("expected config error")
	}
}
