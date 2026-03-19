FROM node:20-alpine AS web-builder
WORKDIR /app/apps/web
ARG VITE_APP_BASE_PATH=/
ARG VITE_API_BASE_URL=/api
ENV VITE_APP_BASE_PATH=${VITE_APP_BASE_PATH}
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}
COPY apps/web/package.json apps/web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY apps/web ./
RUN npm run build
RUN test -f dist/index.html
RUN test -d dist/assets
RUN test "$(find dist/assets -maxdepth 1 -type f | wc -l)" -gt 0
RUN grep -q 'assets/' dist/index.html

FROM golang:1.25-alpine AS api-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /api ./cmd/api

FROM alpine:3.19
RUN apk add --no-cache ca-certificates curl
COPY --from=api-builder /api /api
COPY migrations /migrations
COPY --from=web-builder /app/apps/web/dist /web
RUN test -f /web/index.html
RUN test -d /web/assets
RUN test "$(find /web/assets -maxdepth 1 -type f | wc -l)" -gt 0
EXPOSE 8080
ENV PORT=8080
ENV STATIC_DIR=/web
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/health || exit 1
ENTRYPOINT ["/api"]
