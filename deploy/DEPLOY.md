# Загрузка приложения на сервер с внешними PostgreSQL и Redis

## 1. На сервере

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
mkdir -p ~/rendycrm && cd ~/rendycrm
```

## 2. Копирование файлов

```bash
git clone https://github.com/paperchimera8/rendycrm-backend.git ~/rendycrm
cd ~/rendycrm
```

## 3. Переменные окружения

Создайте `.env` в корне проекта и укажите внешние сервисы.

```bash
cat > .env << 'EOF'
API_PORT=3000
WEB_PORT=8081
NGINX_PORT=8080
API_BASE_URL=/api
API_UPSTREAM=http://api:3000
PUBLIC_BASE_URL=https://your-domain.com/api

APP_ENCRYPTION_SECRET=change-me-to-a-long-random-secret
ENABLE_DEMO_SEED=false
BOT_RUNTIME_INTERNAL_TOKEN=change-me-to-a-long-random-secret

POSTGRES_HOST=your-postgres-host
POSTGRES_PORT=5432
POSTGRES_DB=default_db
POSTGRES_USER=gen_user
POSTGRES_PASSWORD=change-me
POSTGRES_SSLMODE=verify-full
POSTGRES_SSLROOTCERT=/run/certs/postgres-root.crt
POSTGRES_SSLROOTCERT_URL=https://st.timeweb.com/cloud-static/ca.crt

REDIS_HOST=your-redis-host
REDIS_PORT=6379
REDIS_USERNAME=default
REDIS_PASSWORD=change-me
REDIS_DB=0
EOF
```

## 4. Запуск

```bash
docker compose -f deploy/docker-compose.vps.yml up -d --build
docker compose -f deploy/docker-compose.vps.yml ps
curl http://localhost:8081/api/health
```

## 5. Проверка внешних сервисов

```bash
redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" --user "$REDIS_USERNAME" --pass '***' ping

docker run --rm postgres:16 \
  psql "postgresql://$POSTGRES_USER:$POSTGRES_PASSWORD@$POSTGRES_HOST:$POSTGRES_PORT/$POSTGRES_DB?sslmode=$POSTGRES_SSLMODE" \
  -c 'select 1;'
```

## 6. Обновление

```bash
git pull
docker compose -f deploy/docker-compose.vps.yml up -d --build
```

## Порты

- `8081` — фронтенд (nginx)
- `3000` — backend API

## Важно

- `bot-ts` поднимается в том же compose-стеке и обязателен для Telegram client/operator bot.
- Если обновили код бота, но не пересобрали весь compose-стек, Telegram-кнопки и логика меню останутся старыми.
