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
- **Existing category-level store rows** → ruled 2026-09-16: scorched earth.
  Pre-cutover `gg.deals/retail` / `gg.deals/keyshop` rows are deleted when real
  store names arrive — neither left mixed nor backfilled. Moved to
  `PRODUCT.md`; executed by `GG-002`.
