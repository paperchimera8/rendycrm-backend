package app

import "testing"

func TestPostgresDSNFromComponentsEncodesCredentialsAndTLS(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "")
	t.Setenv("POSTGRES_HOST", "e2c27be93d6abd03536c6f96.twc1.net")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_DB", "default_db")
	t.Setenv("POSTGRES_USER", "gen_user")
	t.Setenv("POSTGRES_PASSWORD", "K-bq?8WLoZY;GR")
	t.Setenv("POSTGRES_SSLMODE", "verify-full")
	t.Setenv("POSTGRES_SSLROOTCERT", "/run/certs/postgres-root.crt")

	cfg := LoadConfig()

	want := "postgres://gen_user:K-bq%3F8WLoZY;GR@e2c27be93d6abd03536c6f96.twc1.net:5432/default_db?sslmode=verify-full&sslrootcert=%2Frun%2Fcerts%2Fpostgres-root.crt"
	if cfg.PostgresDSN != want {
		t.Fatalf("unexpected postgres dsn: got %q want %q", cfg.PostgresDSN, want)
	}
}

func TestRedisConfigFromComponentsIncludesUsername(t *testing.T) {
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("REDIS_HOST", "5.42.125.34")
	t.Setenv("REDIS_PORT", "6379")
	t.Setenv("REDIS_USERNAME", "default")
	t.Setenv("REDIS_PASSWORD", "secret")

	cfg := LoadConfig()

	if cfg.RedisAddr != "5.42.125.34:6379" {
		t.Fatalf("unexpected redis addr: %q", cfg.RedisAddr)
	}
	if cfg.RedisUsername != "default" {
		t.Fatalf("unexpected redis username: %q", cfg.RedisUsername)
	}
	if cfg.RedisPassword != "secret" {
		t.Fatalf("unexpected redis password: %q", cfg.RedisPassword)
	}
}
