API_PORT ?= 8080
INTEGRATION_TEST_DSN ?= postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable

.PHONY: infra infra-down api web test test-integration test-web test-e2e verify

infra:
	docker compose -f deploy/docker-compose.yml up -d

infra-down:
	docker compose -f deploy/docker-compose.yml down

api: infra
	@PIDS=$$(lsof -ti tcp:$(API_PORT) -sTCP:LISTEN || true); \
	if [ -n "$$PIDS" ]; then \
		echo "Stopping existing process on :$(API_PORT): $$PIDS"; \
		kill $$PIDS || true; \
		sleep 1; \
	fi
	GOCACHE=/tmp/go-build-cache PORT=$(API_PORT) go run ./cmd/api

web:
	cd apps/web && npm run dev -- --host 127.0.0.1

test:
	GOCACHE=/tmp/go-build-cache go test ./...

test-integration:
	TEST_POSTGRES_ADMIN_DSN=$(INTEGRATION_TEST_DSN) RUN_INTEGRATION_TESTS=1 GOCACHE=/tmp/go-build-cache go test ./internal/app

test-web:
	cd apps/web && npm run test

test-e2e:
	cd apps/web && npm run test:e2e

verify:
	GOCACHE=/tmp/go-build-cache go test ./...
	$(MAKE) test-integration
	cd apps/web && npm run build
	cd apps/web && npm run test
	cd apps/web && npm run test:e2e
