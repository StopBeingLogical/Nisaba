# Unreleased Changes

**Instructions:**
1. Add one-liners below as you make changes
2. Before committing, review this file
3. At session start, ask Claude to "consolidate and commit changelog entries"
4. Claude will move entries to appropriate CHANGELOG.md files and clear this file

---

## Top-Level Changes (Major only)
- Playnite is no longer the GOG owner: the embedded script skips `gog` entries (the server syncs GOG itself), while every other store keeps arriving through it, and Playnite runs are now visible in Recent Activity (2026-09-16)
- Prices now refresh themselves once a day — a price-only scheduled sync (ITAD + GG.deals, ~8s) with the full sync left manual at ~15 minutes (2026-09-16)
- Switched the pricing provider to ITAD so `best_current_store` and price history hold real storefront names; GG.deals is kept as a comparison source and feeds a "cheaper on GG.deals" callout (2026-09-16)
- Removed the dead `sqlc.yaml` + `queries/` scaffolding — `db/store.go` is the only query source; CLAUDE.md stack line updated to match (2026-09-16)
- Fixed CLAUDE.md deploy/dev paths to ~/code/nisaba (dead Nextcloud scheme removed); git remotes corrected: origin → Forgejo SSH :2222, GitHub demoted to `github` mirror remote, token-embedded HTTP remote removed (2026-07-10)
- Added Playnite-to-Nisaba automated library sync via PowerShell (2026-04-26)
- Added support for dynamic storefronts (Xbox, Itch, Battle.net, etc.) via database migration
- Added Chrome extension scrape API for mystery pack integration (2026-04-24)
- Added template caching, performance indexes, library pagination (2026-06-28)
- Added wishlist autoclean pipeline (link by IGDB → link by store → delete linked) (2026-06-28)
- Simplified sync page to core cards only (2026-06-28)
- Added price threshold system with 4 seeded defaults (2026-06-28)
- Added 3 lowest prices display on wishlist detail pages (2026-06-28)

## db/ Changes
- `multi_store_owned` now counts the game's own owned store links instead of looking for another game row sharing its `igdb_id`. The old test reported 796 games as owned on multiple stores where 102 are — 786 false positives and 92 false negatives (2026-09-17)
- Added additive `gg_deals_price` / `gg_deals_url` columns to wishlist_entries, with `UpdateWishlistGGDealsComparison()` — GG.deals comparison only, ITAD keeps owning best_current_* and history (2026-09-16)
- Narrowed the ListGames multi_store_owned EXISTS to a nested EXISTS — the JOIN-inside-EXISTS shape cost ~2.5s per library page under the pure-Go SQLite driver (2026-09-16)
- Added FindGameByTitle() and makeSortTitle() for robust deduplication (2026-04-27)
- Removed restrictive CHECK constraints from game_stores, wishlist_stores, and game_install_sources
- Added mystery_pack_scrape_queues table for queueing scraped data
- Added mystery_pack_offers table for multi-seller price tracking
- Added mystery_pack_price_history table for price snapshots
- Added 12 new store methods for queue, offer, and price history management
- Added ListWishlistTitleIndex() for game title matching
- Added LinkWishlistToLibraryByStore() and DeleteLinkedWishlistEntries() for autoclean (2026-06-28)
- Added CountMatchingGames() for pagination count queries (2026-06-28)
- Added LowestPrices() using window function for top-3 store prices (2026-06-28)
- Added migration: game_genres(game_id), game_tags(game_id), game_stores(game_id, owned), games(is_hidden, parent_id), games(igdb_id) indexes (2026-06-28)
- Added migration: price_thresholds table with 4 seeded default rows (2026-06-28)

## handlers/ Changes
- The full sync now enriches wishlist entries as well as games. The wishlist pass had no caller since commit 9557160, so every entry added after 2026-04-23 arrived with no IGDB id and no cover art — 396 of 676 on the live database (2026-09-17)
- Playnite runs record to `sync_log` and `sync_errors` as type `ownership`; the old `playnite` value failed the schema's CHECK, so those runs and their errors were silently discarded and never reached Recent Activity (2026-09-16)
- The full sync no longer runs a GOG wishlist pass — Steam is the only wishlist source, and the GOG settings card now documents itself as the library's credentials (2026-09-16)
- Wishlist views show the GG.deals price where ITAD has no price, labelled GG.deals, instead of "no pricing data" (2026-09-16)
- `storeShortLabel` maps ITAD storefront names (Humble Store → Humble, Epic Game Store → Epic, GreenManGaming → Green Man Gaming, Blizzard → Battle.net) and falls back to the raw value, never blank (2026-09-16)
- Full sync now runs ITAD as the authoritative pricing step, then GG.deals as a comparison step; an unconfigured ITAD key logs and continues instead of failing the run (2026-09-16)
- Added POST /api/sync/playnite for automated library updates (2026-04-26)
- Added Playnite sync card to Sync UI with copyable PowerShell script
- Added Sync API Secret to settings for securing automated syncs
- Replaced Heroic import UI with automated Playnite sync
- Added POST /api/mystery-packs/scrape/queue for data ingestion with validation
- Added GET /api/mystery-packs/scrape/review for diff computation
- Added POST /api/mystery-packs/scrape/apply for user-approved changes
- Added template caching — pre-parse all page templates at startup (2026-06-28)
- Added cleanupWishlistLinks() helper used by all sync handlers (2026-06-28)
- Simplified sync page — removed individual sync routes, kept 4 core cards (2026-06-28)
- Added library pagination with page selector (200 games/page) (2026-06-28)
- Added timing logging to library handler (>100ms threshold) (2026-06-28)
- Added storeShortLabel template func with improved store name mapping (2026-06-28)
- Added 3 lowest prices display to wishlist detail page (2026-06-28)

## sync/ Changes
- Removed `sync/gog_wishlist.go` — the retired GOG wishlist pass (2026-09-16)
- Added `sync/gog_library.go` — the GOG library syncs itself once a day (`sync.gog_hour`, default 11 = 07:00 US Eastern) from the library view, 11 requests for the whole account, in its own window so the price run stays price-only (2026-09-16)
- Added `sync/gog_auth.go` — GOG access tokens now refresh themselves from the stored refresh token (`gog.client_secret`), replacing the expiry check that rejected tokens GOG still accepts (2026-09-16)
- Added `sync/schedule.go` — a daily price-only sync (`sync.price_hour`, default 11 = 07:00 US Eastern) that skips while another sync runs, skips without an ITAD key, and catches up after downtime (2026-09-16)
- ITAD ID resolution is now one bulk `POST /lookup/id/shop/61/v1` per 100 Steam App IDs — 11 requests for the whole wishlist instead of ~609, which was 6× the key's 100-per-5-minute budget; the whole pricing pass dropped from ~13 minutes to 16.5 seconds (2026-09-16)
- ITAD now writes `best_price_url` from the response's `current.url`, so deal links keep their affiliate tags rather than being reconstructed (2026-09-16)
- Added standalone sync_playnite.ps1 PowerShell script for Playnite SDK (2026-04-26)
- Added 'Steam Family Sharing' category filter to Playnite sync script
- Added batching support (size: 50) and robust sanitization to Playnite sync
- Added game title suffix stripping (Steam Key, Global, PC, ROW, Windows, etc.)
- Added DescriptionSimilarity() for change detection using Jaccard similarity
- Added MatchGameTitle() for local library/wishlist matching before IGDB
- Added NormalizePack() for URL-safe pack ID slug generation

## schema/ Changes
- Added mystery_pack_scrape_queues with ISO timestamps and applied_at tracking
- Added mystery_pack_offers with seller-specific pricing and validity dates
- Added mystery_pack_price_history for trend analysis and price auditing

---

**Format:** One-liner per bullet. Include date if notable: `- Added X feature (2026-04-23)`

**Examples:**
- `- Replaced ITAD with gg.deals API for Steam pricing (2026-03-07)`
- `- Added store page HTML scrape fallback for wishlist name resolution`
- `- Fixed arrow visibility on game detail store links`

## spec/ Changes
- Added executable specification: SESSION_SEED.md, spec/ (PRODUCT, contracts, tasks.yaml, context-map, STATE, ACCEPTANCE, OPEN), scripts/spec-validate.sh + spec-next.sh (2026-08-01)
