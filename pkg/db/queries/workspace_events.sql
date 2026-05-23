-- name: CreateWorkspaceEvent :one
INSERT INTO workspace_events (
    workspace_id,
    event_type,
    actor_email,
    actor_type,
    thread_id,
    session_id,
    run_id,
    doc_path,
    comment_id,
    payload_json,
    event_key
)
VALUES (
    sqlc.arg('workspace_id'),
    sqlc.arg('event_type'),
    sqlc.narg('actor_email'),
    sqlc.arg('actor_type'),
    sqlc.narg('thread_id'),
    sqlc.narg('session_id'),
    sqlc.narg('run_id'),
    sqlc.narg('doc_path'),
    sqlc.narg('comment_id'),
    sqlc.narg('payload_json'),
    sqlc.narg('event_key')
)
RETURNING * ;

-- name: GetWorkspaceEventByKey :one
SELECT *
FROM workspace_events
WHERE workspace_id = sqlc.arg ('workspace_id')
AND event_key = sqlc.arg ('event_key') ;

-- name: ListWorkspaceEventsAfter :many
SELECT *
FROM workspace_events
WHERE workspace_id = sqlc.arg ('workspace_id')
AND id > sqlc.arg ('after_id')
ORDER BY id ASC
LIMIT sqlc.arg ('limit') ;

-- name: ListWorkspaceEvents :many
SELECT *
FROM workspace_events
WHERE workspace_id = sqlc.arg ('workspace_id')
ORDER BY id ASC
LIMIT sqlc.arg ('limit') ;

-- name: ListRecentWorkspaceLogEvents :many
SELECT *
FROM workspace_events
WHERE workspace_id = sqlc.arg ('workspace_id')
AND event_type IN (
'artifact_created',
'artifact_updated',
'artifact_deleted',
'session_imported',
'session_import_diverged',
'session_sync_failed',
'run_started',
'run_completed',
'run_failed'
)
ORDER BY id DESC
LIMIT sqlc.arg ('limit') ;
