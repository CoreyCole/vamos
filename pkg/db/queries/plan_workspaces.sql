-- name: ListCurrentPlanWorkspaces :many
SELECT *
FROM plan_workspaces
WHERE
    archived_at IS NULL
    AND qrspi_lifecycle NOT IN ('merged', 'closed')
    AND (
        CAST(sqlc.arg('project_id') AS TEXT) = ''
        OR project_id = CAST(sqlc.arg('project_id') AS TEXT)
    )
ORDER BY artifact_updated_at DESC, LOWER(label), plan_dir_rel;

-- name: ListPlanWorkspaces :many
SELECT *
FROM plan_workspaces
WHERE
    archived_at IS NULL
    AND (
        CAST(sqlc.arg('project_id') AS TEXT) = ''
        OR project_id = CAST(sqlc.arg('project_id') AS TEXT)
    )
ORDER BY artifact_updated_at DESC, LOWER(label), plan_dir_rel;

-- name: GetPlanWorkspace :one
SELECT *
FROM plan_workspaces
WHERE plan_dir_rel = sqlc.arg('plan_dir_rel');

-- name: UpsertDiscoveredPlanWorkspace :one
INSERT INTO plan_workspaces (
    plan_dir_rel,
    project_id,
    plan_dir,
    label,
    workspace_slug,
    impl_workspace_path,
    impl_workspace_url,
    impl_workspace_discovered_at,
    artifact_updated_at,
    qrspi_lifecycle,
    qrspi_lifecycle_updated_at,
    qrspi_closed_reason
)
VALUES (
    sqlc.arg('plan_dir_rel'),
    sqlc.arg('project_id'),
    sqlc.arg('plan_dir'),
    sqlc.arg('label'),
    sqlc.arg('workspace_slug'),
    sqlc.narg('impl_workspace_path'),
    sqlc.narg('impl_workspace_url'),
    sqlc.narg('impl_workspace_discovered_at'),
    sqlc.arg('artifact_updated_at'),
    COALESCE(NULLIF(sqlc.arg('qrspi_lifecycle'), ''), 'question'),
    sqlc.narg('qrspi_lifecycle_updated_at'),
    sqlc.arg('qrspi_closed_reason')
)
ON CONFLICT (plan_dir_rel) DO UPDATE SET
project_id = excluded.project_id,
plan_dir = excluded.plan_dir,
label = excluded.label,
workspace_slug = excluded.workspace_slug,
impl_workspace_path = excluded.impl_workspace_path,
impl_workspace_url = excluded.impl_workspace_url,
impl_workspace_discovered_at = excluded.impl_workspace_discovered_at,
artifact_updated_at = excluded.artifact_updated_at,
qrspi_lifecycle = excluded.qrspi_lifecycle,
qrspi_lifecycle_updated_at = excluded.qrspi_lifecycle_updated_at,
qrspi_closed_reason = excluded.qrspi_closed_reason,
last_discovered_at = CURRENT_TIMESTAMP,
archived_at = NULL
RETURNING * ;

-- name: ArchiveMissingPlanWorkspaces :execrows
UPDATE plan_workspaces
SET archived_at = CURRENT_TIMESTAMP
WHERE archived_at IS NULL
AND plan_dir_rel NOT IN (sqlc.slice ('plan_dir_rels')) ;

-- name: ArchiveAllActivePlanWorkspaces :execrows
UPDATE plan_workspaces
SET archived_at = CURRENT_TIMESTAMP
WHERE archived_at IS NULL ;
