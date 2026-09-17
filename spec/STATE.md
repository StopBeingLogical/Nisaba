# Current specification state

**Updated:** 2026-09-16

## Current milestone

**The 2026-08-01 round is complete in production.** All 21 tasks are `done` with
evidence. Both changes landed: library responsiveness (5.63s → 0.081s, target
<1s) and real storefront names in price data and the UI, with GG.deals kept as a
comparison source. The gate is recorded in `evidence/REL-001.md`.

Nothing is in flight. New work has no open rulings and no unanswered entries in
`OPEN.md`; corrections Bobby asks for become new tasks.

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

## Where the pricing data stands (live, 2026-09-16)

- Provider: ITAD, authoritative for `best_current_price`, `best_current_store`,
  `best_price_url`, the historical low, and price history. IDs resolve in bulk
  (one `POST /lookup/id/shop/61/v1` per 100 Steam App IDs) — 11 requests for the
  whole wishlist, 2.2% of the key's 100-per-5-minute budget, where the old
  per-entry loop needed ~609 (6× over) and the full sync took ~13 minutes.
- Live after the cutover sync: 494 priced entries across 20 real storefronts,
  494 deal URLs, 494 history rows, **zero** `gg.deals/*` category values left in
  either column. 284 entries are cheaper on GG.deals and show the callout.
- GG.deals is a comparison source only: it writes `gg_deals_price` /
  `gg_deals_url` and nothing else — verified by re-running its pass over a synced
  database and diffing all 667 rows' ITAD columns (identical).
- Scorched earth (18,335 history rows, 493 category values) ran once, by hand,
  after the migrations — **not code**, and it never runs again.

## Next task

None. The round's gate passed in full (`evidence/REL-001.md`); two gate lines are
recorded there as partial-with-explanation rather than claimed clean: "nothing
references" the deleted sqlc scaffolding (13 files mention it, all describing the
removal or the superseded design doc) and "no user data was deleted" (the ruled
scorched-earth delete removed pre-cutover *price* rows; no entry, game, or
user-authored field was touched).

A full sync from the UI has not been run since the cutover, so the current prices
arrived via the one-off harness rather than the button — the next button-driven
sync will exercise the same code path and log to `sync_log`.

## Blockers

None. `OPEN.md` has no unanswered entries, no task is blocked, and the deployed
instance is healthy.

Reference facts a future session should not have to re-derive:

- `atlas` is reachable from Ergaster as `truenas_admin@192.168.3.174` with the
  SSH key, non-interactively. `sudo -n` works over plain SSH — **no `-t` and no
  password prompt**, which contradicts what `SESSION_SEED.md` and `CLAUDE.md`
  used to claim and is corrected in both. Live DB:
  `/mnt/MemoryAlpha/nisaba/data/nisaba.db`, source and `deploy.sh` in
  `/mnt/MemoryAlpha/nisaba/source/`.
- `/tmp` on Atlas is `noexec`; a binary that has to run on the host belongs on a
  dataset (`/mnt/MemoryAlpha/...`), not in `/tmp`.
- `itad.api_key` is set in the live `app_config` (app registered 2026-09-16 as
  `Nisaba_redux`, limit 100 requests / 5 minutes per Bobby's setup page — not the
  1000 the docs state; no rate-limit headers are exposed). It is never committed,
  never written into evidence, and never in a changelog entry.
- Migrations must run before any SQL that references a newly added column: the
  scorched-earth update fails to prepare against a pre-migration database
  (`no such column: gg_deals_price`).
