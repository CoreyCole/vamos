-- name: ListActivePlanWorkspaces :many
SELECT *
FROM plan_workspaces
WHERE archived_at IS NULL
ORDER BY artifact_updated_at DESC, lower(label), plan_dir_rel;

-- name: GetPlanWorkspace :one
SELECT *
FROM plan_workspaces
WHERE plan_dir_rel = sqlc.arg('plan_dir_rel');

-- name: UpsertDiscoveredPlanWorkspace :one
INSERT INTO plan_workspaces (
    plan_dir_rel,
    plan_dir,
    label,
    workspace_slug,
    impl_workspace_path,
    impl_workspace_url,
    impl_workspace_discovered_at,
    artifact_updated_at
)
VALUES (
    sqlc.arg('plan_dir_rel'),
    sqlc.arg('plan_dir'),
    sqlc.arg('label'),
    sqlc.arg('workspace_slug'),
    sqlc.narg('impl_workspace_path'),
    sqlc.narg('impl_workspace_url'),
    sqlc.narg('impl_workspace_discovered_at'),
    sqlc.arg('artifact_updated_at')
)
ON CONFLICT (plan_dir_rel) DO UPDATE SET
plan_dir = excluded.plan_dir,
label = excluded.label,
workspace_slug = excluded.workspace_slug,
impl_workspace_path = excluded.impl_workspace_path,
impl_workspace_url = excluded.impl_workspace_url,
impl_workspace_discovered_at = excluded.impl_workspace_discovered_at,
artifact_updated_at = excluded.artifact_updated_at,
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
