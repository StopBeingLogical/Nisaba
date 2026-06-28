-- name: ListDevices :many
SELECT * FROM devices ORDER BY label ASC;

-- name: GetDevice :one
SELECT * FROM devices WHERE id = sqlc.arg('id');

-- name: GetDeviceByName :one
SELECT * FROM devices WHERE name = sqlc.arg('name');

-- name: UpsertDevice :one
INSERT INTO devices (id, name, label, platform)
VALUES (sqlc.arg('id'), sqlc.arg('name'), sqlc.arg('label'), sqlc.arg('platform'))
ON CONFLICT(id) DO UPDATE SET
    name     = excluded.name,
    label    = excluded.label,
    platform = excluded.platform,
    last_seen = CURRENT_TIMESTAMP
RETURNING *;

-- name: ListStorageVolumes :many
SELECT * FROM storage_volumes WHERE device_id = sqlc.arg('device_id') ORDER BY label ASC;

-- name: GetStorageVolume :one
SELECT * FROM storage_volumes WHERE id = sqlc.arg('id');

-- name: UpsertStorageVolume :one
INSERT INTO storage_volumes (id, device_id, label, path, total_bytes, free_bytes)
VALUES (sqlc.arg('id'), sqlc.arg('device_id'), sqlc.arg('label'), sqlc.arg('path'), sqlc.narg('total_bytes'), sqlc.narg('free_bytes'))
ON CONFLICT(id) DO UPDATE SET
    label       = excluded.label,
    path        = excluded.path,
    total_bytes = excluded.total_bytes,
    free_bytes  = excluded.free_bytes
RETURNING *;

-- name: GetConfig :one
SELECT value FROM app_config WHERE key = sqlc.arg('key');

-- name: SetConfig :exec
INSERT INTO app_config (key, value) VALUES (sqlc.arg('key'), sqlc.arg('value'))
ON CONFLICT(key) DO UPDATE SET value = excluded.value;

-- name: ListConfig :many
SELECT key, value FROM app_config ORDER BY key ASC;

-- name: CountNeedsReview :one
SELECT COUNT(*) FROM games WHERE enrichment_status = 'needs_review' AND parent_id IS NULL;

-- name: ListNeedsReview :many
SELECT
    g.id, g.title, g.artwork,
    (SELECT GROUP_CONCAT(store || ':' || COALESCE(store_id, '')) FROM game_stores WHERE game_id = g.id AND owned = 1) AS store_info
FROM games g
WHERE g.enrichment_status = 'needs_review'
  AND g.parent_id IS NULL
ORDER BY g.date_added ASC;
