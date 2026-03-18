# Rendy CRM MVP

Demo operator center with persistent runtime:

- Go backend with REST + Redis-backed SSE
- React/TypeScript/Vite SPA
- PostgreSQL as source of truth
- Redis for sessions, events, and demo jobs

## GitHub Auto-Deploy

For services that deploy directly from GitHub, this repository should use the root [Dockerfile](/Users/vital/Documents/rendycrm-app/Dockerfile), not `docker-compose`.

The app container serves the backend and the compiled frontend from one image.
For that mode you need to provide external infrastructure through environment variables:

- `POSTGRES_DSN`, or component vars `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`
- `POSTGRES_SSLMODE` and optionally `POSTGRES_SSLROOTCERT` / `POSTGRES_SSLROOTCERT_URL`
- `REDIS_ADDR`, or component vars `REDIS_HOST`, `REDIS_PORT`, `REDIS_USERNAME`, `REDIS_PASSWORD`
- `APP_ENCRYPTION_SECRET`
- `PUBLIC_BASE_URL`

## Run With Docker Compose

For VPS or local `docker compose`, use [deploy/docker-compose.vps.yml](/Users/vital/Documents/rendycrm-app/deploy/docker-compose.vps.yml).

The runtime topology is:

- `web` serves the compiled frontend SPA through nginx
- `web` proxies `/api/*` to `api`
- `api` connects to an external PostgreSQL instance
- `api` connects to an external Redis instance

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
- `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`
- `POSTGRES_SSLMODE` — `prefer`, `require`, or `verify-full`
- `POSTGRES_SSLROOTCERT_URL` — optional URL for downloading the CA cert inside the container before startup
- `REDIS_HOST`, `REDIS_PORT`, `REDIS_USERNAME`, `REDIS_PASSWORD`, `REDIS_DB`

## Seed credentials

- email: `operator@rendycrm.local`
- password: `password`

## Main paths

- Frontend: `http://localhost:8081`
- API health through frontend proxy: `GET http://localhost:8081/api/health`
- Direct API health: `GET http://localhost:3000/health`
- Login page: `http://localhost:8081/login`

## Compose wiring

- `api` connects to PostgreSQL via external host credentials from env
- `api` connects to Redis via external host credentials from env
- `web` listens on its own port and reverse-proxies `/api/*` to `api`
- the browser never talks to PostgreSQL or Redis directly

## External service checks

Managed Redis with ACL user:

```bash
redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" --user "$REDIS_USERNAME" --pass '***' ping
```

Managed PostgreSQL with custom CA:

```bash
export POSTGRES_SSLROOTCERT_URL=https://st.timeweb.com/cloud-static/ca.crt
docker compose -f deploy/docker-compose.vps.yml up -d --build
```

## Notes

- Backend runs migrations on startup. Demo seed is opt-in through `ENABLE_DEMO_SEED=true`.
- One-time cleanup for existing demo content: `go run ./cmd/api cleanup-demo-data`
- Sessions are stored in Redis with TTL; SSE events are delivered through Redis pub/sub.
- Demo workers use a Redis-backed queue for `analytics.refresh`, `notification.send`, and `delivery.retry`.
- `POST /webhooks/telegram` and `POST /webhooks/whatsapp` simulate inbound leads and persist them in PostgreSQL.
- Restarting the backend no longer loses dialogs, bookings, settings, or reviews.
