# tr-bah-service — implementation status

Plan: `2026-09-18-tr-bah-service.md` (33 tasks). All 33 are implemented and committed.

| Task | Deliverable | State |
|---|---|---|
| 1 | Module, Fiber app, Dockerfile, env example | done |
| 2 | Typed config + slog logger | done |
| 3 | Mongo/Postgres/Redis/HTTP factories | done |
| 4 | IST + UTC date helpers, moment-style formatting | done |
| 5 | HTTP errors, envelope writers, Joi-compatible validators | done |
| 6 | Mongo models, store, index bootstrap | done |
| 7 | Mongo repositories | done |
| 8 | Traya Postgres reads, Log & Earn store, SQL migration | done |
| 9 | Tenant registry + middleware | done |
| 10 | Auth strategies (JWT, V2 token, internal token, gateway) | done |
| 11 | Kit calculator, kit windows, variant tables | done |
| 12 | tr order-service client, config-service variant map | done |
| 13 | Legacy constants, copy strings, idempotency keys | done |
| 14 | Legacy streak engine | done |
| 15 | Reward credits, Shopflo, CCD, cohorts, tasks | done |
| 16 | Banner widget, challenge banner, post-log copy, modals | done |
| 17 | `streakAndRewardBalance`, balance helpers, 15-day nudge | done |
| 18 | Latest medicines, how-to-use, recommendation proxies | done |
| 19 | `bahLogForGivenDate`, write path, multiple-log update, dispatcher | done |
| 20 | `coinTransaction` pagination, calendar | done |
| 21 | CRM handlers | done |
| 22 | Habit constants, reward tiers, habit credit | done |
| 23 | 16-state resolver, bottom sheets | done |
| 24 | Habit tracker data, month calendar, badges | done |
| 25 | Scratch cards, archived products | done |
| 26 | Kit tracker page assembly, feedback card | done |
| 27 | Habit coin redeem | done |
| 28 | Redis lifeline scheduler, worker, reconcile | done |
| 29 | Log & Earn constants, config client, cap info | done |
| 30 | Log & Earn state/log/backfill/lifeline/redeem | done |
| 31 | Controllers, routes, auth wiring, main | done |
| 32 | Migration runner, parity harness | done |
| 33 | Docs, swagger annotations, README | done |

## Verification state on this machine

- Unit tests: run and pass for every package.
- Postgres integration tests (`repositories/pg`, `internal/logearn`): run and pass against a local
  Postgres. They found and fixed a real concurrency bug — `cash_ledger.balance_after` was read from
  the newest row by `created_at`, which is the transaction-start time and so does not follow commit
  order; the balance is now summed under the per-customer advisory lock.
- Redis integration tests (`internal/habit/lifeline`): run and pass against a local Redis.
- Mongo integration tests (`repositories/mongo`, `internal/legacy`, and the service-level tests that
  build on them): **skipped** — no MongoDB on this machine. Homebrew refuses the `mongodb/brew` tap
  without an explicit `brew trust`, and Docker is unavailable. Run them with:
  `TEST_MONGO_URI=mongodb://localhost:27017 go test ./... -count=1`
- `go test ./...` must be run with `-p 1` on this machine: its Go toolchain is the amd64 build on
  an arm64 Mac, so the test binaries run under Rosetta and starting nine at once stalls them before
  they execute. Each package passes on its own, and serially. A native arm64 toolchain removes this.
- `swag init` was not run: this Mac's Go toolchain is x86_64 while its command-line tools are
  arm64-only, so cgo cannot build `swag`. Annotations are in the handlers; generate `docs/` in CI.
- End-to-end boot was exercised up to the Mongo dependency: the service loads config, builds every
  client, and exits with a clear error when Mongo is unreachable (the intended fail-fast).

## Not implemented, by design

The 17 endpoints the migration runbook deletes in Wave 0, the legacy MOOL Mongo BAH module, and the
app-backend home-page widget handlers. See the design spec §2.2.
