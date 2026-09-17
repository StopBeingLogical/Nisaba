# GOGL-003 — sync the GOG library on a daily schedule

**Date:** 2026-09-16 · **Commit:** this commit · **Environment:** local

## Acceptance

```bash
$ go build ./...
build ok
$ go vet ./...
vet ok
$ go test ./...
?   	nisaba	[no test files]
?   	nisaba/db	[no test files]
ok  	nisaba/handlers	(cached)
ok  	nisaba/sync	(cached)
```

Functional check, run through a throwaway test in the `sync` package against a
scratch copy of the database (`/tmp/gog-lib-check.db`, copied from
`nisaba-perf.db`), deleted afterwards:

```bash
$ GOG_DB=/tmp/gog-lib-check.db go test ./sync/ -run TestManualGogLibrary -v
=== RUN   TestManualGogLibrary
    run 1: added 3, skipped 0, errors 0, err <nil>
    run 2: added 0, skipped 0, errors 0, err <nil>
    gog rows: 1037 -> 1040 -> 1040
    gog rows carrying a store url: 992
    ownership sync_log rows (direct call does not write one): 10
--- PASS: TestManualGogLibrary (32.19s)
```

Those 992 are the complete set, not a partial write — a sweep of the library
view returns 992 products with a `url` field and **48 with none**, which is a
property of GOG's payload:

```bash
$ # products with an empty url / products with a url, across all 11 pages
48
992
```

## Result

`sync/gog_library.go` reads `/account/getFilteredProducts?mediaType=1` a page at
a time and upserts games and their gog store links. `sync/schedule.go` gained a
daily job for it and `main.go` starts it.

- **It closes exactly the gap `GOGL-001` measured.** Run one added the 3 products
  the database was missing (`Jazz Jackrabbit 2 Plus`, `State of Mind`,
  `Pyramids and Aliens: Escape Room`); 1037 → 1040 rows against a 1040-product
  library view, and a second run added nothing, so it is idempotent.
- **It reads the library view, not the owned list.** Syncing `/user/data/games`
  would have created ~358 rows for entitlements GOG's own library view hides
  (packs, Prime Gaming and Luna rewards, delisted products) — measured in
  `GOGL-001`.
- **It deduplicates by title before inserting**, the same fallback the Playnite
  sync uses (`CLAUDE.md`), so a game owned on two stores becomes one game with
  two store links rather than a second row.
- **It has its own daily window** (`sync.gog_hour`, default 11 = 07:00 US
  Eastern), separate from the price job: `PRODUCT-2.md` locks that run as
  price-only, so the library pass does not ride on it. It skips when another
  sync is running or when GOG is not configured, and catches up after downtime
  the way the price job does.
- **It logs as `sync_log` type `ownership`**, which the schema allows, so it
  appears in Recent Activity. The database already holds 10 historic `ownership`
  rows, the newest from 2026-04-23, so the staleness check will treat the first
  deploy's window as overdue and run once, then settle into the daily schedule.
- New games are inserted with `ArtworkJSON: "{}"` and their `worksOn` platform
  flags — parity with the Playnite path, which also inserts empty artwork and
  leaves cover art to IGDB enrichment. Existing rows keep their artwork,
  descriptions and playtime untouched; only their store link is re-upserted.
- No comment was added to code this task did not change.

What this proves: the sync's logic, against a copy of the live database.
What it does not: that the scheduler fires on the deployed instance — that needs
a real clock past the window and is `DEPLOY-004`'s to show — and it does not
touch the full-sync button, which still leaves GOG to this job.

## Deliberately not done

`handlers/sync.go`'s full sync was left alone: adding the GOG pass there is a
different task's scope, and its ownership step is Steam-only by design (`GOGL-004`
touches that file for the wishlist retirement only).
