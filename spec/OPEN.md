# Open questions — Nisaba, 2026-08-01 round

Nothing in this file is a requirement. **No task may be derived from it.** When
a question is ruled on, the ruling moves to `PRODUCT.md` and the entry here is
deleted.

## Awaiting a ruling

- **`enrichment_queue` has no consumer — delete the machinery, or give it one?**
  Measured 2026-09-17: a `grep` for `enrichment_queue` across every `*.go` and
  `*.sql` file returns exactly two references — the `INSERT` in
  `EnqueueEnrichment` (`db/store.go:1729`) and a `COUNT` in `QueueCounts`
  (`db/store.go:1739`). **Nothing ever reads a row to process it**, and no
  function anywhere selects a queued entity (`FROM enrichment_queue` and
  `status = 'pending'` both match only those two sites). The live table is
  therefore a write-only ledger, and the consequence is a real one: the older
  `/review` page's manual match calls `SetIGDBMatch`, which sets
  `enrichment_status = 'manual'` — removing the game from the pool
  `EnrichLibrary` selects (`WHERE enrichment_status = 'needs_review'`) — and then
  enqueues into the queue nobody drains. Its comment claims "the enrichment
  pipeline handles the rest"; it does not. **A game manually matched from
  `/review` is linked and never enriched.** Found while building the match-review
  true-up (`MATCH-004`), which deliberately avoided that path and used the
  pipeline's own per-game write instead. Options I can see, not a recommendation:
  (a) delete `enrichment_queue`, `EnqueueEnrichment`, `QueueCounts` and the
  `/review` call site, and have manual matches enrich directly as the true-up now
  does; (b) build the missing consumer so the queue means something again;
  (c) leave both dormant but correct the misleading comment. Not derived into a
  task, and **`/review` still links without enriching** until this is ruled.
- **Should the Steam Deck, ProtonDB and Steam cross-ref fetchers be wired back
  up, or deleted?** Measured 2026-09-17: `SyncSteamDeckStatus`
  (`sync/steam_deck.go:25`), `SyncProtonRatings` (`sync/protondb.go:24`) and
  `SyncSteamCrossRefs` (`sync/igdb.go:253`) have **no caller anywhere in the
  repository** — a `grep` for `SyncSteamDeckStatus(`, `SyncProtonRatings(` and
  `SyncSteamCrossRefs(` across every `*.go` file matches only their definitions.
  The store methods they use (`SetSteamDeckVerified`, `SetWishlistDeckVerified`,
  `SetProtonRating`) and the columns they fill are live, and the library grid and
  filters still read them: the live database holds 1779 games with
  `steam_deck_verified` and 826 with `proton_rating`, all from before the 2026-06-28
  sync-page cleanup. So the data is displayed and stale rather than absent —
  `README.md` advertised both as working features until this was measured. The
  full sync never called them. Options I can see, not a recommendation: (a) call
  them from the daily or full sync again; (b) leave them dormant and say so in
  `README.md`; (c) delete the three files. Not derived into a task.
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

Both original entries here are settled and were deleted, per this file's own
rule: the library fix landed as a query-shape change (`PERF-004` → `PERF-007`,
page 1 5.63s → 0.081s live), and ITAD was adopted as the pricing provider
(`PRODUCT.md`, `GG-002`).

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
  the "scrape the page" option `CLAUDE.md` → Recent Session Context records, and
  `GG-001` could not verify whether another GG.deals endpoint carries per-shop
  data.)
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
