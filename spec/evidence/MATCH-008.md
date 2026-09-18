# MATCH-008 — the search now asks IGDB by name, not only by relevance

**Status:** done. Ruling: `spec/PRODUCT-11.md`. Deployed in `DEPLOY-013`.

## The reported bug

`/match-review`'s inline search box, given `Against the Storm`, offered the wrong
games. IGDB's own website found it.

The game was in IGDB all along. The query was not reaching it.

## Probed against the live API (credentials read from the DB into shell variables; nothing printed)

| query | result |
|---|---|
| `search "Against the Storm"; limit 5;` | `The Demon Crystal 3: Dark Storm`, `Metal Storm`, 3× `Life is Strange: Before the Storm …` |
| `search "Against the Storm"; limit 30;` | correct entry `147519` at **position 14** |
| `search "…"; where platforms = (6); limit 40;` | correct entry at **position 10** |
| `where name ~ *"Against the Storm"*; sort name asc; limit 10;` | `147519 Against the Storm`, then its two DLC |
| `where name = "Against the Storm";` | `147519 Against the Storm` |
| `where name = "Against the storm";` | **empty** (the `=` operator is case-sensitive) |
| `search "Fallout 2: A Post Nuclear Role Playing Game";` | **empty** |
| `search "Fallout 2";` | `14 Fallout 2` first |
| `where name = "The Falconeer";` | **empty** — IGDB's entry is `The Falconeer: Revolution Remaster` |
| `where name ~ *"Falconeer"*; limit 10;` | 10 of 18, base entry absent |
| `where name ~ *"Falconeer"*; limit 50;` | 18 of 18, base entry present at position 1 of the sorted set |
| `search "Falcon"; limit 50;` | `The Falconeer: Revolution Remaster` at position 35 |

**What that establishes.** `search` is a conjunction over every term matched against
the summary as well as the name (that is why a subtitle yields an empty set and why
"Against the Storm" yields *Life is Strange*), its ranking is poor enough to put an
exact entry past position 10, the platform filter reshapes that ranking rather than
merely narrowing it, the `~` wildcard is case-insensitive where `=` is not, and a
wildcard lookup is unordered so a small limit can drop the exact entry.

## Before and after, over the live no-candidate rows

The 110 undecided rows carrying no candidate, plus three controls, scored with the
real `scoreMatch`:

```
  rows                          exact   usable
  before (search limit 5)           1        7  of 113
  after  (fix)                     10       48  of 113
  still nothing either way:     65
```

The three controls, which are the point:

```
  CONTROL Against the Storm                        old 0.05 (Metal Storm)  ->  new 1.00 (Against the Storm)
  CONTROL Fallout 2: A Post Nuclear Role Playing…  old 0.00 (nothing)      ->  new 0.85 (Fallout 2)
  CONTROL Warhammer 40,000: Dawn of War II - Ann…  old 0.00 (nothing)      ->  new 0.85 (…Dawn of War II)
```

Gained a usable candidate (first six of 41):

```
  Fallout 2: A Post Nuclear Role Playing Game   -> Fallout 2                          (0.85)
  Daggerfall Unity - GOG Cut                    -> Daggerfall Unity                   (0.85)
  Disciples 2 - Dark Prophecy and Gallean's …   -> Disciples II: Dark Prophecy        (0.85)
  Earth 2150 - Escape from the Blue Planet      -> Earth 2150                         (0.85)
  Going Under                                   -> Going Under                        (1.00)
  Space Quest 6 - Roger Wilco in the Spinal …   -> Space Quest 6: The Spinal Frontier (0.63)
```

**No regressions.** The fix asserts it rather than assuming it: the widened ranked
query asks for 25 rows of the *same* ranking the old query asked 5 of, so the new
result set strictly contains the old one and can only score the same or better. The
harness fails the run on any drop; none occurred.

## The measurement that changed the implementation

The first version cut the subtitle at the **first** separator. That turned
`Warhammer 40,000: Dawn of War II - Anniversary Edition` into `Warhammer 40,000`, and
the best available match for the franchise prefix is **`Warhammer 40,000: Dawn of War`
— the wrong game**, at the confident 0.85 prefix tier. Cutting at the **last**
separator keeps the game's own name, and the same run then produced
`Warhammer 40,000: Dawn of War II`. Both heads are generated, narrowest first.

## Acceptance

    go build ./...   # pass
    go vet ./...     # pass
    go test ./...    # pass

`TestSearchForms`, `TestHeadSearchTitleRejectsBroadHeads`, `TestAnchoredBody`,
`TestRankedBodyIsWideEnough`, `TestWildcardLiteralStripsDirectives`,
`TestAppendUniqueDropsDuplicates`, `TestRankSearchResultsPutsExactFirst` and
`TestScoreMatchIgnoresSpacing` pin the mechanism, so the limit cannot quietly go back
to 5 and the anchor cannot be dropped. `sync/igdb.go` also came out gofmt-clean: the
one pre-existing hunk in it was the doc comment describing the platform filter this
round removed.

## Verified through the handler

Driving `MatchReviewSearch` over a copy of the live database and reading the rendered
partial — what the owner actually sees:

```
  "Against the Storm"                            -> 15 results, first igdb_id 147519
  "against the storm"                            -> 15 results, first igdb_id 147519   (case-insensitive)
  "Fallout 2: A Post Nuclear Role Playing Game"  ->  5 results, first igdb_id 14
  "Dragonview"                                   ->  1 result,  first igdb_id 42635    (Dragon View)
```

## Second pass — a bracketed note in the middle of a title

Asked instead what the 67 rows that still found nothing had in common. Twelve were the
same shape: a store series marker sitting mid-title, which `cleanSearchTitle` never
reached because it stripped a **trailing** bracket group only.

| title | IGDB name it should find | before | after |
|---|---|---|---|
| `Heroes Chronicles [Chapter 1] - Warlords of the Wasteland` | `Heroes Chronicles: Warlords of the Wasteland` | nothing | 0.65 tokens |
| `Heroes Chronicles [Chapter 8] - The Sword of Frost` | `Heroes Chronicles: The Sword of Frost` | nothing | 0.65 tokens |
| `Leisure Suit Larry 1 (VGA) - In the Land of the Lounge Lizards` | `Leisure Suit Larry 1: In the Land of the Lounge Lizards` | nothing | 0.68 tokens |
| `Leisure Suit Larry 6 (VGA) - Shape Up Or Slip Out` | `Leisure Suit Larry 6: Shape Up or Slip Out!` | nothing | 0.68 tokens |
| `Tomb Raider (VI): The Angel of Darkness (2003)` | `Tomb Raider: The Angel of Darkness` | (had a candidate) | query corrected |

`stripBracketSegments` now removes every bracket group wherever it sits. Measured
over the 67: **10 rows recovered**. Two more are explained rather than missed — the
search does find a match for them and the owner had already rejected it:

```
  MDK 2          held out: MDK 2 HD (0.85, rejected)
  Battle Isle 3  held out: Battle Isle 2220: Shadow of the Emperor (0.05, rejected)
```

Of the 30 rows that already had a candidate and carry a bracket in the title, **0
scored worse**. One flipped: `Tomb Raider (VI): The Angel of Darkness (2003)` now
prefers `Tomb Raider` at 0.85 over the correct game at 0.65 — the prefix tier again,
and recorded with the rest of it in `spec/OPEN.md`.

Stripping a group can leave a space before punctuation the earlier pass had already
closed, which produced the query `Tomb Raider : The Angel of Darkness`. The cleanup
now runs again after cleaning and is pinned by a test.

## Considered and deliberately not built

Anchoring on the title with its spaces removed, for names IGDB concatenates and the
store does not: `MDK 2` → IGDB's `MDK2`, scoring **1.00 exact**. Measured over the
same 67 rows: **1 recovered**. The search box finds it as soon as the title is typed
`MDK2`, and the form would cost 270ms on the front of every search, so it is not
implemented. The numbers are here so it does not need re-deriving.

## Third pass — IGDB's alternative-names index

For the 55 rows that found nothing under the game's own name, the question was not
what query to send but whether IGDB knows the game under *another* name. It does, and
the index is a separate endpoint:

    POST /v4/alternative_names
    where name ~ *"UBERMOSH:BLACK"*; fields game,name; limit 20;

Measured over those 55 rows: **10 recovered**, **3 exact**:

```
  UBERMOSH:BLACK         -> Ubermosh: Black          1.00 exact
  UBERMOSH:SANTICIDE     -> Ubermosh: Santicide      1.00 exact
  UBERMOSH:WRAITH        -> Ubermosh: Wraith         1.00 exact
  Grand Theft Auto V Legacy            -> Grand Theft Auto V          0.85 prefix
  Football Manager 2024 Pre-game editor -> Football Manager 2024       0.85 prefix  (WRONG)
  Football Manager 2024 Resource archiver -> Football Manager 2024     0.85 prefix  (WRONG)
  Tomb Raider I-III Remastered Starring Lara Croft -> Tomb Raider I•II•III Remastered  0.08 weak
  Brewmaster: Beer Brewing Simulator   -> Brewmaster                   0.05 weak
  Minecraft for Windows                -> Minecraft                    0.07 weak
  Uplink: Hacker Elite                 -> Uplink                       0.07 weak
```

The alternative-names index is loose — one query returned 20 unrelated names — so it
is resolved to games, capped, and only consulted when every name-based form has
failed. Cost: two requests, paid only by rows that would otherwise come back empty.

`IGDBClient.post` is now the single place a request leaves the package, so the rate
limit cannot be bypassed by adding an endpoint.

**The two wrong ones are the prefix tier again.** `Football Manager 2024 Pre-game
editor` and `… Resource archiver` are store *tools*, and the index cannot know that;
they resolve to the game at 0.85. Recorded in `spec/OPEN.md` with the rest.

## Can the scorer be improved? Measured against the owner's own verdicts

With 50 confirmed pairs and 36 rejections on file, the open question in `OPEN.md`
stopped being a matter of taste. Four options were ranked from the **same** search
results, so the comparison isolates the scorer:

| option | confirmed entry ranked #1 | rejected entry ranked #1 | rejected entry at a confident tier |
|---|---:|---:|---:|
| **V0** shipped: tier only | 39/50 | 12/36 | 14/36 |
| V5 tier, then coverage | 40/50 | 12/36 | 14/36 |
| V6 coverage first, then tier | 43/50 | 12/36 | 14/36 |
| V4 exact, then coverage, then tier | 43/50 | 12/36 | 14/36 |

**No confirmed entry is missing from the search results at all (0/50).** The search
reaches every answer the owner gave; only the ordering is imperfect.

V4 is +4 on the confirmed pairs with **no confirmed entry pushed off #1**. That looks
decisive until the same ranking is applied to the stored shortlists of the undecided
queue, where it changes **16 of 221** rows' best candidate — and the changes are mixed:

    WINS    Quake II: The Reckoning        -> Quake II Mission Pack: The Reckoning
            Quake II: Ground Zero          -> Quake II Mission Pack: Ground Zero
            The Legend of Kyrandia: Malcolm's Revenge -> The Legend of Kyrandia 3: …
            Tomb Raider (VI): The Angel of Darkness    -> Tomb Raider: The Angel of Darkness
    LOSSES  Deus Ex™ GOTY Edition          -> Deus Ex: Human Revolution - A Criminal Past
            Earth 2150 - Escape from the Blue Planet  -> Earth 2150: The Moon Project
            Temple of Elemental Evil, The  -> Dungeons & Dragons Online: The Temple of …

Coverage-first has a failure mode of its own: a **long wrong name that contains every
word of the stored title** outscores the short right one. `Temple of Elemental Evil,
The` is the clearest case — the correct entry and the wrong one both cover 5 of 5
tokens, and the tie goes to the wrong one.

**Conclusion: do not change the scorer on this evidence.** V5 is +1 of 50, which is
noise; V4/V6 are +4 on the pairs but demonstrably trade them for new wrong picks on
the queue the owner actually reads. The tiers themselves, not the tie-break, are what
misfire — all four options leave the same 12 rejections ranked first and 14 at a
confident tier. The route worth trying next is not a reshuffle of the four tiers but a
mechanism that can tell *Temple of Elemental Evil* from *Dungeons & Dragons Online:
The Temple of Elemental Evil*, and coverage alone cannot.

## Not fixed here, and why

`search` returning summary noise for common words is inherent to the endpoint, not a
bug in our use of it — the anchored form is the answer, and it is now the primary one.
The remaining 65 rows that find nothing either way are titles with no IGDB entry of a
resembling name (store tools such as `Football Manager 2024 Resource archiver`, and
subtitles describing a different product). Some of the newly-found candidates are
**confidently wrong**; that is the scorer's prefix tier, and it is `spec/OPEN.md`.
