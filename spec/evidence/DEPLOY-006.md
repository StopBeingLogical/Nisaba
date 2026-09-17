# DEPLOY-006 — deploy the wishlist enrichment and backfill

**Date:** 2026-09-17 · **Commit:** `1ad613f` · **Environment:** atlas (Bobby
confirmed the deploy, and chose the out-of-band backfill over pressing Full sync)

## Acceptance

```bash
$ curl -s -o /dev/null -w '%{http_code}' http://192.168.3.174:8090/
200
$ grep -q 'before' spec/evidence/DEPLOY-006.md   # this file
$ grep -q 'after' spec/evidence/DEPLOY-006.md    # this file
```

## Deployment

Image `sha256:0f0208ad7d11b2585fb5cf7bc0b0aaf0c63c2819e4f5e35b0120e6f4d3b7c518`,
replacing `76bd4b22c8bb` (`DEPLOY-005`). Container up, schedules intact:

```
price sync: next run 2026-09-18 11:00 (in 13h22m0s)
gog library sync: next run 2026-09-18 11:00 (in 13h22m0s)
NISABA running on :8080
```

The new code is in the running binary — `sudo docker exec nisaba grep -ac
'IGDB wishlist enrichment' /app/nisaba` returns **2**, the log line and the
step label from `WISH-001`'s block, which cannot appear in the previous image.
`/`, `/library` and `/wishlist` are 200; `/sync` and `/settings` are 303, the
auth redirect.

### Tree checked before the container was killed

`deploy.sh` removes the container *before* it builds, so a stale file is an
outage rather than a failed deploy (`DEPLOY-004`). Compared before deploying:

- 27 files on the server that the repository lacks, and **every one is an
  AppleDouble `._*` fork** — including `sync/._gog_wishlist.go`, the fork of the
  file `GOGL-004` deleted. Go ignores names beginning with `.` or `_`, so none
  can break a build. No stale *real* `.go` file was outstanding.
- `func EnrichWishlist` is defined exactly once on the server (`sync/igdb.go:492`),
  so the rsync did not leave a duplicate.

## The backfill

Bobby chose to run this out-of-band rather than press Full sync, so the
enrichment was run directly against the live database by a temporary program
(removed after use, never committed). **Read the scope limit below before
treating the wiring as proven.**

```
BEFORE with_art=280 with_igdb_id=280 needs_review=396
rawg fallback available: false
  progress  50/396 matched= 40 errors=0
  progress 100/396 matched= 84 errors=0
  progress 150/396 matched=127 errors=0
  progress 200/396 matched=171 errors=0
  progress 250/396 matched=213 errors=0
  progress 300/396 matched=259 errors=0
  progress 350/396 matched=306 errors=0
RUN processed=396 matched=343 errors=0
AFTER with_art=622 with_igdb_id=623 needs_review=53
```

| | before | after |
|---|---:|---:|
| `wishlist_entries` | 676 | 676 |
| with artwork | **280** | **622** |
| missing artwork | 396 | **54** |
| with `igdb_id` | 280 | 623 |
| `needs_review` | 396 | 53 |
| `games` | 4101 | 4101 |
| `sync_log` rows `running` | 0 | 0 |

**343 of 396 matched, no errors, in about 100 seconds.** Coverage went from
41% to **92%** of the wishlist. The 54 that remain are 53 unmatched and 1 IGDB
record with no cover; only four carry an edition, remaster or `&` marker, so
these are `bestMatch` misses rather than a suffix-parsing problem — the risk
`PRODUCT-4.md` named and accepted before the change was written. The 1
difference between `with_art` (622) and `with_igdb_id` (623) is a matched record
with an empty `cover.url`, which is correctly marked matched.

### Verified

- `PRAGMA quick_check` → `ok`, before and after.
- **The rendered page matches the database**: `/wishlist` returns **54
  title-only placeholders and 622 covers**, the same two numbers as the query.
- All 622 artwork sources are `igdb` — one consistent source, no mixed-origin
  rows.
- 10 of 10 freshly written cover URLs resolve `200` directly.
- Nothing else moved: `games` is still 4101 and no sync was left running.

### Backups

On Ergaster, before the write: a full `.dump` of `wishlist_entries`
(`/tmp/wish-entries-backup-2026-09-17.sql`, 715 lines) and a JSON snapshot of
the 396 rows' `id`, `igdb_id`, `artwork`, `enrichment_status` and
`last_enriched` (`/tmp/wish-396-before-2026-09-17.json`). Only those fields were
written.

## What this does NOT prove

**The deployed wiring is not exercised by this run.** `WISH-001` added a step to
`SyncAll`, and this backfill called `EnrichWishlist` directly instead — so what
is verified is the function and the live result, not that `POST /sync/all` runs
the new step in the container.

The reason: `/sync/all` is registered inside the authenticated route group
(`main.go:147`) and accepts no secret, unlike `/api/sync/playnite`. The binary's
only CLI flag is `-set-password`. There is no non-interactive trigger.

**So the first Full sync after this deploy is still worth watching.** It should
log `sync-all: step — IGDB wishlist enrichment` and a `wishlist enriched N/M`
summary, and process the 53 `needs_review` entries plus anything the Steam sync
adds. Completion is the *only* evidence for the wiring in production;
`evidence/WISH-001.md` is the evidence for the function.
