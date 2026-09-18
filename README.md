# tr-bah-service

Multi-tenant Build-A-Habit service in Go. Tenants: `traya` (legacy 3/7/21 economy + v85 Habit Tracker),
`mool` and `acne` (Log & Earn ledger). Every request carries `x-tenant-id`.

Ground truth for behaviour, copy strings and constants: `docs/reference/inventory-*.md` and the migration
runbook in `docs/reference/`. Design: `docs/superpowers/specs/2026-09-18-tr-bah-service-design.md`.
Rollout: `docs/superpowers/specs/2026-09-18-rollout-plan.md`.

## Run
1. `cp .env.example .env` and fill in values.
2. `go run .` (or `air` for live reload).
3. Swagger: `go install github.com/swaggo/swag/cmd/swag@latest && swag init`, then `/api/docs/`.

## Test
```
go test ./...                                  # unit tests
docker compose -f docker-compose.test.yml up -d
TEST_MONGO_URI=mongodb://localhost:27117 \
TEST_PG_URI=postgres://postgres:test@localhost:55432/bah_test \
TEST_REDIS_ADDR=localhost:63790 go test ./...  # + integration tests
```
