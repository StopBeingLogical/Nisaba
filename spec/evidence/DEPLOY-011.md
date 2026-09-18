# DEPLOY-011 — the true-up deployed, and the first real verdicts applied

**Status:** done. Task: `MATCH-004`.

## Deploy

Server tree checked first: **37 `.go` files before the sync, 38 after** (the new
`sync/match_apply.go`), with no file present only on the server — so the rsync
without `--delete` had nothing stale to leave behind. Dry run listed exactly the
intended files.

New image **`912505fd941d`**; previous **`85455eb667b3`** kept for rollback.
Container came up clean. The startup migration created the new table on live:

    CREATE TABLE match_review_rejections (
        game_id     TEXT    NOT NULL REFERENCES games(id) ON DELETE CASCADE,
        igdb_id     INTEGER NOT NULL,
        rejected_at TEXT    NOT NULL DEFAULT (datetime('now')),
        PRIMARY KEY (game_id, igdb_id)
    );

Routes: `/` 200 · `/library` 200 (0.073s) · `/wishlist` 200 (0.333s) ·
`/match-review` 303 · `/match-review/apply/status` 303 — session redirects, as
expected. (Nisaba is on **8090**; 8080 on Atlas is another service.)

## The verdicts applied

Backup first: `/tmp/nisaba-backup-2026-09-17e/nisaba-pre-trueup.db`. The button
cannot be pressed without a browser session, so the run called the same function
the button calls:

    50 linked and re-enriched; 31 rejected and re-matched, 5 with nothing else found; 0 errors

| | before | after |
|---|---:|---:|
| games | 3388 | 3388 |
| games with an `igdb_id` | 3060 | **3110** |
| games with artwork | 3388 | 3388 |
| match_review undecided | 242 | **278** |
| match_review yes | 50 | 50 (record only — these games left the queue) |
| match_review no | 36 | **0** |
| rejections recorded | 0 | **36** |
| rows whose candidate equals a rejected id | — | **0** |

The 50 accepted games left the review queue because `enrichFromIGDB` marks them
`matched`; their `match_review` rows stay as the record of the decision, which is
why the yes count is unchanged while the queue total fell by 50.

`PRAGMA integrity_check` returns `ok`, with no foreign-key violations.

Spot-checks of applied matches, read back from the library:

    Brutal Legend                -> igdb 212   Double Fine Productions / Electronic Arts
    Alone in the Dark 1          -> igdb 1956  Infogrames / (multiple publishers)
    BioShock Infinite Complete   -> igdb 41595 Irrational Games / 2K Games

Each also carries the IGDB description, confirming the re-enrichment ran rather
than just the link.

## Not proven

The **button over HTTP** is still the untested sliver — same shape of gap as
`MATCH-001`/`MATCH-002`, because the page sits behind session auth with no
non-interactive trigger. What *was* exercised is the handler itself, its background
job, its status polling and every store write, by calling it directly against a
copy (`MATCH-004`). Pressing it in a browser once closes the gap, and it is safe to
press: a re-run with no outstanding verdicts does nothing.

## Runner cleanup

Removed from the container, the host and the working tree; never committed.
Local `tools/` removed entirely.
