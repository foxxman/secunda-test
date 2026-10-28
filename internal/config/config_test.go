package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("MYSQL_DSN", "")
	t.Setenv("JWT_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing dsn")
	}
	t.Setenv("MYSQL_DSN", "user:pass@tcp(localhost:3306)/tasks")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing secret")
	}
	t.Setenv("JWT_SECRET", "short")
	if _, err := Load(); err == nil {
		t.Fatal("expected short secret")
	}
	t.Setenv("JWT_SECRET", "dev-secret-change-me")
	t.Setenv("JWT_TTL", "bad")
	if _, err := Load(); err == nil {
		t.Fatal("expected ttl error")
	}
	t.Setenv("JWT_TTL", "12h")
	t.Setenv("CACHE_TTL", "0s")
	if _, err := Load(); err == nil {
		t.Fatal("expected cache ttl error")
	}
	t.Setenv("CACHE_TTL", "5m")
	t.Setenv("HTTP_ADDR", ":9090")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":9090" || cfg.JWTTTL.Hours() != 12 || cfg.CacheTTL.Minutes() != 5 {
		t.Fatalf("cfg = %+v", cfg)
	}
}
