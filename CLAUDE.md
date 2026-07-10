# NISABA — Session Seed & Instructions

**Project:** Nisaba (Game Library & Wishlist Manager)  
**Type:** Go web application (Docker/TrueNAS)  
**Status:** Production (Atlas), development at `~/code/nisaba/`  
**Live:** `nisaba.damnaliens.us` (port 8090 → container 8080)  
**Maintenance:** Changelog system in `.changelog/UNRELEASED.md`

---

## Session Start Checklist

When you start a session on Nisaba:

1. **Load project context** (3 min)
   - Read `README.md` (feature overview)
   - Review `CLAUDE.md` → "Deployment" section for current workflow

2. **Check for changelog updates** (1 min)
   - Ask: "Should I consolidate pending changelog entries from `.changelog/UNRELEASED.md` into the appropriate CHANGELOG.md files?"
   - If yes: move one-liners to their destination files and reset UNRELEASED.md
   - If no: continue

3. **Verify access** (if deploying)
   - Run `ssh-add` (key with passphrase)
   - Confirm `ssh truenas_admin@192.168.3.174` works
   - Check live DB: `sqlite3 /mnt/MemoryAlpha/nisaba/data/nisaba.db "SELECT COUNT(*) FROM games;"`

---

## What This Is
Personal game library + wishlist manager. Go web app running in Docker on a TrueNAS SCALE NAS (Atlas) at `192.168.3.174`. Exposed publicly via Cloudflare tunnel, protected by session-cookie auth.

## Tech Stack
- **Go 1.25**, Chi v5 router, SQLite (WAL, single-writer), `html/template` with `//go:embed`
- **Frontend:** HTMX + TailwindCSS (CDN), no build step
- **DB layer:** Hand-rolled `db/store.go` (no ORM, no sqlc)
- **Docker:** single container on port 8090→8080, DB volume at `/data/`

## SSH & Deployment Workflow

### Access
```bash
ssh truenas_admin@192.168.3.174   # key with passphrase — run ssh-add first
```

### Deploy loop (always do all three steps)
```bash
# 1. Compile locally first — catch errors before touching the server
cd ~/code/nisaba
go build ./...

# 2. Sync to server (always use these excludes)
rsync -av --exclude='.git' --exclude='*.db' --exclude='imgcache' --exclude='._*' \
  ~/code/nisaba/ \
  truenas_admin@192.168.3.174:/mnt/MemoryAlpha/nisaba/source/

# 3. Deploy (requires interactive terminal for sudo)
ssh -t truenas_admin@192.168.3.174 "cd /mnt/MemoryAlpha/nisaba/source && bash deploy.sh"
```

### Sync server → local (backup)
```bash
rsync -av --exclude='*.db' --exclude='imgcache' --exclude='._*' \
  truenas_admin@192.168.3.174:/mnt/MemoryAlpha/nisaba/source/ \
  ~/code/nisaba/
```

### Query the live DB (sqlite3 is on the host, not in the container)
```bash
sqlite3 /mnt/MemoryAlpha/nisaba/data/nisaba.db "SELECT ..."
```

## Key Paths
| Location | Path |
|---|---|
| Local working copy | `~/code/nisaba/` |
| Production source | `atlas:/mnt/MemoryAlpha/nisaba/source/` |
| Live database | `atlas:/mnt/MemoryAlpha/nisaba/data/nisaba.db` |
| DB inside container | `/data/nisaba.db` |
| Git origin (Forgejo) | `ssh://git@192.168.3.174:2222/bobby/nisaba.git` |
| GitHub (remote `github`) | `https://github.com/StopBeingLogical/Nisaba.git` — auto-mirror, never push directly |

**Note:** `atlas` = `truenas_admin@192.168.3.174` (SSH shorthand if configured)

## Critical Constraints

### SQLite single-writer
`sqlDB.SetMaxOpenConns(1)` is intentional. Never remove it. SQLite allows only one writer; multiple connections cause `SQLITE_BUSY` errors even in WAL mode.

### Additive migrations only
Schema changes go in `runMigrations()` in `main.go` as `ALTER TABLE ADD COLUMN` statements. They must be idempotent — duplicate-column errors are silently ignored. Never DROP or rename columns. Never modify existing rows in migrations.

### No `sudo docker` via non-interactive SSH
`sudo` requires a TTY for password input. Always use `deploy.sh` (which handles this) or an interactive `-t` SSH session. Inline `ssh host "sudo docker ..."` will always fail.

### Shell passwords need single quotes
When passing passwords to `docker exec` or any shell command, always use single quotes to prevent special characters (`!`, `$`, `@`, etc.) from being interpreted:
```bash
sudo docker exec nisaba /app/nisaba -set-password 'your$password!'
```

### `._*` macOS resource forks
Always include `--exclude='._*'` in rsync commands. macOS creates `._filename` resource fork files that pollute the server.

## Auth System
- Session cookie (`nisaba_session`), HMAC-SHA256 signed, configurable timeout
- Username is hardcoded: `bobby`
- Password stored as bcrypt hash in `app_config` table under key `auth.password_hash`
- Set/change password: `sudo docker exec nisaba /app/nisaba -set-password 'password'`
- Public routes: `/`, `/library`, `/library/{id}`, `/wishlist`, `/wishlist/{id}`, `/static/*`, `/img/proxy`, `/sync/status`, `/auth/login`, `/api/sync/playnite` (secret required)
- Everything else requires auth

## Database Migrations Added (beyond schema.sql)
All in `runMigrations()` in `main.go`:
- `wishlist_entries.flag_remove` — INTEGER, default 0
- `price_thresholds` table
- `sync_errors` table + index
- `wishlist_entries.best_price_url` — TEXT
- Store Constraints: Removed hardcoded `CHECK` constraints on `store` columns via `migrateStoreConstraints()` to support arbitrary Playnite sources.

## Image Proxy
All external images are served through `/img/proxy?url=...` to bypass corporate firewall CDN blocks. Disk cache at `/data/imgcache/`, 7-day TTL. The `proxyURL` template func handles encoding. CSS background images in `static/app.css` are also proxied.

## IGDB / Enrichment Notes
- `bestMatch()` in `sync/igdb.go` prefers PC platform (platform ID 6) over mobile/console variants when multiple entries share the same normalised title — prevents matching iOS versions of games
- Playnite Sync: Uses `FindGameByTitle` as a fallback deduplication strategy if no matching store link is found.
- Steam cross-refs for non-Steam games: query IGDB `websites` field (not `external_games`) and parse `store.steampowered.com/app/NNNNN` URLs
- Category 1 in `external_games` is NOT reliable for Steam — use `websites` field instead

## Coding Conventions
- No comments on unchanged code
- No docstrings added to existing functions
- No error handling for impossible cases
- Secrets never rendered as `value=` in HTML — use boolean `FooSet bool` fields and render placeholder text instead
- Template partials registered in `handlers.New()` — add new ones there if needed

---

## Changelog Maintenance

**Location:** `.changelog/UNRELEASED.md` (working file), `**/CHANGELOG.md` (committed files)

**When making changes:**
- Add one-liners to `.changelog/UNRELEASED.md` under the relevant section (db/, handlers/, sync/, schema/)
- At session start, I'll ask if you want to consolidate entries
- Before committing, I'll move entries to the appropriate CHANGELOG.md files

**Format:** One-liner per bullet. Include date if notable: `- Added X feature (2026-04-23)`

**Top-level CHANGELOG.md records:** Major features, significant API integrations, deployment changes only. Detail goes in subdirectory files.

---

## Recent Session Context (2026-06-28)

### What was done
- **Wishlist autoclean:** `LinkWishlistToLibraryByStore()` and `DeleteLinkedWishlistEntries()` in db/store.go. `cleanupWishlistLinks()` helper in handlers/handlers.go. Replaces individual `LinkWishlistToLibrary()` calls in sync handlers.
- **Sync page cleanup:** Removed individual sync routes (Heroic, Steam, Wishlist, IGDB, etc.). Only 4 cards remain: Full Sync, Install State, Playnite Library Sync, plus Status/Recent Activity. Simplified handlers/sync.go, updated main.go routes.
- **Template caching:** All 12 page templates pre-parsed at startup in `New()`, stored in `pageTmpls` map. `render()` uses cached template — avoids re-parsing from embed.FS per request.
- **Performance indexes:** `game_genres(game_id)`, `game_tags(game_id)`, `game_stores(game_id, owned)`, composite `games(is_hidden, parent_id)`, `games(igdb_id)` via migrations.
- **Library pagination:** 200 games/page. `CountMatchingGames()` query. `gameGridData` struct. Numbered page selector with `pages()` template func.
- **Timing log:** Library handler logs elapsed time when >100ms.
- **Price thresholds:** Seeded 4 default rows (Instant Buy $2, Consider $5, Moderate $10, Sale Watch $20). Enables max_price radio filter on wishlist sidebar.
- **3 lowest prices:** `LowestPrices()` (window function, cheapest 3 per game). Displayed as "Best Prices" card on wishlist_detail.html. "View deal ↗" link using `best_price_url`.
- **StoreShortLabel improved:** Mapped `gg.deals/retail` → "GG Retail", `gg.deals/keyshop` → "GG Keyshops". Added explicit mappings for Steam, GOG, Epic, Amazon, Humble, Fanatical.

### Blockers / Known Issues
- **GG.deals store names are category-level only** — API returns `gg.deals/retail` or `gg.deals/keyshop`, never individual store names. The `best_current_store` and `wishlist_price_history.store` values are always these categories. Fix options: (a) switch to ITAD sync (provides real store names via `p.Current.Shop.Name`), (b) different GG.deals endpoint, (c) scrape GG.deals URL page.

### Working context
- Live DB stats: 3675 games, 558 wishlist entries, 15566 price history entries (403 of 558 have prices).
- Only GG.deals API configured (no ITAD).
- Repo on Forgejo at `ssh://git@192.168.3.174:2222/bobby/nisaba.git` (pushed this session).
- GitHub mirror at `https://github.com/StopBeingLogical/Nisaba.git`.
- Build: `go build ./...` and `go vet ./...` pass clean.
- Deploy path: `/mnt/MemoryAlpha/nisaba/source/` on TrueNAS, deployed via `deploy.sh`.
- Local clone now at `~/code/nisaba` (was at `~/nextcloud/Mneme/code/nisaba/`).

### Relevant files
- `db/store.go` — ListGames (LIMIT/OFFSET), CountMatchingGames, LowestPrices, LinkWishlistToLibraryByStore, DeleteLinkedWishlistEntries
- `handlers/handlers.go` — Handler struct, template funcs (add, sub, pages, storeShortLabel), template caching (pageTmpls), render() uses cached
- `handlers/sync.go` — Simplified to core sync handlers only
- `handlers/library.go` — Pagination (200/page), timing log
- `handlers/wishlist.go` — Detail handler loads LowestPrices
- `main.go` — Chi routes, runMigrations (all index + threshold seed migrations)
- `templates/game_grid_partial.html` — Numbered page selector with pages()
- `templates/wishlist_detail.html` — Pricing card, Best Prices card
- `templates/wishlist_grid_partial.html`, `wishlist_cards_partial.html` — Store names on price displays
- `sync/ggdeals.go` — GG.deals API, category-level store names only
- `sync/itad.go` — ITAD sync (not configured, but would provide real store names)
