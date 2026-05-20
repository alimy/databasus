.PHONY: run build test lint wal-generator e2e e2e-clean e2e-backup-restore e2e-backup-restore-clean e2e-backup-restore-docker e2e-backup-restore-docker-matrix

-include .env
export

run:
	go run cmd/main.go start \
		--databasus-host http://localhost:4005 \
		--db-id $(AGENT_DB_ID) \
		--token $(AGENT_TOKEN) \
		--pg-host 127.0.0.1 \
		--pg-port 7433 \
		--pg-user devuser \
		--pg-password devpassword \
		--pg-type docker \
		--pg-docker-container-name agent-db \
		--pg-wal-dir ../wal-queue \
		--skip-update

# We are using it to make PostgreSQL produce WAL segments
# so out development agent can capture it and we can develop
wal-generator:
	go run cmd/wal-generator/main.go

build:
	CGO_ENABLED=0 go build -ldflags "-X main.Version=$(VERSION)" -o databasus-agent ./cmd/main.go

test:
	go test -count=1 -failfast ./internal/...

lint:
	golangci-lint fmt ./cmd/... ./internal/... ./e2e/... && golangci-lint run ./cmd/... ./internal/... ./e2e/...

e2e:
	cd e2e && docker compose down -v --remove-orphans 2>/dev/null || true
	rm -rf e2e/pgdata e2e/artifacts
	cd e2e && docker compose build --no-cache e2e-mock-server
	cd e2e && docker compose build
	cd e2e && docker compose run --rm e2e-agent-builder
	cd e2e && docker compose up -d e2e-postgres e2e-mock-server
	cd e2e && docker compose run --rm e2e-agent-runner
	cd e2e && docker compose run --rm e2e-agent-docker
	cd e2e && docker compose down -v

e2e-clean:
	cd e2e && docker compose down -v --rmi local
	cd e2e && docker compose -f docker-compose.backup-restore.yml down -v --rmi local 2>/dev/null || true
	rm -rf e2e/artifacts

# Single-version docker-mode restore lifecycle (defaults to PG 17). Override with PG_VERSION=18.
e2e-backup-restore-docker:
	rm -rf e2e/artifacts
	mkdir -p e2e/artifacts
	cd e2e && docker compose -f docker-compose.backup-restore.yml down -v --remove-orphans 2>/dev/null || true
	cd e2e && docker compose build --no-cache e2e-br-mock-server
	cd e2e && docker compose -f docker-compose.backup-restore.yml build e2e-br-mock-server e2e-br-docker-runner
	cd e2e && docker compose run --rm e2e-agent-builder
	cd e2e && PG_VERSION=$(or $(PG_VERSION),17) docker compose -f docker-compose.backup-restore.yml up -d e2e-br-mock-server
	cd e2e && PG_VERSION=$(or $(PG_VERSION),17) docker compose -f docker-compose.backup-restore.yml run --rm e2e-br-docker-runner
	cd e2e && PG_VERSION=$(or $(PG_VERSION),17) docker compose -f docker-compose.backup-restore.yml down -v

# Matrix: runs the docker-mode restore lifecycle across PG 15, 16, 17, 18.
e2e-backup-restore-docker-matrix:
	rm -rf e2e/artifacts
	mkdir -p e2e/artifacts
	cd e2e && docker compose build --no-cache e2e-mock-server
	cd e2e && docker compose -f docker-compose.backup-restore.yml build e2e-br-mock-server e2e-br-docker-runner
	cd e2e && docker compose run --rm e2e-agent-builder
	for v in 15 16 17 18; do \
	  echo "===== PG $$v ====="; \
	  cd e2e && docker compose -f docker-compose.backup-restore.yml down -v --remove-orphans 2>/dev/null || true; \
	  cd e2e && PG_VERSION=$$v docker compose -f docker-compose.backup-restore.yml up -d e2e-br-mock-server || exit 1; \
	  cd e2e && PG_VERSION=$$v docker compose -f docker-compose.backup-restore.yml run --rm e2e-br-docker-runner || exit 1; \
	  cd e2e && PG_VERSION=$$v docker compose -f docker-compose.backup-restore.yml down -v; \
	done
