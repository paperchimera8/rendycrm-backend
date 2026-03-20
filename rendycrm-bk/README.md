# rendycrm-bk

TypeScript runtime для Telegram bot ingress.

Сервис может работать в двух режимах:

- standalone: отвечает в Telegram напрямую через Bot API
- proxy mode: быстро подтверждает webhook, дедуплицирует `update_id` и форвардит update во внутренний Go bot runtime

## Endpoints

- `GET /health`
- `POST /auth/login`
- `GET /auth/me` (Bearer token)
- `POST /webhooks/telegram/client/:workspace/:secret`
- `POST /webhooks/telegram/operator`

## Telegram

### Proxy mode

Если заданы:

- `GO_API_BASE_URL`
- `BOT_RUNTIME_INTERNAL_SECRET`

то сервис работает как TS webhook gateway для основного CRM:

- принимает Telegram webhook
- сразу отвечает `200 OK`
- дедуплицирует повторные update
- отправляет update в Go API на `/internal/bot-runtime/...`

В этом режиме Telegram bot token не нужны самому `rendycrm-bk`, потому что отправкой сообщений продолжает управлять Go CRM.

### Standalone mode

Чтобы бот действительно отвечал, нужно задать токены:

- `TELEGRAM_CLIENT_BOT_TOKEN`
- `TELEGRAM_OPERATOR_BOT_TOKEN`

Client bot отвечает на:

- `/start`
- текстовые сообщения
- inline callback'и (`client:book`, `client:slots`, `client:prices`, `client:address`, `client:human`)

Operator bot отвечает на:

- `/start`
- `/dashboard`
- `/dialogs`
- `/slots`
- `/settings`
- inline callback'и с теми же командами

## Local run

```bash
cp .env.example .env
npm install
npm run dev
```

## Docker

```bash
docker build -t rendycrm-bk .
docker run --rm -p 3000:3000 --env-file .env rendycrm-bk
```

## Docker Compose

```bash
docker compose up -d --build
```

## Tests

```bash
npm test
```

По умолчанию:

- `PUBLIC_PORT=3001`
- `PORT=3000`

Основные переменные:

- `AUTH_SECRET`
- `ADMIN_EMAIL`
- `ADMIN_PASSWORD`
- `CORS_ALLOWED_ORIGINS`
- `CORS_ALLOW_CREDENTIALS`
- `TELEGRAM_CLIENT_WEBHOOK_SECRET`
- `TELEGRAM_OPERATOR_WEBHOOK_SECRET`
- `TELEGRAM_CLIENT_BOT_TOKEN`
- `TELEGRAM_OPERATOR_BOT_TOKEN`
- `GO_API_BASE_URL`
- `BOT_RUNTIME_INTERNAL_SECRET`

## Production Notes

- Не коммитьте реальные bot token в git. Держите их в `.env`, secrets manager или в переменных окружения на платформе деплоя.
- Для production обязательно замените `AUTH_SECRET`, `TELEGRAM_CLIENT_WEBHOOK_SECRET` и `TELEGRAM_OPERATOR_WEBHOOK_SECRET`.
- Контейнер теперь публикует `healthcheck` на `GET /health`.

## CORS

- `CORS_ALLOWED_ORIGINS=*` — разрешить все origin.
- Поддерживаются маски вида `https://*.twc1.net`.
- Для cookie-сценария включите `CORS_ALLOW_CREDENTIALS=true` и задайте точные origin.
