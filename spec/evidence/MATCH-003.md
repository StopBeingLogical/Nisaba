# MATCH-003 — evidence for the rows already ruled on

**Status:** done. Ruling: `spec/PRODUCT-8.md` §4. Follows `MATCH-002` / `DEPLOY-010`.

## Why it was needed

The candidate search skips rows already ruled on — the guard that stops a re-run
disturbing an owner's answer. That also meant the **86 decided rows** kept a
candidate with no summary, genres, platforms or IGDB link, so the Yes/No tabs
would show a bare pairing. Bobby asked for those to be filled too.

## Why it needed its own path

Relaxing the skip would have re-searched and re-ranked candidates under the
verdicts — the exact thing the guard exists to prevent. So the repair is narrower:
it reads the `igdb_id` already stored and does an `UPDATE` naming **only** the
four evidence columns.

    UPDATE match_review
    SET summary = ?, genres = ?, platforms = ?, igdb_url = ?
    WHERE game_id = ?

No `bestMatch`, no scoring, no candidate write, no verdict write. `decision`,
`igdb_id`, `igdb_name`, `confidence`, `score`, `in_library`, `cover_url` and
`release_year` are not in the statement.

## Acceptance

    go build ./...     # pass (the runner compiled and ran; it was deleted after use)

Run against a copy first, then live. Both runs: **86 rows found, 86 filled, 0
failed.** One lookup per row, 4 req/s, ~35 s.

## The check that matters

Every non-evidence column was dumped for all **328** rows before and after, as
`game_id|igdb_id|igdb_name|confidence|decision|score|in_library|cover_url|release_year`,
and diffed:

    NO — all 328 rows identical in every non-evidence column, verdicts untouched

The verdict split is unchanged at **50 yes / 36 no / 242 undecided**, and
`PRAGMA integrity_check` returns `ok` with no foreign-key violations.

## Live coverage

| decision | rows | candidates | evidence | summary | genres | platforms |
|---|---:|---:|---:|---:|---:|---:|
| no (0) | 36 | 36 | **36** | 33 | 32 | 34 |
| yes (1) | 50 | 50 | **50** | 49 | 49 | 49 |
| undecided | 242 | 99 | 99 | 92 | 94 | 95 |

**All 185 candidates now carry evidence**, where before this only the 99 undecided
ones did. The shortfalls from 185 are IGDB entries that genuinely carry no summary,
genres or platforms — small remaster and DLC entries — not decode failures. The
first decided row filled reads as expected: *Above Snakes* → *Demon's Sword Snakes:
Sweet Dreams of the Cursed Snake* (`weak`, verdict `no`) now shows its summary,
`PC (Microsoft Windows)` and its IGDB link.

## Notes

- No application code changed, so **nothing was deployed**: the app never performs
  this operation, and the running image is unchanged at `85455eb667b3`. The runner
  was built `CGO_ENABLED=0 GOOS=linux` for the alpine runtime, executed against
  the mounted database, then removed from the container, the host and the tree.
  It was never committed.
- Backup before the write: `/tmp/nisaba-backup-2026-09-17d/nisaba-pre-decided-evidence.db`.
- Re-running it is a no-op: it selects only rows with an `igdb_id` and an **empty**
  `igdb_url`, so the 86 are now skipped.
