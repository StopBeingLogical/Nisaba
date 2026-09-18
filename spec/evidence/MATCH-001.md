# MATCH-001 — side-by-side match review page

**Date:** 2026-09-17 · **Environment:** local (against a `.backup` of the live
database with real IGDB) · **Ruling:** `spec/PRODUCT-7.md`

## Acceptance

```bash
$ go build ./... && go vet ./... && go test ./...
?   nisaba          [no test files]
?   nisaba/db       [no test files]
ok  nisaba/handlers 0.005s
ok  nisaba/sync     0.004s
$ grep -c 'match_review' schema.sql
1
$ grep -q 'MATCH REVIEW' spec/evidence/MATCH-001.md && echo ok
ok
```

## MATCH REVIEW — what was built

| piece | where |
|---|---|
| `match_review` table | `schema.sql` (idempotent, created on startup) |
| store methods | `db/store.go` — list, count, save decisions, upsert candidate, matched-id set |
| candidate search + scoring | `sync/igdb.go` — `FindMatchCandidates`, `scoreMatch`, `tokenOverlap` |
| page, save, find, status | `handlers/match_review.go` |
| routes | `main.go` — `/match-review`, `/match-review/save`, `/match-review/find`, `/match-review/status` |
| template | `templates/match_review.html`, plus a nav entry in `base.html` |

**Three-valued verdict.** `match_review.decision` is `NULL` (undecided) / 1
(correct) / 0 (wrong). That is why the control is a radio pair rather than one
checkbox: an untouched row and a rejected row must be distinguishable, or the
queue cannot be resumed across sittings. Clearing both radios writes `NULL` back,
because every row on the page submits its id and the save path reads the radio
state for each one.

**Nothing is applied.** The page writes only to `match_review`; `games` is never
touched. `SyncAll`, `EnrichLibrary`, `EnrichWishlist` and `bestMatch` are
unchanged.

## Verification against real IGDB on a database copy

The handler needs a session, so it was driven directly rather than over HTTP —
the same code path the router calls.

| check | result |
|---|---|
| `GET /match-review` (no candidates yet) | **200**, 74 KB, contains the form action, `game_ids` field and Find button |
| filters `todo` / `yes` / `no` / `all` | all **200** |
| save `yes` → DB | `decision = 1` |
| clear → DB | `decision = NULL` |
| `POST /match-review/save` | **303** → `?filter=todo&offset=0&saved=1`, and the row read back as `0` |
| candidate search | **185 of 328** games got a candidate, **0 errors**, 120s |
| page after search, `filter=todo` | a full 25 rows, all showing a candidate |

Confidence spread after the two guards below were added:

| label | games |
|---|---:|
| no candidate | 143 |
| prefix | 74 |
| weak | 64 |
| word overlap | 35 |
| contains | 12 |
| exact | **0** |

`exact` is 0 by construction — an exactly-matching title would have been matched
by the enrichment pass, so only variants remain.

## The two guards that cut noise

1. **Partial-title tests need two words on the shorter side.** Without it a
   one-word title swallowed everything beginning with it: `Diablo` ranked `Diablo
   IV: Season of Divine Intervention` as a **prefix** match. With the guard the
   prefix tier fell **101 → 74** and those cases dropped to `weak`.
2. **A candidate must be a real roman numeral to be read as one**, decided by
   round-tripping the canonical spelling, so `mix` and `did` are left alone while
   `VI` becomes `6`.

Rows that have a candidate also sort first, so a page of the queue is always
actionable rather than padded with `no candidate` rows.

## The background job

Exercised by calling the handler directly with all but three games pre-marked
decided, so the run was seconds rather than minutes:

```
POST /match-review/find -> 200, 667 bytes of status
  status poll  0: running=true progress=0/0
  status poll  1: running=false Searched 3 games — 1 with a candidate, 0 errors.
rows the job stamped as searched: 3 (expected 3)
second POST while idle -> 200
```

So the goroutine, the status transition, the progress fragment the page polls, and
the DB writes all behave. The only part not exercised is `chi`'s routing and the
auth middleware in front of it, which are shared with every other page.

## Not done, deliberately

**No verdict is applied.** A `yes` does not set `igdb_id`, fetch artwork, or touch
the game. The true-up is a separate pass, and it needs its own ruling on what a
`yes` should do (link only, or link and re-enrich) and what a `no` should do.
