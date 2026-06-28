-- name: ListWishlistEntries :many
SELECT
    w.id,
    w.library_id,
    w.title,
    w.sort_title,
    w.enrichment_status,
    w.artwork,
    w.steam_deck_verified,
    w.currency,
    w.best_current_price,
    w.best_current_store,
    w.historical_low_price,
    w.historical_low_store,
    w.target_price,
    w.priority,
    w.date_added,
    (SELECT GROUP_CONCAT(tag) FROM wishlist_tags WHERE wishlist_id = w.id) AS tags,
    (SELECT GROUP_CONCAT(store) FROM wishlist_stores WHERE wishlist_id = w.id) AS stores
FROM wishlist_entries w
WHERE (sqlc.narg('search') IS NULL OR w.title LIKE '%' || sqlc.narg('search') || '%')
ORDER BY w.priority DESC, w.sort_title ASC;

-- name: GetWishlistEntry :one
SELECT w.*,
    (SELECT GROUP_CONCAT(tag) FROM wishlist_tags WHERE wishlist_id = w.id) AS tags
FROM wishlist_entries w
WHERE w.id = sqlc.arg('id');

-- name: GetWishlistStores :many
SELECT * FROM wishlist_stores WHERE wishlist_id = sqlc.arg('wishlist_id');

-- name: GetWishlistBundles :many
SELECT * FROM wishlist_bundles WHERE wishlist_id = sqlc.arg('wishlist_id') AND active = 1 ORDER BY expires_at ASC;

-- name: GetWishlistResellers :many
SELECT * FROM wishlist_resellers WHERE wishlist_id = sqlc.arg('wishlist_id') ORDER BY price ASC;

-- name: GetPriceHistory :many
SELECT * FROM wishlist_price_history WHERE wishlist_id = sqlc.arg('wishlist_id') ORDER BY recorded_at ASC;

-- name: CountWishlistEntries :one
SELECT COUNT(*) FROM wishlist_entries;

-- name: UpsertWishlistEntry :one
INSERT INTO wishlist_entries (
    id, library_id, igdb_id, rawg_id, title, sort_title,
    enrichment_status, last_enriched, artwork,
    steam_deck_verified, currency, itad_id,
    best_current_price, best_current_store,
    historical_low_price, historical_low_store,
    target_price, priority, preferred_store, notes, date_added
) VALUES (
    sqlc.arg('id'), sqlc.narg('library_id'), sqlc.narg('igdb_id'), sqlc.narg('rawg_id'),
    sqlc.arg('title'), sqlc.arg('sort_title'),
    sqlc.arg('enrichment_status'), sqlc.narg('last_enriched'), sqlc.narg('artwork'),
    sqlc.narg('steam_deck_verified'), sqlc.arg('currency'), sqlc.narg('itad_id'),
    sqlc.narg('best_current_price'), sqlc.narg('best_current_store'),
    sqlc.narg('historical_low_price'), sqlc.narg('historical_low_store'),
    sqlc.narg('target_price'), sqlc.arg('priority'),
    sqlc.narg('preferred_store'), sqlc.narg('notes'), sqlc.arg('date_added')
)
ON CONFLICT(id) DO UPDATE SET
    library_id            = excluded.library_id,
    igdb_id               = excluded.igdb_id,
    rawg_id               = excluded.rawg_id,
    title                 = excluded.title,
    sort_title            = excluded.sort_title,
    enrichment_status     = excluded.enrichment_status,
    last_enriched         = excluded.last_enriched,
    artwork               = excluded.artwork,
    steam_deck_verified   = excluded.steam_deck_verified,
    currency              = excluded.currency,
    itad_id               = excluded.itad_id,
    best_current_price    = excluded.best_current_price,
    best_current_store    = excluded.best_current_store,
    historical_low_price  = excluded.historical_low_price,
    historical_low_store  = excluded.historical_low_store,
    target_price          = excluded.target_price,
    priority              = excluded.priority,
    preferred_store       = excluded.preferred_store,
    notes                 = excluded.notes
RETURNING *;

-- name: UpdateWishlistPricing :exec
UPDATE wishlist_entries SET
    best_current_price   = sqlc.narg('best_current_price'),
    best_current_store   = sqlc.narg('best_current_store'),
    historical_low_price = sqlc.narg('historical_low_price'),
    historical_low_store = sqlc.narg('historical_low_store'),
    last_price_sync      = CURRENT_TIMESTAMP
WHERE id = sqlc.arg('id');

-- name: DeleteWishlistEntry :exec
DELETE FROM wishlist_entries WHERE id = sqlc.arg('id');

-- name: UpsertWishlistStore :exec
INSERT INTO wishlist_stores (wishlist_id, store, store_id, store_url)
VALUES (sqlc.arg('wishlist_id'), sqlc.arg('store'), sqlc.narg('store_id'), sqlc.narg('store_url'))
ON CONFLICT(wishlist_id, store) DO UPDATE SET
    store_id  = excluded.store_id,
    store_url = excluded.store_url;

-- name: UpsertWishlistTag :exec
INSERT OR IGNORE INTO wishlist_tags (wishlist_id, tag) VALUES (sqlc.arg('wishlist_id'), sqlc.arg('tag'));

-- name: DeleteWishlistTags :exec
DELETE FROM wishlist_tags WHERE wishlist_id = sqlc.arg('wishlist_id');

-- name: InsertPriceSnapshot :exec
INSERT INTO wishlist_price_history (wishlist_id, price, store, recorded_at)
VALUES (sqlc.arg('wishlist_id'), sqlc.arg('price'), sqlc.arg('store'), sqlc.arg('recorded_at'));

-- name: UpsertBundle :exec
INSERT INTO wishlist_bundles (wishlist_id, bundle_name, store, url, price, expires_at, active)
VALUES (sqlc.arg('wishlist_id'), sqlc.arg('bundle_name'), sqlc.arg('store'), sqlc.narg('url'), sqlc.narg('price'), sqlc.narg('expires_at'), 1)
ON CONFLICT(wishlist_id, bundle_name) DO UPDATE SET
    store      = excluded.store,
    url        = excluded.url,
    price      = excluded.price,
    expires_at = excluded.expires_at,
    active     = 1;

-- name: ExpireOldBundles :exec
UPDATE wishlist_bundles SET active = 0
WHERE expires_at IS NOT NULL AND expires_at < CURRENT_TIMESTAMP;

-- name: UpsertReseller :exec
INSERT INTO wishlist_resellers (wishlist_id, reseller_name, url, price, currency, last_checked)
VALUES (sqlc.arg('wishlist_id'), sqlc.arg('reseller_name'), sqlc.narg('url'), sqlc.arg('price'), sqlc.arg('currency'), CURRENT_TIMESTAMP)
ON CONFLICT(wishlist_id, reseller_name) DO UPDATE SET
    url          = excluded.url,
    price        = excluded.price,
    currency     = excluded.currency,
    last_checked = CURRENT_TIMESTAMP;

-- name: ListWishlistNeedingPriceSync :many
SELECT id, title, itad_id FROM wishlist_entries
WHERE itad_id IS NOT NULL
  AND (last_price_sync IS NULL OR last_price_sync < datetime('now', '-6 hours'))
ORDER BY last_price_sync ASC NULLS FIRST;
