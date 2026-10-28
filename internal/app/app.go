package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"task-service/internal/auth"
	"task-service/internal/cache"
	"task-service/internal/config"
	"task-service/internal/httpserver"
	mysqlrepo "task-service/internal/repository/mysql"
	"task-service/internal/service"
)

func Run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	repo, err := mysqlrepo.Open(ctx, cfg.MySQLDSN)
	if err != nil {
		return err
	}
	defer func() { _ = repo.Close() }()

	lists := cache.Open(cfg.RedisAddr, cfg.RedisPass)
	defer func() { _ = lists.Close() }()
	if err := lists.Ping(ctx); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}

	tokens := auth.NewManager(cfg.JWTSecret, cfg.JWTTTL)
	authService := service.NewAuth(repo, tokens)
	teams := service.NewTeam(repo)
	tasks := service.NewTask(repo, lists, cfg.CacheTTL, logger)
	handler := httpserver.New(authService, teams, tasks, tokens, logger)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.HTTPAddr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		logger.Info("shutting down")
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
