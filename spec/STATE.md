# Current specification state

**Updated:** 2026-09-16

## Current milestone

Specification ratified; the SPEC and BASE chains are closed, both discovery tasks
have reported, and both rulings are in. The library fix is implemented and
verified locally; nothing is deployed. The 2026-08-01 round covers library page
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
- **Fixed and verified locally 2026-09-16** (`PERF-005` / `PERF-006`): page 1
  2.554s → 0.048s, page 21 12.099s → 0.074s, row output identical over all 4033
  rows, and the handler's >100ms timing line no longer fires. Atlas is unmeasured
  until `PERF-007`.
- `sqlc.yaml` and 503 lines of `queries/*.sql` described queries no code path
executed; deleted by `BASE-002` (2026-09-16), so `db/store.go` is the only query
source.
- Two test files exist in roughly 15,500 lines, both from the mystery-packs
  feature. `main.go` and `db/store.go` have none.

## Next task

`DEPLOY-001` — Bobby confirms, then rsync + `deploy.sh` put the library fix on
Atlas, and `PERF-007` verifies sub-second against the deployed instance. Nothing
promotes itself. The GG chain (`GG-002` → `GG-003` → `DEPLOY-002` → `GG-004`) is
ruled on provider and scope but still needs the two `OPEN.md` answers below, and
`REL-001` closes the round.

## Blockers

- `DEPLOY-001`, `DEPLOY-002`, and `GG-004` need authorized `atlas` access in the
  executing session. Verified reachable from Ergaster on 2026-09-16
  (`ssh truenas_admin@192.168.3.174` → `truenas`; live DB `games` = 4033).
- `DEPLOY-001` and `DEPLOY-002` are `human` tasks: they need Bobby at a terminal,
  because `sudo docker` needs a TTY (`CLAUDE.md:94-95`).
- `GG-002` is ruled on provider and scope but waits on two `OPEN.md` answers:
  whether scorched earth covers `best_current_store` as well as history, and
  whether the GG.deals entry point is retired.
