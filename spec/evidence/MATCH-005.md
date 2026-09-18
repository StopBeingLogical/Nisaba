# MATCH-005 — cleaning the IGDB search string

**Status:** done. Ruling: `spec/PRODUCT-10.md` §1. Deployed in `DEPLOY-012`.

## The finding

The 148 rows with no candidate were not a scoring failure — IGDB's `search`
returned an **empty set**. Probed against live, with controls to prove the probe
worked:

| stored title | IGDB search |
|---|---|
| `Hollow Knight` (control) | finds Hollow Knight |
| `Batman: Arkham Asylum` (control) | finds it **and** its GOTY edition |
| `Batman: Arkham Asylum GOTY Edition` | **0 results** → cleaned finds both |
| `Batman: Arkham City GOTY` | **0 results** → cleaned finds the game |
| `Baldur's Gate: The Original Saga` | **0 results** → cleaned finds results |
| `Astebreed: Definitive Edition` | **0 results** → cleaned finds the game |
| `BloodNet (FDD version)` | **0 results** → cleaned finds the game |
| `Amerzone: The Explorer's Legacy (1999)` | **0 results** → cleaned finds the game |

The controls matter: a first pass reported that *every* title returned nothing,
including `Hollow Knight`. The probe was broken — IGDB pretty-prints its JSON and
the pattern assumed compact — and that wrong result would have become a wrong
recommendation.

## What changed

`searchTitle` (`sync/igdb.go`) now removes, before the query: a trailing
parenthetical or bracket note, a trailing bundle part (` + …`), and edition /
collection suffixes. ™/®/© are removed rather than replaced with a space, which had
been leaving `Batman :`.

`sync/igdb_search_test.go` is a permanent test file with **28 cases**, 17 of them
real library titles pinned to the query that finds them, plus guards that cleaning
can never empty or gut a title (`Deluxe` → `Deluxe`, `(1999)` → `(1999)`).

## The safety property, and its test

Cleaning happens **only** in the query. Scoring uses the stored title unchanged,
and `bestMatch` still requires exact normalised equality — so a wider search can
only reach a match, never re-rank one. `TestBestMatchUsesRawTitle` pins this: the
decorated title `Batman: Arkham Asylum GOTY Edition` must **not** be accepted
automatically against results containing `Batman: Arkham Asylum`.

Variant products are deliberately left alone: `Chicken Invaders 5: Christmas
Edition` is a different game from `Chicken Invaders 5`, and is pinned as unchanged.

## Acceptance

    go build ./...                          # pass
    go vet ./...                            # pass
    go test ./...                           # pass (28 new cases)
    go test ./sync -run TestSearchTitle     # pass

No new `gofmt` hunks.

## Measured gain on live

The finder was run against a copy, before and after:

| | undecided | with a candidate | without |
|---|---:|---:|---:|
| before | 278 | 130 | 148 |
| after | 278 | **168** | **110** |

**38 rows gained a candidate** — prefix 24 · word overlap 6 · weak 8. Nothing
regressed: every row that had a candidate still does.

110 rows remain without one. Those are titles IGDB does not carry, or carries under
a name no cleaning rule can derive; they are the inline search box's job, which
also got the benefit of this change (pasting a decorated title into it used to
return nothing, which reads as "IGDB doesn't have this game").
