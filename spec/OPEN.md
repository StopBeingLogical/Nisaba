# Open questions — Nisaba, 2026-08-01 round

Nothing in this file is a requirement. **No task may be derived from it.** When
a question is ruled on, the ruling moves to `PRODUCT.md` and the entry here is
deleted.

## Awaiting a ruling

- **Which GG.deals fix?** Deliberately deferred to `GG-001`, which reports on
  ITAD's key/limit requirements, on whether a GG.deals endpoint with per-store
  data exists, and on what `sync/itad.go` already implements. Bobby rules after
  that task, and the ruling moves here → `PRODUCT.md` before `GG-002` may
  start. `CLAUDE.md:169` names the three candidates: switch to ITAD, a
  different GG.deals endpoint, or scrape the page.

- **What happens to existing category-level store rows?** `best_current_store`
  and `wishlist_price_history.store` already hold thousands of
  `gg.deals/retail` / `gg.deals/keyshop` values. Once real store names arrive,
  history is mixed. Options: leave them (new rows correct, old rows stale),
  backfill from the new provider where it can resolve them, or drop the
  pre-cutover price history. Note the constraint interaction: "never modify
  existing rows" binds *migrations* (`CLAUDE.md:91-92`); a one-off backfill job
  is a different thing, and whether that distinction is acceptable is Bobby's
  call. No recommendation offered — this depends on how much he values the
  existing 15,566 price-history entries.

## Model inferences, unratified

- **The library fix may need a query-shape or schema change rather than an
  index.** Basis: the 2026-06-28 pass already added five indexes and pagination
  (`CLAUDE.md:161-162`) and the page still serves in 5.34s. Not locked —
  `PERF-002` decides on measurements, and `PERF-003` puts the approach to
  Bobby before any fix is written.

- **ITAD is the likely GG.deals answer.** Basis: `sync/itad.go` already exists
  and `CLAUDE.md:191` notes it "would provide real store names." Not locked;
  `GG-001` may find it needs a paid or approved key.

## Resolved

- **`/library` target** → ruled 2026-08-01: under 1 second, page 1 and any
  later page. Moved to `PRODUCT.md`.
- **May an executing model deploy?** → ruled 2026-08-01: no. Deployment is
  always its own `atlas` task that stops for confirmation. Moved to
  `PRODUCT.md`.
- **sqlc / `queries/`** → ruled 2026-08-01: delete the scaffolding. Moved to
  `PRODUCT.md`; executed by `BASE-002`.
