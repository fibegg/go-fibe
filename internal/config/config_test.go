package config

import "testing"

func TestFromEnvReadsCookiePartitioned(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/app")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("APP_COOKIE_PARTITIONED", "true")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if !cfg.CookiePartitioned {
		t.Fatal("expected CookiePartitioned to be true")
	}
}
