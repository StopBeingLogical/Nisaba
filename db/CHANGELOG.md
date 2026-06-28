# Nisaba Database — Changelog

Schema changes, migrations, and data model updates.

---

## [Unreleased]

### Added (2026-06-28)
- `LinkWishlistToLibraryByStore()` and `DeleteLinkedWishlistEntries()` — wishlist autoclean pipeline
- `CountMatchingGames()` — pagination count query with same filters as ListGames
- `LowestPrices()` — window function query returning 3 cheapest prices per wishlist entry
- Migration: `game_genres(game_id)`, `game_tags(game_id)`, `game_stores(game_id, owned)` indexes
- Migration: composite `games(is_hidden, parent_id)` index
- Migration: `games(igdb_id)` index
- Migration: `price_thresholds` table with 4 seeded defaults (Instant Buy, Consider, Moderate, Sale Watch)

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
