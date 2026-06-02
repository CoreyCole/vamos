-- name: CreateAgentSession :one
INSERT INTO agent_sessions (
    id,
    workspace_id,
    thread_id,
    user_email,
    source,
    session_path,
    session_id,
    parent_session_id,
    cwd,
    agent,
    parent_plan_dir,
    source_review_dir,
    workflow_id,
    workflow_node_id,
    continued_from_session_id,
    forked_from_session_id,
    file_size,
    file_mtime,
    file_hash,
    last_indexed_offset,
    needs_hydration,
    status,
    inferred_workspace_id,
    inferred_plan_dir,
    imported_head_entry_id,
    last_error,
    metadata_json
)
VALUES (
    sqlc.arg('id'),
    sqlc.narg('workspace_id'),
    sqlc.narg('thread_id'),
    sqlc.narg('user_email'),
    sqlc.arg('source'),
    sqlc.narg('session_path'),
    sqlc.narg('session_id'),
    sqlc.narg('parent_session_id'),
    sqlc.narg('cwd'),
    'pi',
    NULL,
    NULL,
    NULL,
    NULL,
    NULL,
    NULL,
    0,
    NULL,
    NULL,
    0,
    1,
    sqlc.arg('status'),
    sqlc.narg('inferred_workspace_id'),
    sqlc.narg('inferred_plan_dir'),
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
WHERE session_path = sqlc.arg ('session_path') ;

-- name: ListAgentSessionsByWorkspace :many
SELECT *
FROM agent_sessions
WHERE workspace_id = sqlc.arg ('workspace_id')
ORDER BY updated_at DESC ;

-- name: ListAgentSessionsByPlanDir :many
SELECT *
FROM agent_sessions
WHERE user_email = sqlc.arg ('user_email')
AND inferred_plan_dir = sqlc.arg ('plan_dir')
AND (sqlc.arg ('workspace_id') = '' OR workspace_id = sqlc.arg ('workspace_id'))
ORDER BY updated_at DESC ;

-- name: ListAgentSessionsByPlanDirPrefix :many
SELECT *
FROM agent_sessions
WHERE user_email = sqlc.arg ('user_email')
AND (sqlc.arg ('workspace_id') = '' OR workspace_id = sqlc.arg ('workspace_id'))
AND (
inferred_plan_dir = sqlc.arg ('plan_dir')
OR inferred_plan_dir LIKE sqlc.arg ('plan_dir_prefix')
)
ORDER BY updated_at DESC ;

-- name: ListAgentSessionsForUser :many
SELECT *
FROM agent_sessions
WHERE user_email = sqlc.arg ('user_email')
AND inferred_plan_dir IS NOT NULL
ORDER BY updated_at DESC ;

-- name: UpsertAgentSessionIndex :one
INSERT INTO agent_sessions (
id,
workspace_id,
thread_id,
user_email,
source,
session_path,
session_id,
parent_session_id,
cwd,
agent,
parent_plan_dir,
source_review_dir,
workflow_id,
workflow_node_id,
continued_from_session_id,
forked_from_session_id,
file_size,
file_mtime,
file_hash,
last_indexed_offset,
needs_hydration,
status,
inferred_workspace_id,
inferred_plan_dir,
imported_head_entry_id,
last_error,
metadata_json
)
VALUES (
sqlc.arg ('id'),
sqlc.narg ('workspace_id'),
sqlc.narg ('thread_id'),
sqlc.narg ('user_email'),
sqlc.arg ('source'),
sqlc.arg ('session_path'),
sqlc.narg ('session_id'),
sqlc.narg ('parent_session_id'),
sqlc.narg ('cwd'),
COALESCE (NULLIF (sqlc.arg ('agent'), ''), 'pi'),
sqlc.narg ('parent_plan_dir'),
sqlc.narg ('source_review_dir'),
sqlc.narg ('workflow_id'),
sqlc.narg ('workflow_node_id'),
sqlc.narg ('continued_from_session_id'),
sqlc.narg ('forked_from_session_id'),
sqlc.arg ('file_size'),
sqlc.narg ('file_mtime'),
sqlc.narg ('file_hash'),
sqlc.arg ('last_indexed_offset'),
sqlc.arg ('needs_hydration'),
sqlc.arg ('status'),
sqlc.narg ('inferred_workspace_id'),
sqlc.narg ('inferred_plan_dir'),
NULL,
sqlc.narg ('last_error'),
sqlc.narg ('metadata_json')
)
ON CONFLICT (session_path) WHERE session_path IS NOT NULL DO UPDATE SET
user_email = excluded.user_email,
source = excluded.source,
session_id = excluded.session_id,
parent_session_id = excluded.parent_session_id,
cwd = excluded.cwd,
agent = excluded.agent,
parent_plan_dir = excluded.parent_plan_dir,
source_review_dir = excluded.source_review_dir,
workflow_id = excluded.workflow_id,
workflow_node_id = excluded.workflow_node_id,
continued_from_session_id = excluded.continued_from_session_id,
forked_from_session_id = excluded.forked_from_session_id,
file_size = excluded.file_size,
file_mtime = excluded.file_mtime,
file_hash = excluded.file_hash,
last_indexed_offset = excluded.last_indexed_offset,
needs_hydration = CASE
WHEN agent_sessions.file_size = excluded.file_size
AND COALESCE (agent_sessions.file_mtime,
'') = COALESCE (excluded.file_mtime,
'')
AND COALESCE (agent_sessions.file_hash, '') = COALESCE (excluded.file_hash, '')
THEN agent_sessions.needs_hydration
ELSE excluded.needs_hydration
END,
status = CASE
WHEN agent_sessions.status IN ('importing', 'imported', 'diverged')
THEN agent_sessions.status
ELSE excluded.status
END,
inferred_workspace_id = excluded.inferred_workspace_id,
inferred_plan_dir = excluded.inferred_plan_dir,
last_error = excluded.last_error,
metadata_json = excluded.metadata_json,
updated_at = CURRENT_TIMESTAMP
RETURNING * ;

-- name: BackfillAgentSessionsWorkspaceForThread :exec
UPDATE agent_sessions
SET workspace_id = sqlc.arg ('workspace_id')
WHERE thread_id = sqlc.arg ('thread_id')
AND (workspace_id IS NULL OR workspace_id = '') ;

-- name: UpdateAgentSessionImportingState :exec
UPDATE agent_sessions
SET workspace_id = sqlc.narg ('workspace_id'),
thread_id = sqlc.narg ('thread_id'),
status = 'importing',
inferred_workspace_id = sqlc.narg ('inferred_workspace_id'),
inferred_plan_dir = sqlc.narg ('inferred_plan_dir'),
last_error = NULL,
metadata_json = sqlc.narg ('metadata_json'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: UpdateAgentSessionImportFinalState :exec
UPDATE agent_sessions
SET workspace_id = sqlc.narg ('workspace_id'),
thread_id = sqlc.narg ('thread_id'),
status = sqlc.arg ('status'),
inferred_workspace_id = sqlc.narg ('inferred_workspace_id'),
inferred_plan_dir = sqlc.narg ('inferred_plan_dir'),
imported_head_entry_id = sqlc.narg ('imported_head_entry_id'),
last_imported_at = CURRENT_TIMESTAMP,
needs_hydration = 0,
last_error = NULL,
metadata_json = sqlc.narg ('metadata_json'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: MarkAgentSessionHydratedByPath :exec
UPDATE agent_sessions
SET needs_hydration = 0,
last_error = NULL,
updated_at = CURRENT_TIMESTAMP
WHERE session_path = sqlc.arg ('session_path') ;

-- name: UpdateAgentSessionImportFailedState :exec
UPDATE agent_sessions
SET status = 'failed',
last_error = sqlc.narg ('last_error'),
metadata_json = sqlc.narg ('metadata_json'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: UpdateAgentSessionInferenceState :exec
UPDATE agent_sessions
SET workspace_id = sqlc.narg ('workspace_id'),
thread_id = sqlc.narg ('thread_id'),
status = sqlc.arg ('status'),
inferred_workspace_id = sqlc.narg ('inferred_workspace_id'),
inferred_plan_dir = sqlc.narg ('inferred_plan_dir'),
imported_head_entry_id = NULL,
last_error = sqlc.narg ('last_error'),
metadata_json = sqlc.narg ('metadata_json'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;
