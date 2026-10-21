# Sovereign -- local, zero-cost operation.
#
# Everything here runs on your own machine against the compose stack. No AWS
# account, no domain, no spend. See docs/costs.md for what is deferred.

SHELL := /bin/bash
DB_URL ?= postgresql://sovereign:sovereign@localhost:55432/sovereign
API_PORT ?= 8000
SERVER ?= http://127.0.0.1:$(API_PORT)

.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: up
up: ## Start Postgres and apply migrations
	docker compose up -d db
	@until [ "$$(docker inspect --format '{{.State.Health.Status}}' sovereign-db-1 2>/dev/null)" = "healthy" ]; do \
		sleep 1; done
	cd api && DATABASE_URL=$(DB_URL) uv run sovereign migrate

.PHONY: down
down: ## Stop Postgres (data is preserved in the volume)
	docker compose down

.PHONY: serve
serve: ## Run the control plane in the foreground
	cd api && DATABASE_URL=$(DB_URL) uv run uvicorn sovereign_api.app:create_app \
		--factory --host 127.0.0.1 --port $(API_PORT)

.PHONY: token
token: ## Mint an enrolment token
	@cd api && DATABASE_URL=$(DB_URL) uv run sovereign issue-token --note "local probe"

.PHONY: probe
probe: ## Run the probe against the local control plane
	cd agent && go run ./cmd/probe -server $(SERVER)

.PHONY: rollup
rollup: ## Recompute 5m and 1h rollups
	cd api && DATABASE_URL=$(DB_URL) uv run sovereign rollup --lookback 120

.PHONY: backup
backup: ## Dump the database to data/backups/
	@ops/backup.sh

.PHONY: snapshot
snapshot: ## Export JSON from the database into the site
	cd api && DATABASE_URL=$(DB_URL) uv run sovereign snapshot

.PHONY: site
site: snapshot ## Build the static site (dist/ is deployable as-is)
	cd web && npm install --silent && npm run build

.PHONY: site-dev
site-dev: ## Run the site with live reload
	cd web && npm run dev

.PHONY: deploy
deploy: snapshot ## Build with fresh data and upload to Cloudflare Pages
	cd web && npm install --silent && npm run deploy

.PHONY: deploy-indexed
deploy-indexed: snapshot ## Deploy AND allow search engines (see docs/deploying.md)
	@echo "This makes the site discoverable. Checklist: docs/deploying.md"
	@read -p "Continue? [y/N] " ok && [ "$$ok" = "y" ]
	cd web && PUBLIC_INDEXABLE=true npm run deploy

.PHONY: test
test: ## Run every test suite
	cd agent && gofmt -l . && go vet ./... && go test -race ./...
	cd api && uv run pytest -q
	cd web && npm test
	terraform -chdir=infra/bootstrap fmt -check -recursive
	terraform -chdir=infra/aws fmt -check -recursive

.PHONY: verify-targets
verify-targets: ## Check which endpoints still resolve
	cd agent && go run ./cmd/probe -verify

.PHONY: stats
stats: ## How much history have we accumulated?
	@docker exec sovereign-db-1 psql -U sovereign -d sovereign -t -c "\
		SELECT '  samples:      ' || count(*) FROM samples; \
		SELECT '  oldest:       ' || coalesce(min(ts)::text, 'none') FROM samples; \
		SELECT '  newest:       ' || coalesce(max(ts)::text, 'none') FROM samples; \
		SELECT '  days covered: ' || coalesce(round(extract(epoch FROM max(ts)-min(ts))/86400, 1)::text, '0') FROM samples; \
		SELECT '  probes:       ' || count(*) FROM probes WHERE revoked_at IS NULL; \
		SELECT '  hourly rows:  ' || count(*) FROM sample_rollup_1h;"
