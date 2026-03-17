-- name: ListGames :many
SELECT
    g.id,
    g.title,
    g.sort_title,
    g.enrichment_status,
    g.is_complete_edition,
    g.parent_id,
    g.artwork,
    g.steam_deck_verified,
    g.proton_rating,
    g.is_installed,
    g.play_status,
    g.rating,
    g.is_favorite,
    g.is_hidden,
    g.play_time_minutes,
    (SELECT GROUP_CONCAT(genre) FROM game_genres WHERE game_id = g.id) AS genres,
    (SELECT GROUP_CONCAT(tag) FROM game_tags WHERE game_id = g.id) AS tags,
    (SELECT GROUP_CONCAT(store) FROM game_stores WHERE game_id = g.id AND owned = 1) AS owned_stores
FROM games g
WHERE g.is_hidden = 0
  AND g.parent_id IS NULL
  AND (sqlc.narg('play_status') IS NULL OR g.play_status = sqlc.narg('play_status'))
  AND (sqlc.narg('steam_deck') IS NULL OR g.steam_deck_verified = sqlc.narg('steam_deck'))
  AND (sqlc.narg('search') IS NULL OR g.title LIKE '%' || sqlc.narg('search') || '%')
ORDER BY g.sort_title ASC;

-- name: ListGamesByStore :many
SELECT
    g.id,
    g.title,
    g.sort_title,
    g.enrichment_status,
    g.is_complete_edition,
    g.parent_id,
    g.artwork,
    g.steam_deck_verified,
    g.proton_rating,
    g.is_installed,
    g.play_status,
    g.rating,
    g.is_favorite,
    g.is_hidden,
    g.play_time_minutes,
    (SELECT GROUP_CONCAT(genre) FROM game_genres WHERE game_id = g.id) AS genres,
    (SELECT GROUP_CONCAT(tag) FROM game_tags WHERE game_id = g.id) AS tags,
    (SELECT GROUP_CONCAT(store) FROM game_stores WHERE game_id = g.id AND owned = 1) AS owned_stores
FROM games g
JOIN game_stores gs ON gs.game_id = g.id AND gs.store = sqlc.arg('store') AND gs.owned = 1
WHERE g.is_hidden = 0
  AND g.parent_id IS NULL
  AND (sqlc.narg('play_status') IS NULL OR g.play_status = sqlc.narg('play_status'))
  AND (sqlc.narg('steam_deck') IS NULL OR g.steam_deck_verified = sqlc.narg('steam_deck'))
  AND (sqlc.narg('search') IS NULL OR g.title LIKE '%' || sqlc.narg('search') || '%')
ORDER BY g.sort_title ASC;

-- name: ListGamesByGenre :many
SELECT
    g.id,
    g.title,
    g.sort_title,
    g.enrichment_status,
    g.is_complete_edition,
    g.parent_id,
    g.artwork,
    g.steam_deck_verified,
    g.proton_rating,
    g.is_installed,
    g.play_status,
    g.rating,
    g.is_favorite,
    g.is_hidden,
    g.play_time_minutes,
    (SELECT GROUP_CONCAT(genre) FROM game_genres WHERE game_id = g.id) AS genres,
    (SELECT GROUP_CONCAT(tag) FROM game_tags WHERE game_id = g.id) AS tags,
    (SELECT GROUP_CONCAT(store) FROM game_stores WHERE game_id = g.id AND owned = 1) AS owned_stores
FROM games g
JOIN game_genres gg ON gg.game_id = g.id AND gg.genre = sqlc.arg('genre')
WHERE g.is_hidden = 0
  AND g.parent_id IS NULL
ORDER BY g.sort_title ASC;

-- name: GetGame :one
SELECT
    g.*,
    (SELECT GROUP_CONCAT(genre) FROM game_genres WHERE game_id = g.id) AS genres,
    (SELECT GROUP_CONCAT(tag) FROM game_tags WHERE game_id = g.id) AS tags
FROM games g
WHERE g.id = sqlc.arg('id');

-- name: GetGameStores :many
SELECT * FROM game_stores WHERE game_id = sqlc.arg('game_id');

-- name: GetGameInstallSources :many
SELECT gi.*, sv.label AS volume_label, sv.path AS volume_path
FROM game_install_sources gi
LEFT JOIN storage_volumes sv ON sv.id = gi.volume_id
WHERE gi.game_id = sqlc.arg('game_id');

-- name: GetGameContents :many
SELECT * FROM game_contents WHERE game_id = sqlc.arg('game_id') ORDER BY sort_order, title;

-- name: GetGameDLCs :many
SELECT
    g.id, g.title, g.artwork, g.is_installed
FROM games g
WHERE g.parent_id = sqlc.arg('parent_id')
ORDER BY g.sort_title;

-- name: CountGames :one
SELECT COUNT(*) FROM games WHERE is_hidden = 0 AND parent_id IS NULL;

-- name: CountGamesByStatus :many
SELECT play_status, COUNT(*) AS count
FROM games
WHERE is_hidden = 0 AND parent_id IS NULL
GROUP BY play_status;

-- name: TopGenres :many
SELECT genre, COUNT(*) AS count
FROM game_genres gg
JOIN games g ON g.id = gg.game_id
WHERE g.is_hidden = 0 AND g.parent_id IS NULL
GROUP BY genre
ORDER BY count DESC
LIMIT 20;

-- name: AllTags :many
SELECT DISTINCT tag FROM game_tags
JOIN games g ON g.id = game_tags.game_id
WHERE g.is_hidden = 0
ORDER BY tag ASC;

-- name: UpsertGame :one
INSERT INTO games (
    id, igdb_id, rawg_id, title, sort_title, developer, publisher,
    description, short_description, release_date,
    enrichment_status, last_enriched, is_complete_edition, parent_id,
    artwork, windows, mac, linux, steam_deck_verified, proton_rating
) VALUES (
    sqlc.arg('id'), sqlc.narg('igdb_id'), sqlc.narg('rawg_id'),
    sqlc.arg('title'), sqlc.arg('sort_title'),
    sqlc.narg('developer'), sqlc.narg('publisher'),
    sqlc.narg('description'), sqlc.narg('short_description'),
    sqlc.narg('release_date'), sqlc.arg('enrichment_status'),
    sqlc.narg('last_enriched'), sqlc.arg('is_complete_edition'),
    sqlc.narg('parent_id'), sqlc.narg('artwork'),
    sqlc.arg('windows'), sqlc.arg('mac'), sqlc.arg('linux'),
    sqlc.narg('steam_deck_verified'), sqlc.narg('proton_rating')
)
ON CONFLICT(id) DO UPDATE SET
    igdb_id           = excluded.igdb_id,
    rawg_id           = excluded.rawg_id,
    title             = excluded.title,
    sort_title        = excluded.sort_title,
    developer         = excluded.developer,
    publisher         = excluded.publisher,
    description       = excluded.description,
    short_description = excluded.short_description,
    release_date      = excluded.release_date,
    enrichment_status = excluded.enrichment_status,
    last_enriched     = excluded.last_enriched,
    is_complete_edition = excluded.is_complete_edition,
    parent_id         = excluded.parent_id,
    artwork           = excluded.artwork,
    windows           = excluded.windows,
    mac               = excluded.mac,
    linux             = excluded.linux,
    steam_deck_verified = excluded.steam_deck_verified,
    proton_rating     = excluded.proton_rating
RETURNING *;

-- name: UpdateUserData :exec
UPDATE games SET
    play_status       = sqlc.narg('play_status'),
    rating            = sqlc.narg('rating'),
    short_review      = sqlc.narg('short_review'),
    notes             = sqlc.narg('notes'),
    is_favorite       = sqlc.arg('is_favorite'),
    is_hidden         = sqlc.arg('is_hidden'),
    play_time_minutes = sqlc.narg('play_time_minutes'),
    last_played       = sqlc.narg('last_played')
WHERE id = sqlc.arg('id');

-- name: SetGameEnrichmentStatus :exec
UPDATE games SET enrichment_status = sqlc.arg('status'), last_enriched = sqlc.arg('last_enriched')
WHERE id = sqlc.arg('id');

-- name: SetGameDeckStatus :exec
UPDATE games SET steam_deck_verified = sqlc.narg('steam_deck_verified'), proton_rating = sqlc.narg('proton_rating')
WHERE id = sqlc.arg('id');

-- name: UpsertGameStore :exec
INSERT INTO game_stores (game_id, store, store_id, store_url, owned, owned_since)
VALUES (sqlc.arg('game_id'), sqlc.arg('store'), sqlc.narg('store_id'), sqlc.narg('store_url'), sqlc.arg('owned'), sqlc.narg('owned_since'))
ON CONFLICT(game_id, store) DO UPDATE SET
    store_id   = excluded.store_id,
    store_url  = excluded.store_url,
    owned      = excluded.owned,
    owned_since = excluded.owned_since;

-- name: UpsertGameInstallSource :exec
INSERT INTO game_install_sources (game_id, device_id, volume_id, install_path, install_size_bytes, runner)
VALUES (sqlc.arg('game_id'), sqlc.arg('device_id'), sqlc.narg('volume_id'), sqlc.narg('install_path'), sqlc.narg('install_size_bytes'), sqlc.narg('runner'))
ON CONFLICT(game_id, device_id) DO UPDATE SET
    volume_id          = excluded.volume_id,
    install_path       = excluded.install_path,
    install_size_bytes = excluded.install_size_bytes,
    runner             = excluded.runner,
    last_seen          = CURRENT_TIMESTAMP;

-- name: DeleteGameInstallSources :exec
DELETE FROM game_install_sources WHERE game_id = sqlc.arg('game_id') AND device_id = sqlc.arg('device_id');

-- name: UpsertGenres :exec
INSERT OR IGNORE INTO game_genres (game_id, genre) VALUES (sqlc.arg('game_id'), sqlc.arg('genre'));

-- name: DeleteGenres :exec
DELETE FROM game_genres WHERE game_id = sqlc.arg('game_id');

-- name: UpsertTag :exec
INSERT OR IGNORE INTO game_tags (game_id, tag) VALUES (sqlc.arg('game_id'), sqlc.arg('tag'));

-- name: DeleteTags :exec
DELETE FROM game_tags WHERE game_id = sqlc.arg('game_id');

-- name: UpsertContent :exec
INSERT INTO game_contents (game_id, content_type, title, store_id, is_installed, installation_type, sort_order)
VALUES (sqlc.arg('game_id'), sqlc.arg('content_type'), sqlc.arg('title'), sqlc.narg('store_id'), sqlc.arg('is_installed'), sqlc.arg('installation_type'), sqlc.arg('sort_order'))
ON CONFLICT(game_id, title) DO UPDATE SET
    content_type      = excluded.content_type,
    store_id          = excluded.store_id,
    is_installed      = excluded.is_installed,
    installation_type = excluded.installation_type,
    sort_order        = excluded.sort_order;

-- name: ListGamesNeedingEnrichment :many
SELECT id, title, igdb_id, rawg_id,
    (SELECT store_id FROM game_stores WHERE game_id = games.id AND store = 'steam' LIMIT 1) AS steam_id
FROM games
WHERE enrichment_status IN ('pending', 'needs_review')
  AND parent_id IS NULL
ORDER BY date_added ASC;

-- name: ListGamesForRehydration :many
SELECT id, title, igdb_id, rawg_id, last_enriched
FROM games
WHERE enrichment_status IN ('matched', 'scraped', 'manual')
  AND parent_id IS NULL
ORDER BY last_enriched ASC NULLS FIRST;
