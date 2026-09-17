# DEPLOY-007 — deploy the corrected multiple-stores indicator

**Date:** 2026-09-17 · **Commit:** `d33547f` · **Environment:** atlas (Bobby
confirmed)

## Acceptance

```bash
$ curl -s -o /dev/null -w '%{http_code}' http://192.168.3.174:8090/
200
$ grep -q '102' spec/evidence/DEPLOY-007.md
```

## Deployment

Image `sha256:107c7ed2546b7594592092084110b3787e3acc070064689255dbc830f820af41`,
replacing `0f0208ad7d11` (`DEPLOY-006`). Container up, both schedules intact at
`2026-09-18 11:00`. `/` `/library` `/wishlist` are 200; `/review` is 303, the
auth redirect.

The new clause is in the running binary: `docker exec nisaba grep -ac 'COUNT(*)
FROM game_stores gs WHERE gs.game_id' /app/nisaba` returns **1**, and the same
string returns 0 in the previous image.

Deployed **before** `DATA-002` ran, as `PRODUCT-5.md` requires — the duplicate
rows were what the old query counted, so the opposite order would have zeroed
the flag for every game in the gap.

## The page uses the new expression — proved, not assumed

`multi_store_owned` renders as a purple indicator dot, so the rendered page can
be counted against both formulations on the same rows:

| for the first 200 games by `sort_title` | count |
|---|---:|
| what the old sibling-row test says | 31 |
| what the new owned-link test says | **5** |
| **purple dots on the rendered page** | **5** |

The page matches the new expression and not the old one, which is the proof that
the deployed binary is serving it.

## No performance regression

`/library` is the instrument the earlier round set to under a second:

```
page 1  0.070s · 0.071s · 0.069s
page 10 0.090s
```

Three consecutive page-1 runs and a later page, all far under the target, and
the handler's own `>100ms` timing line does not fire.

## Live state

| | value |
|---|---:|
| games | 4101 (pre-dedupe) |
| distinct identities | 3383 |
| games flagged multi-store | 102 pre-dedupe, **243** after `DATA-002` |
| `game_stores` | 4402 pre-dedupe |

The flag reads 102 here and 243 in `DATA-002` for a reason worth keeping
straight: merging each group's store links onto one row legitimately *creates*
multi-store games, so the truthful number rises once the duplicates are gone.
Both numbers are the same expression at two points in the cleanup.
