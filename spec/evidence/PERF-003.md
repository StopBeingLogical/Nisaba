# PERF-003 — Isolate the dominant cost in the library query
**Date:** 2026-09-16 · **Commit:** `dd5ffc9` · **Environment:** local
**Discovery task:** no source changes (`git diff --quiet -- db handlers main.go` → exit 0).

## Acceptance
```bash
$ grep -q 'EXPLAIN QUERY PLAN' spec/evidence/PERF-003.md     # present below
$ grep -q 'Dominant cost' spec/evidence/PERF-003.md          # present below
$ git diff --quiet -- db handlers main.go
exit=0
```

## Method
1. Time each of the library handler's eight store calls against `nisaba-perf.db`
   with the C `sqlite3` CLI (`.timer on`).
2. Run the real binary locally against the same DB copy and time `/library`,
   including the HTMX partial path and later pages.
3. Run the exact `ListGames` SQL through the app's own driver
   (modernc.org/sqlite v1.33.1, same DSN options) in a throwaway program outside
   the repo, isolating one clause at a time.

## Step 1 — every handler query is milliseconds (C sqlite3, `.timer on`)

| Query | Time |
|---|---|
| `CountMatchingGames` (`COUNT(*)` over the same WHERE) | 0.001s |
| `ListGames` full, `LIMIT 200` | 0.003s |
| `ListGames` full, `LIMIT 200 OFFSET 200` | 0.002s |
| `ListGames` full, `LIMIT 200 OFFSET 4000` | 0.009s |
| `ListGames` bare columns, `OFFSET 4000` (no subqueries) | 0.003s |
| `StatusCounts` | 0.005s |
| `StoreCounts` | 0.007s |
| `TopGenres(15)` | 0.006s |
| `AllTags` | 0.002s |
| `SteamDeckCounts` | 0.001s |

Total SQL work for a page render ≈ 30ms. Both hypotheses in `ASSESSMENT.md` die
here: the three `GROUP_CONCAT` subqueries and the `OFFSET` scan are not slow.

`EXPLAIN QUERY PLAN` for the full `ListGames` query at page 1:

```
|--SEARCH g USING INDEX idx_games_visible (is_hidden=? AND parent_id=?)
|--CORRELATED SCALAR SUBQUERY 1
|  `--SEARCH game_genres USING COVERING INDEX sqlite_autoindex_game_genres_1 (game_id=?)
|--CORRELATED SCALAR SUBQUERY 2
|  `--SEARCH game_tags USING COVERING INDEX sqlite_autoindex_game_tags_1 (game_id=?)
|--CORRELATED SCALAR SUBQUERY 3
|  `--SEARCH game_stores USING INDEX idx_game_stores_game_id_owned (game_id=? AND owned=?)
|--CORRELATED SCALAR SUBQUERY 4
|  |--SEARCH gs2 USING INDEX idx_game_stores_owned (owned=?)
|  `--SEARCH g2 USING INDEX sqlite_autoindex_games_1 (id=?)
`--USE TEMP B-TREE FOR ORDER BY
```

Every subquery is index-backed. The plan gives no reason for a 2.5s page.

## Step 2 — the same app, same data, run locally

Local binary against the production DB copy (`nisaba-perf.db`), `curl -w
'%{time_total}'` and the app's own log line:

```
library page 1  -> 2.554s   (log: 2.561002159s)
library page 2  -> 4.177s   (log: 4.203148986s)
library page 11 -> 10.659s
library page 21 -> 12.099s
htmx  page 1    -> 2.515s   (grid partial only: no base.html, no baseData, no sidebar queries)
wishlist        -> 0.289s
dashboard       -> 0.005s
```

The partial path returns the same 200 cards at the same cost, so the shell,
`baseData`, and the sidebar queries are not involved. Page 2 returns *fewer*
bytes than page 1 (247,561 vs 273,268) and takes 1.6s longer, so this is not
response size — it grows with `OFFSET`. Atlas serves page 1 in 5.41–5.66s
(`PERF-002`), about 2.1× this machine on the same statement shape, so the host
is not the difference.

## Step 3 — clause isolation through the app's own driver

Same SQL, same database file, same driver version and DSN options, `LIMIT 200`:

| Variant | offset 0 | offset 4000 |
|---|---|---|
| A — full query, 19 columns, rows scanned | **2.483s** | **11.900s** |
| C — 15 bare columns, no subqueries | 0.006s | 0.015s |
| D — the 3 `GROUP_CONCAT` subqueries, no `EXISTS` | 0.007s | 0.023s |
| E — the `multi_store_owned` `EXISTS` alone | **2.484s** | **11.764s** |

(A `SELECT COUNT(*)` wrapper over the same statement measured 3–8ms at every
offset earlier in this session.)

## Dominant cost

The `multi_store_owned` `EXISTS` subquery in `ListGames`
(`db/store.go`, the `CASE WHEN g.igdb_id IS NOT NULL AND EXISTS (...)` arm). It
accounts for 2.48s of the 2.48s page-1 total and 11.76s of the 11.90s total at
offset 4000 — effectively all of it. The three `GROUP_CONCAT` subqueries cost
7–23ms total; the `OFFSET` walk costs about 10ms.

The same statement finishes in 9ms under C SQLite on the same file with the plan
above, so the cost is in the Go driver's execution of this subquery shape rather
than in the schema, the indexes, or the SQL text.

## What this changes

- Both `ASSESSMENT.md` hypotheses are refuted by measurement, which is what
  `PRODUCT.md`'s ordering rule exists to catch.
- The problem is now a single isolated target with a reproducible harness, so
  `PERF-004` can be written against a named cause instead of a guess.
- Nothing here says the `EXISTS` is *semantically* needed or cheap to express
  differently — that is `PERF-004`'s recommendation and Bobby's ruling.

## What this does not prove

- Why the driver is slow. I measured the boundary between SQLite and the driver,
  not the cause inside modernc.org/sqlite. The 12s is arithmetically consistent
  with a ~4,300-row `game_stores` scan redone for each of the ~4,033 rows the
  statement walks (~17M operations), but that is a hypothesis about driver
  internals, not a measurement.
- Whether Atlas's 2.1× factor is hardware, disk, or load — not investigated.
- Whether other pages or filters behave differently: only pages 1, 2, 11, and 21
  and one search were measured.
