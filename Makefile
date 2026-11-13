VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GO      ?= go
NPM     ?= npm
# 389-ds image for integration tests: built in the 389-ds-base-docker repository
# (ADR-012). DS_IMAGE=389-ds-ui/ds:dev uses the bundled deploy/ds image instead.
DEV_DS_IMAGE := 389-ds-ui/ds:dev
DS_IMAGE ?= 389-ds-base:3.3.1

.PHONY: help
help: ## Show targets
	@grep -hE '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

web/node_modules: web/package-lock.json
	cd web && $(NPM) ci --no-fund --no-audit
	@touch $@

.PHONY: gen
gen: web/node_modules ## Regenerate OpenAPI spec and the TS API client
	$(GO) run ./cmd/ds-ui openapi > api/openapi.json
	cd web && $(NPM) run gen:api

.PHONY: web
web: web/node_modules ## Build the SPA into internal/webui/dist
	cd web && $(NPM) run build

.PHONY: build
build: web ## Build bin/ds-ui with the embedded SPA
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/ds-ui ./cmd/ds-ui

.PHONY: lint
lint: web/node_modules ## go vet + gofmt check + golangci-lint (if installed) + vue-tsc
	$(GO) vet ./...
	@test -z "$$(gofmt -l cmd internal test)" || (gofmt -l cmd internal test; exit 1)
	@if command -v golangci-lint >/dev/null; then golangci-lint run; else echo "golangci-lint not installed, skipped"; fi
	cd web && $(NPM) run typecheck && $(NPM) run -s check:locales && $(NPM) run -s check:licenses

.PHONY: check-gen
check-gen: gen ## Fail if generated files are out of date
	git diff --exit-code -- api/openapi.json web/src/api/schema.d.ts

.PHONY: test
test: web/node_modules ## Unit tests (Go + frontend)
	$(GO) test ./...
	cd web && $(NPM) test

.PHONY: ds-image
ds-image: ## Build the bundled development 389-ds image (deploy/ds)
	docker build -t $(DEV_DS_IMAGE) deploy/ds

.PHONY: itest
itest: $(if $(filter $(DEV_DS_IMAGE),$(DS_IMAGE)),ds-image) ## Integration tests against a real 389-ds container (DS_IMAGE)
	DS_IMAGE=$(DS_IMAGE) $(GO) test -tags integration -count=1 -timeout 10m ./test/integration/...

.PHONY: secrets
secrets: secrets/dm_password ## Create secrets/dm_password if missing

secrets/dm_password:
	@head -c 24 /dev/urandom | base64 | tr -d '/+=' > $@
	@chmod 0644 $@   # bind-mounted into containers running as uid 389
	@echo "created $@"

.PHONY: images
images: ## Build all images
	docker compose build
	docker build -f deploy/ui/Dockerfile --target static --build-arg VERSION=$(VERSION) -t 389-ds-ui/ui:dev-static .

.PHONY: up
up: secrets ## Start ds + ui (sidecar mode)
	DOCKER_GID=$$(stat -c %g /var/run/docker.sock) docker compose up -d --build

.PHONY: seed
seed: ## Load test/fixtures/sample.ldif into the running ds container (existing entries are skipped)
	docker compose exec -T ds ldapadd -c -Y EXTERNAL -Q -H ldapi://%2fdata%2frun%2fslapd-localhost.socket \
		< test/fixtures/sample.ldif 2>&1 | grep -vE '^(adding new entry|$$)' || true

.PHONY: e2e
e2e: web/node_modules ## End-to-end browser tests against the running stack (make up && make seed)
	cd web && npx playwright install chromium >/dev/null && npx playwright test

.PHONY: down
down: ## Stop the stack (keeps the data volume)
	docker compose down

.PHONY: dev
dev: secrets ## Run ds in docker, ds-ui from source in remote mode; then: cd web && npm run dev
	docker compose up -d ds
	DSUI_MODE=remote DSUI_LDAP_URL=ldap://127.0.0.1:$${DS_LDAP_PORT:-389} \
	DSUI_LDAP_BIND_DN="cn=Directory Manager" DSUI_LDAP_BIND_PASSWORD_FILE=secrets/dm_password \
	DSUI_ALLOWED_ORIGINS=http://localhost:5173,http://127.0.0.1:5173 \
	DSUI_LOG_LEVEL=debug $(GO) run ./cmd/ds-ui

.PHONY: clean
clean: ## Remove build artefacts
	rm -rf bin
	find internal/webui/dist -mindepth 1 ! -name .gitkeep -exec rm -rf {} +
