# Current specification state

**Updated:** 2026-09-16

## Current milestone

Specification ratified and its two tooling tasks closed; nothing in the round is
implemented. The 2026-08-01 round covers library page performance and GG.deals
store-name granularity.

## Established baseline

- Clean `main` at `2b8701c`, in sync with origin. `go build`, `go vet`, and
  `go test ./...` all pass on go1.25.0 (`evidence/ASSESSMENT.md`).
- The deployed instance is live and matches repo HEAD — `/mystery-packs`
  resolves, `/nonexistent-xyz` 404s.
- `/library` serves in 5.34s and `/library?page=2` in 8.84s against dashboard
  0.006s and wishlist 0.235s. Reproduced three times within 40ms. Cause not
  isolated.
- `sqlc.yaml` and 503 lines of `queries/*.sql` describe queries no code path
  executes; `db/store.go` is the live implementation.
- Two test files exist in roughly 15,500 lines, both from the mystery-packs
  feature. `main.go` and `db/store.go` have none.

## Next task

`BASE-001` (record the green baseline). `SPEC-001` and `SPEC-002` are `done`;
every other task is `blocked` in `tasks.yaml`, and a status only moves to
`ready` when someone promotes it — `scripts/spec-next.sh local,atlas` prints
nothing until then.

## Blockers

- `PERF-001`, `PERF-002`, `DEPLOY-001`, `DEPLOY-002`, and `GG-004` need
  authorized `atlas` access in the executing session. Verified reachable from
  Ergaster on 2026-09-16 (`ssh truenas_admin@192.168.3.174` → `truenas`; live DB
  `games` = 4033).
- `PERF-005` cannot start until Bobby rules on `PERF-004`'s recommendation.
- `GG-002` cannot start until Bobby rules on `GG-001`'s recommendation.
- Two questions remain unruled in `OPEN.md`: which GG.deals fix, and what
  happens to existing category-level price-history rows.
