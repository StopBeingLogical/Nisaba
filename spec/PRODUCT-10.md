# Round 2026-09-17 (sixth pass) — search reach, the shortlist, and the dead queue

Locked owner requirements. Supersedes `PRODUCT-9.md` where they disagree; that file
stays as the record of what was ruled then.

## The request

Bobby: *"What are your suggestions for each of the suggested steps"*, then *"Ok, go
for all 3"* — adopting all three recommendations, including the measured caveats
attached to each.

## Ruled, 2026-09-17 (sixth pass)

### 1. Clean the search string; do not touch the scorer

**The finding this rests on:** IGDB's `search` is literal. A decorated title
returns an **empty set**, not partial matches. Probed against live, all of
`Batman: Arkham Asylum GOTY Edition`, `Batman: Arkham City GOTY`, `Baldur's Gate:
The Original Saga`, `Astebreed: Definitive Edition`, `BloodNet (FDD version)` and
`Amerzone: The Explorer's Legacy (1999)` returned zero results, and the
undecorated title found the game every time.

So `searchTitle` now strips store decoration before the query: a trailing
parenthetical or bracket note, a trailing bundle part (` + …`), and edition or
collection suffixes (`GOTY`, `Definitive Edition`, `Complete Edition`, `Deluxe`,
`The Original Saga`, …). Trademarks are removed rather than replaced with a space,
which had been leaving `Batman :`.

**Ruled constraint — the safety argument, stated so it can be checked:** cleaning
happens **only** in the query. Scoring still runs against the title exactly as
stored, and `bestMatch` still demands exact normalised equality. A wider search can
therefore only *reach* a match; it cannot re-rank one, and it cannot cause the
enrichment pipeline to attach a different game. That is pinned by a test
(`TestBestMatchUsesRawTitle`).

**Deliberately not stripped:** variant products that are genuinely different games
— `Christmas Edition`, `Scenery CD`. Stripping those would send the search after
the wrong entry entirely.

**Measured gain:** of the 148 undecided rows with no candidate, **38 gained one**
(prefix 24 · word overlap 6 · weak 8), leaving 110. Nothing regressed — the rows
that already had a candidate still do (130 → 168).

### 2. A shortlist instead of a single take-it-or-leave-it pairing

`PRODUCT-9` established that a rejection's replacement is always the next-ranked
entry, and that **0 of 31** such replacements ever improved the confidence tier.
The scorer's ordering past the first hit is not trustworthy enough to decide on the
owner's behalf, so it now offers a choice instead.

The ranked candidates from each search are stored in `match_review_candidates`
(up to 5 per game). A row shows its current candidate plus up to **2 alternates**,
each with **Use this**. Picking one is the existing manual-pick path. Measured on
live: 501 candidates across 168 games, and **117 rows have alternates to show**.

**A no now promotes locally.** It records the rejection and takes the next
non-rejected entry from the stored shortlist — no API call at all, because the
lower ranks are already on file and a fresh search could only re-derive them. Rows
seeded before shortlists existed fall back to one search to seed their alternatives.

**Honest limit:** where the scorer is simply lost, all three offered candidates are
wrong — *Against the Storm* offers `Metal Storm` and two *Life is Strange* episodes.
Those rows cost slightly more to scan and still end in the manual search. The gain
is on rows where a lower-ranked entry genuinely *is* the game: it is now visible and
one click away instead of requiring the owner to type it.

### 3. The dead queue is deleted, and its three call sites now actually work

`enrichment_queue` was write-only — an `INSERT`, a `COUNT`, and no reader anywhere.
It had **three** call sites, not one, so the damage was wider than `PRODUCT-9`
recorded:

| call site | what it did |
|---|---|
| `/review` manual match | linked the game, marked it `manual` (removing it from the pool `EnrichLibrary` selects), never enriched |
| **`Rehydrate` button** on a game page | enqueued into the dead queue and answered **"Queued."** — it did nothing at all |
| manual game add | linked the chosen IGDB entry but never fetched it |

The table is dropped by an idempotent migration, `EnqueueEnrichment` and
`QueueCounts` are gone, and all three sites now enrich directly through the shared
`enrichFromIGDB` path. `Rehydrate` fetches by the stored IGDB id when there is one,
otherwise searches by title and accepts only an exact match — and reports what
happened instead of claiming success.

Measured before deleting: the table held **0 rows** and **0 games** were marked
`manual`, so nothing was lost by removing it.
