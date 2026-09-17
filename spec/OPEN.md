# Open questions — Nisaba, 2026-08-01 round

Nothing in this file is a requirement. **No task may be derived from it.** When
a question is ruled on, the ruling moves to `PRODUCT.md` and the entry here is
deleted.

## Awaiting a ruling

- **Should the deploy rsync use `--delete`?** The documented command
  (`CLAUDE.md`) has no `--delete`, so the deployed tree keeps files the
  repository has deleted. `DEPLOY-004` hit this: the deleted
  `sync/gog_wishlist.go` stayed on the server beside the new `sync/gog_auth.go`
  and duplicated its `GOGClientID` constant and `gogGetAccessToken` function — a
  build failure, and `deploy.sh` removes the container *before* it builds, so the
  site would have gone down. It was fixed by hand for that one file. Still stale
  in `/mnt/MemoryAlpha/nisaba/source/`: `queries/`, `sqlc.yaml` (both deleted by
  `BASE-002`) and an `enrichment/` directory the repository does not contain. All
  inert today — only `templates/*`, `static/*` and `schema.sql` are embedded
  (`main.go:24-30`) — but any deleted file under them that a build reaches would
  fail the same way. **Measured at `DEPLOY-005`:** every `*.go` file on the server
  that the repository lacks is now a `._*` AppleDouble resource fork, which Go
  ignores, so no stale Go source is outstanding; the deleted paths that remain
  hold no `.go` files. The rsync also leaves the server's own git checkout at
  `40c8491` with a dirty tree, which the build never reads. Options I can see, not a recommendation: (a) add `--delete`
  to the documented rsync (the existing `--exclude` patterns still protect `.git`,
  `*.db` and `imgcache` from deletion); (b) keep the command as it is and remove
  stale paths by hand at each deploy; (c) something else. Not derived into a task.

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
- **May `sync_log` be rebuilt so its `type` constraint matches the code?** →
  ruled 2026-09-16: **no rebuild; log Playnite runs as type `ownership`.**
  `sync_log.type` allows only `('full', 'ownership', 'install', 'pricing',
  'wishlist', 'rehydrate')` (`schema.sql:265`) while the code logged
  `StartSync("playnite")` (`handlers/sync.go:323`) and
  `StartSync("mystery_packs")` (`handlers/sync.go:470`), which failed the
  constraint — `logID` stayed 0, so `FinishSync`/`AppendSyncErrors` were skipped
  and no Playnite run or error was ever recorded (verified 2026-09-16 on a
  throwaway copy of the live database). Ruled rather than rebuilt, because
  SQLite cannot alter a CHECK without replacing the table and migrations are
  additive-only. Executed by `GOGL-007`. The `mystery_packs` call site keeps its
  original type and is untouched.
- **Existing category-level store rows** → ruled 2026-09-16: scorched earth.
  Pre-cutover `gg.deals/retail` / `gg.deals/keyshop` rows are deleted when real
  store names arrive — neither left mixed nor backfilled. Moved to
  `PRODUCT.md`; executed by `GG-002`.
