# MATCH-004 — the true-up: applying saved verdicts

**Status:** done, deployed in `DEPLOY-011`. Ruling: `spec/PRODUCT-9.md`.
Verified on copies before it was allowed near the live library.

## What was built

| file | change |
|---|---|
| `sync/match_apply.go` | new — `ApplyMatchVerdicts`, the only review path that writes to `games` |
| `sync/igdb.go` | `enrichFromIGDB` extracted from `EnrichLibrary` and shared; `bestCandidate` takes a rejection set; the finder honours rejections |
| `db/store.go` | `RecordMatchRejection`, `RejectedIGDBIDs` |
| `schema.sql` | `match_review_rejections` (game_id, igdb_id), created on startup |
| `handlers/match_review.go` | `MatchReviewApply` background job + `MatchReviewApplyStatus` |
| `templates/match_review.html` | **Apply verdicts (N yes, M no)** button, with a confirm |
| `templates/sync_status_partial.html` | `PollURL` — see the bug below |
| `handlers/handlers.go`, `handlers/sync.go`, `main.go` | job state, route, `PollURL` field |

## Acceptance

    go build ./...    # pass
    go vet ./...      # pass
    go test ./...     # pass

`gofmt` adds no new hunks: every touched file matches its `HEAD` hunk count, and
`sync/match_apply.go` is clean.

## Verified on a copy, twice

**The true-up itself** (`sync` package, real IGDB credentials) on a snapshot of the
live database — 50 yes, 36 no:

    result: Applied:50 Rematch:31 Stranded:5 Errors:0
    yes: 50/50 linked to the candidate igdb_id, 50/50 gained artwork, 0 unchanged
    no:  36/36 rejections recorded, 36 returned to undecided, 31 got a different candidate
    counts: {Total:328 Yes:50 No:36 Undecided:242} -> {Total:278 Yes:0 No:0 Undecided:278}

A sample of what a yes wrote: *Brütal Legend* → `igdb 212`, `status=matched`,
`Double Fine Productions` / `Electronic Arts`, description filled.

**The two checks that matter most:**

- **A no never touches `games`.** Every no-verdict game row was snapshotted
  (`igdb_id`, `artwork`, `enrichment_status`) before and after: **0 of 36 changed.**
- **A rejected entry is never offered again.** Each re-matched row's new candidate
  was compared with the id it rejected: **0 rows were offered the same id back**,
  and on the final live state the join between `match_review` and
  `match_review_rejections` returns **0**.

Re-running was checked too: the second pass reports *"No verdicts to apply."* with
`applied=0 rematch=0 stranded=0`, so pressing the button twice is harmless.

**The button path** (`handlers` package) on a second fresh copy, through the
handler the page calls: the page offers the button, pressing it returns the running
status polling its **own** endpoint, the background job completes, and the polls
stop. Result `50 / 31 / 5 / 0 errors`, counts `328 → 278`, **3110** games holding an
`igdb_id` (all marked `matched`, all with artwork), 36 rejections recorded.

## A bug this round found and fixed

`PRODUCT-8`'s **Find candidates** button could never have worked:

- `hx-target="#match-find-status"` matched no element — the shared status partial's
  root is `id="sync-status"`. The response had nowhere to go.
- The partial hardcoded `hx-get="/sync/status"`, so whatever it rendered would have
  polled the *sync dashboard's* status rather than the job's own.

Fixed by parameterising the partial's URL (`PollURL`, defaulting to `/sync/status`
so all ten existing call sites are unchanged) and giving the page a real
`#match-job-status` target. The Apply button was then built on the corrected
pattern. **My error, from the previous round, not a pre-existing one.**

## Also found: `enrichment_queue` is dead code

Grepping every reference to the table returns exactly two: the `INSERT` in
`EnqueueEnrichment` and a `COUNT` in `QueueCounts`. **Nothing dequeues it.** The
older `/review` page's manual match therefore sets `igdb_id`, marks the game
`manual`, and enqueues into a queue that is never read — so it links a game and
never enriches it, despite a comment claiming "the enrichment pipeline handles the
rest".

This is why the true-up could not simply reuse that path, and it is why a yes goes
through `enrichFromIGDB` instead. Left unfixed and raised in `spec/OPEN.md`: whether
to delete the machinery or give it a consumer is a ruling, not a cleanup.
