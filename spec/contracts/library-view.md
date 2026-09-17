# Library view contract

Governs what `/library` must keep doing. This is the no-regression boundary for
the performance work: the page may get faster by any means, but everything
below still behaves exactly as it does today.

## Rules

- Every filter in `ListGamesParams` keeps working and keeps composing: play
  status, store, Steam Deck status, platform, title search, genres (AND), tags
  (AND), installed, favorites.
- All four sorts keep working: default `sort_title ASC`, `added`, `playtime`,
  `rating`. Null handling in the `playtime` and `rating` sorts is part of the
  behavior — nulls sort last.
- Pagination stays at 200 games per page. The numbered page selector keeps
  rendering the same set of pages, driven by `CountMatchingGames` against the
  same filters as the row query.
- Hidden games and child entries (`parent_id IS NOT NULL`) stay excluded.
- Every field the grid renders today keeps rendering: artwork, Steam Deck
  status, Proton rating, installed flag, play status, rating, favorite flag,
  playtime, genres, tags, owned stores, and the multi-store-owned indicator.
- The elapsed-time log in `handlers/library.go:117-119` stays. It is the
  in-repo instrument for this work.
- Images continue to be served through `/img/proxy` (`CLAUDE.md` → Image Proxy).

## Performance requirement

`/library` and `/library?page=N` for any valid N each serve in **under 1
second** against the deployed instance, measured as:

```bash
curl -s -o /dev/null -w '%{time_total}\n' 'http://192.168.3.174:8090/library'
```

Current baseline for comparison: 5.34s page 1, 8.84s page 2 (2026-08-01,
`spec/evidence/ASSESSMENT.md`).

The gap between page 1 and page 2 is itself a signal — a fix that speeds up
page 1 while leaving later pages slower has not met this contract.

## Non-goals

- No redesign of the grid, the filter sidebar, or the page selector.
- No change to the 200-per-page size unless a task explicitly authorizes it.
- No new dependency, no frontend build step. HTMX and Tailwind stay CDN-loaded.

## Changing this contract

Not a task-level decision. Dropping a field, a filter, or a sort to make the
page fast is a product change and belongs to Bobby.
