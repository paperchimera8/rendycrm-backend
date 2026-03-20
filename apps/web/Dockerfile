FROM node:20-alpine AS builder
WORKDIR /app

ARG VITE_APP_BASE_PATH=/
ARG VITE_API_BASE_URL=/api
ENV VITE_APP_BASE_PATH=${VITE_APP_BASE_PATH}
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}

COPY apps/web/package.json apps/web/package-lock.json ./
RUN npm ci

COPY apps/web ./
RUN npm run build

FROM nginx:1.27-alpine
ENV NGINX_PORT=8080
ENV APP_BASE_PATH=/
ENV API_BASE_URL=/api
ENV API_UPSTREAM=http://api:3000

COPY apps/web/nginx.conf /etc/nginx/nginx.conf
COPY apps/web/nginx.conf.template /etc/nginx/templates/default.conf.template
COPY apps/web/docker-entrypoint.d/40-runtime-config.sh /docker-entrypoint.d/40-runtime-config.sh
RUN chmod +x /docker-entrypoint.d/40-runtime-config.sh
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 8080

CMD ["nginx", "-g", "daemon off;"]
