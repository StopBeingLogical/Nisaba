# Database access contract

Governs every read and write against `nisaba.db`. Does not govern what the UI
shows — see `library-view.md`.

## Rules

- `db/store.go` is the **only** place SQL is written. No SQL in `handlers/`, no
  SQL in `sync/`, no code generator. `sqlc.yaml` and `queries/` are deleted by
  `BASE-002`; do not re-add them.
- `sqlDB.SetMaxOpenConns(1)` is never removed or raised. SQLite permits one
  writer; more connections produce `SQLITE_BUSY` even in WAL mode.
- Schema changes live in `runMigrations()` in `main.go`, nowhere else.
- Migrations are **additive and idempotent**: `ALTER TABLE ADD COLUMN`,
  `CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`. Duplicate-column
  errors are swallowed deliberately. Never `DROP`, never rename, never `UPDATE`
  existing rows from a migration.
- A one-off data backfill is not a migration and does not belong in
  `runMigrations()`. It requires its own task and Bobby's ruling.
- Store methods take a params struct and return typed rows. Do not return
  `*sql.Rows` or raw `map` values across the package boundary.
- The live database at `atlas:/mnt/MemoryAlpha/nisaba/data/nisaba.db` is
  read-only to every task. Work against a local copy.

## Shapes

`ListGamesParams` (`db/store.go:25-38`) — the filter/sort/page contract for the
library. Fields and their meaning are fixed; adding a field is a contract
change:

```go
PlayStatus *string   // exact match on games.play_status
Store      *string   // JOIN game_stores WHERE store = ? AND owned = 1
SteamDeck  *string   // exact match on games.steam_deck_verified
Platform   string    // "windows" | "mac" | "linux" → g.<platform> = 1
Search     *string   // g.title LIKE %?%
Genres     []string  // AND filter — one JOIN per genre
Tags       []string  // AND filter — one JOIN per tag
Installed  bool      // g.is_installed = 1
Favorites  bool      // g.is_favorite = 1
Sort       string    // "" → sort_title ASC | "added" | "playtime" | "rating"
Limit      int       // 0 = no limit
Offset     int
```

`GameListRow` (`db/models.go:36-56`) — what the library grid renders. Every
field is currently populated by `ListGames`. A performance change may alter
*how* a field is produced but must not drop one or change its type.

Baseline rows always filtered out: `g.is_hidden = 0` and `g.parent_id IS NULL`.

## Changing this contract

Not a task-level decision. A task that cannot be completed without changing
this file stops, parks the question, and reports. `PERF-003` is the designated
place to propose a change to how `ListGames` produces its rows.
