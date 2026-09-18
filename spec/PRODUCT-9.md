# Round 2026-09-17 (fifth pass) — applying the verdicts

Locked owner requirements. Supersedes `PRODUCT-8.md` where they disagree; that file
stays as the record of what was ruled then.

## The request

The true-up `PRODUCT-7` deferred. Bobby ruled on the three open questions:

| question | ruling |
|---|---|
| what a **yes** does | **Link and re-enrich** — set `igdb_id` and run the full enrichment for that game |
| what a **no** does | **Try to match again** |
| when it runs | **Now, for the 50 verdicts already given** |

## Ruled, 2026-09-17 (fifth pass)

### 1. A yes links the game and re-enriches it, through the pipeline's own write

Not `SetIGDBMatch` (which marks a game `manual`) but the same call the enrichment
pipeline makes, so a match applied by hand is written identically to one found by
a search. The per-game write was extracted into `enrichFromIGDB`, used by both, so
the two cannot drift: IGDB id, cover artwork, summary, release date, developer,
publisher, genres, and `enrichment_status = 'matched'` — which is what takes the
game out of the review queue.

**Ruled constraint:** the true-up is the only path in the review flow that writes
to `games`, and it only ever runs when asked for by name. Everything before it —
candidate search, verdicts, manual picks — still writes only to `match_review`.

### 2. A no means "this is the wrong entry, look again"

The rejected IGDB id is recorded in `match_review_rejections` and can never be
offered again; the game is searched afresh; the best **other** result becomes its
new candidate; the verdict is cleared so the row returns to the undecided queue
with something new to look at. When nothing else is found the row returns with no
candidate and the manual search box open.

**A no never writes to `games`** — verified, not assumed (0 of 36 rows changed).

The exclusion is the whole mechanism. The search is deterministic, so without it a
re-match would offer back the same candidate it was just told to discard.

### 3. It is re-runnable, and a second run is a no-op

Bobby chose to apply the verdicts already stored rather than wait, so the true-up
is built to be pressed repeatedly as the queue is worked. A second run finds no
verdicts and changes nothing — measured on a copy and again on live.

### 4. The re-match produces weaker candidates, and this is stated up front

**Measured, and this is the honest part of the round.** Of the 36 rejected rows,
31 got a replacement and 5 had nothing else to offer. Because the first candidate
was the *best*-scoring one, the replacement is by construction lower-ranked, and
in the sample it is often visibly worse:

| game | rejected | replaced with |
|---|---|---|
| Against the Storm | `Metal Storm` (weak) | `Life is Strange: Before the Storm — Episode 1` (weak) |
| Batman™: Arkham Knight | `Batman: Arkham Knight - 2008 Movie Batman Skin` | `Batman: Arkham Knight - Batman: Noel Skin` |

So a no does not usually *find* the right match — it removes the wrong one from the
running and puts the row back in front of the owner, whose inline search box is
what actually resolves it. That is still worth having (the rejection is remembered
and the row comes back), but it is **not** a matcher improvement, and it should not
be read as one. Raised with the measurements attached; not silently shipped.

### 5. Fixed en route: the Find candidates button never worked

`PRODUCT-8`'s page pointed the button at `#match-find-status`, an id that existed
nowhere (the shared status partial's root is `id="sync-status"`), so the response
had no target. The partial also hardcoded `hx-get="/sync/status"`, so any job's
progress bar would have polled the *sync dashboard's* status instead of its own.

Both are fixed by giving the partial a `PollURL`, defaulting to `/sync/status` so
the ten existing call sites are untouched, and by giving the page a real
`#match-job-status` target. The Apply button was built on the corrected pattern
rather than copying the broken one.

## Not ruled, and now recorded as an open finding

`enrichment_queue` **has no consumer**. `EnqueueEnrichment` inserts rows and
`QueueCounts` counts them; nothing anywhere reads a row to process it. The older
`/review` page's manual match calls `SetIGDBMatch` — which sets
`enrichment_status = 'manual'`, taking the game out of the pool `EnrichLibrary`
selects from — and then enqueues into a queue nobody drains. Its comment claims
"the enrichment pipeline handles the rest"; it does not. That path therefore links
a game and never enriches it.

Left unfixed on purpose: whether to delete the dead queue machinery or give it a
consumer is a bigger question than this round, and it needs its own ruling. See
`spec/OPEN.md`.
