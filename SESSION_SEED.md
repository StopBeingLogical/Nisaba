# NISABA — Session Seed

Read this at the start of any session to get up to speed quickly.
Production source lives on Atlas (TrueNAS NAS). Local copy at `/Volumes/Shuttle/projects/gamerepo/` may lag behind — always treat the server as canonical.

---

## What it is

Unified personal game library + wishlist manager. Web UI, self-hosted.
~2000 games across Steam, GOG, Epic, Amazon. Wishlist on Steam + GOG.

**Running at:** `nisaba.damnaliens.us` (nginx reverse proxy → port 8090 → container 8080)
**Stack:** Go 1.25 · Chi router · SQLite (WAL) · html/template · HTMX · Tailwind CSS · Docker on TrueNAS SCALE (Atlas, 192.168.3.174)

---

## Server access

```bash
ssh truenas_admin@192.168.3.174   # key with passphrase — run ssh-add first
```

Key paths on Atlas:
- Source: `/mnt/MemoryAlpha/nisaba/source/`  ← edit here
- App:    `/mnt/MemoryAlpha/nisaba/app/`     ← keep in sync with source
- DB:     `/mnt/MemoryAlpha/nisaba/data/nisaba.db`
- Logs:   `/mnt/MemoryAlpha/nisaba/data/nisaba.log`
- Images: `/mnt/MemoryAlpha/nisaba/data/imgcache/`

**Deploy:** `./deploy.sh` from `/mnt/MemoryAlpha/nisaba/source/` (sudo password required).
After edits: rsync changed files to `app/` and `source/`, then run `./deploy.sh`.

**Compile check locally:** Go is at `/opt/homebrew/bin/go`. Rsync source → `/Volumes/Shuttle/projects/gamerepo/`, run `go build ./...`.

---

## Project structure

```
main.go                    router, migrations, embed
db/store.go                all SQL (hand-rolled; sqlc deferred)
db/models.go               Go structs for DB rows
handlers/
  handlers.go              Handler struct, TemplateFuncMap, render helpers
  dashboard.go             /
  library.go               /library, /library/{id}
  wishlist.go              /wishlist, /wishlist/{id}
  sync.go                  /sync — trigger endpoints + status polling
  enrichment.go            /review — IGDB match queue
  settings.go              /settings
  gog_auth.go              /auth/gog — token paste + curl push
  logbuffer.go             in-memory ring buffer + file persistence → /data/nisaba.log
  logs.go                  /logs — console output + sync error table
  imgproxy.go              /img/proxy — caches external images to /data/imgcache/
sync/
  steam.go                 Steam ownership
  igdb.go                  IGDB enrichment (batched 10/req)
  rawg.go                  RAWG fallback enrichment
  ggdeals.go               gg.deals pricing API (retail + keyshops, batch 100/req)
  itad.go                  ITAD (retained, not primary)
  resellers.go             instant-gaming + loaded.com (concurrent, for non-Steam)
  allkeyshop.go            AKS fallback (utls + HTTP/2, jitter — avoids Akamai ban)
  chromeclient.go          Chrome TLS fingerprint HTTP client
  wishlist.go              Steam wishlist sync + 3-stage name resolution
  gog_wishlist.go          GOG wishlist sync
  heroic.go                Heroic library file import
  protondb.go              ProtonDB ratings
  steam_deck.go            Steam Deck status
templates/                 html/template files (embedded at build time)
static/app.css             component CSS (store/deck/proton badges proxy simpleicons.org)
static/htmx.min.js         HTMX 1.9.12
schema.sql                 SQLite schema
schema/                    TypeScript schema docs + pipeline notes
store_library_files/       uploaded library JSON/text files (epic, gog, legendary, nile)
```

---

## Key decisions & architecture

**Enrichment pipeline:** IGDB primary → RAWG fallback → store scrape → `needs_review` queue
**Pricing:** gg.deals API (single call, up to 100 Steam IDs, covers retail + keyshops + historical).
  Reseller scrapers (IG + loaded.com, concurrent) retained for non-Steam entries.
  AKS as final fallback; uses utls Chrome TLS fingerprint to avoid Akamai bans.
**Wishlist name resolution (3 stages):**
  1. Look up owned library by Steam App ID (free)
  2. Steam `appdetails` API — batch then per-game fallback (300 ms rate limit)
  3. Store page HTML scrape — reads first 32 KB, extracts `og:title`, bypasses age gate
     with cookies, forces `?cc=US&l=english` (500 ms rate limit)
**Image proxy:** All external images (artwork CDNs, simpleicons.org badges) route through
  `/img/proxy?url=<encoded>` → cached on disk in `/data/imgcache/` → 7-day browser cache.
  Prevents breakage on firewalled networks (e.g. work).
**Logs:** `log.Printf` → ring buffer (500 lines) + `/data/nisaba.log` (append, persists restarts).
  `/logs` page seeds from file on startup so previous-session logs are visible.
**SQLite:** max 1 open connection (single-writer), WAL mode, 5 s busy timeout.
  Additive migrations via `ALTER TABLE ADD COLUMN` (idempotent, dup errors ignored).
**Auth:** network-level only (VPN for external access). No in-app login.

---

## Devices

- **Praxis** — Steam Deck (SteamOS)
- **Ergaster** — Acemagic MiniPC (Windows)
- **Atlas** — TrueNAS SCALE NAS (server)

---

## Active known issues / recent work

See `CHANGELOG.md` in this directory for full history.

Recent sessions covered:
- Image proxy (all external images routed through server)
- Log persistence across container restarts
- Arrow/button visibility fix on game detail store links
- Steam wishlist name scrape fallback for unreleased/restricted games
- `deploy.sh` script for rebuilding + restarting the container

---

## Useful DB queries

```bash
# Connect
sqlite3 /mnt/MemoryAlpha/nisaba/data/nisaba.db

# Wishlist entries still without a proper name
SELECT id, title FROM wishlist_entries WHERE title LIKE 'Steam App %';

# Recent sync errors
SELECT sync_type, message, created_at FROM sync_errors ORDER BY id DESC LIMIT 20;

# Enrichment status breakdown
SELECT enrichment_status, COUNT(*) FROM games GROUP BY enrichment_status;
SELECT enrichment_status, COUNT(*) FROM wishlist_entries GROUP BY enrichment_status;
```
