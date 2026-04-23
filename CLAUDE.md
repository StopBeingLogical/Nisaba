# NISABA — Session Seed & Instructions

**Project:** Nisaba (Game Library & Wishlist Manager)  
**Type:** Go web application (Docker/TrueNAS)  
**Status:** Production (Atlas), development at `~/nextcloud/Mneme/code/nisaba/`  
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
cd ~/nextcloud/Mneme/code/nisaba
go build ./...

# 2. Sync to server (always use these excludes)
rsync -av --exclude='.git' --exclude='*.db' --exclude='imgcache' --exclude='._*' \
  ~/nextcloud/Mneme/code/nisaba/ \
  truenas_admin@192.168.3.174:/mnt/MemoryAlpha/nisaba/source/

# 3. Deploy (requires interactive terminal for sudo)
ssh -t truenas_admin@192.168.3.174 "cd /mnt/MemoryAlpha/nisaba/source && bash deploy.sh"
```

### Sync server → local (backup)
```bash
rsync -av --exclude='*.db' --exclude='imgcache' --exclude='._*' \
  truenas_admin@192.168.3.174:/mnt/MemoryAlpha/nisaba/source/ \
  ~/nextcloud/Mneme/code/nisaba/
```

### Query the live DB (sqlite3 is on the host, not in the container)
```bash
sqlite3 /mnt/MemoryAlpha/nisaba/data/nisaba.db "SELECT ..."
```

## Key Paths
| Location | Path |
|---|---|
| Local working copy | `~/nextcloud/Mneme/code/nisaba/` |
| Production source | `atlas:/mnt/MemoryAlpha/nisaba/source/` |
| Live database | `atlas:/mnt/MemoryAlpha/nisaba/data/nisaba.db` |
| DB inside container | `/data/nisaba.db` |

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
- Public routes: `/`, `/library`, `/library/{id}`, `/wishlist`, `/wishlist/{id}`, `/static/*`, `/img/proxy`, `/sync/status`, `/auth/login`
- Everything else requires auth

## Database Migrations Added (beyond schema.sql)
All in `runMigrations()` in `main.go`:
- `wishlist_entries.flag_remove` — INTEGER, default 0
- `price_thresholds` table
- `sync_errors` table + index
- `wishlist_entries.best_price_url` — TEXT

## Image Proxy
All external images are served through `/img/proxy?url=...` to bypass corporate firewall CDN blocks. Disk cache at `/data/imgcache/`, 7-day TTL. The `proxyURL` template func handles encoding. CSS background images in `static/app.css` are also proxied.

## IGDB / Enrichment Notes
- `bestMatch()` in `sync/igdb.go` prefers PC platform (platform ID 6) over mobile/console variants when multiple entries share the same normalised title — prevents matching iOS versions of games
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
