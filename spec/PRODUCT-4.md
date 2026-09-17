# Round 2026-09-17 — wishlist enrichment restored

Locked owner requirements. This file supersedes `PRODUCT-3.md` where they
disagree; the earlier file stays as the record of what was ruled then.

## The report

Bobby, 2026-09-17: *"There are many games missing cover art."*

Measured before anything was changed:

| View | Missing a cover | Share |
|---|---:|---:|
| Wishlist | **396 of 676** | **59%** |
| Library | 148 of 4101 | 3.6% |

The library number was confirmed by rendering the page rather than reading the
database: `/library` page 1 returns 195 covers and 5 title-only placeholders out
of 200 tiles. The library gap is real but small and has a different cause — 133
of its 148 are `needs_review` games that IGDB and RAWG both failed to match, and
`EnrichLibrary` is called on every full sync. It is not part of this round.

## The cause

**`EnrichWishlist` (`sync/igdb.go:492`) has no caller.** It is the only thing
that writes wishlist `igdb_id` and `artwork`; nothing invokes it. Its two call
sites were routes deleted by commit `9557160` (2026-06-27, *"Simplify sync page"*),
the same commit that orphaned the Steam Deck, ProtonDB and CrossRefs fetchers
recorded in `OPEN.md`. This is the fourth casualty of that commit and the only
one a user sees.

**The Full sync has never enriched wishlists.** Verified on the pre-change code
rather than assumed: `SyncAll` at `9557160^` contains zero calls to
`EnrichWishlist`. Enrichment came only from the "Refresh Wishlist" and "Enrich
Wishlist" buttons. So wiring it into `SyncAll` is new behaviour, not a
regression being undone.

The evidence is consistent to the day:

- The 280 entries that have art are all `matched` with `last_enriched` between
  2026-03-07 and **2026-04-23** — the last time that button was pressed.
- The 396 without art are all `needs_review`, `last_enriched` NULL, `igdb_id` 0,
  and none has a `library_id`. Every one is a `steam-wish-<appid>` entry.
- 377 of the 396 were added after 2026-04-23. The four newest arrived in the
  2026-09-17 20:47 Full sync.

The gap is an unwired step, not a matching failure: titles in the backlog
include *Roboquest*, *PRAGMATA*, *Ember Knights* and *SYNTHETIK: Legion
Rising*, all of which IGDB matches trivially.

## Ruled: wire it in, then backfill

**`EnrichWishlist` joins `EnrichLibrary` in the Full sync's enrichment step.**
Ruled 2026-09-17, choosing the durable route over the cheaper one below. This
restores `igdb_id`, description and release date for wishlist entries, not only
cover art, and it uses the function as written — no new matching logic.

**The backfill needs no separate migration or hand-run SQL.** The first Full sync
after the deploy processes every `needs_review` wishlist entry (396 of them),
because `ListWishlistNeedsEnrichment` selects on status alone and
`EnrichWishlistEntry` stamps the row on write.

Cost: 396 IGDB searches at the function's existing 250ms tick — about 100
seconds, once. Later runs see only entries the Steam wishlist sync has just
added, so the cost falls away rather than recurring.

Known risk, accepted: `bestMatch` is title-matching and can mis-match. The
library's 501 `needs_review` games are the evidence that it does not always
resolve, though those are misses rather than wrong assignments. Wrong art on a
wishlist row is visible and correctable through the existing manual cover field
(`handlers/wishlist.go`); no automated undo is being built.

## Rejected: derive covers from the Steam appid

The appid is already in the entry id (`steam-wish-<appid>`), and
`cdn.cloudflare.steamstatic.com/steam/apps/<appid>/library_600x900.jpg` returns a
cover with no API call at all. Sampled 10 at random from the 396: **8 returned
200, 2 returned 404** — both unreleased titles with no store assets yet.

Rejected as the primary route because it supplies art and nothing else: no
`igdb_id`, no description, no release date, and it leaves roughly one in five
entries blank. It stays available as a fallback if IGDB fails to match specific
entries, and that is the condition under which Bobby should raise it again.

## Not in scope

- **The library's 148 missing covers.** Different cause (unmatched rather than
  unenriched), and `EnrichLibrary` already runs. No task.
- **The three dormant fetchers in `OPEN.md`.** Same commit caused them, but they
  are unruled and nothing here derives from them.
