# PERF-005 — Implement the ruled library performance fix
**Date:** 2026-09-16 · **Commit:** `cb8d373` (parent; the change lands with this file) · **Environment:** local

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
ok  	nisaba/sync	(cached)
exit=0
```

## What changed

`db/store.go`, `ListGames`, the `multi_store_owned` arm — the cost that
`PERF-003` isolated and `PERF-004` recommended replacing. One expression:

```diff
     CASE WHEN g.igdb_id IS NOT NULL AND EXISTS (
         SELECT 1 FROM games g2
-        JOIN game_stores gs2 ON gs2.game_id = g2.id AND gs2.owned = 1
-        WHERE g2.igdb_id = g.igdb_id AND g2.id != g.id
+        WHERE g2.igdb_id = g.igdb_id AND g2.id != g.id
+          AND EXISTS (SELECT 1 FROM game_stores gs2 WHERE gs2.game_id = g2.id AND gs2.owned = 1)
     ) THEN 1 ELSE 0 END AS multi_store_owned
```

Nothing else moved. `main.go` is in this task's declared scope and was not
touched — no route, template, filter, sort, or rendered field changed, so the
library-view contract's behaviour is preserved by construction and verified by
row-output comparison in `PERF-006`.

A `db/` changelog one-liner was added to `.changelog/UNRELEASED.md`.

**Bobby's ruling (2026-09-16)** was "the goal is to optimize the responsiveness of
the site, so whatever serves that goal" — this is the approach `PERF-004` measured
as serving it, at 2.53s → 0.011s for the query with output proven identical.
