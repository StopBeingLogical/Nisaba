# NISABA

Personal game library and wishlist manager. Self-hosted web app for tracking owned games across multiple storefronts, monitoring wishlist prices, and managing enrichment metadata.

Named after the Sumerian goddess of writing and record-keeping.

**Designed by [StopBeingLogical](https://github.com/StopBeingLogical) · Built by [Claude](https://claude.ai)**

---

## Features

- **Unified library** — aggregates games from Steam, GOG, Epic, and Amazon into a single browsable grid
- **Playnite Sync** — automated library synchronization from Playnite (Windows) via PowerShell; imports metadata, playtime, and ownership for all non-Steam sources
- **Wishlist tracking** — imports wishlists from Steam and GOG, resolves names for unreleased/restricted titles
- **Price monitoring** — fetches current prices, keyshop prices, and historical lows via gg.deals; falls back to reseller scrapers for non-Steam entries
- **IGDB enrichment** — auto-matches games to IGDB records for cover art, descriptions, genres, and release dates; unmatched games go to a review queue
- **Steam Deck compatibility** — fetches Valve's official compatibility rating for every Steam game
- **ProtonDB ratings** — community Proton compatibility ratings for Steam-owned games
- **Install state tracking** — reads Steam `appmanifest_*.acf` files directly from the browser via the File System Access API; no client software required
- **Image proxy** — all external images (Steam CDN, IGDB, simpleicons.org) are routed through the server and cached locally, so the UI renders correctly on firewalled networks
- **Persistent logs** — all sync output is written to disk and visible in the UI across container restarts
- **Mystery pack analysis** — tracks multi-seller game bundle prices and computes ROI, overlap, and value metrics across keyshops (G2A, K4G, Kinguin, Eneba, Fanatical)
- **Extension integration** — Chrome extension can scrape pack pages and import them with diff review before applying changes

---

## Stack

| Layer | Technology |
|---|---|
| Language | Go 1.25 |
| Router | Chi v5 |
| Database | SQLite (WAL mode, single writer) |
| Templates | `html/template` (embedded at build time) |
| Frontend | HTMX + Tailwind CSS (CDN, no build step) |
| Deployment | Docker on TrueNAS SCALE |

---

## Architecture

```
main.go                    router, migrations, embed
db/store.go                all SQL queries (hand-rolled)
db/models.go               Go structs for DB rows
handlers/                  HTTP handlers (one file per feature area)
sync/                      store sync, enrichment, and pricing integrations
templates/                 html/template files
static/                    app.css + htmx.min.js
schema.sql                 SQLite schema
```

### Sync pipeline

```
Steam ownership → Playnite automated sync (Title-based deduplication fallback) → IGDB enrichment → RAWG fallback → review queue
```

**Note:** The Playnite sync automatically excludes games in the "Steam Family Sharing" category to ensure only personal ownership is tracked.

### Pricing pipeline

```
gg.deals API (Steam-linked games, batch 100/req)
  └─ instant-gaming + loaded.com scrapers (non-Steam entries, concurrent)
       └─ Allkeyshop fallback (utls Chrome TLS fingerprint to bypass Akamai)
```

### Wishlist name resolution

```
1. Match against owned library by Steam App ID
2. Steam appdetails API (batch, then per-game fallback at 300ms/req)
3. Store page HTML scrape — extracts og:title, bypasses age gate (500ms/req)
```

---

## Deployment

Runs as a single Docker container. Database and image cache are mounted from the host.

```yaml
# docker-compose.yml (abridged)
services:
  nisaba:
    build: .
    ports:
      - "8090:8080"
    volumes:
      - /path/to/data:/data
    environment:
      - NISABA_DB_PATH=/data/nisaba.db
```

```bash
# Build and start
docker compose up --build -d

# Set initial password
docker exec nisaba /app/nisaba -set-password 'yourpassword'
```

---

## Extension Integration

Chrome extension support for importing mystery game packs with semi-automated diffing:

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/mystery-packs/scrape/queue` | POST | Queue scraped page data for review |
| `/api/mystery-packs/scrape/review?queue={id}` | GET | Retrieve diff (changes) vs. stored packs |
| `/api/mystery-packs/scrape/apply` | POST | Apply user-approved changes |
| `/api/sync/playnite` | POST | Automated library sync from Playnite (PowerShell) |

All mystery-pack endpoints require session authentication. The Playnite sync endpoint supports either a session cookie or a pre-shared `X-Nisaba-Secret` header.

---

## Configuration

Settings are stored in the `app_config` table and managed through the `/settings` UI:

| Key | Description |
|---|---|
| `igdb.client_id` / `igdb.client_secret` | IGDB API credentials (Twitch app) |
| `rawg.api_key` | RAWG API key (fallback enrichment) |
| `ggdeals.api_key` | gg.deals API key (pricing) |
| `steam.api_key` | Steam Web API key |
| `steam.steam_id` | Your Steam ID64 |
| `gog.refresh_token` | GOG OAuth refresh token |
| `sync.api_secret` | Pre-shared secret for automated Playnite sync |

---

## Notes

- SQLite is configured with a single open connection (`SetMaxOpenConns(1)`) — this is intentional and must not be changed
- Schema migrations are additive only (`ALTER TABLE ADD COLUMN`), idempotent, and run on every startup
- The AKS scraper uses [`utls`](https://github.com/refraction-networking/utls) to impersonate a Chrome TLS fingerprint and avoid Akamai bot detection
