# Local build and checks. dist builds the release assets CI publishes: one ZIP per Lambda function, plus SHA256SUMS
# and manifest.json. check needs the Postgres from db; CI provides its own.
.PHONY: dist check db db-stop migrate clean

# Postgres for integration tests and local development, matching Neon's major version.
DB_IMAGE := postgres:18
DB_CONTAINER := rulemart-postgres
DB_PORT := 55432
export RULEMART_TEST_DATABASE_URL ?= postgres://postgres:postgres@127.0.0.1:$(DB_PORT)/postgres?sslmode=disable

dist:
	python3 scripts/package-release.py --commit "$$(git rev-parse HEAD)" --output dist

check:
	go vet ./...
	go test -race ./...

db:
	@docker start $(DB_CONTAINER) >/dev/null 2>&1 || docker run -d --name $(DB_CONTAINER) \
		-e POSTGRES_PASSWORD=postgres -p 127.0.0.1:$(DB_PORT):5432 $(DB_IMAGE) >/dev/null
	@until docker exec $(DB_CONTAINER) pg_isready -U postgres >/dev/null 2>&1; do sleep 1; done
	@echo "Postgres is ready at 127.0.0.1:$(DB_PORT)"

db-stop:
	docker rm -f $(DB_CONTAINER)

# Applies migrations to the database at DATABASE_URL: Neon's direct connection string, or a local database.
migrate:
	go run ./cmd/migrate

clean:
	rm -rf dist
