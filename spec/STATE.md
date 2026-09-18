# Current specification state

**Updated:** 2026-09-17

## Current milestone

**The 2026-08-01 round is complete in production.** All 21 tasks are `done` with
evidence. Both changes landed: library responsiveness (5.63s → 0.081s, target
<1s) and real storefront names in price data and the UI, with GG.deals kept as a
comparison source. The gate is recorded in `evidence/REL-001.md`.

The follow-up round closed with `DEPLOY-003`. The **GOG round (below) opened,
ran as `GOGL-001`…`GOGL-007`, and closed with `DEPLOY-005`** — its only remaining
tasks are two Bobby-side actions listed under *Next task*.

A **wishlist round opened and closed 2026-09-17** (`PRODUCT-4.md`) after Bobby
reported missing cover art: `WISH-001` wired the orphaned wishlist enrichment
back in, and `DEPLOY-006` deployed it and backfilled the live database —
wishlist cover coverage went **41% → 92%**. Both are `done`.

A **data audit and cleanup round opened and closed 2026-09-17** (`PRODUCT-5.md`):
`AUDIT-001` measured every anomaly from a read-only copy, then `DATA-001`..`003`
and `DEPLOY-007` cleaned it. The library went from **4101 tiles to 3390 games**,
the multiple-stores flag from 796 wrong to **243 right**, and the database is
clear of duplicates, reference links and stale errors. See below.

A **second pass opened and closed the same day** (`PRODUCT-6.md`): `AUDIT-002`
re-measured everything against the *cleaned* database and found the remaining
gaps were **code, not debris** — `publisher` was empty on every game and the
fetch never asked for companies. `ENRICH-001`, `DATA-004`, `DATA-005` and
`DEPLOY-008` closed them: the library is now **3388 games**, unmatched games
428 → **328**, and `publisher` is populated for the first time. See below.

A **third pass opened and closed the same day** (`PRODUCT-7.md`): instead of
loosening the matcher further, `MATCH-001` hands the decision to the owner. The
**Match Review** page puts each unmatched game beside the best IGDB candidate
found for it, with a Yes/No verdict that saves per page and survives sessions.
`DEPLOY-009` shipped it and seeded the queue — **185 of 328** games have a
candidate. Nothing is applied to `games` yet. See below.

A **fourth pass opened and closed the same day** (`PRODUCT-8.md`), after Bobby
reported the queue needed inspecting one row at a time. The measurement found why:
**143 of the 242 undecided rows had no candidate**, so there was nothing to judge.
`MATCH-002` and `DEPLOY-010` fixed that — every candidate now carries IGDB's
summary, genres, platforms and a link, and every row has its own IGDB search box.
All 99 undecided candidates hold evidence and the re-seed moved **0** pairings.
`MATCH-003` then filled the same evidence on the 86 rows already ruled on. See below.

A **fifth pass opened and closed the same day** (`PRODUCT-9.md`) — the true-up
`PRODUCT-7` deferred. `MATCH-004` and `DEPLOY-011` turn saved verdicts into
library changes: **50 games linked and re-enriched**, 31 rejected candidates
replaced with a different offer, 5 with nothing else to offer, 0 errors. This is
the first review path that writes to `games`. See below.

A **sixth pass opened and closed the same day** (`PRODUCT-10.md`), acting on three
recommendations Bobby adopted in full: the search string is now cleaned so decorated
titles can be found at all (**+38 candidates**, 130 → 168), candidates are offered as
a **shortlist** rather than one auto-chosen replacement, and the **dead
`enrichment_queue` is deleted** with all three of its call sites fixed — including a
`Rehydrate` button that had been answering "Queued." while doing nothing. See below.

`OPEN.md` holds **three unanswered entries** — the `sync_log` CHECK still
rejecting `mystery_packs` (`handlers/sync.go:457`), whether the deploy rsync
should use `--delete`, and the three dormant fetchers (`SyncSteamDeckStatus`,
`SyncProtonRatings`, `SyncSteamCrossRefs`) that nothing calls. Nothing may derive
work from any of them; they need a ruling first. The fourth — `enrichment_queue`
being write-only — was ruled and executed this round (`MATCH-007`); the entry moved
to *Resolved*.

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

## Follow-up round (2026-09-16)

Bobby ruled two changes after the 2026-08-01 gate closed; they are locked in
`PRODUCT-2.md` and tracked as `PRICE-001`, `PRICE-002`, `DEPLOY-003`.

- **`PRICE-001`** — the app now refreshes prices itself once a day: ITAD then the
  GG.deals comparison, nothing else, ~8 seconds and ~11 requests against the
  button's 14.6 minutes. Hour in `app_config` → `sync.price_hour` (default 11,
  container clock = UTC, so 07:00 US Eastern). It skips while another sync runs,
  skips when no ITAD key is set, and catches up at startup if the day's window
  passed with prices 20 hours old. Verified locally, both the catch-up run and
  the restart-does-not-rerun path (`evidence/PRICE-001.md`).
- **`PRICE-002`** — where ITAD has no price and GG.deals does (4 entries), the
  wishlist now shows that price labelled `GG.deals` instead of "no pricing data",
  in the detail view and both list views. Display only; no shop is named
  (`evidence/PRICE-002.md`).

**Deferred by Bobby, not forgotten:** the GOG access token is expired, so the GOG
wishlist is skipped on every full sync — `re-paste auth.json in Settings`.
**Superseded by the round below (2026-09-16):** the GOG wishlist is retired and
its rows deleted, and that same token now serves the server-side GOG library
sync, which refreshes it itself.

## GOG round (2026-09-16) — open

Bobby ruled two changes after the follow-up round closed; they are locked in
`PRODUCT-3.md` and tracked as `GOGL-001`…`GOGL-005` and `DEPLOY-004`.

- **The GOG owned library syncs itself, server-side** (route ruled 2026-09-16).
  The only reachable path today is a Playnite script Bobby pastes by hand
  (`templates/sync.html:71-76`); the Heroic import handler has no route
  (`handlers/sync.go:520`), and `gog.refresh_token` is stored and never read
  (`handlers/gog_auth.go:71`). The app will refresh its own access token from
  the GOG API and pull the owned list, so no machine and no paste sit in the
  loop.
- **The GOG wishlist is retired and its 58 `gog-wish-` rows are deleted** —
  wishlist is Steam-only from here. The auth push and paste stay, because they
  are now the library's credential path.

**`GOGL-001` passed 2026-09-16** (`evidence/GOGL-001.md`), and it changed the
shape of the work:

- The stored refresh token still refreshes, returns a rotated refresh token, and
  **rotation does not invalidate the old one** — so nothing in the live config
  was broken by the proof and no re-push from Praxis is needed.
- The stored access token is still accepted by GOG about six months past its
  recorded expiry, so the recorded expiry must not be the refresh gate
  (`gogGetAccessToken` rejects a token GOG would accept).
- **`/account/getFilteredProducts?mediaType=1` is the sync source, not
  `/user/data/games`.** It returns 1040 products over 11 requests with titles,
  images, platforms and flags. The owned endpoint's 1397 ids include 358
  entitlements the library view does not show (packs, Prime/Luna rewards,
  delisted products).
- The library is not short of 361 games: the 1037 `gog` rows reconcile against
  the 1040-product library view as 3 missing and 0 stale.

**`GOGL-002` landed 2026-09-16.** `sync/gog_auth.go` refreshes the access token
from the stored refresh token and writes the rotated pair back, replacing the
expiry gate that rejected tokens GOG still accepts. The client secret lives in
`app_config` as `gog.client_secret` (Bobby ruled it a credential after the
`GOGL-001` finding that the token is not read-only), set once through Settings —
so it must be set before the new sync can authenticate. Verified against the
local database copy: token returned, access token, refresh token and expiry all
rewritten (`evidence/GOGL-002.md`).

**`GOGL-003` landed 2026-09-16.** `sync/gog_library.go` pages the library view
(11 requests) and upserts games and gog store links; `sync/schedule.go` gained a
daily job for it with its own window (`sync.gog_hour`, default 11) so the price
run stays price-only, started from `main.go`. Verified against a scratch copy of
the database: run one added the 3 missing games (1037 → 1040), run two added
nothing, 0 errors (`evidence/GOGL-003.md`). It logs as `sync_log` type
`ownership`, so it shows in Recent Activity. Not yet deployed.

**`GOGL-004` landed 2026-09-16.** The GOG wishlist step is out of the full sync
and `sync/gog_wishlist.go` is deleted — its only caller was that step. The auth
path (push, paste, the three config keys) stays, because the library sync
authenticates with it, and the store methods it used are still live for the
Steam wishlist (`evidence/GOGL-004.md`).

**`GOGL-005` ran 2026-09-16** on Bobby's go-ahead (the live database is
otherwise his alone). One DELETE against
`/mnt/MemoryAlpha/nisaba/data/nisaba.db`: 58 entries, 58 store links and 7
history rows went, wishlist 730 → 672, `gog_links_anywhere` 0, `quick_check`
ok, and the files stayed `root:root` with no WAL residue. The first attempt was
refused — the database is `root:root 644` and the SSH user is uid 950 — so it
was re-run under `sudo -n`, which is how the root-running container writes it
too. Backups of all three row sets are on Ergaster under `/tmp/gog005-backup-*`
(`evidence/GOGL-005.md`).

**Caveat until `DEPLOY-004` (now closed):** the binary running at the time was
the pre-round build, so a Full sync could recreate those entries. One ran on that
build — `sync_log` id 86, `2026-09-17 00:33:11`, 736 added — and could not have:
the GOG step was gated on the recorded expiry, so it logged a skip and added
nothing. The writer has since been replaced, so nothing attempts it now.

**`DEPLOY-004` completed 2026-09-16** (commit `8e33a46`, image
`d0c5bc6d`). The deployed container logs both schedules, and the GOG one is new
— the proof the round is live:

```
price sync: next run 2026-09-17 11:00
 gog library sync: next run 2026-09-17 11:00
```

Routes 200, wishlist still 672 with no GOG entries, `game_stores` gog rows 1037,
nothing mid-sync (`evidence/DEPLOY-004.md`). One near-miss: the deleted
`sync/gog_wishlist.go` was still on the server because the documented rsync uses
no `--delete`, and it would have failed the build after `deploy.sh` had already
removed the container. Removed by hand; the wider tree drift is in `OPEN.md`.

**`GOGL-006` and `GOGL-007` landed 2026-09-16**, from two rulings the same day
("no need for playnite sync, at least for GOG. It can still be useful for all the
other libraries"):

- **`GOGL-006`** — the embedded Playnite script skips `source: "gog"` entries one
  line after deriving the store, so GOG has a single writer; every other store
  still arrives through Playnite unchanged. The Playnite and Full Sync card copy
  was corrected in the same file, since both still claimed Playnite covers GOG
  and that the full sync refreshes a GOG wishlist (`evidence/GOGL-006.md`).
- **`GOGL-007`** — the Playnite endpoint now logs `sync_log` type `ownership`
  instead of `playnite`, which the schema's CHECK rejected, leaving `logID` 0 and
  silently discarding the run and its errors. No migration; the GOG scheduled run
  shares the type, so the two are told apart by content. `mystery_packs` keeps its
  invalid type by ruling (`evidence/GOGL-007.md`).

**`DEPLOY-005` completed 2026-09-17** (commit `0e3286c`, image
`sha256:76bd4b22c8bb`), on Bobby's confirmation. Both new strings are in the
running binary — the template is embedded, so that is the proof they are served,
not the rendered page (`/sync` is behind auth) — and the server tree was checked
before the container was killed, because `deploy.sh` removes it *before* it
builds. `/`, `/library` and `/wishlist` are 200; wishlist 672 with 0 GOG entries,
`game_stores` gog rows 1037, nothing mid-sync (`evidence/DEPLOY-005.md`).

## Wishlist round (2026-09-17) — `WISH-001` done, `DEPLOY-006` ready

Bobby reported *"There are many games missing cover art."* Measured before
anything changed: the **wishlist** had **396 of 676 entries without a cover**
(59%); the library had 148 of 4101 (3.6%), confirmed by rendering the page —
`/library` page 1 returns 195 covers and 5 placeholders out of 200 tiles. The
library gap has a different cause and is out of scope.

**The cause was a missing call, not missing data.** `EnrichWishlist`
(`sync/igdb.go:492`) is the only writer of wishlist `igdb_id` and `artwork`, and
nothing called it. Its two call sites were routes deleted by commit `9557160`
(2026-06-27, *"Simplify sync page"*) — the same commit that orphaned the Steam
Deck, ProtonDB and CrossRefs fetchers in `OPEN.md`, making this the fourth
casualty and the only visible one. Verified on the pre-change code rather than
assumed: **`SyncAll` never called it either**, so wiring it in is new behaviour,
not a regression being undone. The 280 entries that had art were all enriched by
2026-04-23, the last time the deleted button was pressed; every entry added
since is `needs_review` with `last_enriched` NULL.

**`WISH-001` landed 2026-09-17.** One call added to `SyncAll`'s enrichment step,
after `EnrichLibrary`, with the same progress plumbing and non-fatal error
handling. Matching, the 250ms tick and every linking behaviour are untouched.
Proven against a copy of the live database with the real IGDB API: **283 of 339
backlogged entries matched in 143 seconds, 0 errors**, artwork coverage 328 →
610 of 667, and a second run matched nothing and changed no row
(`evidence/WISH-001.md`).

**The backfill is the sync itself** — no hand-run SQL. The first Full sync after
the deploy processes every `needs_review` entry because
`ListWishlistNeedsEnrichment` selects on status alone. On live that is 396
entries; the copy's 83.5% match rate projects to roughly **610 of 676 with art**.
`DEPLOY-006` records the actual numbers.

Two things are named and accepted rather than solved: `bestMatch` is title
matching and misses some titles (`Baldur's Gate 3`, *Darkest Dungeon® II*),
leaving 53 unmatched with the manual cover field as the correction path; and the
retry cost is real on the first run but falls away, because later runs see only
entries the Steam sync just added.

**`DEPLOY-006` completed 2026-09-17** (`1ad613f`, image `0f0208ad7d11`). The
wishlist went from **280 to 622 of 676 entries with a cover** — 396 placeholders
down to **54** — by matching **343 of 396** in about 100 seconds with 0 errors,
and the rendered page returns exactly those numbers. `PRAGMA quick_check` is
`ok`, all 622 covers are IGDB-sourced, 10 of 10 sampled URLs resolve, and
`games` is untouched at 4101. Backups of the table and of the 396 rows' changed
fields are on Ergaster, and the temporary runner was deleted from Atlas and
never committed.

## Data audit and cleanup round (2026-09-17) — complete

Bobby asked for *"a full audit and clean up of the data"*. `AUDIT-001`
(`evidence/AUDIT-001.md`) took a `.backup` copy and measured everything with no
write to production, including what was clean: `integrity_check` `ok`, no
foreign-key violations, and no out-of-range value anywhere.

**The headline was duplication, not corruption.** 4101 rows held 3383 games.
549 of the duplicates had no store link at all and were created on
**2026-04-27**, during the Playnite rollout before title deduplication worked.
`ListGames` joins `game_stores` only for a store *filter*, so all of them were
being listed.

**The finding that ordered the work:** `multi_store_owned` asked whether another
*row* shared the `igdb_id`, not whether the game was owned on more than one
store — **796 games flagged where 102 were**. The duplicate rows were
load-bearing for that flag, so `PRODUCT-5.md` mandated the order: correct the
query, deploy it, then dedupe.

- **`DATA-001`** — the flag now counts owned store links, served by
  `idx_game_stores_game_id_owned` as a covering index. 796 → 102 measured with
  the shipped expression.
- **`DEPLOY-007`** — live, image `107c7ed2546b`. **The page proves which
expression it runs**: 5 purple dots rendered against 31 for the old test and 5
for the new one. `/library` page 1 in 0.070s, `page=10` in 0.090s.
- **`DATA-002`** — 711 rows merged and deleted: **4101 → 3390 games, 549 → 0
  without a store link, 477 → 418 with playtime**, and **all 4294 distinct
  `(store, store_id)` identifiers preserved** with every DLC row intact. Seven
  groups were left alone, because `game_stores` holds one link per store per
  game and merging them would discard an identifier — including
  `steam/3286930`, which resolves to **"Heretic + Hexen"**, a different product
  stored under Heretic's title.
- **`DATA-003`** — 3 mojibake and 12 whitespace titles repaired with their
  `sort_title` rebuilt by the same rule as `makeSortTitle`, 1 sentinel date
  nulled, 653 unread reference links deleted, 131 stale ProtonDB errors cleared.

**A dry-run bug worth remembering:** the first merge used a correlated subquery
against `games` *inside* an `UPDATE` of `games`, which SQLite does not bind to
the row being updated. A minimal reproduction caught a survivor inheriting an
unrelated group's playtime; on the full copy it inflated games-with-playtime
from 477 to 645. Precomputing the merge values fixed it. On live that version
would have written wrong data.

**Left alone by ruling:** the **124 Steam Family Sharing games**, which are
games Bobby does not own and still render with a badge. Recorded in
`PRODUCT-5.md` as an unruled question, not actioned.

## Enrichment round, second audit pass (2026-09-17) — complete

Bobby asked again for *"a full audit and clean up of the data"*. `AUDIT-002`
(`evidence/AUDIT-002.md`) re-measured every `AUDIT-001` category against the
**post-cleanup** database. All clean, and the earlier repairs held: no duplicate
wishlist identities, `game_stores.owned = 0` still 0 rows, `sync_errors` empty,
games text fully clean, and the **124 family-sharing games intact** — they carry
`owned = 1` and were never part of the deleted reference set.

**Two of the remaining gaps were code, not data.** `publisher` was empty on
**all 3390 games** because no code path writes it (`InsertGame` omits the column
and `EnrichGame` never set it), and `IGDBGame` requested no companies at all — so
`developer` (2089 empty) could never be backfilled by enrichment either.

- **`ENRICH-001`** — the fetch now asks for
  `involved_companies.company.name/developer/publisher`, and `EnrichGame` writes
  `developer` and `publisher` through `COALESCE`. Title comparison also gained
  three spelling equivalences and nothing more: `™`/`®`/`©` stripped from the
  search string (a `®` made a query return zero results), `&` folded to `and`,
  and canonical roman numerals normalised to arabic. Measured against live IGDB
  on a `.backup` copy: **3 → 100 matches of 428**, 0 errors, idempotent on a
  second run.
- **`DATA-004`** — the 2 duplicate groups blocked only by `owned = 0` reference
  links merged: **3390 → 3388 games**, duplicate groups 7 → **5**, all 3652
  distinct store identifiers preserved. The remaining 5 each hold two genuine
  owned ids for one store. The indicator reads **245**, up from 243, because two
  split rows are now one row owned on two stores.
- **`DATA-005`** — the 3 wishlist titles carrying `&amp;` decoded, and all three
  then matched; the 12 entries whose current price sat below their own recorded
  low had the low corrected. Both **3 → 0** and **12 → 0**.
- **`DEPLOY-008`** — live, image **`310c30aa044f`** (previous `107c7ed2546b`).
  Backfilled with real IGDB: games `needs_review` **428 → 328**, `developer`
  1301 → **1327**, `publisher` **0 → 92**, wishlist `needs_review` 53 → **41**
  (`igdb_id` and artwork 623 → **635**). `/library` renders **3388 games**. The
  100 library matches are the number the scratch-copy dry run predicted.

**The deploy tree is `source/`, not `app/`.** `/mnt/MemoryAlpha/nisaba/app/` is a
stale March copy that nothing builds — it still holds a real
`sync/gog_wishlist.go` deleted from the repo, which reads exactly like a
dangerous rsync leftover and is not one. The live tree was verified to hold
precisely the repo's **36** real `.go` files before deploying, because
`deploy.sh` removes the container *before* it builds.

**Unruled, with measurements attached:** the **328 games still unmatched**.
Roughly half sit in IGDB under a decorated or abbreviated name (`Halcyon 6` →
`Halcyon 6: Starbase Commander`, `GTA IV` → `Grand Theft Auto IV`,
`LostWinds 2` → `LostWinds`). Catching those needs containment or
suffix-stripping matching, which *can* pair a game with the wrong entry, so it
was left as a question rather than assumed.

## Match review round (2026-09-17) — complete

Bobby: *"Make a review page for unmatched games … the other the most likely match
from the IGDB that you can find … Add a save button so I can do it in sessions
instead of all at once. After it's done, I'll have you go through and true up
based on those."*

This answers the question `PRODUCT-6` left open — how far matching may loosen —
by moving the decision out of the matcher and into the owner's hands: the search
may be as loose as it likes because **nothing it finds is ever applied**.

- **`MATCH-001`** — `/match-review`, behind session auth. Left is the game as
  listed today (cover, title, store badges); right is the best IGDB candidate
  (cover, name, year, confidence, plus a warning when that IGDB entry is already
  matched to another game). 25 pairings a page, candidates first.
- **The verdict is three-valued** — `NULL` undecided / 1 correct / 0 wrong — which
  is why the control is a radio pair rather than one checkbox: an untouched row
  and a rejected row must be distinguishable, or the queue could not be resumed
  across sittings. Every row submits its id, so a cleared radio writes `NULL`
  back instead of keeping the old answer.
- **`sync.FindMatchCandidates`** searches IGDB and stores the best-ranked result
  with a confidence label: exact → prefix → contains → word overlap → weak. Two
  guards keep the loose tiers honest — partial-title tests need **two words** on
  the shorter title (without it `Diablo` ranked `Diablo IV: Season of Divine
  Intervention` as a prefix, and the tier fell 101 → 74 once guarded), and a token
  counts as a roman numeral only if its canonical spelling round-trips.
- **Nothing is applied.** It writes only to `match_review`; `games`, `igdb_id`
  and artwork are untouched. `UpsertMatchCandidate` refuses to overwrite a decided
  row and the search skips decided games, so it is safe to re-run.
- **`DEPLOY-009`** — live, image **`afc4167e49b2`** (previous `310c30aa044f`).
  `match_review` is created on startup from `schema.sql`, so the deploy is the
  migration. Seeded through the same function the page's button calls: **328 rows,
  185 with a candidate**, 0 errors — prefix 74 · weak 64 · word overlap 35 ·
  contains 12 · exact 0 · none 143. `exact` is 0 by construction, since an exactly
  matching title would already have been matched by enrichment.

**The button path through HTTP is the one thing not exercised** — the seed called
`FindMatchCandidates` directly, because `/match-review` sits behind session auth
with no non-interactive trigger. The same shape of limit as `DEPLOY-006` and
`DEPLOY-008`. Pressing **Find candidates** once closes it, and it is safe to press
at any time.

## Match review round, fourth pass (2026-09-17) — complete

Bobby: *"I went through several pages. A lot of these I would need to inspect one
by one, it may take awhile."* That is a report of friction, and the measurement
behind it was decisive: **143 of the 242 undecided rows had no candidate at all**,
so the right column said `No candidate found` and ruling on them meant leaving the
page. None of those 143 had been started. Of the rest, `tokens` was running 18 yes
to 1 no (nearly all safe) while `weak` was a coin flip at 15/21.

Bobby chose **inline IGDB search** and **more candidate evidence**, and declined
keyboard shortcuts and a bulk-accept, so neither was built.

- **`MATCH-002`** — `match_review` gains `summary`, `genres`, `platforms` and
  `igdb_url`, filled when a candidate is found. Fetched at render time, 25 rows a
  page would have meant 25 IGDB calls per page view; stored once, the page stays a
  single database read. Display is bounded to keep a row readable — summary cut at
  **280 characters** on a word boundary, **4** platform names plus a `+N more`.
- **Every row has an IGDB search box** — open by default where there is nothing to
  compare, collapsed behind *"Wrong game? Search IGDB"* where a candidate exists.
  Finding a game by hand is now a single interaction, which is what the 143 rows
  needed. This is the one place a loosened matcher is not required: the owner does
  the matching.
- **Picking a result is the verdict, and saves at once.** It stores the candidate
  and marks the row yes, labelled `you picked this` — the click is already an
  unambiguous instruction, and a row resolved that way should not depend on the
  page being saved first. **Save** still governs only the yes/no radios, so the two
  paths cannot fight. This deliberately overrides `MATCH-001`'s
  never-overwrite-a-decided-row rule, using an unconditional write.
- **`DEPLOY-010`** — live, image **`85455eb667b3`** (previous `afc4167e49b2`). The
  startup migration added the four columns; **37 server `.go` files, 37 local,
  none only-on-server** before syncing. Re-seeded: 242 searched, 99 with a
  candidate, 0 errors.
- **The re-seed moved nothing.** All **242 undecided pairings are byte-identical**
  before and after, and the verdict counts are unchanged at 50 yes / 36 no / 242
  undecided. Adding `platforms.name` to the fetched fields does not reach the
  scoring function, so re-running the search cannot shift a candidate under an unruled row.
- Live now: **99 of 99** undecided candidates carry an IGDB URL, 92 a summary, 94
  genres, 95 platforms.
- **`MATCH-003`** — the 86 decided rows were initially left without evidence, then
  filled on Bobby's word once it was clear the Yes/No tabs get revisited during the
  true-up. Not by relaxing the skip (that would re-rank candidates under his
  verdicts) but by looking each candidate up **by the `igdb_id` already stored** and
  updating **only** the four evidence columns. 86 filled, 0 failed. Every
  non-evidence column was diffed across all **328** rows before and after and is
  identical, so the split stays 50 yes / 36 no. **All 185 candidates now carry
evidence.** No application code changed, so there was nothing to deploy.

**A false alarm worth remembering.** The first post-deploy route check returned
`404` for `/library` and `/match-review`, which looked like a regression; the
responses carried `server: uvicorn`. Port **8080** on Atlas is another service —
Nisaba is published on **8090**.

## Match review round, fifth pass (2026-09-17) — the true-up, complete

Bobby ruled the three questions `PRODUCT-7` left open: a **yes** links and
re-enriches, a **no** tries to match again, and it runs **now** for the verdicts
already given.

- **`MATCH-004`** — `sync/match_apply.go`. A yes writes through `enrichFromIGDB`,
extracted from `EnrichLibrary` so a hand-applied match cannot drift from a
  searched one: IGDB id, artwork, summary, release date, developer, publisher,
  genres, and `enrichment_status = 'matched'` (which removes the game from the
  queue). A no records the rejected id in `match_review_rejections`, searches
  afresh **excluding it**, and returns the row to undecided — and never writes to
  `games`, verified as **0 of 36** rows changed.
- **The exclusion is the whole mechanism.** The search is deterministic, so
  without remembering the rejection a re-match would offer back the exact
  candidate it was just told to discard. A deliberate manual pick still overrides
  a rejection.
- **Re-runnable by design.** Bobby applied the verdicts already stored rather than
  waiting, so a second press must be harmless: it is, and reports *"No verdicts to
  apply."*
- **`DEPLOY-011`** — live, image **`912505fd941d`** (previous `85455eb667b3`).
  Startup migration created the rejections table. Run result: **50 linked and
  re-enriched, 31 re-matched, 5 stranded, 0 errors**; games holding an IGDB id
  **3060 → 3110**; queue **242 → 278 undecided**; 36 rejections recorded; **0**
  rows whose stored candidate equals a rejected id; `integrity_check ok`.
- **Status wiring fixed en route.** The Find candidates button from the fourth
  pass could never have worked: its `hx-target` matched no element, and the shared
  status partial polled a hardcoded `/sync/status`. Both fixed and the new button
  built on the corrected pattern. That was my error, not a pre-existing one.
- **The honest limit of a no.** Because the rejected candidate was the
  *best*-scoring one, its replacement is by construction lower-ranked and in the
  sample often worse — *Against the Storm* went from `Metal Storm` to *Life is
  Strange: Before the Storm*, and *Batman: Arkham Knight* from one skin DLC to
  another. **A no does not find the right match**; it removes the wrong one and
  returns the row to the owner, whose inline search box is what actually resolves
  it. Raised with the data attached, not shipped as a matcher improvement.

**A second finding, unfixed on purpose:** `enrichment_queue` has **no consumer** —
grepping the table returns only its `INSERT` and a `COUNT`. The older `/review`
page's manual match therefore sets `igdb_id`, marks the game `manual` (which takes
it out of the pool `EnrichLibrary` selects from), and enqueues into a queue nothing
drains. **`/review` links games without enriching them.** Recorded in `OPEN.md`
rather than fixed, because whether to delete the machinery or build the missing
consumer is a ruling.

## Match review round, sixth pass (2026-09-17) — search reach, shortlists, dead queue

Bobby adopted all three recommendations in full, caveats included.

- **`MATCH-005`** — the rows with no candidate were a *search* failure, not a
  scoring one. IGDB's `search` is literal and returned **nothing** for `Batman:
  Arkham Asylum GOTY Edition`, `Baldur's Gate: The Original Saga`, `Astebreed:
  Definitive Edition`, `BloodNet (FDD version)` and 144 more, while the undecorated
  title found the game every time. Decoration (parentheticals, bundle tails,
  edition suffixes) is now stripped **from the query only** — scoring still sees the
  stored title and `bestMatch` still demands exact equality, so a wider search can
  reach a match but never re-rank one. Pinned by 28 unit tests, 17 of them real
  library titles. **Gain: 38 of the 148** rows, 130 → **168** with a candidate.
- **`MATCH-006`** — the scorer's ordering past the first hit is not trustworthy
  (0 of 31 rejections ever yielded a better-tier replacement), so it no longer
  decides for the owner. `match_review_candidates` stores the ranking; a row shows
  its current candidate plus up to **2 alternatives**, each one click to take; and a
  no **promotes locally** from the stored list with no API call. 501 candidates
  across 168 games; **117 rows** have alternatives to show.
- **`MATCH-007`** — `enrichment_queue` was write-only and had **three** call sites,
  not one: `/review`'s match linked without enriching, the **`Rehydrate`** button
  answered "Queued." and did nothing, and manual game add linked without fetching.
  Measured 0 rows and 0 games marked `manual` before deleting, so nothing was lost.
  All three now enrich via the shared `enrichFromIGDB` path.
- **`DEPLOY-012`** — live, image **`f20da8a75091`** (previous `912505fd941d`).
  Migration confirmed: `match_review_candidates` created, `enrichment_queue` gone.
  Re-seed: 278 searched, 168 with a candidate, 0 errors, **0 rejected entries still
  offered**.

## Match review round, seventh pass (2026-09-17) — the search box was asking the wrong question

Bobby reported that searching `Against the Storm` in the review page's box returned
the wrong game while IGDB's own website found it.

- **`MATCH-008`** — the game was in IGDB the whole time and our query was not
  reaching it. Three defects in the query layer, all measured against the live API:
  `limit 5` truncated it away (the exact entry sat at position 10 of the
  platform-filtered ranking, 14 unfiltered); IGDB's `search` is a **conjunction over
  every term matched against the summary as well as the name**, so `Against the
  Storm` returned *Life is Strange: Before the Storm* (their blurb says "against")
  and `Fallout 2: A Post Nuclear Role Playing Game` returned **nothing at all**; and
  `where platforms = (6)` reshaped the ranking rather than merely narrowing it. A
  search is now a **name-anchored** wildcard lookup merged ahead of the ranked one,
  25 rows each, with a subtitle retried on both of its possible heads. **Exact matches
  1 → 10 and usable candidates 7 → 48** across the 110 rows that had none.
- **`MATCH-008`, second pass** — asked what the rows that *still* found nothing had
  in common, and twelve were one shape: a store series marker in the middle of the
  title, `Heroes Chronicles [Chapter 1] - Warlords of the Wasteland` (×8) and
  `Leisure Suit Larry (VGA)` (×2). Only a *trailing* bracket group was ever stripped.
  Now any group is, and **10 of the 67** gained a candidate; two more are explained
  rather than missed — the search offers `MDK 2 HD` and `Battle Isle 2220`, both of
  which the owner had already **rejected**, so the rejection memory is holding them
  out. Of the 30 rows with a candidate and a bracket, **0 scored worse**. A de-spaced
  anchor (`MDK 2` → IGDB's `MDK2`) was measured at **1 row of 67** and deliberately
  **not built** — the box finds it when typed, and the form would cost 270ms on every
  search.
- **`MATCH-008`, third pass** — asked what IGDB knows about the rows that match
  nothing under their own name, and the answer is an **alternative-names index** on
  its own endpoint. Asking it recovered **10 of the 55** — three **exact**
  (`UBERMOSH:BLACK` is IGDB's `Ubermosh: Black`) plus `Grand Theft Auto V Legacy`,
  `Tomb Raider I-III Remastered`, `Brewmaster`, `Minecraft for Windows`, `Uplink`.
  It is tried only when every name-based form fails, so two requests are paid only by
  rows that would otherwise come back empty. Two of the ten are wrong — the store's
  `Football Manager 2024 Pre-game editor` and `Resource archiver` are *tools* that
  resolve to the game at 0.85 — the prefix tier again.
- **The scorer question in `OPEN.md` was measured against the owner's own verdicts**
  rather than argued: the shipped scorer ranks the confirmed entry first in **39/50**,
  a coverage-based reshuffle in **43/50** — but that reshuffle also swaps in
  wrong-but-longer names on 16 queue rows (`Temple of Elemental Evil` →
  `Dungeons & Dragons Online: The Temple of Elemental Evil`; `Deus Ex GOTY` →
  `Deus Ex: Human Revolution`), and every option leaves the same 12 rejections ranked
  first. **Recommendation: change nothing.** `0 of 50` confirmed entries are missing
  from the search results, so the search reaches every answer given and only the
  ordering is imperfect.
- **`DEPLOY-013`** — live in three passes, images **`0bc200e39e63`** (previous
  `f20da8a75091`), **`dd5101c15dbd`**, then **`26889edead64`**. Dry-run first, since a re-seed re-searches
  rows already looked at: 278 rows, 43 gained a candidate, 70 replaced (47 scored
  higher, **0 lower**), 0 lost, 0 errors. Re-seed live in 4m51s: **168 → 211** with a
  candidate, 67 without, shortlists 501 → 753. After the bracket pass, a second
  re-seed in 4m20s: **211 → 221** with a candidate, **67 → 57** without, all 8
  `Heroes Chronicles` rows filled. The 50 decided rows are byte-identical both times.

**Two bugs were caught inside this pass, before deploy.** The client's rate limiting
had to move: a search is no longer one request, and every caller paced itself per
*game* at 4 req/sec, so the change would have collected 429s. And the first version
cut a subtitle at the *first* separator, turning `Warhammer 40,000: Dawn of War II`
into `Warhammer 40,000` and matching **Dawn of War — the wrong game**; the sample
caught it, and the rule is now last-separator-first.

## Next task

None — `DEPLOY-013` closed the seventh pass on 2026-09-17, and
`scripts/spec-next.sh local,atlas,network` prints nothing.

**The queue is ready to keep working through at `/match-review`**: 278 games still
undecided, **231 with a candidate** all carrying evidence, and the **47 with nothing
to compare against searchable in place** — and that search box is now the thing this
round fixed. Of those 47, **45 have nothing resembling them anywhere in IGDB, under
any name**: demos, betas, PTR branches, store tools, launchers, goodie packs and
`Plex`. That is a floor rather than a backlog — they are not games, or are branch
builds IGDB does not model — and the honest options are to hide them from the queue or
leave them. `Against the Storm`, `Fallout 2`, `Dragonview` and
`Heroes Chronicles [Chapter 1] - Warlords of the Wasteland` all resolve to the right
entry. Answers save per page and
survive sessions, and **Apply verdicts** is safe to press at any point. The 50 games
already accepted are linked and fully enriched.

**The scorer is now the limiting factor, not the search, and that is a ruling to
make rather than a tweak to slip in** (`spec/OPEN.md`). A wider search finds more
entries, and the **prefix tier is generous**: when the stored title extends an IGDB
name the extension need not look like a subtitle, so the re-seed replaced some
weak-but-correct candidates with confident-but-wrong ones — `Call of Duty: WaW` →
`Call of Duty`, `STAR WARS™: Rebel Assault 1` → `Star Wars`, `M.A.X.` → `Max`
(exact, via punctuation stripping), and `Tomb Raider (VI): The Angel of Darkness` →
`Tomb Raider` over the correct entry at a lower tier. Measured over the queue, 47 replacements scored
higher and **none scored lower**, so this is the cost of reaching further rather
than a regression — but a confidently wrong pairing costs a click to reject, and the
rejection is remembered.

**Two older gaps are recorded rather than closed, and neither blocks anything.**

**The enrichment wiring is proven only out-of-band.** `DEPLOY-008`'s backfill
called `EnrichLibrary`/`EnrichWishlist` directly, so the functions and the live
result are proven and the **deployed Full-sync step is not**: no Full sync has
run on image `310c30aa044f`. This is the same limit `DEPLOY-006` recorded. And
the 328 unmatched games stay unmatched until a ruling says how far matching may
loosen.

**One observation from the previous round is still outstanding.**
The backfill was run **out-of-band** on Bobby's choice, calling `EnrichWishlist`
directly against the live database — because `/sync/all` sits behind session
auth with no secret-based trigger and the binary has no sync CLI. So the
function and the live result are proven, and **the `WISH-001` wiring is not**:
nothing has yet run the new step inside the container. The first Full sync is
the only thing that can, and it should log `sync-all: step — IGDB wishlist
enrichment` and a `wishlist enriched N/M` summary. Watch for that line
(`evidence/DEPLOY-006.md`).

**Both scheduled runs fired on 2026-09-17 and were observed**, exactly as
predicted — `sync_log` id 87 `pricing` (539 updated) and id 88 `ownership`
(**3 added**, the three games the library view held and the database did not), no
errors on either. `game_stores` `gog` rows went 1037 → **1040** with **992** now
carrying GOG's canonical URL, where the column had never held one; `gog-wish-`
rows are 0. A **Full sync ran at 20:47 on the new binary** (id 89, 677 added) and
recreated none of them, so the retired writer is measurably gone rather than
merely deleted (`evidence/DEPLOY-005.md`).

Two actions remain, both Bobby's, and neither blocks anything:

- **Set `gog.client_secret` in Settings.** It is still unset in the live config,
  so row 88 ran on the fallback: the refresh was never attempted and GOG accepted
  the stored access token whose recorded expiry passed in March. The GOG sync
  keeps working only while GOG keeps honouring that token — the day it stops, the
  sync fails until the secret is set. It is the public Galaxy constant published
  in Heroic's `gogdl`, so it is a paste through the tunnel, not a machine.
- **Re-copy the Playnite script from the Sync page.** `GOGL-006` is in the served
template, but the Windows-side copy still posts GOG until it is replaced. No
Playnite run has happened since `DEPLOY-005`, so this is untested rather than
broken.

The round that preceded this one is **deployed and verified** (`DEPLOY-003`):
the live container reports its next price window as `2026-09-17 11:00` UTC, and
the GG.deals fallback renders for all five of the entries it applies to.

**One thing was scheduled rather than observed, and is now observed:** the first
automatic price run fired at 2026-09-17 11:00 UTC — `sync_log` id 87,
`type='pricing'`, `status='done'`, 539 updated, no error.

The 2026-08-01 gate passed in full (`evidence/REL-001.md`); two gate
lines are
recorded there as partial-with-explanation rather than claimed clean: "nothing
references" the deleted sqlc scaffolding (13 files mention it, all describing the
removal or the superseded design doc) and "no user data was deleted" (the ruled
scorched-earth delete removed pre-cutover *price* rows; no entry, game, or
user-authored field was touched).

A full sync from the UI has now been run: `sync_log` id 89, `2026-09-17 20:47:24`
to `20:59:45`, `done`, 677 added and 543 updated, no error. The earlier cutover
prices arrived via a one-off harness rather than the button; the button path is
now exercised on the deployed build, and it logged to `sync_log` as expected.

## Blockers

None. `OPEN.md` holds three unanswered entries, but nothing derives work from
them, no task is blocked, and the deployed instance is healthy. The live database
passed `quick_check` and `foreign_key_check` after every write in the data
round, and holds no duplicates, no reference-only links and no stale errors.

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
