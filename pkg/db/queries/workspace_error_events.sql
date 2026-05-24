-- name: UpsertWorkspaceErrorEvent :one
INSERT INTO workspace_error_events (
    workspace_slug,
    source,
    severity,
    message,
    detail,
    dedupe_key,
    payload_json
)
VALUES (
    sqlc.arg('workspace_slug'),
    sqlc.arg('source'),
    sqlc.arg('severity'),
    sqlc.arg('message'),
    sqlc.arg('detail'),
    sqlc.arg('dedupe_key'),
    sqlc.arg('payload_json')
)
ON CONFLICT (dedupe_key) DO UPDATE SET
message = excluded.message,
detail = excluded.detail,
payload_json = excluded.payload_json,
occurrence_count = workspace_error_events.occurrence_count + 1,
last_seen_at = CURRENT_TIMESTAMP
RETURNING * ;

-- name: ListRecentWorkspaceErrorEvents :many
SELECT *
FROM workspace_error_events
ORDER BY last_seen_at DESC, id DESC
LIMIT sqlc.arg ('limit') ;

-- name: ListRecentWorkspaceErrorEventsForWorkspace :many
SELECT *
FROM workspace_error_events
WHERE workspace_slug = sqlc.arg ('workspace_slug')
ORDER BY last_seen_at DESC, id DESC
LIMIT sqlc.arg ('limit') ;
