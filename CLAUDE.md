# NISABA — Claude Instructions

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
go build ./...

# 2. Sync to server (always use these excludes)
rsync -av --exclude='.git' --exclude='*.db' --exclude='imgcache' --exclude='._*' \
  /Volumes/Shuttle/projects/nisaba/ \
  truenas_admin@192.168.3.174:/mnt/MemoryAlpha/nisaba/source/

# 3. Deploy (requires interactive terminal for sudo)
ssh -t truenas_admin@192.168.3.174 "cd /mnt/MemoryAlpha/nisaba/source && bash deploy.sh"
```

### Sync server → local (backup)
```bash
rsync -av --exclude='*.db' --exclude='imgcache' --exclude='._*' \
  truenas_admin@192.168.3.174:/mnt/MemoryAlpha/nisaba/source/ \
  /Volumes/Shuttle/projects/nisaba/
```

### Query the live DB (sqlite3 is on the host, not in the container)
```bash
sqlite3 /mnt/MemoryAlpha/nisaba/data/nisaba.db "SELECT ..."
```

## Key Paths
| Location | Path |
|---|---|
| Production source | `truenas_admin@192.168.3.174:/mnt/MemoryAlpha/nisaba/source/` |
| Live database | `truenas_admin@192.168.3.174:/mnt/MemoryAlpha/nisaba/data/nisaba.db` |
| Local working copy | `/Volumes/Shuttle/projects/nisaba/` |
| DB inside container | `/data/nisaba.db` |

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
