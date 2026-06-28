# Nisaba Handlers — Changelog

HTTP handlers and user-facing features.

---

## [Unreleased]

### Added (2026-06-28)
- Template caching: pre-parse all 12 page templates at startup in `New()`, stored in `pageTmpls` map
- `cleanupWishlistLinks()` helper — unified wishlist autoclean pipeline (link by IGDB → link by store → delete linked)
- Library pagination with numbered page selector (200 games/page)
- Timing logging in library handler (>100ms threshold)
- `storeShortLabel` template func — improved real store names for Steam, GOG, Epic, Amazon, Humble, Fanatical
- 3 lowest prices display on wishlist detail page with "View deal ↗" link

### Changed (2026-06-28)
- Sync page simplified: removed individual sync routes, only Full Sync, Install State, Playnite cards remain
- `render()` now uses cached templates instead of re-parsing from embed.FS on every request

---

## [2026-03-12 to 2026-03-07] — Major Features

### Image Proxy (`handlers/imgproxy.go`)
- `GET /img/proxy?url=<encoded>` — server-side caching proxy
- All game cover art and store/deck/ProtonDB icons routed through proxy
- Cache at `/data/imgcache/` with SHA-256 hash, 7-day TTL
- SSRF-safe: http/https only
- Template function: `proxyURL` pipe for encoding URLs

### Log Persistence (`handlers/logbuffer.go`)
- `/logs` endpoint — persistent log viewer
- Logs written to `/data/nisaba.log` (append mode)
- Ring buffer seeded from file on startup (persist across restarts)
- Console output + sync error table visible on UI

### Wishlist Features (`handlers/wishlist.go`)
- Three-stage name resolution for unnamed Steam wishlist entries
- Store page HTML scrape with age-gate bypass (fallback method)
- Manual game entry form (`GET/POST /library/add`)
- IGDB search integration in add form

### UI Improvements
- Arrow visibility fix on game detail store links (pill-style buttons)
- Per-step progress bar in sync status panel
- Pricing summary breakdown: ITAD, resellers, historical lows

---

## Handler Files
- `handlers.go` — Router, TemplateFuncMap, render helpers
- `dashboard.go` — `/` dashboard
- `library.go` — `/library`, `/library/{id}` (owned games)
- `wishlist.go` — `/wishlist`, `/wishlist/{id}` (wishlists)
- `sync.go` — `/sync` endpoints (trigger + status polling)
- `enrichment.go` — `/review` IGDB match queue
- `settings.go` — `/settings` configuration
- `gog_auth.go` — `/auth/gog` token handling
- `logbuffer.go` — Log capture + ring buffer
- `logs.go` — `/logs` console viewer
- `imgproxy.go` — `/img/proxy` image caching
