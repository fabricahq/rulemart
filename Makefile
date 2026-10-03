# Local build and checks. dist builds the release assets Release Planner publishes: one ZIP per Lambda function, plus
# SHA256SUMS and manifest.json. check needs the Postgres from db; CI provides its own.
.PHONY: dist check check-js generate check-generated db db-stop migrate ingest worker web web-dev clean

# Local Postgres for integration tests and development, matching Neon's major version. make db also creates the
# rulemart database and the roles infrastructure creates in production: the NOLOGIN group roles that migrations grant
# access to, rulemart_catalog_reader, rulemart_accounts_writer, and rulemart_catalog_writer, and the functions' login
# roles that are their members, rulemart_web, of the first two, and rulemart_worker, of the third.
LOCAL_DB_IMAGE := postgres:18
LOCAL_DB_CONTAINER := rulemart-postgres
# IPv4, because make db publishes the port only on IPv4 loopback; localhost can resolve to ::1 first on macOS.
LOCAL_DB_HOST := 127.0.0.1
LOCAL_DB_PORT := 55432
# The rulemart database as its owner, which migrates it.
LOCAL_DATABASE_URL ?= postgres://postgres:postgres@$(LOCAL_DB_HOST):$(LOCAL_DB_PORT)/rulemart?sslmode=disable
# The web function's login role, which may only read the catalog, through its membership in rulemart_catalog_reader,
# and sign visitors in and out, through its membership in rulemart_accounts_writer.
# Infrastructure creates it in production; locally its password is a test value, which internal/platform/postgrestest also uses.
LOCAL_WEB_ROLE_PASSWORD := rulemart-web-local
# The rulemart database as rulemart_web, as the deployed web function connects.
LOCAL_WEB_DATABASE_URL ?= postgres://rulemart_web:$(LOCAL_WEB_ROLE_PASSWORD)@$(LOCAL_DB_HOST):$(LOCAL_DB_PORT)/rulemart?sslmode=disable
# The worker function's login role, which may only write the catalog, through its membership in
# rulemart_catalog_writer. Infrastructure creates it in production; locally its password is a test value, which
# internal/platform/postgrestest also uses.
LOCAL_WORKER_ROLE_PASSWORD := rulemart-worker-local
# The rulemart database as rulemart_worker, as the deployed worker function connects, and as make ingest and make
# worker connect.
LOCAL_WORKER_DATABASE_URL ?= postgres://rulemart_worker:$(LOCAL_WORKER_ROLE_PASSWORD)@$(LOCAL_DB_HOST):$(LOCAL_DB_PORT)/rulemart?sslmode=disable
# The server where tests create their own databases. The Go tests and CI read this name.
export RULEMART_TEST_DATABASE_URL ?= postgres://postgres:postgres@$(LOCAL_DB_HOST):$(LOCAL_DB_PORT)/postgres?sslmode=disable
# Set DATABASE_URL to the local database, as its owner or as rulemart_worker, for a command, unless the environment
# already names a database with DATABASE_URL or DATABASE_URL_PARAMETER, so a command meant for Neon never falls back
# to the local one, or the other way round.
LOCAL_DATABASE_ENV = $(if $(DATABASE_URL)$(DATABASE_URL_PARAMETER),,DATABASE_URL='$(LOCAL_DATABASE_URL)')
LOCAL_WORKER_DATABASE_ENV = $(if $(DATABASE_URL)$(DATABASE_URL_PARAMETER),,DATABASE_URL='$(LOCAL_WORKER_DATABASE_URL)')

# fabricahq/lambda-build's packager, which builds the release assets as lambda-build.toml says, in Docker. Keep the
# commit equal to the one .github/workflows/build-release.yml checks out, and the SHA-256 equal to lambda_build.py's
# at that commit.
LAMBDA_BUILD_COMMIT := 15992fe67ee5af77c338558bcb4a4203c4aaed58
LAMBDA_BUILD_SHA256 := d8a1514e9088b87e502d9ac6db2f545eaef5a05c14a64fa5e5eb6dfdacb4bf67
LAMBDA_BUILD := bin/lambda_build-$(LAMBDA_BUILD_COMMIT).py

# Tailwind's standalone CLI, so building the stylesheet needs no Node. Each platform's binary is pinned by the
# SHA-256 in the release's sha256sums.txt; update the version and every checksum together.
TAILWIND_VERSION := 4.3.3
TAILWIND_SHA256_macos-arm64 := cdf646702987a743464dff4d9c60fd4480d1c1e73dd819a9a67f1078815dce9d
TAILWIND_SHA256_macos-x64 := 7922e0953f2110c05976e3bf58f14e643d90427575e766b7d433f5f80cbee7e1
TAILWIND_SHA256_linux-arm64 := 55fd0b241214eff3de1e8ee4f22796662f2d2e7a49bcfca7477cfd0bac398195
TAILWIND_SHA256_linux-x64 := dc61b3ac6b8c9ca874c0cc4c57b2409791a64c5540404ca5f5367360babc313a
TAILWIND_OS := $(subst Darwin,macos,$(subst Linux,linux,$(shell uname -s)))
TAILWIND_ARCH := $(subst aarch64,arm64,$(subst x86_64,x64,$(shell uname -m)))
TAILWIND_PLATFORM := $(TAILWIND_OS)-$(TAILWIND_ARCH)
TAILWIND := bin/tailwindcss-$(TAILWIND_VERSION)-$(TAILWIND_PLATFORM)

# Generated files, committed so builds need no generators. CI fails when they're stale. sqlc and Tailwind write into
# generated/ directories, sqlc one for each context's store; templ output must sit beside its source, in the same
# package, so it's named *_templ.generated.go instead. make generate deletes all of them first, so a stale or renamed
# file shows as a deletion.
SQLC_OUT := internal/contexts/catalog/store/postgres/generated internal/contexts/accounts/store/postgres/generated
TEMPL_DIR := internal/platform/web
STYLESHEET_OUT := internal/platform/web/static/generated
GENERATED := $(SQLC_OUT) $(STYLESHEET_OUT) ':(glob)$(TEMPL_DIR)/*_templ*.go'

# Builds HEAD's committed tree twice, as the release does, and requires identical ZIPs. Uncommitted changes aren't in
# the build.
dist: $(LAMBDA_BUILD)
	rm -rf dist
	python3 $(LAMBDA_BUILD) package --output dist

# Local builds may add the rulemartdev tag, which compiles in the dev sign-in, so check runs the packages that tag
# changes with it too. Release builds never set it.
DEV_TAG := rulemartdev

check: check-js
	go vet ./...
	go vet -tags $(DEV_TAG) ./...
	go test -race ./...
	go test -race -tags $(DEV_TAG) ./internal/platform/web/... ./cmd/web/...

# Runs the tests of the site's scripts, *.test.mjs beside the web package, with Node's own test runner, so they need no
# packages. Node is optional locally, so without it this says the tests didn't run; CI's runners have it.
check-js:
	@if command -v node >/dev/null 2>&1; then node --test internal/platform/web/*.test.mjs; \
	else echo "Skipping the JavaScript tests: node isn't installed."; fi

# Regenerates the sqlc queries, the templ components, and the stylesheet. templ always writes x_templ.go, so each is
# renamed x_templ.generated.go.
generate: $(TAILWIND)
	rm -rf $(SQLC_OUT) $(STYLESHEET_OUT)
	rm -f $(TEMPL_DIR)/*_templ*.go
	go tool sqlc generate
	go tool templ generate -path $(TEMPL_DIR)
	@for f in $(TEMPL_DIR)/*_templ.go; do mv "$$f" "$${f%.go}.generated.go"; done
	$(TAILWIND) --input $(TEMPL_DIR)/styles/app.css --output $(STYLESHEET_OUT)/app.css --minify

# Fails when a generated file differs from what its sources generate: changed, missing, or one they no longer
# generate, which make generate deleted. It lists files the sources generate that git doesn't track.
check-generated: generate
	@git diff --exit-code -- $(GENERATED) && test -z "$$(git ls-files --others --exclude-standard -- $(GENERATED) | tee /dev/stderr)" \
		|| { echo "Generated files are stale: run make generate and commit the result."; exit 1; }

$(TAILWIND):
	@test -n "$(TAILWIND_SHA256_$(TAILWIND_PLATFORM))" || { echo "No pinned Tailwind binary for $(TAILWIND_PLATFORM)"; exit 1; }
	@mkdir -p bin
	curl -fsSL -o $@.download https://github.com/tailwindlabs/tailwindcss/releases/download/v$(TAILWIND_VERSION)/tailwindcss-$(TAILWIND_PLATFORM)
	@echo "$(TAILWIND_SHA256_$(TAILWIND_PLATFORM))  $@.download" | shasum -a 256 -c - >/dev/null || { echo "Tailwind binary checksum mismatch"; rm -f $@.download; exit 1; }
	@chmod +x $@.download && mv $@.download $@

$(LAMBDA_BUILD):
	@mkdir -p bin
	curl -fsSL -o $@.download https://raw.githubusercontent.com/fabricahq/lambda-build/$(LAMBDA_BUILD_COMMIT)/lambda_build.py
	@echo "$(LAMBDA_BUILD_SHA256)  $@.download" | shasum -a 256 -c - >/dev/null || { echo "lambda_build.py checksum mismatch"; rm -f $@.download; exit 1; }
	@mv $@.download $@

db:
	@docker start $(LOCAL_DB_CONTAINER) >/dev/null 2>&1 || docker run -d --name $(LOCAL_DB_CONTAINER) \
		-e POSTGRES_PASSWORD=postgres -p $(LOCAL_DB_HOST):$(LOCAL_DB_PORT):5432 $(LOCAL_DB_IMAGE) >/dev/null
	@until docker exec $(LOCAL_DB_CONTAINER) pg_isready -U postgres >/dev/null 2>&1; do sleep 1; done
	@docker exec $(LOCAL_DB_CONTAINER) psql -U postgres -tAc "SELECT 1 FROM pg_database WHERE datname = 'rulemart'" | grep -q 1 \
		|| docker exec $(LOCAL_DB_CONTAINER) createdb -U postgres rulemart
	@$(call local_role,rulemart_catalog_reader,NOLOGIN)
	@$(call local_role,rulemart_accounts_writer,NOLOGIN)
	@$(call local_role,rulemart_web,LOGIN PASSWORD '$(LOCAL_WEB_ROLE_PASSWORD)')
	@$(call local_role,rulemart_catalog_writer,NOLOGIN)
	@$(call local_role,rulemart_worker,LOGIN PASSWORD '$(LOCAL_WORKER_ROLE_PASSWORD)')
	@# Granted every time, so a container whose logins predate their group roles gains the memberships too.
	@docker exec -e PGOPTIONS='-c client_min_messages=warning' $(LOCAL_DB_CONTAINER) psql -U postgres -qc "GRANT rulemart_catalog_reader TO rulemart_web"
	@docker exec -e PGOPTIONS='-c client_min_messages=warning' $(LOCAL_DB_CONTAINER) psql -U postgres -qc "GRANT rulemart_accounts_writer TO rulemart_web"
	@docker exec -e PGOPTIONS='-c client_min_messages=warning' $(LOCAL_DB_CONTAINER) psql -U postgres -qc "GRANT rulemart_catalog_writer TO rulemart_worker"
	@echo "Postgres is ready at $(LOCAL_DB_HOST):$(LOCAL_DB_PORT), with a rulemart database, rulemart_web as a member of rulemart_catalog_reader and rulemart_accounts_writer, and rulemart_worker as a member of rulemart_catalog_writer for local development"

# Creates role $(1) in the local container with the attributes $(2), unless it exists.
local_role = docker exec $(LOCAL_DB_CONTAINER) psql -U postgres -tAc "SELECT 1 FROM pg_roles WHERE rolname = '$(1)'" | grep -q 1 \
	|| docker exec $(LOCAL_DB_CONTAINER) psql -U postgres -qc "CREATE ROLE $(1) $(2)"

db-stop:
	docker rm -f $(LOCAL_DB_CONTAINER)

# Applies migrations to the local rulemart database, or to the one DATABASE_URL names, such as Neon's direct
# connection string.
migrate:
	$(LOCAL_DATABASE_ENV) go run ./cmd/migrate-database

# Ingests the library at URL, such as https://github.com/fabricahq/code-rules-test-library, into the local rulemart
# database as rulemart_worker, or into the one DATABASE_URL or DATABASE_URL_PARAMETER names.
ingest:
	@test -n "$(URL)" || { echo "usage: make ingest URL=https://github.com/<owner>/<repository>"; exit 2; }
	$(LOCAL_WORKER_DATABASE_ENV) go run ./cmd/ingest '$(URL)'

# Runs the worker's scheduled poll once: it checks every library catalog/vetted.yaml lists, and ingests the ones
# whose release tags changed, through a queue in memory instead of SQS. It writes to the local rulemart database as
# rulemart_worker, or to the one DATABASE_URL or DATABASE_URL_PARAMETER names.
worker:
	$(LOCAL_WORKER_DATABASE_ENV) go run ./cmd/worker

# Serves the pages at http://127.0.0.1:8080 from the local rulemart database, connecting as rulemart_web as the
# deployed function does. Set GITHUB_CLIENT_ID and GITHUB_CLIENT_SECRET to sign in with a GitHub OAuth app.
web:
	DATABASE_URL='$(LOCAL_WEB_DATABASE_URL)' go run ./cmd/web

# Serves the pages as make web does, built with the dev sign-in, so a browser can sign in as a test user without
# GitHub. Only this local build has it.
web-dev:
	DATABASE_URL='$(LOCAL_WEB_DATABASE_URL)' go run -tags $(DEV_TAG) ./cmd/web

clean:
	rm -rf dist bin
