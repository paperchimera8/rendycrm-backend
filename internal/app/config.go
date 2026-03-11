package app

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port                string
	StaticDir           string
	PostgresDSN         string
	RedisAddr           string
	RedisPassword       string
	RedisDB             int
	SessionTTL          time.Duration
	EventsChannel       string
	JobsQueue           string
	MigrationsPath      string
	PublicBaseURL       string
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
		PostgresDSN:         envOrDefault("POSTGRES_DSN", "postgres://postgres:postgres@127.0.0.1:55432/rendycrm?sslmode=disable"),
		RedisAddr:           envOrDefault("REDIS_ADDR", "127.0.0.1:56379"),
		RedisPassword:       os.Getenv("REDIS_PASSWORD"),
		RedisDB:             envOrDefaultInt("REDIS_DB", 0),
		SessionTTL:          envOrDefaultDuration("SESSION_TTL", 24*time.Hour),
		EventsChannel:       envOrDefault("REDIS_EVENTS_CHANNEL", "rendycrm:events"),
		JobsQueue:           envOrDefault("REDIS_JOBS_QUEUE", "rendycrm:jobs"),
		MigrationsPath:      envOrDefault("MIGRATIONS_PATH", filepath.Join("migrations")),
		PublicBaseURL:       envOrDefault("PUBLIC_BASE_URL", "http://127.0.0.1:8080"),
		OperatorBotUsername: envOrDefault("TELEGRAM_OPERATOR_BOT_USERNAME", "rendycrm_operator_bot"),
		TelegramAPIBaseURL:  envOrDefault("TELEGRAM_API_BASE_URL", "https://api.telegram.org"),
		EncryptionSecret:    strings.TrimSpace(os.Getenv("APP_ENCRYPTION_SECRET")),
		CORSAllowedOrigins:  parseCSV(os.Getenv("CORS_ALLOWED_ORIGINS")),
		EnableDemoSeed:      envOrDefaultBool("ENABLE_DEMO_SEED", false),
	}
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
