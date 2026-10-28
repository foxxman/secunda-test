package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"task-service/internal/app"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, logger); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
