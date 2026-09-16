# Open questions — Nisaba, 2026-08-01 round

Nothing in this file is a requirement. **No task may be derived from it.** When
a question is ruled on, the ruling moves to `PRODUCT.md` and the entry here is
deleted.

## Awaiting a ruling


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
- **Does scorched earth cover `best_current_store` too, or only history rows?**
  → ruled 2026-09-16: both — "yes, we are starting with clean data".
- **Is the GG.deals sync entry point retired once ITAD is live?** → ruled
  2026-09-16: neither retired nor dormant — GG.deals stays as a **comparison**
  source, and a cheaper GG.deals price surfaces as a wishlist callout linking to
  that game's GG.deals page (existence only, no listing detail). Moved to
  `PRODUCT.md`; executed by `GG-005` and `GG-006`.
- **Which provider for real store names?** → ruled 2026-09-16: ITAD, on the
  strength of `shop.name`; GG.deals' store breadth is moot without shop names. An
  alternative may displace it only if measured to supply real storefront names.
  Moved to `PRODUCT.md`; executed by `GG-002`. (An unsearched candidate remains
  `CLAUDE.md:169`'s "scrape the page", and `GG-001` could not verify whether
  another GG.deals endpoint carries per-shop data.)
- **Existing category-level store rows** → ruled 2026-09-16: scorched earth.
  Pre-cutover `gg.deals/retail` / `gg.deals/keyshop` rows are deleted when real
  store names arrive — neither left mixed nor backfilled. Moved to
  `PRODUCT.md`; executed by `GG-002`.
