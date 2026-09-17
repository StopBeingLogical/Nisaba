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
- **Fixed, deployed, and verified 2026-09-16** (`PERF-005` → `PERF-007`): locally
  page 1 2.554s → 0.048s and page 21 12.099s → 0.074s with row output identical
  over all 4033 rows; live on Atlas page 1 5.63s → **0.081s** and page 2 9.31s →
  **0.085s**, three runs each, against a <1s target. The handler's >100ms timing
  line no longer fires.
- `sqlc.yaml` and 503 lines of `queries/*.sql` described queries no code path
executed; deleted by `BASE-002` (2026-09-16), so `db/store.go` is the only query
source.
- Two test files exist in roughly 15,500 lines, both from the mystery-packs
  feature. `main.go` and `db/store.go` have none.

## Next task

The PERF half of the round is **complete in production**. The GG half is next:
`GG-002` (ITAD authoritative, scorched earth on both columns, `best_price_url`
from `current.url`) once `itad.api_key` is in the live config, then `GG-003`,
`GG-005`, `GG-006`, `DEPLOY-002`, `GG-004`, and `REL-001` closes the gate. Nothing
promotes itself.

The GG chain is fully ruled as of 2026-09-16: `GG-002` (ITAD authoritative;
scorched earth on both columns) → `GG-003` (shop-name labels) → `GG-005` (GG.deals
kept as a comparison source) → `GG-006` (cheaper-on-GG.deals callout), with
`DEPLOY-002` → `GG-004` for the live check, then the `REL-001` gate. `GG-002` needs
`itad.api_key` in the live config first.

## Blockers

- `DEPLOY-001`, `DEPLOY-002`, and `GG-004` need authorized `atlas` access in the
  executing session. Verified reachable from Ergaster on 2026-09-16
  (`ssh truenas_admin@192.168.3.174` → `truenas`; live DB `games` = 4033).
- `DEPLOY-001` and `DEPLOY-002` are `human` tasks: they need Bobby at a terminal,
  because `sudo docker` needs a TTY (`CLAUDE.md:94-95`).
- `GG-002` needs `itad.api_key` present in the live `app_config`. Registered
  2026-09-16 as `Nisaba_redux` and verified working against both endpoints used;
  not yet loaded — that is Bobby's step in Settings.
- `OPEN.md` has no unanswered entries as of 2026-09-16.
