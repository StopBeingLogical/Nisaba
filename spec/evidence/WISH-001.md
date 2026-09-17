# WISH-001 — enrich wishlist entries in the full sync

**Date:** 2026-09-17 · **Environment:** local · **Ruling:** `spec/PRODUCT-4.md`

## Acceptance

```bash
$ go build ./...
$ go vet ./...
$ go test ./...
ok      nisaba/handlers
ok      nisaba/sync

$ grep -q 'EnrichWishlist' handlers/sync.go && echo yes
yes
```

`gofmt -l handlers/sync.go` lists the file, and it did so at `HEAD` too — the
differences are pre-existing struct-tag alignment and import ordering in regions
this task does not touch, so they were left alone rather than swept in.

## The change

One call added to `SyncAll`'s enrichment step in `handlers/sync.go`, immediately
after the existing `EnrichLibrary` pass, with the same progress plumbing and the
same non-fatal error handling:

```go
wlErr := storesync.EnrichWishlist(h.store, client, func(p storesync.EnrichProgress) { … }, h.rawgClient())
```

Nothing else moved. `EnrichWishlist`, `bestMatch`, the 250ms tick,
`ListWishlistNeedsEnrichment` and `EnrichWishlistEntry` are untouched, and
`cleanupWishlistLinks` and every linking behaviour are untouched — this restores
a call, it does not rework matching.

## Functional verification

The wiring is one line, so the thing worth proving is that the function it calls
actually backfills the backlog. Run against a copy of the live database
(`nisaba-perf.db`, the 2026-09-16 copy) and the **real IGDB API** — a temporary
test, removed after the run:

```
BEFORE              total=667 with_art=328 with_igdb_id=328 needs_review=339
rawg fallback:      false
RUN1                processed=339 matched=283 errors=0
AFTER RUN1          total=667 with_art=610 with_igdb_id=611 needs_review=56
RUN2                processed=56 matched=0 errors=0
AFTER RUN2          total=667 with_art=610 with_igdb_id=611 needs_review=56
```

- **283 of 339 matched in 143 seconds**, which is the 250ms tick doing what it
  promises: 339 × 0.25s ≈ 85s of throttling plus IGDB latency.
- **Run 2 is the idempotency proof.** It found only the 56 that failed to match,
  matched none of them, and changed no row. A second full sync therefore costs
  seconds rather than re-walking the library.
- Artwork coverage on the copy went **328 → 610 of 667 (49% → 91%)**.

The local copy has no `rawg.api_key`, so the RAWG fallback did not engage —
these are IGDB-only numbers, and the live system has the key configured, so live
results should be no worse.

### What the 56 are, and why they are not a defect here

55 are `needs_review` and 1 is `matched` with no artwork, which is an IGDB record
whose `cover.url` is empty — `EnrichWishlistEntry` stamps the row regardless, so
the entry is correctly marked matched and simply has no art to show.

The 55 are title-matching misses, concentrated in edition suffixes and
punctuation: *Baldur's Gate 3*, *Divinity: Original Sin 2 - Definitive Edition*,
*Darkest Dungeon® II*, *Empire Earth 3*. This is the risk named and accepted in
`PRODUCT-4.md` before the change was written. The manual cover field on the
wishlist detail page remains the correction path.

**One incidental finding, recorded not fixed:** three live wishlist titles are
stored HTML-escaped — `steam-wish-3828500` `Deck &amp; Conn`,
`steam-wish-2400510` `Dungeons &amp; Degenerate Gamblers` and
`steam-wish-1836560` `Aether &amp; Iron`. (The 2026-09-16 copy used for the
functional run has only the first two; the third confirms the check on live, not
on the copy.) The entity guarantees a match failure, since IGDB has neither
string. No `games` row carries one, so this is a wishlist-writer quirk rather
than a systemic encoding problem. It is not derived into a task.

## Live expectation for DEPLOY-006

The live database is the 2026-09-17 state: 676 entries, 280 with art, **396
missing**. Applying this pass's 83.5% match rate puts the live result at roughly
**610 of 676 with art (~90%)**, leaving ~66 — the same 55-ish unmatched titles
plus the entries added between the copy and the run. `DEPLOY-006` records the
actual numbers rather than these.
