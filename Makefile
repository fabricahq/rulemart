# Local build and checks. dist builds the release assets CI publishes: one ZIP per Lambda function, plus SHA256SUMS
# and manifest.json. check needs the Postgres from db; CI provides its own.
.PHONY: dist check generate check-generated db db-stop migrate web clean

# Postgres for integration tests and local development, matching Neon's major version.
DB_IMAGE := postgres:18
DB_CONTAINER := rulemart-postgres
DB_PORT := 55432
export RULEMART_TEST_DATABASE_URL ?= postgres://postgres:postgres@127.0.0.1:$(DB_PORT)/postgres?sslmode=disable

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

# Generated files, committed so builds need no generators. CI fails when they're stale.
GENERATED := internal/ingest/ingestdb internal/site/sitedb internal/site/*_templ.go internal/site/static/app.css

dist:
	python3 scripts/package-release.py --commit "$$(git rev-parse HEAD)" --output dist

check:
	go vet ./...
	go test -race ./...

# Regenerates the sqlc queries, the templ components, and the stylesheet.
generate: $(TAILWIND)
	go tool sqlc generate
	go tool templ generate -path internal/site
	$(TAILWIND) --input internal/site/styles/app.css --output internal/site/static/app.css --minify

# Fails when a generated file differs from what its sources generate.
check-generated: generate
	@git add --intent-to-add $(GENERATED)
	@git diff --exit-code -- $(GENERATED) || { echo "Generated files are stale: run make generate and commit the result."; exit 1; }

$(TAILWIND):
	@test -n "$(TAILWIND_SHA256_$(TAILWIND_PLATFORM))" || { echo "No pinned Tailwind binary for $(TAILWIND_PLATFORM)"; exit 1; }
	@mkdir -p bin
	curl -fsSL -o $@.download https://github.com/tailwindlabs/tailwindcss/releases/download/v$(TAILWIND_VERSION)/tailwindcss-$(TAILWIND_PLATFORM)
	@echo "$(TAILWIND_SHA256_$(TAILWIND_PLATFORM))  $@.download" | shasum -a 256 -c - >/dev/null || { echo "Tailwind binary checksum mismatch"; rm -f $@.download; exit 1; }
	@chmod +x $@.download && mv $@.download $@

db:
	@docker start $(DB_CONTAINER) >/dev/null 2>&1 || docker run -d --name $(DB_CONTAINER) \
		-e POSTGRES_PASSWORD=postgres -p 127.0.0.1:$(DB_PORT):5432 $(DB_IMAGE) >/dev/null
	@until docker exec $(DB_CONTAINER) pg_isready -U postgres >/dev/null 2>&1; do sleep 1; done
	@docker exec $(DB_CONTAINER) psql -U postgres -tAc "SELECT 1 FROM pg_database WHERE datname = 'rulemart'" | grep -q 1 \
		|| docker exec $(DB_CONTAINER) createdb -U postgres rulemart
	@echo "Postgres is ready at 127.0.0.1:$(DB_PORT), with a rulemart database for local development"

db-stop:
	docker rm -f $(DB_CONTAINER)

# Applies migrations to the database at DATABASE_URL: Neon's direct connection string, or a local database.
migrate:
	go run ./cmd/migrate

# Serves the pages at http://127.0.0.1:8080 from the database at DATABASE_URL.
web:
	go run ./cmd/web

clean:
	rm -rf dist bin
