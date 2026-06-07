-- name: ListImplWorkspaces :many
SELECT *
FROM impl_workspaces
WHERE
    CAST(sqlc.arg('project_id') AS TEXT) = ''
    OR project_id = CAST(sqlc.arg('project_id') AS TEXT)
ORDER BY
    status = 'active' DESC,
    updated_at DESC,
    LOWER(display_name),
    project_id,
    workspace_slug;

-- name: ListActiveImplWorkspaces :many
SELECT *
FROM impl_workspaces
WHERE status = 'active'
ORDER BY LOWER(display_name), project_id, workspace_slug;

-- name: GetImplWorkspace :one
SELECT *
FROM impl_workspaces
WHERE
    project_id = sqlc.arg('project_id')
    AND workspace_slug = sqlc.arg('workspace_slug');

-- name: ClearInvalidImplWorkspacePlanRefs :execrows
UPDATE impl_workspaces
SET
    plan_dir_rel = NULL,
    plan_dir = NULL
WHERE
    plan_dir_rel IS NOT NULL
    AND NOT EXISTS (
        SELECT 1
        FROM plan_workspaces
        WHERE plan_workspaces.plan_dir_rel = impl_workspaces.plan_dir_rel
    );

-- name: UpsertDiscoveredImplWorkspace :one
INSERT INTO impl_workspaces (
    project_id,
    workspace_slug,
    checkout_role,
    checkout_path,
    display_name,
    host,
    url,
    plan_dir_rel,
    plan_dir,
    status,
    merged_at,
    merge_evidence,
    cleanup_proof_kind,
    cleanup_proof_source_ref,
    cleanup_proof_target_commit,
    cleanup_proof_at,
    cleanup_risk_reason,
    branch,
    commit_hash,
    trunk_branch,
    top_branch,
    bottom_branch,
    bottom_parent_branch,
    base_branch,
    ahead_count,
    behind_count,
    git_detail,
    activity_hash
)
VALUES (
    sqlc.arg('project_id'),
    sqlc.arg('workspace_slug'),
    sqlc.arg('checkout_role'),
    sqlc.arg('checkout_path'),
    sqlc.arg('display_name'),
    sqlc.arg('host'),
    sqlc.arg('url'),
    sqlc.narg('plan_dir_rel'),
    sqlc.narg('plan_dir'),
    sqlc.arg('status'),
    (
        CASE
            WHEN sqlc.arg('status') = 'merged' THEN CURRENT_TIMESTAMP ELSE NULL
        END
    ),
    sqlc.narg('merge_evidence'),
    COALESCE(NULLIF(sqlc.arg('cleanup_proof_kind'), ''), 'unknown'),
    sqlc.narg('cleanup_proof_source_ref'),
    sqlc.narg('cleanup_proof_target_commit'),
    sqlc.narg('cleanup_proof_at'),
    sqlc.narg('cleanup_risk_reason'),
    sqlc.narg('branch'),
    sqlc.narg('commit_hash'),
    sqlc.narg('trunk_branch'),
    sqlc.narg('top_branch'),
    sqlc.narg('bottom_branch'),
    sqlc.narg('bottom_parent_branch'),
    sqlc.narg('base_branch'),
    sqlc.arg('ahead_count'),
    sqlc.arg('behind_count'),
    sqlc.narg('git_detail'),
    sqlc.arg('activity_hash')
)
ON CONFLICT (project_id, workspace_slug) DO UPDATE SET
checkout_role = excluded.checkout_role,
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
cleanup_proof_kind = excluded.cleanup_proof_kind,
cleanup_proof_source_ref = excluded.cleanup_proof_source_ref,
cleanup_proof_target_commit = excluded.cleanup_proof_target_commit,
cleanup_proof_at = excluded.cleanup_proof_at,
cleanup_risk_reason = excluded.cleanup_risk_reason,
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
activity_checked_at = CURRENT_TIMESTAMP,
updated_at = CASE
WHEN impl_workspaces.status IS NOT excluded.status THEN CURRENT_TIMESTAMP
WHEN COALESCE (impl_workspaces.activity_hash,
'') = '' THEN impl_workspaces.updated_at
WHEN COALESCE (impl_workspaces.activity_hash,
'') IS NOT COALESCE (excluded.activity_hash,
'') THEN CURRENT_TIMESTAMP
ELSE impl_workspaces.updated_at
END,
activity_hash = excluded.activity_hash
RETURNING * ;

-- name: ReassignImplWorkspaceCheckoutPathIdentity :execrows
UPDATE impl_workspaces
SET project_id = sqlc.arg ('project_id'),
workspace_slug = sqlc.arg ('workspace_slug')
WHERE impl_workspaces.checkout_path = sqlc.arg ('checkout_path')
AND NOT (project_id = sqlc.arg ('project_id') AND workspace_slug = sqlc.arg ('workspace_slug'))
AND NOT EXISTS (
SELECT 1
FROM impl_workspaces target
WHERE target.project_id = sqlc.arg ('project_id')
AND target.workspace_slug = sqlc.arg ('workspace_slug')
) ;

-- name: MarkImplWorkspaceCleanedUp :execrows
UPDATE impl_workspaces
SET status = 'cleaned_up',
cleaned_up_at = CURRENT_TIMESTAMP,
updated_at = CURRENT_TIMESTAMP
WHERE project_id = sqlc.arg ('project_id')
AND workspace_slug = sqlc.arg ('workspace_slug')
AND status IN ('active', 'merged') ;

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
cleanup_proof_kind = COALESCE (NULLIF (sqlc.arg ('cleanup_proof_kind'),
''),
'unknown'),
cleanup_proof_source_ref = sqlc.narg ('cleanup_proof_source_ref'),
cleanup_proof_target_commit = sqlc.narg ('cleanup_proof_target_commit'),
cleanup_proof_at = CURRENT_TIMESTAMP,
cleanup_risk_reason = NULL,
updated_at = CURRENT_TIMESTAMP
WHERE project_id = sqlc.arg ('project_id')
AND workspace_slug = sqlc.arg ('workspace_slug') ;

-- name: MarkImplWorkspaceMergeUnknown :execrows
UPDATE impl_workspaces
SET cleanup_proof_kind = 'unknown',
cleanup_proof_source_ref = sqlc.narg ('cleanup_proof_source_ref'),
cleanup_proof_target_commit = NULL,
cleanup_proof_at = NULL,
cleanup_risk_reason = sqlc.narg ('cleanup_risk_reason'),
merge_evidence = sqlc.narg ('merge_evidence')
WHERE project_id = sqlc.arg ('project_id')
AND workspace_slug = sqlc.arg ('workspace_slug') ;

-- name: RecordImplWorkspaceEnvRepair :exec
UPDATE impl_workspaces
SET env_last_repaired_at = CURRENT_TIMESTAMP,
env_last_error = NULL
WHERE project_id = sqlc.arg ('project_id')
AND workspace_slug = sqlc.arg ('workspace_slug') ;

-- name: RecordImplWorkspaceEnvError :exec
UPDATE impl_workspaces
SET env_last_error = sqlc.arg ('env_last_error')
WHERE project_id = sqlc.arg ('project_id')
AND workspace_slug = sqlc.arg ('workspace_slug') ;
