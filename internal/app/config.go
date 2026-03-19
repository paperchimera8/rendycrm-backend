package app

import (
	"bufio"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port                string
	StaticDir           string
	AppBasePath         string
	PostgresDSN         string
	RedisAddr           string
	RedisUsername       string
	RedisPassword       string
	RedisDB             int
	SessionTTL          time.Duration
	EventsChannel       string
	JobsQueue           string
	MigrationsPath      string
	PublicBaseURL       string
	BotEngineBaseURL    string
	OperatorBotUsername string
	TelegramAPIBaseURL  string
	EncryptionSecret    string
	CORSAllowedOrigins  []string
	EnableDemoSeed      bool
}

func LoadConfig() Config {
	loadDotEnv(".env")

	return Config{
		Port:                envOrDefault("PORT", "8080"),
		StaticDir:           envOrDefault("STATIC_DIR", ""),
		AppBasePath:         normalizeBasePath(os.Getenv("APP_BASE_PATH")),
		PostgresDSN:         postgresDSNFromEnv(),
		RedisAddr:           redisAddrFromEnv(),
		RedisUsername:       firstNonEmpty("REDIS_USERNAME", "REDIS_USER"),
		RedisPassword:       os.Getenv("REDIS_PASSWORD"),
		RedisDB:             envOrDefaultInt("REDIS_DB", 0),
		SessionTTL:          envOrDefaultDuration("SESSION_TTL", 24*time.Hour),
		EventsChannel:       envOrDefault("REDIS_EVENTS_CHANNEL", "rendycrm:events"),
		JobsQueue:           envOrDefault("REDIS_JOBS_QUEUE", "rendycrm:jobs"),
		MigrationsPath:      envOrDefault("MIGRATIONS_PATH", filepath.Join("migrations")),
		PublicBaseURL:       envOrDefault("PUBLIC_BASE_URL", "http://127.0.0.1:8080"),
		BotEngineBaseURL:    strings.TrimSpace(os.Getenv("BOT_ENGINE_BASE_URL")),
		OperatorBotUsername: envOrDefault("TELEGRAM_OPERATOR_BOT_USERNAME", "rendycrm_operator_bot"),
		TelegramAPIBaseURL:  envOrDefault("TELEGRAM_API_BASE_URL", "https://api.telegram.org"),
		EncryptionSecret:    strings.TrimSpace(envOrDefault("APP_ENCRYPTION_SECRET", "change-me-in-production")),
		CORSAllowedOrigins:  parseCSV(os.Getenv("CORS_ALLOWED_ORIGINS")),
		EnableDemoSeed:      envOrDefaultBool("ENABLE_DEMO_SEED", false),
	}
}

func postgresDSNFromEnv() string {
	if dsn := strings.TrimSpace(os.Getenv("POSTGRES_DSN")); dsn != "" {
		return dsn
	}

	if !hasAnyEnv("POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_DB", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_SSLMODE", "POSTGRES_SSLROOTCERT") {
		return "postgres://postgres:postgres@postgres:5432/rendycrm?sslmode=disable"
	}

	host := envOrDefault("POSTGRES_HOST", "postgres")
	port := envOrDefault("POSTGRES_PORT", "5432")
	database := envOrDefault("POSTGRES_DB", "rendycrm")
	user := envOrDefault("POSTGRES_USER", "postgres")
	password := envOrDefault("POSTGRES_PASSWORD", "postgres")
	sslMode := envOrDefault("POSTGRES_SSLMODE", "prefer")
	sslRootCert := strings.TrimSpace(os.Getenv("POSTGRES_SSLROOTCERT"))

	dsn := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + strings.TrimLeft(database, "/"),
	}
	query := dsn.Query()
	query.Set("sslmode", sslMode)
	if sslRootCert != "" {
		query.Set("sslrootcert", sslRootCert)
	}
	dsn.RawQuery = query.Encode()

	return dsn.String()
}

func redisAddrFromEnv() string {
	if addr := strings.TrimSpace(os.Getenv("REDIS_ADDR")); addr != "" {
		return addr
	}
	if !hasAnyEnv("REDIS_HOST", "REDIS_PORT") {
		return "redis:6379"
	}
	return net.JoinHostPort(
		envOrDefault("REDIS_HOST", "redis"),
		envOrDefault("REDIS_PORT", "6379"),
	)
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func firstNonEmpty(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func hasAnyEnv(keys ...string) bool {
	for _, key := range keys {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

func envOrDefaultInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func envOrDefaultDuration(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func envOrDefaultBool(key string, fallback bool) bool {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		switch strings.ToLower(value) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}

func parseCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			items = append(items, value)
		}
	}
	return items
}

func normalizeBasePath(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || value == "/" {
		return ""
	}
	return "/" + strings.Trim(value, "/")
}
