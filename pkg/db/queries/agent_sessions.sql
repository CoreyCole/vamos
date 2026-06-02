-- name: CreateAgentSession :one
INSERT INTO agent_sessions (
    id,
    identity_kind,
    artifact_path,
    plan_dir,
    parent_plan_dir,
    source_review_dir,
    agent,
    external_session_id,
    parent_session_id,
    cwd,
    workflow_id,
    workflow_node_id,
    continued_from_session_id,
    forked_from_session_id,
    file_size,
    file_mtime,
    file_hash,
    last_indexed_offset,
    projection_state,
    projected_thread_id,
    indexed_by_user_email,
    attached_workspace_id,
    imported_head_entry_id,
    last_error,
    metadata_json
)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('identity_kind'),
    sqlc.narg('artifact_path'),
    sqlc.narg('plan_dir'),
    NULL,
    NULL,
    'pi',
    sqlc.narg('external_session_id'),
    sqlc.narg('parent_session_id'),
    sqlc.narg('cwd'),
    NULL,
    NULL,
    NULL,
    NULL,
    0,
    NULL,
    NULL,
    0,
    sqlc.arg('projection_state'),
    sqlc.narg('projected_thread_id'),
    sqlc.narg('indexed_by_user_email'),
    sqlc.narg('attached_workspace_id'),
    sqlc.narg('imported_head_entry_id'),
    sqlc.narg('last_error'),
    sqlc.narg('metadata_json')
)
RETURNING * ;

-- name: GetAgentSession :one
SELECT *
FROM agent_sessions
WHERE id = sqlc.arg ('id') ;

-- name: GetAgentSessionByPath :one
SELECT *
FROM agent_sessions
WHERE artifact_path = sqlc.arg ('artifact_path') ;

-- name: ListAgentSessionsByWorkspace :many
SELECT *
FROM agent_sessions
WHERE attached_workspace_id = sqlc.arg ('attached_workspace_id')
ORDER BY updated_at DESC ;

-- name: ListPlanOwnedSessionArtifactsByPlanDir :many
SELECT *
FROM agent_sessions
WHERE identity_kind = 'plan_owned'
AND plan_dir = sqlc.arg ('plan_dir')
ORDER BY updated_at DESC ;

-- name: ListPlanOwnedSessionArtifactsByPlanDirPrefix :many
SELECT *
FROM agent_sessions
WHERE identity_kind = 'plan_owned'
AND (plan_dir = sqlc.arg ('plan_dir') OR plan_dir LIKE sqlc.arg ('plan_dir_prefix'))
ORDER BY updated_at DESC ;

-- name: ListPrivateSessionArtifactsByPlanDir :many
SELECT *
FROM agent_sessions
WHERE identity_kind != 'plan_owned'
AND indexed_by_user_email = sqlc.arg ('indexed_by_user_email')
AND plan_dir = sqlc.arg ('plan_dir')
AND (sqlc.arg ('attached_workspace_id') = '' OR attached_workspace_id = sqlc.arg ('attached_workspace_id'))
ORDER BY updated_at DESC ;

-- name: ListPrivateSessionArtifactsByPlanDirPrefix :many
SELECT *
FROM agent_sessions
WHERE identity_kind != 'plan_owned'
AND indexed_by_user_email = sqlc.arg ('indexed_by_user_email')
AND (sqlc.arg ('attached_workspace_id') = '' OR attached_workspace_id = sqlc.arg ('attached_workspace_id'))
AND (plan_dir = sqlc.arg ('plan_dir') OR plan_dir LIKE sqlc.arg ('plan_dir_prefix'))
ORDER BY updated_at DESC ;

-- name: ListPrivateSessionArtifactsForUser :many
SELECT *
FROM agent_sessions
WHERE identity_kind != 'plan_owned'
AND indexed_by_user_email = sqlc.arg ('indexed_by_user_email')
AND plan_dir IS NOT NULL
ORDER BY updated_at DESC ;

-- name: UpsertAgentSessionIndex :one
INSERT INTO agent_sessions (
id,
identity_kind,
artifact_path,
plan_dir,
parent_plan_dir,
source_review_dir,
agent,
external_session_id,
parent_session_id,
cwd,
workflow_id,
workflow_node_id,
continued_from_session_id,
forked_from_session_id,
file_size,
file_mtime,
file_hash,
last_indexed_offset,
projection_state,
projected_thread_id,
indexed_by_user_email,
attached_workspace_id,
imported_head_entry_id,
last_error,
metadata_json
)
VALUES (
sqlc.arg ('id'),
sqlc.arg ('identity_kind'),
sqlc.arg ('artifact_path'),
sqlc.narg ('plan_dir'),
sqlc.narg ('parent_plan_dir'),
sqlc.narg ('source_review_dir'),
COALESCE (NULLIF (sqlc.arg ('agent'), ''), 'pi'),
sqlc.narg ('external_session_id'),
sqlc.narg ('parent_session_id'),
sqlc.narg ('cwd'),
sqlc.narg ('workflow_id'),
sqlc.narg ('workflow_node_id'),
sqlc.narg ('continued_from_session_id'),
sqlc.narg ('forked_from_session_id'),
sqlc.arg ('file_size'),
sqlc.narg ('file_mtime'),
sqlc.narg ('file_hash'),
sqlc.arg ('last_indexed_offset'),
sqlc.arg ('projection_state'),
sqlc.narg ('projected_thread_id'),
sqlc.narg ('indexed_by_user_email'),
sqlc.narg ('attached_workspace_id'),
NULL,
sqlc.narg ('last_error'),
sqlc.narg ('metadata_json')
)
ON CONFLICT (artifact_path) WHERE artifact_path IS NOT NULL DO UPDATE SET
identity_kind = excluded.identity_kind,
plan_dir = excluded.plan_dir,
parent_plan_dir = excluded.parent_plan_dir,
source_review_dir = excluded.source_review_dir,
agent = excluded.agent,
external_session_id = excluded.external_session_id,
parent_session_id = excluded.parent_session_id,
cwd = excluded.cwd,
workflow_id = excluded.workflow_id,
workflow_node_id = excluded.workflow_node_id,
continued_from_session_id = excluded.continued_from_session_id,
forked_from_session_id = excluded.forked_from_session_id,
file_size = excluded.file_size,
file_mtime = excluded.file_mtime,
file_hash = excluded.file_hash,
last_indexed_offset = excluded.last_indexed_offset,
projection_state = CASE
WHEN agent_sessions.file_size = excluded.file_size
AND COALESCE (agent_sessions.file_mtime,
'') = COALESCE (excluded.file_mtime,
'')
AND COALESCE (agent_sessions.file_hash, '') = COALESCE (excluded.file_hash, '')
AND agent_sessions.projection_state IN ('importing', 'hydrated', 'diverged')
THEN agent_sessions.projection_state
ELSE excluded.projection_state
END,
projected_thread_id = CASE
WHEN agent_sessions.file_size = excluded.file_size
AND COALESCE (agent_sessions.file_mtime,
'') = COALESCE (excluded.file_mtime,
'')
AND COALESCE (agent_sessions.file_hash, '') = COALESCE (excluded.file_hash, '')
THEN agent_sessions.projected_thread_id
ELSE excluded.projected_thread_id
END,
indexed_by_user_email = COALESCE (excluded.indexed_by_user_email,
agent_sessions.indexed_by_user_email),
attached_workspace_id = COALESCE (excluded.attached_workspace_id,
agent_sessions.attached_workspace_id),
last_error = excluded.last_error,
metadata_json = excluded.metadata_json,
updated_at = CURRENT_TIMESTAMP
RETURNING * ;

-- name: BackfillAgentSessionsWorkspaceForThread :exec
UPDATE agent_sessions
SET attached_workspace_id = sqlc.arg ('attached_workspace_id')
WHERE projected_thread_id = sqlc.arg ('projected_thread_id')
AND (attached_workspace_id IS NULL OR attached_workspace_id = '') ;

-- name: UpdateAgentSessionImportingState :exec
UPDATE agent_sessions
SET attached_workspace_id = sqlc.narg ('attached_workspace_id'),
projected_thread_id = sqlc.narg ('projected_thread_id'),
projection_state = 'importing',
plan_dir = sqlc.narg ('plan_dir'),
last_error = NULL,
metadata_json = sqlc.narg ('metadata_json'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: UpdateAgentSessionImportFinalState :exec
UPDATE agent_sessions
SET attached_workspace_id = sqlc.narg ('attached_workspace_id'),
projected_thread_id = sqlc.narg ('projected_thread_id'),
projection_state = sqlc.arg ('projection_state'),
plan_dir = sqlc.narg ('plan_dir'),
imported_head_entry_id = sqlc.narg ('imported_head_entry_id'),
last_imported_at = CURRENT_TIMESTAMP,
last_error = NULL,
metadata_json = sqlc.narg ('metadata_json'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: MarkAgentSessionHydratedByPath :exec
UPDATE agent_sessions
SET projection_state = CASE WHEN projection_state = 'needs_hydration' THEN 'hydrated' ELSE projection_state END,
last_error = NULL,
updated_at = CURRENT_TIMESTAMP
WHERE artifact_path = sqlc.arg ('artifact_path') ;

-- name: UpdateAgentSessionImportFailedState :exec
UPDATE agent_sessions
SET projection_state = 'failed',
last_error = sqlc.narg ('last_error'),
metadata_json = sqlc.narg ('metadata_json'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: UpdateAgentSessionInferenceState :exec
UPDATE agent_sessions
SET attached_workspace_id = sqlc.narg ('attached_workspace_id'),
projected_thread_id = sqlc.narg ('projected_thread_id'),
projection_state = sqlc.arg ('projection_state'),
plan_dir = sqlc.narg ('plan_dir'),
imported_head_entry_id = NULL,
last_error = sqlc.narg ('last_error'),
metadata_json = sqlc.narg ('metadata_json'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;
