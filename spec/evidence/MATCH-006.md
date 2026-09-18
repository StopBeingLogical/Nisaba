# MATCH-006 — the candidate shortlist

**Status:** done. Ruling: `spec/PRODUCT-10.md` §2. Deployed in `DEPLOY-012`.

## Why

`MATCH-003`/`PRODUCT-9` measured what a rejection actually produced: of 31
replacements, **0 improved the confidence tier**, 2 were worse, 5 found nothing —
and the sample included both genuine finds (`Battle Isle 2` → `Battle Isle 2200`,
the base game under its US title) and obvious misses (`Against the Storm` →
`Life is Strange: Before the Storm`). The scorer's ordering past the first hit is
not good enough to decide on the owner's behalf, so it now offers the choice.

## What changed

| file | change |
|---|---|
| `schema.sql` | `match_review_candidates` — the ranked candidates per game, up to 5 |
| `db/store.go` | `ReplaceMatchCandidates`, `MatchCandidatesFor` (one query per page), `NextStoredCandidate`; `MatchReviewRow.Alt` + `Alternates()` |
| `sync/igdb.go` | `rankCandidates` replaces the single-winner `bestCandidate` and stores the ranking |
| `sync/match_apply.go` | a no promotes from the stored shortlist instead of re-searching |
| `templates/match_row_partial.html` | up to 2 alternates per row, each with **Use this** |

## Acceptance

    go build ./...   # pass
    go vet ./...     # pass
    go test ./...    # pass

No new `gofmt` hunks.

## Verified on a copy of the live database

    501 candidates stored across 168 games (avg 3.0)
    rank-1 == row candidate verified on 25 rows
    rendered: "Against the Storm" offers 2 alternates
    rejected 81104, promoted 91247 without searching
    row now offers 3 candidates, none of them rejected

- **Rank 1 always equals the row's current candidate** — the shortlist and the
  primary cannot disagree.
- Alternates are capped at **2** and never repeat the primary.
- The shortlist renders on the page (`Other options from the same search`).
- **A no promoted the next alternative with no API call**, and the rejected entry
  disappeared from what the row offers.
- `MatchCandidatesFor` excludes rejected ids in SQL, so a rejected entry cannot
  reappear even if the shortlist is rebuilt from a stale ranking.

**A deadlock was avoided here.** The store is limited to one connection, so issuing
the page's shortlist query while the row query's `rows` were still open would have
blocked on itself. `ListMatchReview` now drains and closes them first — noted
because it would have hung the page rather than erroring.

## On live

| | value |
|---|---:|
| undecided rows with a candidate | **168** (was 130) |
| stored candidates | **501** |
| games with a shortlist | 168 |
| **rows that now show alternates** | **117** |
| stored candidates that are rejected | **0** |

## The limit, restated

Where the scorer is lost, all three offered candidates are wrong. The row costs
slightly more to scan and still ends in the manual search box. The gain is on rows
where a lower-ranked entry genuinely is the game — visible and one click away,
instead of requiring the title to be typed.
