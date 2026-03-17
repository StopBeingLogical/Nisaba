-- name: InsertSyncLog :one
INSERT INTO sync_log (type, status, started_at)
VALUES (sqlc.arg('type'), 'running', CURRENT_TIMESTAMP)
RETURNING *;

-- name: UpdateSyncLog :exec
UPDATE sync_log SET
    status       = sqlc.arg('status'),
    finished_at  = CURRENT_TIMESTAMP,
    games_added  = sqlc.narg('games_added'),
    games_updated = sqlc.narg('games_updated'),
    error_message = sqlc.narg('error_message')
WHERE id = sqlc.arg('id');

-- name: GetLatestSyncByType :one
SELECT * FROM sync_log WHERE type = sqlc.arg('type') ORDER BY started_at DESC LIMIT 1;

-- name: ListRecentSyncs :many
SELECT * FROM sync_log ORDER BY started_at DESC LIMIT 20;

-- name: GetRunningSyncs :many
SELECT * FROM sync_log WHERE status = 'running';

-- name: EnqueueEnrichment :exec
INSERT INTO enrichment_queue (entity_type, entity_id, status)
VALUES (sqlc.arg('entity_type'), sqlc.arg('entity_id'), 'pending')
ON CONFLICT(entity_type, entity_id) DO UPDATE SET
    status   = 'pending',
    attempts = 0,
    last_error = NULL;

-- name: GetNextEnrichmentItem :one
SELECT * FROM enrichment_queue
WHERE status = 'pending'
ORDER BY created_at ASC
LIMIT 1;

-- name: MarkEnrichmentRunning :exec
UPDATE enrichment_queue SET status = 'running', attempts = attempts + 1
WHERE entity_type = sqlc.arg('entity_type') AND entity_id = sqlc.arg('entity_id');

-- name: MarkEnrichmentDone :exec
UPDATE enrichment_queue SET status = 'done'
WHERE entity_type = sqlc.arg('entity_type') AND entity_id = sqlc.arg('entity_id');

-- name: MarkEnrichmentFailed :exec
UPDATE enrichment_queue SET status = 'failed', last_error = sqlc.arg('last_error')
WHERE entity_type = sqlc.arg('entity_type') AND entity_id = sqlc.arg('entity_id');

-- name: CountEnrichmentQueue :one
SELECT
    COUNT(*) FILTER (WHERE status = 'pending')  AS pending,
    COUNT(*) FILTER (WHERE status = 'running')  AS running,
    COUNT(*) FILTER (WHERE status = 'failed')   AS failed
FROM enrichment_queue;
