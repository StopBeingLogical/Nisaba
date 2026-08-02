# Price source contract

Governs how external pricing reaches wishlist entries. The current provider is
GG.deals; the round's second half may add or replace one.

## Rules

- A price provider lives in its own file under `sync/`, exposes a single
  `Sync<Provider>Pricing(store *db.Store, progress func(step string, done, total int))`
  entry point, and returns a typed result. `sync/ggdeals.go:30` and
  `sync/itad.go:28` both already follow this; a new provider matches it.
- Providers never write SQL. They call `db.Store` methods (`database.md`).
- API keys come from `app_config`, never from a literal, an env default, or a
  committed file. A provider with no configured key reports that and returns
  cleanly — it does not error the whole sync.
- A provider must supply, per priced entry: a price, a **store identifier**, and
  where available a deal URL. These land in `wishlist_entries.best_current_price`
  / `best_current_store` / `best_price_url` and as a row in
  `wishlist_price_history(wishlist_id, price, store, recorded_at)`.
- **`store` must be the name of an actual storefront** — "GOG", "Fanatical",
  "Humble". Category-level values (`gg.deals/retail`, `gg.deals/keyshop`) do not
  satisfy this contract. That gap is what this round exists to close
  (`CLAUDE.md:169`).
- `storeShortLabel` in `handlers/handlers.go` maps a stored identifier to its
  display label. A new provider's identifiers get mappings there; unmapped
  identifiers must degrade to showing the raw value, never to blank.
- Price history is append-only. Existing rows are not rewritten by a provider
  under any circumstance — what happens to the pre-cutover category-level rows
  is an open question in `OPEN.md` and needs its own task and ruling.
- Switching which provider is authoritative for `best_current_*` is a
  configuration decision, not a code-path decision baked into a sync handler.

## Shapes

```sql
wishlist_entries.best_current_price  REAL
wishlist_entries.best_current_store  TEXT   -- real storefront name
wishlist_entries.best_price_url      TEXT

wishlist_price_history (
    wishlist_id TEXT NOT NULL REFERENCES wishlist_entries(id) ON DELETE CASCADE,
    price       REAL NOT NULL,
    store       TEXT NOT NULL,
    recorded_at TEXT NOT NULL DEFAULT (datetime('now'))
)
```

`LowestPrices()` (`db/store.go`, window function) reads this history for the
"Best Prices" card on `wishlist_detail.html`. Its three-per-game output shape
does not change in this round.

## Changing this contract

Not a task-level decision. `GG-001` may recommend a change; Bobby rules before
`GG-002` starts.
