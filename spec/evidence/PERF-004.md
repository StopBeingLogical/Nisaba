# PERF-004 — Propose the library performance fix
**Date:** 2026-09-16 · **Commit:** `d166fe5` · **Environment:** local
**Recommendation task:** no source changes (`git diff --quiet -- db handlers main.go` → exit 0).
**Bobby rules before `PERF-005` may start.**

## Acceptance
```bash
$ grep -q 'Recommendation' spec/evidence/PERF-004.md      # below
$ grep -q 'Measured basis' spec/evidence/PERF-004.md      # below
$ git diff --quiet -- db handlers main.go
exit=0
```

## Measured basis

`PERF-003` isolated the cost to the `multi_store_owned` `EXISTS` arm in
`ListGames`: 2.48s of a 2.48s page 1, 11.76s of an 11.90s offset-4000 page. I then
ran candidate rewrites of that one arm through the app's own driver
(modernc.org/sqlite v1.33.1, same DSN options, `LIMIT 200`), each returning the
same 19 columns:

| Variant (that arm only) | offset 0 | offset 4000 |
|---|---|---|
| V1 — current: `EXISTS (… FROM games g2 JOIN game_stores gs2 …)` | 2.529s | 12.112s |
| **V2 — nested `EXISTS`** | **0.011s** | **0.038s** |
| V5 — current shape, join order reversed (`game_stores` → `games`) | 2.487s | 12.007s |
| V3 — precomputed per-igdb store counts via `LEFT JOIN … GROUP BY` | 0.019s | 0.042s |
| control — no `EXISTS` at all (wrong output, upper bound only) | 0.009s | 0.032s |

What that establishes:

- The cost is **a `JOIN` inside a correlated `EXISTS`** under this driver, not
  `EXISTS` itself, not the join order (V5 proves join order is irrelevant), and
  not the number of rows returned.
- V2 leaves ~2ms of overhead over the control, so the flag becomes effectively
  free.
- V1 and V2 are **semantically identical**: both expressions were evaluated over
  all 4033 `games` rows and diffed — `V1 == V2 IDENTICAL over all rows`. 788 rows
  carry the flag as 1; 3537 games have a non-null `igdb_id`; 268 `igdb_id`s are
  shared by more than one game.
- V2's plan stays index-backed, one level deeper:
  `SEARCH g2 USING INDEX idx_games_igdb_id` →
  `SEARCH gs2 USING COVERING INDEX idx_game_stores_game_id_owned`.
- C SQLite does the *current* shape in 9ms on the same file, so the schema,
  indexes, and SQL plan were never the problem — only the driver's execution of
  that shape is.

## Recommendation

Replace the join inside the correlated `EXISTS` with a nested `EXISTS`, and change
nothing else:

```sql
-- db/store.go, ListGames, the multi_store_owned arm
CASE WHEN g.igdb_id IS NOT NULL AND EXISTS (
    SELECT 1 FROM games g2
    WHERE g2.igdb_id = g.igdb_id AND g2.id != g.id
      AND EXISTS (SELECT 1 FROM game_stores gs2 WHERE gs2.game_id = g2.id AND gs2.owned = 1)
) THEN 1 ELSE 0 END AS multi_store_owned
```

Projected effect: page 1 from ~2.55s to roughly 0.05s locally (the flag drops to
~2ms over a no-op), and Atlas from 5.41–5.66s to far inside `PRODUCT.md`'s
under-1-second target. Row output is unchanged, measured over every row rather
than assumed.

**Cost:** one expression in one function. No schema change, no migration, no new
index, no change to routes, templates, or `main.go`. The library-view contract
(filters, sorting, rendered fields) is untouched because the selected columns and
their values are identical.

**Measured alternative, not recommended:** V3 precomputes owned-store counts per
`igdb_id` and is nearly as fast (19ms/42ms), but adds a derived join and a second
correlated count to the hot path for a 1.7× gain the target does not need. Hold it
in reserve if the recommended change regresses under a filter combination that was
not measured here.

## What this does not fix

- **Why the driver is slow.** V2 sidesteps the shape; it does not explain it. Any
  other `JOIN`-inside-`EXISTS` in this codebase is a candidate for the same
  pathology — I did not audit for others.
- The `TEMP B-TREE` sort over the ~4033-row candidate set (~10ms) — not the
  problem, and not addressed.
- Atlas running ~2.1× slower than Ergaster on the same statement shape. Not
  investigated; the projected numbers carry that factor.
- Anything in the GG.deals half of the round.

## Handoff

If ruled as recommended, `PERF-005` is: edit that one expression in
`db/store.go`, run `go build ./... && go vet ./... && go test ./...`, add a
`db/` changelog one-liner. Nothing else. `PERF-006` then re-measures with the
harness described in `PERF-003` — a throwaway program outside the repo running the
statement through the app's own driver, plus the local binary against
`nisaba-perf.db`, since `/library` timing is the only acceptance that matters.
