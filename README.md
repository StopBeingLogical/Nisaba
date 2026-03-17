# NISABA

Personal game library and wishlist manager. Self-hosted web app for tracking owned games across multiple storefronts, monitoring wishlist prices, and managing enrichment metadata.

Named after the Sumerian goddess of writing and record-keeping.

---

## Features

- **Unified library** — aggregates games from Steam, GOG, Epic, and Amazon into a single browsable grid
- **Wishlist tracking** — imports wishlists from Steam and GOG, resolves names for unreleased/restricted titles
- **Price monitoring** — fetches current prices, keyshop prices, and historical lows via gg.deals; falls back to reseller scrapers for non-Steam entries
- **IGDB enrichment** — auto-matches games to IGDB records for cover art, descriptions, genres, and release dates; unmatched games go to a review queue
- **Steam Deck compatibility** — fetches Valve's official compatibility rating for every Steam game
- **ProtonDB ratings** — community Proton compatibility ratings for Steam-owned games
- **Install state tracking** — reads Steam `appmanifest_*.acf` files directly from the browser via the File System Access API; no client software required
- **Image proxy** — all external images (Steam CDN, IGDB, simpleicons.org) are routed through the server and cached locally, so the UI renders correctly on firewalled networks
- **Persistent logs** — all sync output is written to disk and visible in the UI across container restarts

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
Steam ownership → GOG/Epic/Amazon (Heroic) → IGDB enrichment → RAWG fallback → review queue
```

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

---

## Notes

- SQLite is configured with a single open connection (`SetMaxOpenConns(1)`) — this is intentional and must not be changed
- Schema migrations are additive only (`ALTER TABLE ADD COLUMN`), idempotent, and run on every startup
- The AKS scraper uses [`utls`](https://github.com/refraction-networking/utls) to impersonate a Chrome TLS fingerprint and avoid Akamai bot detection
