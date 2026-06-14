-- name: UpsertWorkspaceSyncDiagnostic :exec
INSERT INTO workspace_sync_diagnostics (
    project_id,
    sync_kind,
    started_at,
    finished_at,
    status,
    error,
    scanned,
    discovered,
    upserted,
    repaired_env,
    merged,
    cleaned_up,
    changed,
    warnings_json,
    updated_at
)
VALUES (
    sqlc.arg('project_id'),
    sqlc.arg('sync_kind'),
    sqlc.arg('started_at'),
    sqlc.narg('finished_at'),
    sqlc.arg('status'),
    sqlc.arg('error'),
    sqlc.arg('scanned'),
    sqlc.arg('discovered'),
    sqlc.arg('upserted'),
    sqlc.arg('repaired_env'),
    sqlc.arg('merged'),
    sqlc.arg('cleaned_up'),
    sqlc.arg('changed'),
    sqlc.arg('warnings_json'),
    CURRENT_TIMESTAMP
)
ON CONFLICT (project_id, sync_kind) DO UPDATE SET
started_at = excluded.started_at,
finished_at = excluded.finished_at,
status = excluded.status,
error = excluded.error,
scanned = excluded.scanned,
discovered = excluded.discovered,
upserted = excluded.upserted,
repaired_env = excluded.repaired_env,
merged = excluded.merged,
cleaned_up = excluded.cleaned_up,
changed = excluded.changed,
warnings_json = excluded.warnings_json,
updated_at = CURRENT_TIMESTAMP ;

-- name: GetWorkspaceSyncDiagnostic :one
SELECT *
FROM workspace_sync_diagnostics
WHERE project_id = sqlc.arg ('project_id')
AND sync_kind = sqlc.arg ('sync_kind') ;

-- name: ListWorkspaceSyncDiagnostics :many
SELECT *
FROM workspace_sync_diagnostics
WHERE CAST (sqlc.arg ('project_id') AS TEXT) = ''
OR project_id = CAST (sqlc.arg ('project_id') AS TEXT)
ORDER BY updated_at DESC, project_id, sync_kind ;
