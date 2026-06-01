-- name: ListCurrentPlanWorkspaces :many
SELECT *
FROM plan_workspaces
WHERE
    archived_at IS NULL
    AND qrspi_lifecycle NOT IN ('merged', 'closed')
    AND (
        CAST(sqlc.arg('project_id') AS TEXT) = ''
        OR project_id = CAST(sqlc.arg('project_id') AS TEXT)
        OR EXISTS (
            SELECT 1
            FROM plan_workspace_projects pwp
            WHERE
                pwp.plan_dir_rel = plan_workspaces.plan_dir_rel
                AND pwp.project_id = CAST(sqlc.arg('project_id') AS TEXT)
                AND pwp.archived_at IS NULL
        )
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
        OR EXISTS (
            SELECT 1
            FROM plan_workspace_projects pwp
            WHERE
                pwp.plan_dir_rel = plan_workspaces.plan_dir_rel
                AND pwp.project_id = CAST(sqlc.arg('project_id') AS TEXT)
                AND pwp.archived_at IS NULL
        )
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
    sqlc.arg('artifact_updated_at'),
    COALESCE(NULLIF(sqlc.arg('qrspi_lifecycle'), ''), 'question'),
    sqlc.narg('qrspi_lifecycle_updated_at'),
    sqlc.arg('qrspi_closed_reason')
)
ON CONFLICT (plan_dir_rel) DO UPDATE SET
project_id = excluded.project_id,
plan_dir = excluded.plan_dir,
label = excluded.label,
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

-- name: ListPlanWorkspaceProjects :many
SELECT *
FROM plan_workspace_projects
WHERE plan_dir_rel = sqlc.arg ('plan_dir_rel')
AND archived_at IS NULL
ORDER BY CASE role WHEN 'primary' THEN 0 ELSE 1 END, project_id ;

-- name: UpsertPlanWorkspaceProject :one
INSERT INTO plan_workspace_projects (
plan_dir_rel,
project_id,
role,
declared_source
)
VALUES (
sqlc.arg ('plan_dir_rel'),
sqlc.arg ('project_id'),
sqlc.arg ('role'),
sqlc.arg ('declared_source')
)
ON CONFLICT (plan_dir_rel, project_id) DO UPDATE SET
role = excluded.role,
declared_source = excluded.declared_source,
last_discovered_at = CURRENT_TIMESTAMP,
archived_at = NULL
RETURNING * ;

-- name: ArchiveMissingPlanWorkspaceProjects :execrows
UPDATE plan_workspace_projects
SET archived_at = CURRENT_TIMESTAMP
WHERE plan_dir_rel = sqlc.arg ('plan_dir_rel')
AND archived_at IS NULL
AND project_id NOT IN (sqlc.slice ('project_ids')) ;

-- name: ListPlanWorkspaceImplBindings :many
SELECT *
FROM plan_workspace_impl_bindings
WHERE plan_dir_rel = sqlc.arg ('plan_dir_rel')
AND archived_at IS NULL
ORDER BY project_id ;

-- name: UpsertPlanWorkspaceImplBinding :one
INSERT INTO plan_workspace_impl_bindings (
plan_dir_rel,
project_id,
workspace_slug,
checkout_path,
url,
status,
binding_source,
impl_project_id,
impl_workspace_slug
)
VALUES (
sqlc.arg ('plan_dir_rel'),
sqlc.arg ('project_id'),
sqlc.narg ('workspace_slug'),
sqlc.narg ('checkout_path'),
sqlc.narg ('url'),
sqlc.arg ('status'),
sqlc.arg ('binding_source'),
sqlc.narg ('impl_project_id'),
sqlc.narg ('impl_workspace_slug')
)
ON CONFLICT (plan_dir_rel, project_id) DO UPDATE SET
workspace_slug = excluded.workspace_slug,
checkout_path = excluded.checkout_path,
url = excluded.url,
status = excluded.status,
binding_source = excluded.binding_source,
impl_project_id = excluded.impl_project_id,
impl_workspace_slug = excluded.impl_workspace_slug,
last_discovered_at = CURRENT_TIMESTAMP,
archived_at = NULL
RETURNING * ;
