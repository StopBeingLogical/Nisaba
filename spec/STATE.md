# Current specification state

**Updated:** 2026-09-16

## Current milestone

Specification ratified; the SPEC and BASE chains are closed and both discovery
tasks have reported — each ending in a ruling that is Bobby's. Nothing
user-facing is implemented yet. The 2026-08-01 round covers library page
performance and GG.deals store-name granularity.

## Established baseline

- Clean `main` at `2b8701c`, in sync with origin. `go build`, `go vet`, and
  `go test ./...` all pass on go1.25.0 (`evidence/ASSESSMENT.md`).
- The deployed instance is live and matches repo HEAD — `/mystery-packs`
  resolves, `/nonexistent-xyz` 404s.
- `/library` served in 5.34s and `/library?page=2` in 8.84s at 2026-08-01, against
dashboard 0.006s and wishlist 0.235s. Reproduced locally 2026-09-16 at 2.55s /
4.18s, with page 11 at 10.66s and page 21 at 12.10s. **Cause isolated by
`PERF-003`: the `multi_store_owned` `EXISTS` subquery in `ListGames` — 2.48s of
2.48s at page 1, 11.76s of 11.90s at offset 4000. Not the `GROUP_CONCAT`
subqueries (7–23ms) and not the `OFFSET` scan (~10ms). The same statement under
C SQLite takes 9ms on the same file (`evidence/PERF-003.md`).**
- `sqlc.yaml` and 503 lines of `queries/*.sql` described queries no code path
executed; deleted by `BASE-002` (2026-09-16), so `db/store.go` is the only query
source.
- Two test files exist in roughly 15,500 lines, both from the mystery-packs
  feature. `main.go` and `db/store.go` have none.

## Next task

Nothing is promotable until Bobby rules. `PERF-004` and `GG-001` are `done`, and
each ends in a decision that is his — the library fix (`PERF-005`) and the price
provider (`GG-002`); both remain `blocked` for that reason, not for dependency
order. After the rulings: `PERF-005` → `PERF-006` → `DEPLOY-001` → `PERF-007`, and
`GG-002` → `GG-003` → `DEPLOY-002` → `GG-004`, then the `REL-001` gate.

## Blockers

- `PERF-001`, `PERF-002`, `DEPLOY-001`, `DEPLOY-002`, and `GG-004` need
  authorized `atlas` access in the executing session. Verified reachable from
  Ergaster on 2026-09-16 (`ssh truenas_admin@192.168.3.174` → `truenas`; live DB
  `games` = 4033).
- `PERF-005` cannot start until Bobby rules on `PERF-004`'s recommendation.
- `GG-002` cannot start until Bobby rules on `GG-001`'s recommendation.
- One question remains unruled in `OPEN.md`: which GG.deals fix (`GG-001`
  reports first). The price-history question was ruled 2026-09-16 — scorched
  earth, executed by `GG-002`.
