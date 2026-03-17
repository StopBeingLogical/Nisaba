# NISABA Changelog

## 2026-03-12

### Steam wishlist: store-page scrape fallback for unnamed apps

Added a third name-resolution pass in `batchFetchSteamNames` for wishlist entries
the `appdetails` API refuses to name (unreleased, coming-soon, or restricted apps
that return `success: false`).

Flow is now:
1. Batch `appdetails` API call
2. Individual `appdetails` fallback (300 ms rate limit)
3. **New:** store page HTML scrape — reads the first 8 KB of
   `store.steampowered.com/app/{id}/`, extracts the `og:title` meta tag, and
   strips the " on Steam" suffix and any sale prefix ("Save X% on …").
   Age-gate bypass cookies included. 500 ms rate limit.

Fixes ~46 wishlist entries stuck as "Steam App XXXXXXX".

## 2026-03-11

### Image proxy for remote access

Added a server-side image caching proxy at `GET /img/proxy?url=<encoded>`.
All game cover art and store/deck/ProtonDB icon assets now route through the server
instead of loading directly from upstream CDNs (Steam CDN, cdn.simpleicons.org, IGDB, etc.).
This ensures the UI renders correctly when accessed from networks that block external
image CDNs (e.g. corporate firewalls).

- New handler `handlers/imgproxy.go`: fetches URLs, caches to `/data/imgcache/` by
  SHA-256 hash, serves with 7-day Cache-Control headers. SSRF-safe: only http/https allowed.
- `TemplateFuncMap` now includes `proxyURL` pipe function.
- All `<img src>` attributes in library and wishlist templates now pipe through `proxyURL`.
- `app.css` store/deck/proton badge `background-image` URLs updated to use proxy path.

### Log persistence across server restarts

`InitLogCapture` now opens `/data/nisaba.log` for append and tees all log output to it.
On startup, the log file is read to seed the in-memory ring buffer, so operation logs
from previous server sessions are immediately visible on `/logs` — fixing the issue
where the console section showed nothing after a container restart.

### Arrow visibility fix on game detail store links

The `↗` link on the library game detail page (Owned On card) now uses the same
button-style treatment as wishlist store links: `bg-gray-800 hover:bg-gray-700`
pill with hover color, replacing the near-invisible `text-gray-500` bare arrow.

## 2026-03-07

### Pricing: replaced ITAD + reseller scrapers with gg.deals API

**Previous architecture:**
1. ITAD API — current best price + historical low for Steam-linked wishlist games
2. instant-gaming.com scraper — grey market keyshop prices (concurrent goroutine)
3. loaded.com scraper — grey market keyshop prices (concurrent goroutine)
4. Allkeyshop fallback scraper — grey market aggregator, only for games not found on IG/Loaded

**Problems encountered:**
- Allkeyshop (AKS) uses Akamai bot protection. Initial testing with Go's default TLS
  client caused 14 hanging connections which triggered an IP-level ban. Fixed by
  implementing `utls` (Chrome TLS fingerprint impersonation) + HTTP/2 via
  `golang.org/x/net/http2`. Added cookie jar, `sec-ch-ua`/`sec-fetch-*` headers,
  request jitter (2–5s random), and homepage warmup to reduce bot scoring.
- Even with those mitigations, running AKS against the full wishlist (~50 games)
  triggered Akamai's behavioral analysis. Moved AKS to a fallback-only role
  (only queried for games not found on IG or Loaded), reducing request volume enough
  to stay under the threshold.
- gg.deals was identified as a better aggregator — covers both retail stores and
  keyshops in a single API call. Initial investigation showed their website is behind
  Cloudflare managed challenge (requires JS execution, not bypassable with utls).
  However, they have a separate `api.gg.deals` subdomain not behind Cloudflare.
  The correct endpoint is `https://api.gg.deals/v1/prices/by-steam-app-id/`.

**New architecture:**
1. gg.deals API — batch fetch up to 100 Steam App IDs per request, returns
   `currentRetail`, `currentKeyshops`, `historicalRetail`, `historicalKeyshops`
   in one call. Replaces ITAD (retail pricing) and all reseller scrapers (keyshop
   pricing) for Steam-linked wishlist entries.
2. instant-gaming + loaded.com scrapers (concurrent) — retained for wishlist entries
   that have no Steam App ID (e.g. manually added EA/Ubisoft games).
3. Allkeyshop fallback — retained for non-Steam entries not found on IG/Loaded.

**Rate limits:** 100 IDs/minute, 1000/hour. Full wishlist of ~50 games = 1 request.

**Why not keep ITAD?** gg.deals covers the same retail stores as ITAD plus keyshops,
returns historical lows, and requires no per-game ID lookup step. ITAD required a
two-phase process (Steam App ID → ITAD ID lookup, then batch price fetch). gg.deals
takes Steam App IDs directly.

**API key:** stored in app config as `ggdeals.api_key`.

---

### Concurrent reseller scraping

Refactored `SyncResellerPricing` to run instant-gaming and loaded.com in parallel
goroutines, each with their own 2-second rate limiter. Halves wall-clock time for
the reseller scraping phase. Results are merged via a channel; DB write fires once
per game when both sites have responded.

### Manual game entry

Added `GET /library/add` + `POST /library/add` form for manually adding games
(EA, Ubisoft, etc.). Includes IGDB search integration: type a title, pick from
results, form pre-fills with cover art, description, and release date. If an IGDB
result is selected the game is marked as `matched` immediately (skips review queue).

### Sync improvements

- ProtonDB ratings sync (`POST /sync/proton`)
- Steam Deck status sync (`POST /sync/deck`)
- Heroic library file upload via browser file picker
- Per-step progress bar in sync status panel (step label + done/total counter)
- `sync_errors` table for persistent error logging across restarts
- Dedicated log viewer at `/logs`
- Pricing summary now breaks down: ITAD updated/not found, resellers cheaper/not
  listed/already cheaper/errors
