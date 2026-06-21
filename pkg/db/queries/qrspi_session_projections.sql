-- name: UpsertQRSPIProjectionPending :one
INSERT INTO qrspi_session_projections (
    id,
    source_event_id,
    session_id,
    session_artifact_path,
    plan_dir,
    workflow_node_id,
    stage,
    status,
    outcome,
    artifact,
    result_json,
    event_time
) VALUES (
    sqlc.arg('id'),
    sqlc.arg('source_event_id'),
    sqlc.narg('session_id'),
    sqlc.narg('session_artifact_path'),
    sqlc.arg('plan_dir'),
    sqlc.narg('workflow_node_id'),
    sqlc.narg('stage'),
    sqlc.narg('status'),
    sqlc.narg('outcome'),
    sqlc.narg('artifact'),
    sqlc.arg('result_json'),
    sqlc.arg('event_time')
)
ON CONFLICT (source_event_id) DO UPDATE SET
session_id = excluded.session_id,
session_artifact_path = excluded.session_artifact_path,
plan_dir = excluded.plan_dir,
workflow_node_id = excluded.workflow_node_id,
stage = excluded.stage,
status = excluded.status,
outcome = excluded.outcome,
artifact = excluded.artifact,
result_json = excluded.result_json,
event_time = excluded.event_time,
updated_at = CURRENT_TIMESTAMP
RETURNING * ;

-- name: ListPendingQRSPIProjections :many
SELECT *
FROM qrspi_session_projections
WHERE projection_state = 'pending'
ORDER BY event_time ASC
LIMIT sqlc.arg ('limit') ;

-- name: MarkQRSPIProjectionApplied :exec
UPDATE qrspi_session_projections
SET projection_state = 'applied',
applied_at = CURRENT_TIMESTAMP,
last_error = NULL,
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id')
AND projection_state = 'pending' ;

-- name: MarkQRSPIProjectionFailed :exec
UPDATE qrspi_session_projections
SET projection_state = 'failed',
last_error = sqlc.narg ('last_error'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id')
AND projection_state = 'pending' ;

-- name: MarkQRSPIProjectionSkipped :exec
UPDATE qrspi_session_projections
SET projection_state = 'skipped',
last_error = sqlc.narg ('last_error'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id')
AND projection_state = 'pending' ;
