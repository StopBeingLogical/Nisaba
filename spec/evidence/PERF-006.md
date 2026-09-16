# PERF-006 — Verify the fix against the local database copy
**Date:** 2026-09-16 · **Commit:** `cb8d373` (parent) · **Environment:** local

## Acceptance
```bash
$ go test ./...
?   	nisaba	[no test files]
?   	nisaba/db	[no test files]
ok  	nisaba/handlers	(cached)
ok  	nisaba/sync	(cached)
exit=0
$ grep -q 'before' spec/evidence/PERF-006.md     # table below
$ grep -q 'after'  spec/evidence/PERF-006.md     # table below
```

## Row output is unchanged

The full 19-column `ListGames` output was dumped for the old and new expression
and diffed, not sampled:

| Filter set | Rows compared | Result |
|---|---|---|
| every game, `ORDER BY g.id` | 4033 | **IDENTICAL** |
| default page 1 (`is_hidden=0`, `parent_id IS NULL`, `sort_title`, `LIMIT 200`) | 200 | **IDENTICAL** |
| search `title LIKE '%a%'` with `LIMIT 200 OFFSET 200` | 200 | **IDENTICAL** |

## Timings, before and after

Same harness both times: the local binary against `nisaba-perf.db` on this
machine, `curl -w '%{time_total}'`. **before** is `PERF-003`'s measurement of the
same binary and data at `d166fe5`; **after** is the rebuilt binary with the fix.

| Route | before | after | change |
|---|---|---|---|
| `/library` page 1 | 2.554s | 0.048s / 0.048s / 0.052s | ~50× faster |
| `/library?page=2` | 4.177s | 0.053s | ~79× |
| `/library?page=11` | 10.659s | 0.070s | ~152× |
| `/library?page=21` | 12.099s | 0.074s | ~163× |
| `/library` via HTMX partial | 2.515s | 0.030s | ~84× |
| `/wishlist` (untouched path) | 0.289s | 0.284s | unchanged |
| `/` dashboard (untouched path) | 0.005s | 0.005s | unchanged |

The `OFFSET` growth is gone — later pages now cost the same as page 1, which is
consistent with `PERF-003`'s finding that the `EXISTS` was being evaluated for
every row the statement walked rather than for the rows returned.

The handler's own timing log is now **silent**: it writes a line only above 100ms
(`handlers/library.go:117-119`), and no `Library page loaded` line appeared in the
local app's log across every request above. The log going quiet is itself the
evidence that the handler is inside its threshold.

## Result

The ruled fix does what `PERF-004` projected, on production-shaped data, with the
rendered rows proven unchanged. Nothing was tuned by feel: the one changed
expression is the one `PERF-003` isolated and `PERF-004` measured at ~2ms over a
no-op control.

## What this does not prove

- **The deployed instance.** Every number here is Ergaster with a copy of the
  production database. Atlas ran ~2.1× slower on the same shape, which would put
  page 1 near 0.1s — still inside the target, but `PERF-007` measures it for real
  after `DEPLOY-001`.
- **Filter combinations not exercised here.** Only the default view, a search, and
  every-row output were compared. Genre, tag, store, play-status, Steam Deck,
  installed, favourites, and the alternate sorts run additional JOINs and were
  not re-measured after the change.
- **Other queries.** Nothing else in `db/store.go` was audited for the same
  `JOIN`-inside-`EXISTS` shape, so any other instance of it is still unexplained.
