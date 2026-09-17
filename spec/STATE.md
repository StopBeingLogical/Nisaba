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

`OPEN.md` holds **three unanswered entries** — the `sync_log` CHECK still
rejecting `mystery_packs` (`handlers/sync.go:457`), whether the deploy rsync
should use `--delete`, and the three dormant fetchers (`SyncSteamDeckStatus`,
`SyncProtonRatings`, `SyncSteamCrossRefs`) that nothing calls. Nothing may derive
work from any of them; they need a ruling first.

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

## Next task

None — `DEPLOY-008` closed the enrichment round on 2026-09-17, and
`scripts/spec-next.sh local,atlas,network` prints nothing.

**Two gaps are recorded rather than closed, and neither blocks anything.**

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
