# MATCH-007 — the dead enrichment queue removed, and its call sites fixed

**Status:** done. Ruling: `spec/PRODUCT-10.md` §3. Deployed in `DEPLOY-012`.

## What was wrong

`enrichment_queue` was **write-only**: `EnqueueEnrichment` inserted rows and
`QueueCounts` counted them, and nothing anywhere read one to process it. It had
**three** call sites — more than `PRODUCT-9` recorded from the first pass:

| call site | behaviour before |
|---|---|
| `handlers/enrichment.go` — `/review` manual match | linked the game, set `enrichment_status = 'manual'` (which *removes* it from the pool `EnrichLibrary` selects: `WHERE enrichment_status = 'needs_review'`), then enqueued. **Linked, never enriched.** Its comment claimed "the enrichment pipeline handles the rest." |
| `handlers/library.go` — `Rehydrate` | enqueued and answered **"Queued."** The button did nothing at all, and said so convincingly. |
| `handlers/library.go` — manual game add | `SetIGDBMatch` only: the chosen entry was linked but never fetched. |

## Measured before deleting

    0 rows in enrichment_queue
    0 games with enrichment_status = 'manual'

So the bug had never fired — nobody had used `/review`'s match — and removing the
table cost nothing. It was a trap rather than damage, which is why it was the third
item rather than the first.

## What changed

- `sync/enrich_single.go` (new) — `ApplySingleMatch` links a game to an IGDB id and
  enriches it in one call; `RehydrateGame` refreshes by stored id, or by title with
  an **exact** match required.
- `enrichFromIGDB` is now shared by the pipeline, the review true-up and these
  paths, so a hand-applied match cannot drift from a searched one.
- `EnqueueEnrichment`, `QueueCounts` and the `QueueCounts` struct are deleted; the
  table is dropped by an idempotent migration and removed from `schema.sql`.
- All three call sites now enrich directly and report what happened.

## Acceptance

    go build ./...   # pass
    go vet ./...     # pass
    go test ./...    # pass

Surviving references to the dead machinery are comments and the `DROP` only.

## Verified on a copy of the live database

    by id:    "Counter-Strike" -> "Rehydrated from IGDB #266357"
              developer "" -> "Ritual Entertainment" | publisher "" -> "Microsoft Game Studios"
              status "matched"
    by title: "DEFCON Beta Demo" -> "IGDB has no exact match for \"DEFCON Beta Demo\"."
              (igdb_id left empty — it did not guess)
    apply:    "Quake II: The Reckoning" linked to 1029 -> status="matched", developer filled
    invalid id rejected

So `Rehydrate` now fills real metadata, refuses to attach a game it cannot match
exactly, and says so instead of claiming success. A non-numeric IGDB id fails loudly
rather than writing anything.

## Also verified

The migration dropped the table on live: `sqlite_master` now contains
`match_review_candidates` and **not** `enrichment_queue`, and `PRAGMA
integrity_check` returns `ok` with no foreign-key violations.
