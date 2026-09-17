# NISABA

Personal game library and wishlist manager. Self-hosted web app for tracking owned games across multiple storefronts, monitoring wishlist prices, and managing enrichment metadata.

Named after the Sumerian goddess of writing and record-keeping.

**Designed by [StopBeingLogical](https://github.com/StopBeingLogical) · Built by [Claude](https://claude.ai)**

> **Working on Nisaba?** Start at [`SESSION_SEED.md`](SESSION_SEED.md), then
> [`spec/README.md`](spec/README.md), [`spec/STATE.md`](spec/STATE.md), and your
> assigned entry in [`spec/tasks.yaml`](spec/tasks.yaml). `CLAUDE.md` remains
> the reference for how the machine works.

---

## Features

- **Unified library** — aggregates games from Steam, GOG, Epic, and Amazon into a single browsable grid
- **Playnite Sync** — automated library synchronization from Playnite (Windows) via PowerShell; imports metadata, playtime, and ownership for every store except GOG
- **GOG library sync** — GOG maintains itself server-side: the stored OAuth refresh token is renewed and the library view is pulled once a day, no client software involved
- **Wishlist tracking** — imports the Steam wishlist, resolves names for unreleased/restricted titles
- **Price monitoring** — ITAD is authoritative for current price, storefront name and historical low; GG.deals is kept as a comparison source and reseller scrapers cover non-Steam entries
- **IGDB enrichment** — auto-matches games to IGDB records for cover art, descriptions, genres, and release dates; unmatched games go to a review queue
- **Steam Deck compatibility** — displays Valve's official compatibility rating, and filters by it; **nothing refreshes it** — the fetcher has no caller (see `spec/OPEN.md`)
- **ProtonDB ratings** — displays community Proton ratings for Steam-owned games; **nothing refreshes them** — the fetcher has no caller (see `spec/OPEN.md`)
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

Ownership arrives over two independent paths. Steam is fetched from Valve; every
other store comes from Playnite on Windows, which posts to `/api/sync/playnite`:

```
Steam ownership (Steam Web API)                          ─┐
Playnite automated sync (all stores except GOG;           ├─→ IGDB enrichment → RAWG fallback → review queue
  title-based deduplication fallback)                    ─┤
GOG library (server-side, once a day)                    ─┘
```

**Notes:**

- The Playnite sync excludes games in the "Steam Family Sharing" category, so
  only personal ownership is tracked, and it does not send GOG games — the
  server owns that store.
- The GOG library sync runs on its own daily schedule (`sync.gog_hour`) from
  GOG's library view, refreshing its own OAuth token first.

### Full sync sequence

The manual **Full sync** on `/sync` runs these steps in order:

```
Steam ownership → Steam wishlist → ITAD pricing → GG.deals comparison → reseller pricing → IGDB enrichment
```

Only the Steam wishlist is imported; GOG's is retired. `go build ./...` aside,
the authoritative step list is `SyncAll` in `handlers/sync.go`.

### Pricing pipeline

```
ITAD — authoritative for best_current_price, best_current_store, best_price_url,
       historical low and price history. IDs resolve in bulk (100 Steam App IDs
       per lookup request), so the whole wishlist costs ~11 requests.
  └─ GG.deals comparison — writes gg_deals_price / gg_deals_url only, never the
       ITAD columns; a cheaper GG.deals price surfaces as a wishlist callout
  └─ Reseller scrapers — instant-gaming + loaded.com (non-Steam entries,
       concurrent), Allkeyshop fallback (utls Chrome TLS fingerprint to bypass
       Akamai)
```

Prices refresh themselves once a day; the full sequence above stays manual.

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

The mystery-pack scrape endpoints are **unauthenticated** — `main.go` mounts them
outside the auth group with the comment "no auth, raw data only": they queue,
diff and apply raw scrape payloads, and `handlers/mystery_packs_api.go` checks no
session or secret. The Playnite sync endpoint is public too, but validates a
pre-shared `X-Nisaba-Secret` header when `sync.api_secret` is configured.

---

## Configuration

Settings are stored in the `app_config` table and managed through the `/settings` UI:

| Key | Description |
|---|---|
| `igdb.client_id` / `igdb.client_secret` | IGDB API credentials (Twitch app) |
| `rawg.api_key` | RAWG API key (fallback enrichment) |
| `itad.api_key` | IsThereAnyDeal API key — authoritative pricing, storefront names |
| `ggdeals.api_key` | gg.deals API key (comparison price only) |
| `steam.api_key` | Steam Web API key |
| `steam.user_id` | Your Steam ID64 |
| `gog.refresh_token` | GOG OAuth refresh token — the only GOG credential that has to be pasted |
| `gog.access_token` / `gog.access_token_expires` | Cached GOG access token and its expiry; rewritten on every refresh |
| `gog.client_secret` | Galaxy client secret (public constant) the refresh call requires |
| `sync.api_secret` | Pre-shared secret for automated Playnite sync |
| `sync.price_hour` | Hour (0–23, container clock) the daily price sync runs; not in the Settings UI, default 11 |
| `sync.gog_hour` | Hour (0–23, container clock) the daily GOG library sync runs; not in the Settings UI, default 11 |

---

## Notes

- Two jobs refresh themselves once a day (`sync/schedule.go`): prices (ITAD then GG.deals only, type `pricing`) and the GOG library (type `ownership`), each with its own window so the price job stays price-only. The full sync stays manual because it also re-imports ownership, wishlists, resellers and enrichment
- `gog.client_secret` must be set for GOG to refresh its own token. Without it the sync falls back to the stored access token and keeps working only while GOG still honours that token
- SQLite is configured with a single open connection (`SetMaxOpenConns(1)`) — this is intentional and must not be changed
- Schema migrations are additive only (`ALTER TABLE ADD COLUMN`), idempotent, and run on every startup
- The AKS scraper uses [`utls`](https://github.com/refraction-networking/utls) to impersonate a Chrome TLS fingerprint and avoid Akamai bot detection
