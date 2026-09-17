# Current specification state

**Updated:** 2026-09-16

## Current milestone

**The 2026-08-01 round is complete in production.** All 21 tasks are `done` with
evidence. Both changes landed: library responsiveness (5.63s → 0.081s, target
<1s) and real storefront names in price data and the UI, with GG.deals kept as a
comparison source. The gate is recorded in `evidence/REL-001.md`.

The follow-up round closed with `DEPLOY-003` and the **GOG round is now open**
(below). `OPEN.md` has one unanswered entry — the `sync_log` constraint defect
no task may derive from.

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

**Caveat until the deploy:** the running binary is the pre-round build, so a
Full sync still recreates those entries — the DELETE is idempotent, but the rows
come back until `DEPLOY-004` replaces the writer.

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

## Next task

None. The round is complete and deployed. Two things are **scheduled or pending
rather than observed**:

- **The first runs at 2026-09-17 11:00 UTC.** Watch for a `sync_log` row with
  `type='ownership'`, and expect `games_added` 3 for the GOG library.
- **`gog.client_secret` is not set in the live config.** The first run will try
  to refresh, fail with "GOG client secret is not set", and fall back to the
  stored token, which GOG still accepts — so the run should still succeed, but
  the fallback only holds while GOG keeps honouring that token. Set the secret
  in Settings; it is the public Galaxy constant published in Heroic's `gogdl`,
  not something Praxis holds.

The round that preceded this one is **deployed and verified** (`DEPLOY-003`):
the live container reports its next price window as `2026-09-17 11:00` UTC, and
the GG.deals fallback renders for all five of the entries it applies to.

**One thing is scheduled rather than observed:** the first automatic price run
has not happened yet — it is 2026-09-17 11:00 UTC. Verify by looking for a
`sync_log` row with `type='pricing'`, `status='done'`, ~500 in `games_updated`.
The mechanism itself is proven in `evidence/PRICE-001.md`.

The 2026-08-01 gate passed in full (`evidence/REL-001.md`); two gate
lines are
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
