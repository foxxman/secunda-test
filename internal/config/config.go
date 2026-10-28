package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	HTTPAddr  string
	MySQLDSN  string
	RedisAddr string
	RedisPass string
	JWTSecret string
	JWTTTL    time.Duration
	CacheTTL  time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:  env("HTTP_ADDR", ":8080"),
		MySQLDSN:  os.Getenv("MYSQL_DSN"),
		RedisAddr: env("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPass: os.Getenv("REDIS_PASSWORD"),
		JWTSecret: os.Getenv("JWT_SECRET"),
		JWTTTL:    24 * time.Hour,
		CacheTTL:  5 * time.Minute,
	}
	if cfg.MySQLDSN == "" {
		return Config{}, fmt.Errorf("MYSQL_DSN is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}
	if len(cfg.JWTSecret) < 16 {
		return Config{}, fmt.Errorf("JWT_SECRET must be at least 16 characters")
	}
	var err error
	if raw := os.Getenv("JWT_TTL"); raw != "" {
		cfg.JWTTTL, err = time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("JWT_TTL: %w", err)
		}
	}
	if raw := os.Getenv("CACHE_TTL"); raw != "" {
		cfg.CacheTTL, err = time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("CACHE_TTL: %w", err)
		}
	}
	if cfg.JWTTTL <= 0 {
		return Config{}, fmt.Errorf("JWT_TTL must be positive")
	}
	if cfg.CacheTTL <= 0 {
		return Config{}, fmt.Errorf("CACHE_TTL must be positive")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
