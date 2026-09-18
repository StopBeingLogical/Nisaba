# DEPLOY-009 — deploy the match review page and seed the candidates

**Date:** 2026-09-17 · **Environment:** atlas (Bobby confirmed) · **Ruling:**
`spec/PRODUCT-7.md`

## Acceptance

```bash
$ curl -s -o /dev/null -w '%{http_code}' http://192.168.3.174:8090/
200
$ grep -q 'match_review' spec/evidence/DEPLOY-009.md && echo ok
ok
```

## Deploy

| | |
|---|---|
| previous image | `sha256:310c30aa044f…` |
| deployed image | `sha256:afc4167e49b2…` (built 2026-09-17 23:59 UTC) |
| container | `source-nisaba`, up, both daily schedules intact |

`match_review` is created on startup from the embedded `schema.sql`, so the deploy
itself is the migration — no hand-run DDL. Verified present immediately after
restart:

```
name
------------
match_review
```

Routes, checked from the host (a **303** means registered and behind auth, as
`/review` also returns):

| path | status |
|---|---:|
| `/` | 200 |
| `/library` | 200 |
| `/match-review` | **303** |
| `/match-review/status` | **303** |
| `/review` | 303 |

The new code is in the running binary: `match-review/save` ×2,
`FindMatchCandidates` ×2, `match_review` ×12, `Match Review` ×3.

## The seed

Before the seed the table was empty (`match_review` rows **0**). The candidates
were filled by an out-of-band call to the same `sync.FindMatchCandidates` the
page's **Find candidates** button runs in the background, run inside the
container against the live database.

| | before | after |
|---|---:|---:|
| `match_review` rows | **0** | **328** |
| with a candidate | 0 | **185** |
| no candidate | 0 | 143 |
| errors | — | **0** |

Confidence spread on live: prefix 74 · weak 64 · word overlap 35 · contains 12 ·
exact 0 · none 143. Measured pairings include `Halcyon 6` → `Halcyon 6: Starbase
Commander`, `Realms of Arkania 3` → `Realms of Arkania III: Shadows over Riva`,
`System Shock: Classic` → `System Shock`, and `Ultima™ Underworld II` → `Ultima
Underworld II: Labyrinth of Worlds`. Wrong ones are present and labelled —
`Medal of Honor` → `Medal of Honor: Allied Assault - Spearhead`, `Pizza
Connection` → `Pizza Connection 3` — which is what the page exists to catch.

## What is proven, and what is not

**Proven:** the route is registered and auth-gated; the binary carries the new
code; `match_review` is created by the deploy; the candidate search produces the
expected queue against live data; and the background job, its status transition
and its DB writes all work when driven directly (see `MATCH-001`).

**Not proven by this deploy:** the button path *through HTTP as a signed-in
session*. The seed called the same function the button calls, so the search is
proven and the HTTP wiring is not — the same shape of limit `DEPLOY-006` and
`DEPLOY-008` recorded, and it exists because `/match-review` sits behind session
auth with no non-interactive trigger. Pressing **Find candidates** once closes it;
it also re-searches only undecided games and refuses to overwrite a decided row,
so pressing it is safe at any point.

## Cleanup

The seeding runner was removed from the container and the host, and its source was
never committed. The `.backup` copies used for verification were deleted.

## Next

The queue is ready to work through at **`/match-review`**: 328 games, 185 of them
with a candidate, all undecided. Answers save per page and survive sessions. No
verdict is applied to `games` yet — the true-up is a separate pass and needs its
own ruling on what a `yes` and a `no` should each do.
