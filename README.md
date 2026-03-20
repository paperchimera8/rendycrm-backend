# Rendy CRM MVP

Demo operator center with persistent runtime:

- Go backend with REST + Redis-backed SSE
- TypeScript bot runtime for Telegram webhook ingress and deduplication
- React/TypeScript/Vite SPA
- PostgreSQL as source of truth
- Redis for sessions, events, and demo jobs

## GitHub Auto-Deploy

For services that deploy directly from GitHub, this repository should use the root [Dockerfile](/Users/vital/Documents/rendycrm-app/Dockerfile), not `docker-compose`.

The app container serves the backend and the compiled frontend from one image.
For that mode you need to provide external infrastructure through environment variables:

- `POSTGRES_DSN`
- `REDIS_ADDR`
- `APP_ENCRYPTION_SECRET`
- `PUBLIC_BASE_URL`

## Run With Docker Compose

For VPS or local `docker compose`, use [deploy/docker-compose.vps.yml](/Users/vital/Documents/rendycrm-app/deploy/docker-compose.vps.yml).

The runtime topology is:

- `web` serves the compiled frontend SPA through nginx
- `web` proxies `/api/*` to `api`
- `postgres` is used only by `api`
- `redis` is used only by `api`
- `bot-runtime` accepts Telegram webhooks and forwards validated updates to the Go API internal bot runtime

Start everything:

```bash
docker compose -f deploy/docker-compose.vps.yml up -d --build
```

Stop everything:

```bash
docker compose -f deploy/docker-compose.vps.yml down
```

Main env knobs for compose:

- `API_PORT` — host port for backend, default `3000`
- `WEB_PORT` — host port for frontend nginx, default `8081`
- `NGINX_PORT` — container listen port for frontend nginx, default `8080`
- `API_BASE_URL` — browser-visible API base, default `/api`
- `API_UPSTREAM` — nginx upstream inside compose, default `http://api:3000`
- `BOT_RUNTIME_BASE_URL` — Go API target for Telegram webhook proxying, default `http://bot-runtime:3100`
- `BOT_RUNTIME_INTERNAL_SECRET` — shared secret between Go API and TS bot runtime

## Seed credentials

- email: `operator@rendycrm.local`
- password: `password`

## Main paths

- Frontend: `http://localhost:8081`
- API health through frontend proxy: `GET http://localhost:8081/api/health`
- Direct API health: `GET http://localhost:3000/health`
- Login page: `http://localhost:8081/login`

## Compose wiring

- `api` connects to PostgreSQL via `postgres:5432`
- `api` connects to Redis via `redis:6379`
- `api` proxies Telegram webhook ingress to `bot-runtime` when `BOT_RUNTIME_BASE_URL` is configured
- `bot-runtime` forwards deduplicated Telegram updates to `api` internal bot-runtime endpoints
- `web` listens on its own port and reverse-proxies `/api/*` to `api`
- the browser never talks to PostgreSQL or Redis directly
- PostgreSQL data is stored in the `postgres_data` volume
- Redis data is stored in the `redis_data` volume

## Notes

- Backend runs migrations on startup. Demo seed is opt-in through `ENABLE_DEMO_SEED=true`.
- One-time cleanup for existing demo content: `go run ./cmd/api cleanup-demo-data`
- Sessions are stored in Redis with TTL; SSE events are delivered through Redis pub/sub.
- Demo workers use a Redis-backed queue for `analytics.refresh`, `notification.send`, and `delivery.retry`.
- `POST /webhooks/telegram` and `POST /webhooks/whatsapp` simulate inbound leads and persist them in PostgreSQL.
- Restarting the backend no longer loses dialogs, bookings, settings, or reviews.
