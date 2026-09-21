# tr-bah-service

> **Repository location.** This lives at `mishikaraj99/tr-bah-consolidation-service` (private) only
> because the `trayalabs1` org blocks repo creation by members. An org owner should create
> `trayalabs1/tr-bah-consolidation-service` and transfer it, after which the `/ship` + ClickUp
> policy below applies to every change. Note `trayalabs1/tr-bah-service` is a different,
> earlier NestJS BAH flow-engine prototype and is unrelated to this service.

Multi-tenant Build-A-Habit service in Go. Tenants: `traya` (legacy 3/7/21 economy + v85 Habit Tracker),
`mool` and `acne` (Log & Earn ledger). Every request carries `x-tenant-id`.

Ground truth for behaviour, copy strings and constants: `docs/reference/inventory-*.md` and the migration
runbook in `docs/reference/`. Design: `docs/superpowers/specs/2026-09-18-tr-bah-service-design.md`.
Rollout: `docs/superpowers/specs/2026-09-18-rollout-plan.md`.

## Tenants and auth

| Tenant | Economies | Auth |
|---|---|---|
| `traya` | legacy 3/7/21 coins + v85 Habit Tracker | Bearer JWT (`JWT_SECRET`) plus a Redis login-status check; `V2_FORM_DATA_TOKEN` for the caseId-keyed routes; `x-internal-token` for server-to-server routes |
| `mool`, `acne` | Log & Earn rupee ledger | tr-consumer-api-gateway headers (`x-user-info`, `customerId` fallback) |

Every request carries `x-tenant-id`. A route whose economy the tenant does not run answers 404.

## Run
1. `cp .env.example .env` and fill in values.
2. `go run .` (or `air` for live reload). Mongo, Redis and the tenant Postgres must be reachable —
   the service exits at boot if Mongo is not.
3. Swagger: `go install github.com/swaggo/swag/cmd/swag@latest && swag init`, then `/api/docs/`.
   The handlers already carry the annotations; regenerate `docs/` whenever routes change. On a Mac
   whose Go toolchain and command-line tools disagree on architecture, run `swag init` in CI or a
   container instead — it needs cgo.

## Redis keys

One Redis instance serves all three tenants, so every key this service owns is tenant-scoped. Keys it
introduces use `bah:<tenant>:`. The two keys shared with the Node services take a `<tenant>:` prefix in
front of their original name:

| purpose | key | still written in Node by |
| --- | --- | --- |
| kit-tracker calendar cache | `<tenant>:kit-tracker-calendar!<userId>` | traya-app-backend |
| login gate | `<tenant>:user!<userId>login!status` | traya-api-server |

While both stacks run, `LEGACY_REDIS_FALLBACK=true` (the default) lets reads fall back to the unprefixed
names, writes stay on the prefixed key, and invalidation deletes both. Set it to `false` only after the
Node services adopt the prefix. The rollout plan's "Retiring the Redis legacy-key fallback" section has
the exact order.

## Migrations

Log & Earn tables live in each tenant's Postgres database:
```
go run ./cmd/migrate --tenant mool          # add --dry-run to list first
go run ./cmd/migrate --tenant acne
```
Mongo indexes are code (`repositories/mongo.EnsureIndexes`) and are created in the background the
first time a tenant database is resolved.

## Parity harness

The migration runbook's ticket 9 check — replay recorded requests against both services and diff
the JSON field by field (null and a missing key differ; array order matters):
```
go run ./cmd/parity --a https://<api-server> --b http://localhost:3000 \
  --cases cmd/parity/testdata/sample.jsonl --ignore txnDate,createdAt,updatedAt,timestamp
```
Exit code 1 means at least one case differed.

## Test

Unit tests need nothing:
```
go test ./...
```

On an Apple-silicon Mac whose Go toolchain is the amd64 build (`go env GOARCH` reports `amd64`
while `uname -m` reports `arm64`), every test binary runs under Rosetta and starting many at once
stalls. Add `-p 1` there:
```
go test ./... -p 1
```

Integration tests run only when their datastore env vars are set, and skip otherwise.
With the bundled compose file:
```
docker compose -f docker-compose.test.yml up -d
TEST_MONGO_URI=mongodb://localhost:27117 \
TEST_PG_URI=postgres://postgres:test@localhost:55432/bah_test \
TEST_REDIS_ADDR=localhost:63790 go test ./... -count=1
```
Or against locally installed services:
```
createdb bah_test
TEST_PG_URI="postgres://$(whoami)@localhost:5432/bah_test?sslmode=disable" \
TEST_REDIS_ADDR=localhost:6379 \
TEST_MONGO_URI=mongodb://localhost:27017 go test ./... -count=1
```
