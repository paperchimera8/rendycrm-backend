# Загрузка бэкенда на сервер (Ubuntu + Docker)

## 1. На сервере

```bash
# Установить Docker (если ещё нет)
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
# Перелогиниться или: newgrp docker

# Создать папку
mkdir -p ~/rendycrm && cd ~/rendycrm
```

## 2. Копирование файлов на сервер

**Вариант A — через rsync (с локальной машины):**
```bash
rsync -avz --exclude 'apps/web' --exclude 'node_modules' --exclude '.git' \
  /Users/vital/Documents/rendycrm-app/ user@SERVER_IP:~/rendycrm/
```

**Вариант B — через git (если репо на GitHub):**
```bash
# На сервере
git clone https://github.com/paperchimera8/rendycrm-backend.git ~/rendycrm
cd ~/rendycrm
```

**Вариант C — через scp:**
```bash
scp -r cmd internal migrations go.mod go.sum Dockerfile deploy user@SERVER_IP:~/rendycrm/
```

## 3. Запуск на сервере

```bash
cd ~/rendycrm

# Создать .env (опционально)
cat > .env << 'EOF'
POSTGRES_PASSWORD=strong_password_here
APP_ENCRYPTION_SECRET=random-32-chars-secret
PUBLIC_BASE_URL=https://your-domain.com
ENABLE_DEMO_SEED=true
EOF

# Запустить
cd deploy
docker compose -f docker-compose.prod.yml up -d

# Проверить
docker compose -f docker-compose.prod.yml ps
curl http://localhost:8080/health
```

## 4. Обновление

```bash
cd ~/rendycrm
# Подтянуть изменения (git) или скопировать файлы заново (rsync)
cd deploy
docker compose -f docker-compose.prod.yml build api
docker compose -f docker-compose.prod.yml up -d api
```

## Порты

- **8080** — API (пробросить в nginx/caddy или открыть в файрволе)
