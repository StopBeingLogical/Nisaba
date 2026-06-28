# Nisaba Sync Pipeline — Changelog

Store syncing, enrichment, pricing integrations, and data imports.

---

## [Unreleased]

### Status
- No pending changes

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
- `steam.go` — Steam library ownership
- `igdb.go` — IGDB enrichment (batched 10/req)
- `rawg.go` — RAWG fallback enrichment
- `wishlist.go` — Steam wishlist sync (3-stage name resolution)
- `gog_wishlist.go` — GOG wishlist sync + OAuth
- `ggdeals.go` — gg.deals pricing API
- `resellers.go` — instant-gaming + loaded.com concurrent scraping
- `allkeyshop.go` — Allkeyshop fallback (utls, Akamai evasion)
- `chromeclient.go` — Chrome TLS fingerprint HTTP client
- `heroic.go` — Heroic library file import (Epic, GOG, Amazon)
- `protondb.go` — ProtonDB compatibility ratings
- `steam_deck.go` — Steam Deck compatibility status

---

## Enrichment Pipeline

```
Steam ownership
  ↓
GOG/Epic/Amazon (Heroic)
  ↓
IGDB enrichment (batched)
  ↓
RAWG fallback
  ↓
Review queue (unmatched)
```

## Pricing Pipeline

```
gg.deals API (Steam-linked games, batch 100/req)
  ↓
instant-gaming + loaded.com (concurrent, non-Steam entries)
  ↓
Allkeyshop (fallback, utls Akamai evasion)
```

## Wishlist Name Resolution

1. Match against owned library (free)
2. Steam `appdetails` API — batch then per-game (300ms rate limit)
3. Store page HTML scrape — extract `og:title` (500ms rate limit)
