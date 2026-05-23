-- name: ListImplWorkspaces :many
SELECT *
FROM impl_workspaces
ORDER BY
    status = 'active' DESC,
    updated_at DESC,
    lower(display_name),
    workspace_slug;

-- name: ListActiveImplWorkspaces :many
SELECT *
FROM impl_workspaces
WHERE status = 'active'
ORDER BY lower(display_name), workspace_slug;

-- name: GetImplWorkspace :one
SELECT *
FROM impl_workspaces
WHERE workspace_slug = sqlc.arg('workspace_slug');

-- name: UpsertDiscoveredImplWorkspace :one
INSERT INTO impl_workspaces (
    workspace_slug,
    checkout_path,
    display_name,
    host,
    url,
    plan_dir_rel,
    plan_dir,
    status,
    merged_at,
    merge_evidence,
    branch,
    commit_hash,
    trunk_branch,
    top_branch,
    bottom_branch,
    bottom_parent_branch,
    base_branch,
    ahead_count,
    behind_count,
    git_detail
)
VALUES (
    sqlc.arg('workspace_slug'),
    sqlc.arg('checkout_path'),
    sqlc.arg('display_name'),
    sqlc.arg('host'),
    sqlc.arg('url'),
    sqlc.narg('plan_dir_rel'),
    sqlc.narg('plan_dir'),
    sqlc.arg('status'),
    (
        CASE
            WHEN sqlc.arg('status') = 'merged' THEN current_timestamp ELSE NULL
        END
    ),
    sqlc.narg('merge_evidence'),
    sqlc.narg('branch'),
    sqlc.narg('commit_hash'),
    sqlc.narg('trunk_branch'),
    sqlc.narg('top_branch'),
    sqlc.narg('bottom_branch'),
    sqlc.narg('bottom_parent_branch'),
    sqlc.narg('base_branch'),
    sqlc.arg('ahead_count'),
    sqlc.arg('behind_count'),
    sqlc.narg('git_detail')
)
ON CONFLICT (workspace_slug) DO UPDATE SET
checkout_path = excluded.checkout_path,
display_name = excluded.display_name,
host = excluded.host,
url = excluded.url,
plan_dir_rel = excluded.plan_dir_rel,
plan_dir = excluded.plan_dir,
status = excluded.status,
merged_at = (CASE
WHEN excluded.status = 'merged'
AND (
impl_workspaces.status IS NOT excluded.status
OR COALESCE (impl_workspaces.merge_evidence,
'') IS NOT COALESCE (excluded.merge_evidence,
'')
) THEN CURRENT_TIMESTAMP
WHEN excluded.status = 'merged' THEN impl_workspaces.merged_at
ELSE NULL
END),
merge_evidence = (CASE
WHEN excluded.status = 'merged' THEN excluded.merge_evidence
ELSE NULL
END),
branch = excluded.branch,
commit_hash = excluded.commit_hash,
trunk_branch = excluded.trunk_branch,
top_branch = excluded.top_branch,
bottom_branch = excluded.bottom_branch,
bottom_parent_branch = excluded.bottom_parent_branch,
base_branch = excluded.base_branch,
ahead_count = excluded.ahead_count,
behind_count = excluded.behind_count,
git_detail = excluded.git_detail,
last_discovered_at = CURRENT_TIMESTAMP,
updated_at = CURRENT_TIMESTAMP
RETURNING * ;

-- name: MarkImplWorkspaceCleanedUp :execrows
UPDATE impl_workspaces
SET status = 'cleaned_up',
cleaned_up_at = CURRENT_TIMESTAMP,
updated_at = CURRENT_TIMESTAMP
WHERE workspace_slug = sqlc.arg ('workspace_slug')
AND status = 'active' ;

-- name: MarkMissingImplWorkspacesCleanedUp :execrows
UPDATE impl_workspaces
SET status = 'cleaned_up',
cleaned_up_at = CURRENT_TIMESTAMP,
updated_at = CURRENT_TIMESTAMP
WHERE status = 'active'
AND workspace_slug NOT IN (sqlc.slice ('workspace_slugs')) ;

-- name: MarkAllActiveImplWorkspacesCleanedUp :execrows
UPDATE impl_workspaces
SET status = 'cleaned_up',
cleaned_up_at = CURRENT_TIMESTAMP,
updated_at = CURRENT_TIMESTAMP
WHERE status = 'active' ;

-- name: MarkImplWorkspaceMerged :execrows
UPDATE impl_workspaces
SET status = 'merged',
merged_at = CURRENT_TIMESTAMP,
merge_evidence = sqlc.arg ('merge_evidence'),
updated_at = CURRENT_TIMESTAMP
WHERE workspace_slug = sqlc.arg ('workspace_slug') ;

-- name: RecordImplWorkspaceEnvRepair :exec
UPDATE impl_workspaces
SET env_last_repaired_at = CURRENT_TIMESTAMP,
env_last_error = NULL,
updated_at = CURRENT_TIMESTAMP
WHERE workspace_slug = sqlc.arg ('workspace_slug') ;

-- name: RecordImplWorkspaceEnvError :exec
UPDATE impl_workspaces
SET env_last_error = sqlc.arg ('env_last_error'),
updated_at = CURRENT_TIMESTAMP
WHERE workspace_slug = sqlc.arg ('workspace_slug') ;
