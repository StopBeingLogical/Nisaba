# Unreleased Changes

**Instructions:**
1. Add one-liners below as you make changes
2. Before committing, review this file
3. At session start, ask Claude to "consolidate and commit changelog entries"
4. Claude will move entries to appropriate CHANGELOG.md files and clear this file

---

## Top-Level Changes (Major only)
- Added Playnite-to-Nisaba automated library sync via PowerShell (2026-04-26)
- Added support for dynamic storefronts (Xbox, Itch, Battle.net, etc.) via database migration
- Added Chrome extension scrape API for mystery pack integration (2026-04-24)

## db/ Changes
- Added FindGameByTitle() and makeSortTitle() for robust deduplication (2026-04-27)
- Removed restrictive CHECK constraints from game_stores, wishlist_stores, and game_install_sources
- Added mystery_pack_scrape_queues table for queueing scraped data
- Added mystery_pack_offers table for multi-seller price tracking
- Added mystery_pack_price_history table for price snapshots
- Added 12 new store methods for queue, offer, and price history management
- Added ListWishlistTitleIndex() for game title matching

## handlers/ Changes
- Added POST /api/sync/playnite for automated library updates (2026-04-26)
- Added Playnite sync card to Sync UI with copyable PowerShell script
- Added Sync API Secret to settings for securing automated syncs
- Replaced Heroic import UI with automated Playnite sync
- Added POST /api/mystery-packs/scrape/queue for data ingestion with validation
- Added GET /api/mystery-packs/scrape/review for diff computation
- Added POST /api/mystery-packs/scrape/apply for user-approved changes

## sync/ Changes
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
