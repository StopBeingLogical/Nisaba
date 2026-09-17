# Nisaba Sync Pipeline — Changelog

Store syncing, enrichment, pricing integrations, and data imports.

---

## [Unreleased]

### Status
- Pending `sync/` entries live in `.changelog/UNRELEASED.md` until they are
  consolidated here; as of 2026-09-17 that includes the GOG auth and library
  files, the daily scheduler, and the bulk ITAD ID lookup.

---

## [2026-03-07] — Pricing Architecture Redesign

### Replaced ITAD + Reseller Scrapers with gg.deals API
- **gg.deals** (`sync/ggdeals.go`) — batch API (up to 100 Steam IDs/req)
  - Returns: `currentRetail`, `currentKeyshops`, `historicalRetail`, `historicalKeyshops`
  - Rate limits: 100 IDs/min, 1000/hour
  - Replaces ITAD (retail) + reseller pricing in single call
  - Endpoint: `https://api.gg.deals/v1/prices/by-steam-app-id/`

- **Reseller Scrapers** (`sync/resellers.go`) — retained for non-Steam games
  - instant-gaming + loaded.com (concurrent goroutines, 2s rate limiter each)
  - Results merged via channel; DB write once per game
  - Halves wall-clock time for reseller scraping

- **Allkeyshop Fallback** (`sync/allkeyshop.go`) — for games not on IG/Loaded
  - Uses `utls` (Chrome TLS fingerprint) + HTTP/2 for Akamai evasion
  - Request jitter (2–5s random), cookie jar, security headers
  - Homepage warmup before scraping

### Concurrent Sync Improvements
- Parallel goroutines for multi-source pricing
- Channel-based result coordination
- Per-game DB writes only when all sources complete

---

## Sync Pipeline Files

Verified against the directory 2026-09-17.

**Ownership**
- `steam.go` — Steam library ownership (Steam Web API)
- `gog_auth.go` — GOG access token refresh (`gog.client_secret`); no expiry gate
- `gog_library.go` — GOG library sync from the library view, 11 requests/day
- `heroic.go` — Heroic library file import (Epic, GOG, Amazon). **Dead: no route
  mounts it** (`handlers/sync.go`, ruled out of scope in `PRODUCT-3.md`)
- `wishlist.go` — Steam wishlist sync (3-stage name resolution). Steam only; the
  GOG wishlist is retired and its file deleted

**Pricing**
- `itad.go` — IsThereAnyDeal. Authoritative for price, storefront and history;
  IDs resolve in bulk, 100 Steam App IDs per request
- `ggdeals.go` — gg.deals comparison prices only (`gg_deals_*`), never the ITAD
  columns
- `resellers.go` — instant-gaming + loaded.com concurrent scraping
- `allkeyshop.go` — Allkeyshop fallback (utls, Akamai evasion)
- `chromeclient.go` — Chrome TLS fingerprint HTTP client

**Enrichment**
- `igdb.go` — IGDB enrichment (batched 10/req) plus `SyncSteamCrossRefs`
- `rawg.go` — RAWG fallback enrichment
- `protondb.go` — ProtonDB ratings. **No caller** — see `spec/OPEN.md`
- `steam_deck.go` — Steam Deck status. **No caller** — see `spec/OPEN.md`

**Scheduling and mystery packs**
- `schedule.go` — the daily price-only run and the daily GOG library run
- `mystery_packs.go`, `mystery_packs_scrape.go` — Chrome-extension pack ingestion

Removed: `gog_wishlist.go` (GOG wishlist sync + OAuth), deleted by `GOGL-004`
when the GOG wishlist was retired.

---

## Enrichment Pipeline

Ownership arrives on three paths, then enrichment runs over whatever is present:

```
Steam ownership (steam.go, Steam Web API)
GOG library (gog_library.go, daily, server-side)   ─→ IGDB enrichment (batched)
Playnite, all stores except GOG (HTTP POST)          ─→ RAWG fallback
                                                     ─→ Review queue (unmatched)
```

The Heroic file import is dead code and is not part of this flow.

## Pricing Pipeline

```
ITAD (itad.go) — authoritative for best_current_price, best_current_store,
                 best_price_url, historical low and wishlist_price_history
  ↓
GG.deals (ggdeals.go) — comparison only, writes gg_deals_price / gg_deals_url;
  a cheaper price surfaces as a wishlist callout
  ↓
instant-gaming + loaded.com (resellers.go, concurrent, non-Steam entries)
  ↓
Allkeyshop (allkeyshop.go, fallback, utls Akamai evasion)
```

Ownership, wishlist imports and enrichment are not part of the daily job — only
the pricing columns above. The full sync stays manual.

## Wishlist Name Resolution

1. Match against owned library (free)
2. Steam `appdetails` API — batch then per-game (300ms rate limit)
3. Store page HTML scrape — extract `og:title` (500ms rate limit)
