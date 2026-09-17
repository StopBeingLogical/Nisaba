# PERF-007 — Verify the sub-second target on the deployed instance
**Date:** 2026-09-16 · **Commit:** `52586a6` · **Environment:** network

## Acceptance
```bash
$ curl -s -o /dev/null -w '%{time_total}\n' http://192.168.3.174:8090/library
0.081393
0.080159
0.078586

$ curl -s -o /dev/null -w '%{time_total}\n' 'http://192.168.3.174:8090/library?page=2'
0.085050
0.084484
0.084023
```

## Before and after, same instance

| Route | before the restart | after | target |
|---|---|---|---|
| `/library` page 1 | 5.657s / 5.643s / 5.630s | **0.081s / 0.080s / 0.079s** | < 1s ✅ |
| `/library?page=2` | 9.308s | **0.085s / 0.084s / 0.084s** | < 1s ✅ |
| `/wishlist` (untouched) | 0.276s | 0.276s | — |
| `/` dashboard (untouched) | 0.008s | 0.008s | — |

All responses returned HTTP 200. Three consecutive runs per route, as the task
requires. The improvement survives the move from the local copy to live data:
Ergaster measured 0.048s locally, Atlas serves 0.080s, about the same ~2× factor
the broken version showed.

The `PRODUCT.md` target — under 1 second for page 1 **and any later page** — is met
by roughly 12×.

## Result
The route that motivated the round is fixed in production, and the second page no
longer costs more than the first.

## What this does not prove

- Page 11 and page 21 were not measured live. Locally the cost is flat across
  offsets (0.074s at page 21) and page 2 is the acceptance's later page, but the
  live deep-offset numbers are not in hand.
- Filter and sort combinations were not re-measured live; that behaviour is
  `REL-001`'s gate, checked against `contracts/library-view.md`.
- Nothing about the GG.deals half of the round.
