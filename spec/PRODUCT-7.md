# Round 2026-09-17 (third pass) — the match review page

Locked owner requirements. Supersedes `PRODUCT-6.md` where they disagree; that
file stays as the record of what was ruled then.

## The request

Bobby, 2026-09-17: *"Make a review page for unmatched games. One column with the
listing as in the wishlist or library, the other the most likely match from the
IGDB that you can find. Give me a checkbox for each game to select yes or no if
the match is correct. Add a save button so I can do it in sessions instead of all
at once. After it's done, I'll have you go through and true up based on those."*

This is the answer to the question `PRODUCT-6` left open — *how far may matching
loosen?* — by taking the decision away from the matcher and giving it to the
owner: the search may be as loose as it likes because **nothing it finds is ever
applied**. `MATCH-001` fills a queue; a later pass turns the owner's verdicts into
matches.

## Ruled, 2026-09-17 (third pass)

### 1. The page is side-by-side, one pairing per row

`/match-review`, behind the normal session auth. Left is the game as it is listed
today — cover (or a `no art` placeholder), title, store badges. Right is the best
IGDB candidate: cover, name, release year, a confidence label, and a warning when
that IGDB entry is **already matched to another game** in the library. Pagination
is 25 pairings, and rows that have a candidate are ordered first so the top of the
queue is always actionable rather than padded with `no candidate` entries.

### 2. The verdict is **three-valued**, which is why the control is a radio pair

Bobby asked for a checkbox, and the page shows a labelled **Yes / No** pair per
row. They are radios rather than a single checkbox for a specific reason:
resumable sessions need to tell *rejected* apart from *not yet reviewed*. One
checkbox can only express "correct" or "not correct", so an untouched row and a
deliberately rejected row would be indistinguishable — and the queue could not be
worked through in sittings. Three states (`NULL` / 1 / 0) are stored, and clearing
both radios returns a row to undecided.

### 3. Saving is explicit and per-page

A sticky **Save decisions** button posts every row on the page. Each row submits
its id, so a cleared radio writes `NULL` back rather than silently keeping its old
value. Filters (Undecided / Yes / No / All) and progress counts let the queue be
worked in any order across sessions. The page says plainly that answers are not
stored until Save is pressed.

### 4. The candidate search may be loose, because it decides nothing

`sync.FindMatchCandidates` searches IGDB for every undecided game and stores the
best-ranked result and a confidence label. Scoring is deliberately looser than the
enrichment path: exact → prefix → contains → word overlap → weak. Two guards keep
the loose tiers honest — a suffix-edition bump needs **at least two words** on the
shorter title (`Diablo` must not rank `Diablo IV: Season of Divine Intervention`
as a strong match), and a token is only read as a roman numeral if its canonical
spelling round-trips.

It writes to `match_review` and **never to `games`**. `UpsertMatchCandidate`
refuses to overwrite a row that already has a verdict, and the search skips
decided games, so it is safe to re-run. It runs as a background job with a
progress bar, and the same function is what a one-off seed call used.

## Measured on live, 2026-09-17

| | |
|---|---:|
| games searched | **328** |
| with a candidate | **185** |
| — exact | 0 |
| — prefix | 74 |
| — contains | 12 |
| — word overlap | 35 |
| — weak | 64 |
| no candidate | 143 |
| errors | **0** |

The `exact` count is 0 by construction: an exactly-matching title would already
have been matched by the enrichment pass, so everything left over is a variant of
some kind. Correct pairings among the results include `Halcyon 6` → `Halcyon 6:
Starbase Commander`, `Realms of Arkania 3` → `Realms of Arkania III: Shadows over
Riva`, `Imperialism 2` → `Imperialism II: Age of Exploration` and `Ultima™
Underworld II` → `Ultima Underworld II: Labyrinth of Worlds`. Wrong ones exist and
are the point of the page — `Medal of Honor` ranks `Medal of Honor: Allied
Assault - Spearhead`, `Pizza Connection` ranks `Pizza Connection 3` — and each is
labelled so it can be rejected in one click.

## Deliberately not done

**No verdict is applied by this round.** Accepting a match does not set an
`igdb_id`, fetch artwork, or touch `games` at all. The true-up is a separate pass
Bobby asked for explicitly ("I'll have you go through and true up based on
those"), and it will need its own ruling on what a `yes` should do — link only, or
link and re-enrich — and what a `no` should do to the game.
