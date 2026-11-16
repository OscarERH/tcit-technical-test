# Every target runs inside Docker: no Go, Node or PHP needed on the host.

.PHONY: up down test test-backend test-frontend test-workflow

up:
	docker compose up --build -d

down:
	docker compose down

test: test-backend test-frontend test-workflow

# Unit tests plus the repository tests against the Spanner emulator, which is
# started on demand. Each run uses its own throwaway database.
test-backend:
	docker compose up -d spanner
	docker compose run --rm --no-deps -T \
		-e SPANNER_EMULATOR_HOST=spanner:9010 \
		-v "$(CURDIR)/backend":/src -w /src \
		-v tcit-gomod:/go/pkg/mod -v tcit-gocache:/root/.cache/go-build \
		--entrypoint go backend-tests test -count=1 ./...

test-frontend:
	docker run --rm -e CI=true -e NG_CLI_ANALYTICS=false \
		-v "$(CURDIR)/frontend":/app -w /app node:24-alpine \
		sh -c "npm ci --silent && npx ng test --watch=false"

test-workflow:
	docker run --rm -v "$(CURDIR)/workflow":/app -w /app composer:2 \
		sh -c "composer install --no-interaction --no-progress --quiet && php bin/phpunit"
