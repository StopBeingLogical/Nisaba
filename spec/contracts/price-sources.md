# Price source contract

Governs how external pricing reaches wishlist entries.

**ITAD is authoritative** for `best_current_price`, `best_current_store`,
`best_price_url`, the historical low and `wishlist_price_history`. **GG.deals is
a comparison source**: it writes `gg_deals_price` / `gg_deals_url` and never the
ITAD columns. Both are ruled, not incidental — the reasoning is in
`PRODUCT.md` (`GG-001` → `GG-002`), and a provider may displace ITAD only if it
is measured to supply real storefront names.

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
  satisfy this contract. ITAD supplies these; the GG.deals comparison path does
  not write a store at all.
- `storeShortLabel` in `handlers/handlers.go` maps a stored identifier to its
  display label. A new provider's identifiers get mappings there; unmapped
  identifiers must degrade to showing the raw value, never to blank.
- Price history is append-only. Existing rows are not rewritten by a provider
  under any circumstance. The pre-cutover category-level rows were deleted once,
  by hand, as a ruled scorched earth (2026-09-16) — not by a provider and not by
  a recurring job.
- Switching which provider is authoritative for `best_current_*` is a
  configuration decision, not a code-path decision baked into a sync handler.

## Shapes

```sql
-- ITAD owns these three and the history below
wishlist_entries.best_current_price  REAL
wishlist_entries.best_current_store  TEXT   -- real storefront name
wishlist_entries.best_price_url      TEXT

-- GG.deals owns only these two: comparison, never authoritative
wishlist_entries.gg_deals_price      REAL
wishlist_entries.gg_deals_url        TEXT

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

## Visible consequence of the comparison

Where GG.deals is cheaper than ITAD's best price, the wishlist entry shows a
callout — the existence of a lower price plus a link to that game's GG.deals
page, and nothing about the listing or which shop it comes from.

## Changing this contract

Not a task-level decision. A provider may displace ITAD only if measured to
supply real storefront names, and Bobby rules on it before implementation.
