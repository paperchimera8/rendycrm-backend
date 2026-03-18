package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	tgapi "github.com/vital/rendycrm-app/internal/telegram"
)

const demoWorkspaceID = "ws_demo"

type SessionStore interface {
	Create(ctx context.Context, session Session) error
	Get(ctx context.Context, token string) (Session, error)
	Delete(ctx context.Context, token string) error
}

type EventBus interface {
	Publish(ctx context.Context, event SSEEvent) error
	Subscribe(ctx context.Context) *redis.PubSub
}

type JobQueue interface {
	Enqueue(ctx context.Context, kind string, payload any) error
	Consume(ctx context.Context) (*QueuedJob, error)
}

type QueuedJob struct {
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

type Runtime struct {
	cfg        Config
	db         *sql.DB
	redis      *redis.Client
	sessions   SessionStore
	events     EventBus
	jobs       JobQueue
	repository *Repository
	services   ApplicationServices
	telegram   *tgapi.APIClient
}

type RedisSessionStore struct {
	client *redis.Client
	ttl    time.Duration
}

type RedisEventBus struct {
	client  *redis.Client
	channel string
}

type RedisJobQueue struct {
	client *redis.Client
	key    string
}

func NewRuntime(ctx context.Context, cfg Config) (*Runtime, error) {
	if strings.TrimSpace(cfg.EncryptionSecret) == "" {
		return nil, errors.New("APP_ENCRYPTION_SECRET must be configured")
	}
	db, err := sql.Open("pgx", cfg.PostgresDSN)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := pingWithRetry(ctx, 15, 2*time.Second, db.PingContext); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Username: cfg.RedisUsername,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	if err := pingWithRetry(ctx, 15, 2*time.Second, func(ctx context.Context) error {
		return rdb.Ping(ctx).Err()
	}); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	runtime := &Runtime{
		cfg:      cfg,
		db:       db,
		redis:    rdb,
		sessions: &RedisSessionStore{client: rdb, ttl: cfg.SessionTTL},
		events:   &RedisEventBus{client: rdb, channel: cfg.EventsChannel},
		jobs:     &RedisJobQueue{client: rdb, key: cfg.JobsQueue},
		telegram: tgapi.NewAPIClient(cfg.TelegramAPIBaseURL),
	}
	runtime.repository = NewRepository(db)
	runtime.services = newApplicationServices(runtime.repository)

	if err := runtime.bootstrap(ctx); err != nil {
		return nil, err
	}

	return runtime, nil
}

func (r *Runtime) Close() error {
	var errs []string
	if r.db != nil {
		if err := r.db.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if r.redis != nil {
		if err := r.redis.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (r *Runtime) bootstrap(ctx context.Context) error {
	if err := runMigration(ctx, r.db, r.cfg.MigrationsPath); err != nil {
		return err
	}
	if r.cfg.EnableDemoSeed {
		if err := seedDemoData(ctx, r.db); err != nil {
			return err
		}
		if err := r.repository.EnsureSlotSystem(ctx, demoWorkspaceID); err != nil {
			return err
		}
	}
	return nil
}

func runMigration(ctx context.Context, db *sql.DB, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat migration path %s: %w", path, err)
	}
	if info.IsDir() {
		files, err := filepath.Glob(filepath.Join(path, "*.sql"))
		if err != nil {
			return fmt.Errorf("glob migrations %s: %w", path, err)
		}
		sort.Strings(files)
		for _, file := range files {
			if err := runSingleMigration(ctx, db, file); err != nil {
				return err
			}
		}
		return nil
	}
	return runSingleMigration(ctx, db, path)
}

func runSingleMigration(ctx context.Context, db *sql.DB, path string) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	version := filepathBase(path)
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists); err != nil {
		return fmt.Errorf("check migration version: %w", err)
	}
	if exists {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", path, err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, string(content)); err != nil {
		return fmt.Errorf("apply migration %s: %w", path, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return fmt.Errorf("mark migration %s: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", version, err)
	}
	return nil
}

func pingWithRetry(ctx context.Context, attempts int, delay time.Duration, ping func(context.Context) error) error {
	if attempts <= 1 {
		return ping(ctx)
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		if err := ping(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i == attempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

func seedDemoData(ctx context.Context, db *sql.DB) error {
	now := time.Now().UTC()
	passwordHash := hashToken("password")
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO workspaces (id, name, timezone, master_phone_raw, master_phone_normalized) VALUES ($1, $2, $3, $4, $5)
		  ON CONFLICT (id) DO UPDATE SET timezone = EXCLUDED.timezone, master_phone_raw = EXCLUDED.master_phone_raw, master_phone_normalized = EXCLUDED.master_phone_normalized`, []any{demoWorkspaceID, "Rendy CRM Demo", "Europe/Moscow", "+7 (999) 111-22-33", "79991112233"}},
		{`INSERT INTO workspaces (id, name, timezone) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING`, []any{"ws_system", "System Workspace", "UTC"}},
		{`INSERT INTO users (id, email, password_hash, name, status) VALUES ($1, $2, $3, $4, 'active') ON CONFLICT (id) DO NOTHING`, []any{"usr_1", "operator@rendycrm.local", passwordHash, "Main Operator"}},
		{`INSERT INTO workspace_members (id, workspace_id, user_id, role) VALUES ($1, $2, $3, $4) ON CONFLICT (workspace_id, user_id) DO NOTHING`, []any{"wsm_1", demoWorkspaceID, "usr_1", "admin"}},
		{`INSERT INTO customers (id, workspace_id, name, notes) VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO NOTHING`, []any{"cus_1", demoWorkspaceID, "Anna Petrova", "Prefers evening appointments"}},
		{`INSERT INTO customer_contacts (id, customer_id, type, value, is_primary) VALUES ($1, $2, 'phone', $3, true) ON CONFLICT (customer_id, type, value) DO NOTHING`, []any{"cct_1", "cus_1", "+79001234567"}},
		{`INSERT INTO customer_contacts (id, customer_id, type, value, is_primary) VALUES ($1, $2, 'email', $3, true) ON CONFLICT (customer_id, type, value) DO NOTHING`, []any{"cct_2", "cus_1", "anna@example.com"}},
		{`INSERT INTO customer_channel_identities (id, customer_id, workspace_id, provider, external_id, username) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`, []any{"cci_1", "cus_1", demoWorkspaceID, "telegram", "anna-petrova-demo", "anna"}},
		{`INSERT INTO customers (id, workspace_id, name, notes) VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO NOTHING`, []any{"cus_2", demoWorkspaceID, "Ivan Smirnov", "Prefers weekend slots"}},
		{`INSERT INTO customer_contacts (id, customer_id, type, value, is_primary) VALUES ($1, $2, 'phone', $3, true) ON CONFLICT (customer_id, type, value) DO NOTHING`, []any{"cct_3", "cus_2", "+79007654321"}},
		{`INSERT INTO customer_channel_identities (id, customer_id, workspace_id, provider, external_id, username) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`, []any{"cci_2", "cus_2", demoWorkspaceID, "whatsapp", "ivan-smirnov-demo", "ivan"}},
		{`INSERT INTO customers (id, workspace_id, name, notes) VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO NOTHING`, []any{"cus_3", demoWorkspaceID, "Elena Sidorova", "Likes morning appointments"}},
		{`INSERT INTO customer_contacts (id, customer_id, type, value, is_primary) VALUES ($1, $2, 'phone', $3, true) ON CONFLICT (customer_id, type, value) DO NOTHING`, []any{"cct_4", "cus_3", "+79004567890"}},
		{`INSERT INTO customer_channel_identities (id, customer_id, workspace_id, provider, external_id, username) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`, []any{"cci_3", "cus_3", demoWorkspaceID, "telegram", "elena-sidorova-demo", "elena"}},
		{`INSERT INTO customers (id, workspace_id, name, notes) VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO NOTHING`, []any{"cus_4", demoWorkspaceID, "Dmitry Kozlov", "Asks about pricing details"}},
		{`INSERT INTO customer_contacts (id, customer_id, type, value, is_primary) VALUES ($1, $2, 'phone', $3, true) ON CONFLICT (customer_id, type, value) DO NOTHING`, []any{"cct_5", "cus_4", "+79005432109"}},
		{`INSERT INTO customer_channel_identities (id, customer_id, workspace_id, provider, external_id, username) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`, []any{"cci_4", "cus_4", demoWorkspaceID, "whatsapp", "dmitry-kozlov-demo", "dmitry"}},
		{`INSERT INTO customers (id, workspace_id, name, notes) VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO NOTHING`, []any{"cus_5", demoWorkspaceID, "Olga Romanova", "Wants late evening slot"}},
		{`INSERT INTO customer_contacts (id, customer_id, type, value, is_primary) VALUES ($1, $2, 'phone', $3, true) ON CONFLICT (customer_id, type, value) DO NOTHING`, []any{"cct_6", "cus_5", "+79006543210"}},
		{`INSERT INTO customer_channel_identities (id, customer_id, workspace_id, provider, external_id, username) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`, []any{"cci_5", "cus_5", demoWorkspaceID, "telegram", "olga-romanova-demo", "olga"}},
		{`INSERT INTO channel_accounts (id, workspace_id, provider, channel_kind, account_scope, account_name, external_account_id, webhook_secret, connected, is_enabled, bot_username)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, true, true, $9)
		 ON CONFLICT DO NOTHING`, []any{"cha_global_tg", "ws_system", "telegram", "telegram_client", "global", "Telegram global client bot", "tg-global", "global-secret", "rendycrm_client_bot"}},
		{`INSERT INTO channel_accounts (id, workspace_id, provider, channel_kind, account_scope, account_name, external_account_id, webhook_secret, connected, is_enabled, bot_username)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, true, true, '')
		 ON CONFLICT DO NOTHING`, []any{"cha_1", demoWorkspaceID, "telegram", "telegram_client", "workspace", "Telegram salon", "tg-demo", "demo-secret"}},
		{`INSERT INTO channel_accounts (id, workspace_id, provider, channel_kind, account_scope, account_name, external_account_id, webhook_secret, connected, is_enabled, bot_username)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, false, false, '')
		 ON CONFLICT DO NOTHING`, []any{"cha_2", demoWorkspaceID, "whatsapp", "whatsapp_twilio", "workspace", "WhatsApp salon", "wa-demo", "demo-secret"}},
		{`INSERT INTO channel_accounts (id, workspace_id, provider, channel_kind, account_scope, account_name, external_account_id, webhook_secret, connected, is_enabled, bot_username)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, true, true, $9)
		 ON CONFLICT DO NOTHING`, []any{"cha_3", demoWorkspaceID, "telegram", "telegram_operator", "workspace", "Telegram operator bot", "tg-operator-demo", "operator-secret", "rendycrm_operator_bot"}},
		{`INSERT INTO conversations (id, workspace_id, customer_id, channel_account_id, provider, external_chat_id, status, assigned_user_id, unread_count, ai_summary, intent, last_message_text, last_inbound_at, last_outbound_at, updated_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, 'human', $7, 1, $8, 'reschedule', $9, $10, $11, $11, $11) ON CONFLICT (id) DO NOTHING`,
			[]any{"cnv_1", demoWorkspaceID, "cus_1", "cha_1", "telegram", "anna-petrova-demo", "usr_1", "Клиент хочет перенести запись на более позднее время вечером.", "Можно на пятницу после 18:00?", now.Add(-15 * time.Minute), now.Add(-10 * time.Minute)}},
		{`INSERT INTO messages (id, conversation_id, workspace_id, external_message_id, direction, sender_type, body, delivery_status, dedup_key, created_at) VALUES ($1, $2, $3, $4, 'inbound', 'customer', $5, 'received', $6, $7) ON CONFLICT (conversation_id, dedup_key) DO NOTHING`,
			[]any{"msg_1", "cnv_1", demoWorkspaceID, "tg-msg-1", "Можно на пятницу после 18:00?", "seed-msg-1", now.Add(-15 * time.Minute)}},
		{`INSERT INTO messages (id, conversation_id, workspace_id, external_message_id, direction, sender_type, body, delivery_status, dedup_key, created_at) VALUES ($1, $2, $3, $4, 'outbound', 'operator', $5, 'delivered', $6, $7) ON CONFLICT (conversation_id, dedup_key) DO NOTHING`,
			[]any{"msg_2", "cnv_1", demoWorkspaceID, "op-msg-1", "Проверяю свободные окна. Есть пятница 18:00–19:00, подойдёт?", "seed-msg-2", now.Add(-10 * time.Minute)}},
		{`UPDATE conversations SET last_message_text = 'Проверяю свободные окна. Есть пятница 18:00–19:00, подойдёт?', updated_at = $2 WHERE id = 'cnv_1' AND workspace_id = $1`, []any{demoWorkspaceID, now.Add(-10 * time.Minute)}},
		{`INSERT INTO conversations (id, workspace_id, customer_id, channel_account_id, provider, external_chat_id, status, assigned_user_id, unread_count, ai_summary, intent, last_message_text, last_inbound_at, last_outbound_at, updated_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, 'human', $7, 1, $8, 'availability_question', $9, $10, $11, $11, $11) ON CONFLICT (id) DO NOTHING`,
			[]any{"cnv_2", demoWorkspaceID, "cus_2", "cha_2", "whatsapp", "ivan-smirnov-demo", "usr_1", "Клиент спрашивает про доступные окна на выходных.", "Есть запись на субботу после обеда?", now.Add(-7 * time.Minute), now.Add(-5 * time.Minute)}},
		{`INSERT INTO messages (id, conversation_id, workspace_id, external_message_id, direction, sender_type, body, delivery_status, dedup_key, created_at) VALUES ($1, $2, $3, $4, 'inbound', 'customer', $5, 'received', $6, $7) ON CONFLICT (conversation_id, dedup_key) DO NOTHING`,
			[]any{"msg_3", "cnv_2", demoWorkspaceID, "wa-msg-1", "Есть запись на субботу после обеда?", "seed-msg-3", now.Add(-7 * time.Minute)}},
		{`INSERT INTO messages (id, conversation_id, workspace_id, external_message_id, direction, sender_type, body, delivery_status, dedup_key, created_at) VALUES ($1, $2, $3, $4, 'outbound', 'operator', $5, 'delivered', $6, $7) ON CONFLICT (conversation_id, dedup_key) DO NOTHING`,
			[]any{"msg_4", "cnv_2", demoWorkspaceID, "op-msg-2", "Да, есть окно в субботу в 15:00. Подтвердить запись?", "seed-msg-4", now.Add(-5 * time.Minute)}},
		{`UPDATE conversations SET last_message_text = 'Да, есть окно в субботу в 15:00. Подтвердить запись?', updated_at = $2 WHERE id = 'cnv_2' AND workspace_id = $1`, []any{demoWorkspaceID, now.Add(-5 * time.Minute)}},
		{`INSERT INTO conversations (id, workspace_id, customer_id, channel_account_id, provider, external_chat_id, status, assigned_user_id, unread_count, ai_summary, intent, last_message_text, last_inbound_at, last_outbound_at, updated_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, 'human', $7, 1, $8, 'booking_request', $9, $10, $11, $10, $10) ON CONFLICT (id) DO NOTHING`,
			[]any{"cnv_3", demoWorkspaceID, "cus_3", "cha_1", "telegram", "elena-sidorova-demo", "usr_1", "Клиент уточняет подтверждение утреннего окна.", "Подходит завтра в 12:00?", now.Add(-4 * time.Minute), now.Add(-9 * time.Minute)}},
		{`INSERT INTO messages (id, conversation_id, workspace_id, external_message_id, direction, sender_type, body, delivery_status, dedup_key, created_at) VALUES ($1, $2, $3, $4, 'outbound', 'operator', $5, 'delivered', $6, $7) ON CONFLICT (conversation_id, dedup_key) DO NOTHING`,
			[]any{"msg_5", "cnv_3", demoWorkspaceID, "op-msg-3", "Свободно завтра в 12:00. Подтверждаю?", "seed-msg-5", now.Add(-9 * time.Minute)}},
		{`INSERT INTO messages (id, conversation_id, workspace_id, external_message_id, direction, sender_type, body, delivery_status, dedup_key, created_at) VALUES ($1, $2, $3, $4, 'inbound', 'customer', $5, 'received', $6, $7) ON CONFLICT (conversation_id, dedup_key) DO NOTHING`,
			[]any{"msg_6", "cnv_3", demoWorkspaceID, "tg-msg-3", "Да, подходит завтра в 12:00. Запишите меня, пожалуйста.", "seed-msg-6", now.Add(-4 * time.Minute)}},
		{`INSERT INTO conversations (id, workspace_id, customer_id, channel_account_id, provider, external_chat_id, status, assigned_user_id, unread_count, ai_summary, intent, last_message_text, last_inbound_at, last_outbound_at, updated_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, 'human', $7, 2, $8, 'price_question', $9, $10, $11, $10, $10) ON CONFLICT (id) DO NOTHING`,
			[]any{"cnv_4", demoWorkspaceID, "cus_4", "cha_2", "whatsapp", "dmitry-kozlov-demo", "usr_1", "Клиент обсуждает цену и просит закрепить время.", "Сколько будет стоить с покрытием?", now.Add(-3 * time.Minute), now.Add(-8 * time.Minute)}},
		{`INSERT INTO messages (id, conversation_id, workspace_id, external_message_id, direction, sender_type, body, delivery_status, dedup_key, created_at) VALUES ($1, $2, $3, $4, 'outbound', 'operator', $5, 'delivered', $6, $7) ON CONFLICT (conversation_id, dedup_key) DO NOTHING`,
			[]any{"msg_7", "cnv_4", demoWorkspaceID, "op-msg-4", "Есть окно сегодня в 17:00, могу закрепить.", "seed-msg-7", now.Add(-8 * time.Minute)}},
		{`INSERT INTO messages (id, conversation_id, workspace_id, external_message_id, direction, sender_type, body, delivery_status, dedup_key, created_at) VALUES ($1, $2, $3, $4, 'inbound', 'customer', $5, 'received', $6, $7) ON CONFLICT (conversation_id, dedup_key) DO NOTHING`,
			[]any{"msg_8", "cnv_4", demoWorkspaceID, "wa-msg-4", "Сколько это будет стоить с покрытием?", "seed-msg-8", now.Add(-6 * time.Minute)}},
		{`INSERT INTO messages (id, conversation_id, workspace_id, external_message_id, direction, sender_type, body, delivery_status, dedup_key, created_at) VALUES ($1, $2, $3, $4, 'inbound', 'customer', $5, 'received', $6, $7) ON CONFLICT (conversation_id, dedup_key) DO NOTHING`,
			[]any{"msg_9", "cnv_4", demoWorkspaceID, "wa-msg-5", "И можно ли сделать в субботу после 16:00?", "seed-msg-9", now.Add(-3 * time.Minute)}},
		{`INSERT INTO conversations (id, workspace_id, customer_id, channel_account_id, provider, external_chat_id, status, assigned_user_id, unread_count, ai_summary, intent, last_message_text, last_inbound_at, last_outbound_at, updated_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, 'human', $7, 1, $8, 'availability_question', $9, $10, NULL, $10, $10) ON CONFLICT (id) DO NOTHING`,
			[]any{"cnv_5", demoWorkspaceID, "cus_5", "cha_1", "telegram", "olga-romanova-demo", "usr_1", "Клиент спрашивает про поздний вечерний слот.", "Есть ли окно после 20:00 на этой неделе?", now.Add(-2 * time.Minute)}},
		{`INSERT INTO messages (id, conversation_id, workspace_id, external_message_id, direction, sender_type, body, delivery_status, dedup_key, created_at) VALUES ($1, $2, $3, $4, 'inbound', 'customer', $5, 'received', $6, $7) ON CONFLICT (conversation_id, dedup_key) DO NOTHING`,
			[]any{"msg_10", "cnv_5", demoWorkspaceID, "tg-msg-4", "Есть ли окно после 20:00 на этой неделе?", "seed-msg-10", now.Add(-2 * time.Minute)}},
		{`INSERT INTO reviews (id, workspace_id, customer_id, booking_id, rating, body, status, created_at) VALUES ($1, $2, $3, $4, $5, $6, 'open', $7) ON CONFLICT (id) DO NOTHING`,
			[]any{"rev_1", demoWorkspaceID, "cus_1", nil, 4, "Все понравилось, но пришлось подождать 10 минут.", now.AddDate(0, 0, -1)}},
		{`INSERT INTO bot_configs (workspace_id, auto_reply, handoff_enabled, tone) VALUES ($1, true, true, 'helpful') ON CONFLICT (workspace_id) DO NOTHING`, []any{demoWorkspaceID}},
		{`INSERT INTO faq_items (id, workspace_id, question, answer) VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO NOTHING`, []any{"faq_1", demoWorkspaceID, "Какие есть окна вечером?", "Проверяю доступность и предлагаю ближайшие слоты."}},
		{`INSERT INTO faq_items (id, workspace_id, question, answer) VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO NOTHING`, []any{"faq_2", demoWorkspaceID, "Как отменить запись?", "Могу отменить запись и предложить новое время."}},
		{`INSERT INTO analytics_daily (id, workspace_id, bucket_date, revenue_cents, confirmation_rate, no_show_rate, repeat_bookings, conversation_to_booking) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (workspace_id, bucket_date) DO UPDATE SET revenue_cents = EXCLUDED.revenue_cents, confirmation_rate = EXCLUDED.confirmation_rate, no_show_rate = EXCLUDED.no_show_rate, repeat_bookings = EXCLUDED.repeat_bookings, conversation_to_booking = EXCLUDED.conversation_to_booking`,
			[]any{"anl_1", demoWorkspaceID, now.Format("2006-01-02"), 0, 0, 0, 0, 0}},
	}

	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return fmt.Errorf("seed statement failed: %w", err)
		}
	}
	cleanupStatements := []string{
		`UPDATE conversations c
		 SET last_message_text = lm.body,
		     updated_at = lm.created_at
		 FROM (
		     SELECT DISTINCT ON (conversation_id) conversation_id, body, created_at
		     FROM messages
		     WHERE workspace_id = $1
		     ORDER BY conversation_id, created_at DESC
		 ) lm
		 WHERE c.workspace_id = $1 AND c.id = lm.conversation_id`,
		`UPDATE conversations c
		 SET unread_count = COALESCE((
		     SELECT COUNT(*)
		     FROM messages mi
		     WHERE mi.workspace_id = c.workspace_id
		       AND mi.conversation_id = c.id
		       AND mi.direction = 'inbound'
		       AND mi.created_at > COALESCE((
		           SELECT MAX(mo.created_at)
		           FROM messages mo
		           WHERE mo.workspace_id = c.workspace_id
		             AND mo.conversation_id = c.id
		             AND mo.direction = 'outbound'
		       ), '-infinity'::timestamptz)
		 ), 0)
		 WHERE c.workspace_id = $1`,
		`UPDATE slot_settings SET timezone = (SELECT timezone FROM workspaces WHERE id = $1) WHERE workspace_id = $1`,
		`UPDATE availability_rules SET day_of_week = 0 WHERE workspace_id = $1 AND day_of_week = 7`,
		`DELETE FROM availability_rules WHERE workspace_id = $1 AND id IN ('avr_1', 'avr_2', 'avr_3', 'avr_4', 'avr_5')`,
		`DELETE FROM slot_holds WHERE workspace_id = $1 AND daily_slot_id IN (SELECT COALESCE(daily_slot_id, '') FROM bookings WHERE id = 'bok_1' AND workspace_id = $1)`,
		`DELETE FROM daily_slots WHERE workspace_id = $1 AND id IN (SELECT COALESCE(daily_slot_id, '') FROM bookings WHERE id = 'bok_1' AND workspace_id = $1)`,
		`DELETE FROM bookings WHERE id = 'bok_1' AND workspace_id = $1`,
		`DELETE FROM daily_slots WHERE workspace_id = $1 AND source_template_id IS NOT NULL AND is_manual = FALSE AND status IN ('free', 'blocked') AND NOT EXISTS (SELECT 1 FROM bookings b WHERE b.daily_slot_id = daily_slots.id AND b.status <> 'cancelled')`,
		`UPDATE daily_slots SET is_manual = TRUE, source_template_id = NULL WHERE workspace_id = $1 AND source_template_id IS NOT NULL`,
		`DELETE FROM slot_templates WHERE workspace_id = $1`,
	}
	for _, query := range cleanupStatements {
		if _, err := db.ExecContext(ctx, query, demoWorkspaceID); err != nil {
			return fmt.Errorf("seed cleanup failed: %w", err)
		}
	}
	return nil
}

func (s *RedisSessionStore) Create(ctx context.Context, session Session) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, s.key(session.Token), data, time.Until(session.ExpiresAt)).Err()
}

func (s *RedisSessionStore) Get(ctx context.Context, token string) (Session, error) {
	raw, err := s.client.Get(ctx, s.key(token)).Result()
	if err != nil {
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *RedisSessionStore) Delete(ctx context.Context, token string) error {
	return s.client.Del(ctx, s.key(token)).Err()
}

func (s *RedisSessionStore) key(token string) string {
	return "session:" + token
}

func (b *RedisEventBus) Publish(ctx context.Context, event SSEEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return b.client.Publish(ctx, b.channel, data).Err()
}

func (b *RedisEventBus) Subscribe(ctx context.Context) *redis.PubSub {
	return b.client.Subscribe(ctx, b.channel)
}

func (q *RedisJobQueue) Enqueue(ctx context.Context, kind string, payload any) error {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	job, err := json.Marshal(QueuedJob{Kind: kind, Payload: rawPayload})
	if err != nil {
		return err
	}
	return q.client.LPush(ctx, q.key, job).Err()
}

func (q *RedisJobQueue) Consume(ctx context.Context) (*QueuedJob, error) {
	values, err := q.client.BRPop(ctx, 5*time.Second, q.key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	if len(values) != 2 {
		return nil, nil
	}
	var job QueuedJob
	if err := json.Unmarshal([]byte(values[1]), &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func filepathBase(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
