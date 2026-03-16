# Rendy CRM MVP

Demo operator center with persistent runtime:

- Go backend with REST + Redis-backed SSE
- React/TypeScript/Vite SPA
- PostgreSQL as source of truth
- Redis for sessions, events, and demo jobs

## Run With Docker Compose

For local `docker compose`, the runtime topology is:

- `web` serves the compiled frontend SPA through nginx
- `web` proxies `/api/*` to `api`
- `postgres` is used only by `api`
- `redis` is used only by `api`

Start everything:

```bash
docker compose up -d --build
```

Stop everything:

```bash
docker compose down
```

Main env knobs for compose:

- `API_PORT` — host port for backend, default `3000`
- `WEB_PORT` — host port for frontend nginx, default `8081`
- `NGINX_PORT` — container listen port for frontend nginx, default `8080`
- `API_BASE_URL` — browser-visible API base, default `/api`
- `API_UPSTREAM` — nginx upstream inside compose, default `http://api:3000`

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
