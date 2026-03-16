# Landing + CRM On One Domain

This project can run behind a shared nginx where:

- `/` goes to the landing from `paperchimera8/rendycrm`
- `/app` goes to this CRM

For the CRM container, use:

```env
APP_BASE_PATH=/app
PUBLIC_BASE_URL=https://rendycrm.ru/app/api
```

When building the root Docker image, pass:

```bash
docker build \
  --build-arg VITE_APP_BASE_PATH=/app/ \
  --build-arg VITE_API_BASE_URL=/app/api \
  -t rendycrm-backend .
```

The shared nginx example is in:

- `deploy/nginx-landing-crm-app.conf.example`

The example already includes:

- `rendycrm.ru` as the main domain
- redirect from `www.rendycrm.ru` to `rendycrm.ru`

Why `PUBLIC_BASE_URL` includes `/app/api`:

- Telegram webhook URLs are formed from `PUBLIC_BASE_URL + /webhooks/...`
- when the app is mounted under `/app`, API traffic lives under `/app/api`
