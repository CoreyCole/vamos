-- name: CreateReleaseQueueItem :one
INSERT INTO release_queue_items (
    id,
    definition_id,
    definition_version,
    workflow_id,
    workflow_version,
    flow_id,
    source_slug,
    target_lane,
    expected_source_commit,
    expected_target_commit,
    status,
    current_node_id,
    actor_email,
    error_message,
    payload_json
)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('definition_id'),
    sqlc.arg('definition_version'),
    sqlc.arg('workflow_id'),
    sqlc.arg('workflow_version'),
    sqlc.arg('flow_id'),
    sqlc.arg('source_slug'),
    sqlc.arg('target_lane'),
    sqlc.arg('expected_source_commit'),
    sqlc.arg('expected_target_commit'),
    sqlc.arg('status'),
    sqlc.arg('current_node_id'),
    sqlc.arg('actor_email'),
    sqlc.arg('error_message'),
    sqlc.arg('payload_json')
)
RETURNING * ;

-- name: GetReleaseQueueItem :one
SELECT *
FROM release_queue_items
WHERE id = sqlc.arg ('id') ;

-- name: ListActiveReleaseQueueItems :many
SELECT *
FROM release_queue_items
WHERE status IN ('pending', 'running')
ORDER BY created_at ASC, id ASC ;

-- name: ListRecentReleaseQueueItems :many
SELECT *
FROM release_queue_items
WHERE status IN ('succeeded', 'failed', 'canceled')
ORDER BY finished_at DESC, created_at DESC, id DESC
LIMIT sqlc.arg ('limit') ;

-- name: ClaimNextPendingReleaseQueueItem :one
UPDATE release_queue_items
SET status = 'running',
started_at = COALESCE (started_at, CURRENT_TIMESTAMP),
updated_at = CURRENT_TIMESTAMP
WHERE id = (
SELECT id
FROM release_queue_items
WHERE status = 'pending'
ORDER BY created_at ASC, id ASC
LIMIT 1
)
RETURNING * ;

-- name: MarkReleaseQueueItemRunning :one
UPDATE release_queue_items
SET status = 'running',
current_node_id = sqlc.arg ('current_node_id'),
started_at = COALESCE (started_at, CURRENT_TIMESTAMP),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id')
RETURNING * ;

-- name: MarkReleaseQueueItemTerminal :one
UPDATE release_queue_items
SET status = sqlc.arg ('status'),
error_message = sqlc.arg ('error_message'),
finished_at = COALESCE (finished_at, CURRENT_TIMESTAMP),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id')
RETURNING * ;

-- name: AppendReleaseQueueEvent :one
INSERT INTO release_queue_events (
item_id,
level,
node_id,
message,
payload_json
)
VALUES (
sqlc.arg ('item_id'),
sqlc.arg ('level'),
sqlc.arg ('node_id'),
sqlc.arg ('message'),
sqlc.arg ('payload_json')
)
RETURNING * ;

-- name: ListReleaseQueueEvents :many
SELECT *
FROM release_queue_events
WHERE item_id = sqlc.arg ('item_id')
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg ('limit') ;
