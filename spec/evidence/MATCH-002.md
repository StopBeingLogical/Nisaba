# MATCH-002 — candidate evidence and inline IGDB search on the review page

**Status:** done, deployed in `DEPLOY-010`. Ruling: `spec/PRODUCT-8.md`.

## What changed

| file | change |
|---|---|
| `schema.sql` | `match_review` gains `summary`, `genres`, `platforms`, `igdb_url`; confidence comment adds `manual` |
| `main.go` | four additive `ALTER TABLE` migrations for existing databases; two new routes |
| `db/store.go` | row + candidate structs carry the evidence; `SetManualMatch` (unconditional, sets `decision = 1`); `GetMatchReviewRow` for a single-row swap; summary truncation and platform-list helpers |
| `sync/igdb.go` | `platforms.name` added to the fetched fields; `PlatformNames()`; `FetchGame(id)`; `CandidateFromGame()` — one builder used by both the finder and the manual pick |
| `handlers/match_review.go` | `MatchReviewSearch` (GET `/match-review/{id}/search`) and `MatchReviewSetCandidate` (POST `/match-review/{id}/candidate`) |
| `templates/` | row markup extracted to `match_row_partial.html`; new `match_search_results_partial.html` |

## Acceptance

    go build ./...                 # pass
    go vet ./...                   # pass
    go test ./...                  # pass (nisaba/handlers, nisaba/sync ok)
    grep -q 'igdb_url' schema.sql  # pass

`gofmt` adds **no new hunks**: `sync/igdb.go`, `main.go` and
`handlers/handlers.go` are unchanged in hunk count against `HEAD`, and
`db/store.go` goes **8 → 7** because the `MatchReviewRow` struct was one of the
pre-existing misaligned blocks that this change happened to rewrite.

## Verified functionally, not just compiled

The two write paths and the render were exercised against a copy of the live
database with real IGDB credentials.

**Manual pick, end to end.** Page rendered (109 KB) with the search box and
`hx-get` wiring present. Searching `Assassin's Creed` returned results; picking
the first stored a full candidate:

    igdb_id=133004  name="Assassin's Creed Valhalla"  year=2020
    genres="Role-playing (RPG), Adventure"
    platforms="Google Stadia, Xbox Series X|S, PlayStation 4, PC (Microsoft Windows), PlayStation 5, Xbox One"

The returned row fragment carried `checked` and `you picked this`, and the stored
row read back `decision=1, confidence=manual` with a non-empty `igdb_url` and
summary. Save then cleared the radio back to undecided, so the two controls do
not interfere.

**The finder's write path.** `UpsertMatchCandidate` now inserts twelve columns,
so it was exercised directly rather than assumed: the copy was pre-decided down
to three undecided games, and `FindMatchCandidates` stored full evidence on the
one that matched (summary, genres, platforms, IGDB URL). **0 errors.**

**The rendered evidence.** A snapshot of the *live, re-seeded* database rendered
129 KB with `genres:`, `platforms:` and `https://www.igdb.com/games/` links
present, and each sampled row's own summary text and IGDB URL found in the HTML
(compared against `html.EscapeString`, since `html/template` escapes the output).

The `platforms.name` expansion was confirmed against the live API before being
relied on: IGDB accepts `platforms.id,platforms.name` and always returns `id`
alongside an expanded relation, so `HasPCPlatform()` — which matching depends
on — still sees the ids it needs.

## Live result after `DEPLOY-010`

| | before | after |
|---|---:|---:|
| undecided rows with a candidate | 99 | 99 |
| undecided rows with evidence | **0** | **99** |
| …with a summary | 0 | 92 |
| …with genres | 0 | 94 |
| …with platforms | 0 | 95 |
| `decision` counts | 50 yes / 36 no / 242 undecided | unchanged |
| multi-store indicator | 245 | 245 |
| games | 3388 | 3388 |

**All 242 undecided pairings are byte-identical before and after the re-seed**
(`game_id|igdb_id|igdb_name|confidence` diffed between snapshots), so nothing
moved under Bobby's answers, and the re-seed touched no game row at all.

The 7 rows with a candidate and URL but no summary are the expected case — those
IGDB entries carry no summary (small remaster/DLC entries such as the
`Asterix & Obelix XXL 2` remaster), not a decode failure. The 86 already-decided
rows keep no evidence by design (`PRODUCT-8` ruling 4).

## What this does not do

Nothing is applied to `games`: a yes still stores a verdict and nothing else. The
true-up remains unbuilt and unruled.
