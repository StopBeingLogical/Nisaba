# Nisaba Database — Changelog

Schema changes, migrations, and data model updates.

---

## [Unreleased]

### Status
- No pending changes

---

## [2026-03-12 to 2026-03-07] — Migration Additions

### Added Migrations (in `main.go::runMigrations()`)
- `wishlist_entries.flag_remove` — INTEGER, default 0
- `price_thresholds` table
- `sync_errors` table + index (persistent error logging)
- `wishlist_entries.best_price_url` — TEXT field

### Notes
- All migrations are additive (`ALTER TABLE ADD COLUMN`)
- Idempotent; duplicate-column errors silently ignored
- SQLite config: single writer (`SetMaxOpenConns(1)`), WAL mode, 5s busy timeout

See `schema.sql` for base schema and `main.go` for all migrations.
