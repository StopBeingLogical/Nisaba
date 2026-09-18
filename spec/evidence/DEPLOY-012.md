# DEPLOY-012 — search reach, shortlists and the queue removal shipped

**Status:** done. Tasks: `MATCH-005`, `MATCH-006`, `MATCH-007`.

## Deploy

Server tree checked first: **38 `.go` files before, 40 after** (the new
`sync/enrich_single.go` and the permanent `sync/igdb_search_test.go`), with no file
present only on the server.

New image **`f20da8a75091`**; previous **`912505fd941d`** kept for rollback.
Container up clean.

Routes: `/` 200 (0.009s) · `/library` 200 (0.073s) · `/wishlist` 200 (0.328s) ·
`/match-review` 303 — session redirects, as expected. Nisaba is on **8090**.

**Startup migration, confirmed on live:**

    sqlite_master → match_review_candidates     (created)
                 → enrichment_queue             (ABSENT — dropped)

## Re-seed

Backup first: `/tmp/nisaba-backup-2026-09-17f/nisaba-pre-shortlist.db`. The button
cannot be pressed without a session, so the run called the same function it calls:

    searched 278 — 168 with a candidate, 0 errors

## Live state

| | before | after |
|---|---:|---:|
| undecided rows | 278 | 278 |
| undecided **with a candidate** | 130 | **168** |
| undecided without | 148 | **110** |
| stored candidates | — | **501** across 168 games |
| **rows showing alternates** | — | **117** |
| stored candidates that are rejected | — | **0** |
| flagged `already in library` | 22 | 22 |

`PRAGMA integrity_check` returns `ok`; no foreign-key violations. Verdict counts
untouched.

## Still not proven

Both buttons over HTTP — same gap as `MATCH-001`/`MATCH-002`/`MATCH-004`. The
handlers, their background jobs, their polling and every store write were exercised
directly against copies; what is untested is chi's routing plus the auth middleware
in front of them.

Worth pressing once each, and both are safe to repeat:

- **Find candidates** — re-seeds the queue and shortlists without touching a
  decided row.
- **Apply verdicts** — applies whatever is outstanding and does nothing when
  nothing is.
- **Rehydrate** on a game page — now does real work; previously it only said
  "Queued."

## Runner cleanup

Removed from the container, the host and the tree; never committed. Local `tools/`
removed entirely.
