# Rendy CRM MVP

Demo operator center with persistent runtime:

- Go backend with REST + Redis-backed SSE
- React/TypeScript/Vite SPA
- PostgreSQL as source of truth
- Redis for sessions, events, and demo jobs

## Run

Infra:

```bash
make infra
```

Backend:

```bash
make api
```

Frontend:

```bash
cd apps/web
npm install
npm run dev
```

## Seed credentials

- email: `operator@rendycrm.local`
- password: `password`

## Main paths

- API health: `GET /health`
- Frontend: `http://localhost:5173`
- Login: `/login`
- Demo PostgreSQL: `127.0.0.1:55432`
- Demo Redis: `127.0.0.1:56379`

## Notes

- Backend runs migrations and idempotent demo seed data on startup.
- Sessions are stored in Redis with TTL; SSE events are delivered through Redis pub/sub.
- Demo workers use a Redis-backed queue for `analytics.refresh`, `notification.send`, and `delivery.retry`.
- `POST /webhooks/telegram` and `POST /webhooks/whatsapp` simulate inbound leads and persist them in PostgreSQL.
- Restarting the backend no longer loses dialogs, bookings, settings, or reviews.
