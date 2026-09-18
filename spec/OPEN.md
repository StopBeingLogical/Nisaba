# Open questions — Nisaba, 2026-08-01 round

Nothing in this file is a requirement. **No task may be derived from it.** When
a question is ruled on, the ruling moves to `PRODUCT.md` and the entry here is
deleted.

## Awaiting a ruling

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

- **Should the scorer's prefix tier require the extension to look like a subtitle?**
  Measured 2026-09-17, as a consequence of `MATCH-008` and `DEPLOY-013`. The wider
  search now reaches entries it never saw, and `scoreMatch`'s prefix tier (0.85)
  fires whenever the stored title extends an IGDB name by **any** words. Measured
  effects on the live queue: `Call of Duty: WaW` and `STAR WARS™: Rebel Assault 1`
  each replaced a weak-but-correct candidate with a confident-but-wrong one
  (`Call of Duty`, `Star Wars`); `M.A.X.` resolves to IGDB's **`Max`** at the *exact*
  tier, because punctuation is stripped before comparison; and
  `Warhammer 40,000: Dawn of War II - Anniversary Edition` scores the franchise
  prefix. Over the whole queue the re-seed scored 47 replacements higher and **0
  lower**, so this is the cost of reaching further rather than a regression — but a
  confidently wrong pairing is presented as plausible, and every one costs a click.
  A third instance surfaced in `MATCH-008`'s second pass, and it is the clearest one:
  `Tomb Raider (VI): The Angel of Darkness (2003)` prefers **`Tomb Raider`** at 0.85
  over the correct `Tomb Raider: The Angel of Darkness`, which is only a *token*
  match at 0.11 because the store's `(VI)` normalises to `6` and adds a token the
  IGDB name lacks. A tie-break would not help there — the right entry scores strictly
  lower — which suggests the real question is whether the tiers should be ordered by
  **coverage of the stored title** (how much of it a candidate accounts for) rather
  than by match shape: `Tomb Raider` covers 2 of its 8 tokens, the correct entry 5.
  **Measured 2026-09-17, against the owner's own verdicts** (50 confirmed, 36
  rejected), all four options ranked from the same results:

  | option | confirmed entry #1 | rejected entry #1 | rejected at a confident tier |
  |---|---:|---:|---:|
  | shipped: tier only | 39/50 | 12/36 | 14/36 |
  | tier, then coverage | 40/50 | 12/36 | 14/36 |
  | coverage first, then tier | 43/50 | 12/36 | 14/36 |
  | exact, then coverage, then tier | 43/50 | 12/36 | 14/36 |

  Coverage-first looks like a **+4 improvement with no confirmed entry pushed off**
  #1 — until the same ranking is applied to the stored shortlists, where it changes
  **16 of 221** queue rows and the changes split. Wins: `Quake II: The Reckoning` →
  `Quake II Mission Pack: The Reckoning`, `The Legend of Kyrandia: Malcolm's Revenge`
  → `The Legend of Kyrandia 3: …`, `Tomb Raider (VI): The Angel of Darkness` →
  `Tomb Raider: The Angel of Darkness`. Losses: `Deus Ex™ GOTY Edition` →
  `Deus Ex: Human Revolution`, `Earth 2150 - Escape from the Blue Planet` →
  `Earth 2150: The Moon Project`, and `Temple of Elemental Evil, The` →
  `Dungeons & Dragons Online: The Temple of Elemental Evil`. That last one names
  coverage's own failure mode: a **long wrong name containing every word of the
  stored title** beats the short right one.

  **And `0 of 50` confirmed entries are missing from the search results at all** — the
  search reaches every answer the owner has given, so what remains is ordering, not
  reach. Since all four options leave the *same* 12 rejections ranked first and the
  same 14 at a confident tier, the tiers themselves misfire rather than their
  tie-break, and a reshuffle is not the answer. **Current recommendation: change
  nothing.** Options still open, none recommended: (a) require the extension to be two
  words or more and not a single acronym token; (b) break ties by specificity —
  `Star Wars: TIE Fighter` over `Star Wars`, which would fix the Dawn of War case but
  not Tomb Raider; (c) a mechanism that can tell `Temple of Elemental Evil` from
  `Dungeons & Dragons Online: The Temple of Elemental Evil`, which coverage cannot;
  (d) leave it, since a rejection is remembered and the row shows its evidence. Not
  derived into a task.

## Model inferences, unratified

Both original entries here are settled and were deleted, per this file's own
rule: the library fix landed as a query-shape change (`PERF-004` → `PERF-007`,
page 1 5.63s → 0.081s live), and ITAD was adopted as the pricing provider
(`PRODUCT.md`, `GG-002`).

## Resolved
- **`enrichment_queue` has no consumer — delete the machinery, or give it one?** →
  ruled 2026-09-17: **delete it, and fix the call sites.** Measured: the table was
  write-only (`EnqueueEnrichment` inserted, `QueueCounts` counted, nothing read) and
  held **0 rows**, with **0 games** marked `manual` — so the bug had never fired.
  It had three call sites: `/review`'s manual match linked a game and never enriched
  it, the **`Rehydrate`** button answered "Queued." and did nothing at all, and
  manual game add linked without fetching. All three now enrich through the shared
  `enrichFromIGDB` path (`sync/enrich_single.go`); the table is dropped by an
  idempotent migration. Executed by `MATCH-007`, deployed in `DEPLOY-012`. Ruling in
  `PRODUCT-10.md` §3.

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
