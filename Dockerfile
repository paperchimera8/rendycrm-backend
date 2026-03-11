FROM node:20-alpine AS web-builder
WORKDIR /app/apps/web
COPY apps/web/package.json apps/web/package-lock.json ./
RUN npm ci
COPY apps/web ./
RUN npm run build

FROM golang:1.25-alpine AS api-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /api ./cmd/api

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=api-builder /api /api
COPY migrations /migrations
COPY --from=web-builder /app/apps/web/dist /web
EXPOSE 3000
ENV STATIC_DIR=/web
ENTRYPOINT ["/api"]
