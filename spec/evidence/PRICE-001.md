# PRICE-001 — Add the daily price-only sync
**Date:** 2026-09-16 · **Commit:** this commit · **Environment:** local

## Acceptance
```bash
$ go build ./...
exit=0

$ go vet ./...
exit=0

$ go test ./...
?   	nisaba	[no test files]
?   	nisaba/db	[no test files]
ok  	nisaba/handlers	(cached)
ok  	nisaba/sync	0.004s
exit=0
```

## What changed

| File | Change |
|---|---|
| `sync/schedule.go` | new — the loop, the configured hour, the catch-up rule, the price-only run |
| `db/store.go` | `HasRunningSync()` — one existence query against `sync_log` |
| `main.go` | starts the scheduler alongside the store |

The scheduled run calls `SyncITADPricing` then `SyncGGDealsPricing` and nothing
else. Ownership, the Steam and GOG wishlists, reseller scraping and IGDB
enrichment stay on the manual button — they are 14 of the button's 14.6 minutes,
and they only change when a game is bought or added.

Configuration is `app_config` → `sync.price_hour`, an integer 0–23 in the
container's clock, defaulting to **11** when absent or unparsable. The container
runs UTC, so 11 is 07:00 US Eastern. No Settings UI (ruled out of scope).

## Local proof — production database copy, real APIs

Instance 1, with the copy's last price sync dated 2026-06-19 (far older than the
20-hour staleness threshold) and the configured hour already past:

```
2026/09/16 20:54:07 price sync: starting
2026/09/16 20:54:07 NISABA running on :8098
2026/09/16 20:54:15 price sync: done — ITAD 494 updated (0 errors), GG.deals 498 priced (0 errors)
2026/09/16 20:54:15 price sync: next run 2026-09-17 11:00 (in 14h6m0s)
```

```
id  type     status  started_at           finished_at          games_updated  err
86  pricing  done    2026-09-17 00:54:07  2026-09-17 00:54:15  494
```

Eight seconds for the whole price pass, one `sync_log` row of type `pricing`,
and the next run anchored to the configured hour rather than to when the last one
finished.

Instance 2, restarted immediately afterwards (same database):

```
2026/09/16 20:54:19 price sync: next run 2026-09-17 11:00 (in 14h6m0s)
```

No second run. The `pricing` row count went 15 → 16 across both instances — the
restart announced the next window instead of syncing again, which is what the
20-hour staleness check is for.

## Rules the scheduler follows

- **Skips while another sync is running** (`HasRunningSync`), logging why, and
  looks again in 10 minutes. Two writers on one SQLite file is the thing this
  avoids.
- **Skips when `itad.api_key` is unset**, without writing a row — a
  misconfigured install should not fill `sync_log` with daily failures.
- **Catches up after downtime.** If the day's window has passed and the last
  price sync is 20 hours old, it runs at startup instead of waiting another
  cycle. A deploy or a reboot at the wrong hour still gets prices refreshed.
- **A failed run is recorded once and waits for the next window**; the manual
  full sync is the recovery path. Deliberate: a retry loop against a down API
  would burn the request budget and bury the failure in noise.

## What this does not do

- No dedupe of price-history writes. Every run appends a row per priced entry
  (~500 rows/day, ~180k/year). That is the reason the cadence is daily:
  `ListWishlistPriceHistory` reads the last 90 rows per entry, so daily keeps the
  sparkline a three-month chart. `PRODUCT-2.md` records dedupe as the
  prerequisite for any denser cadence.
- No UI for the hour, and no change to when the button-driven full sync runs —
  it still runs pricing inline, so a manual sync and the schedule can both touch
  prices on the same day. Harmless: the pricing pass is ~11 requests.
