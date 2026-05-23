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
last_error = NULL,
metadata_json = sqlc.narg ('metadata_json'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

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
