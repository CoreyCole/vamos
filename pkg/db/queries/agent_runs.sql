-- name: CreateAgentRun :one
INSERT INTO agent_runs (
    id,
    workspace_id,
    thread_id,
    session_id,
    trigger,
    status,
    prompt_text,
    restore_head_entry_id,
    result_head_entry_id,
    workflow_id,
    temporal_run_id,
    workflow_node_id,
    workflow_attempt,
    workflow_result_status,
    workflow_result_json,
    root_doc_path,
    error_message
)
VALUES (
    sqlc.arg('id'),
    sqlc.narg('workspace_id'),
    sqlc.arg('thread_id'),
    sqlc.narg('session_id'),
    sqlc.arg('trigger'),
    sqlc.arg('status'),
    sqlc.arg('prompt_text'),
    sqlc.narg('restore_head_entry_id'),
    sqlc.narg('result_head_entry_id'),
    sqlc.arg('workflow_id'),
    sqlc.narg('temporal_run_id'),
    sqlc.narg('workflow_node_id'),
    sqlc.arg('workflow_attempt'),
    sqlc.narg('workflow_result_status'),
    sqlc.narg('workflow_result_json'),
    sqlc.arg('root_doc_path'),
    sqlc.narg('error_message')
)
RETURNING * ;

-- name: GetAgentRun :one
SELECT *
FROM agent_runs
WHERE id = sqlc.arg ('id') ;

-- name: ListAgentRunsByThread :many
SELECT *
FROM agent_runs
WHERE thread_id = sqlc.arg ('thread_id')
ORDER BY created_at DESC ;

-- name: GetLatestAgentRunByThread :one
SELECT *
FROM agent_runs
WHERE thread_id = sqlc.arg ('thread_id')
ORDER BY created_at DESC
LIMIT 1 ;

-- name: GetLatestAgentRunByWorkspaceThread :one
SELECT *
FROM agent_runs
WHERE workspace_id = sqlc.arg ('workspace_id')
AND thread_id = sqlc.arg ('thread_id')
ORDER BY created_at DESC
LIMIT 1 ;

-- name: GetAgentRunForWorkspace :one
SELECT *
FROM agent_runs
WHERE id = sqlc.arg ('id')
AND workspace_id = sqlc.arg ('workspace_id') ;

-- name: ListAgentRunsByWorkspace :many
SELECT *
FROM agent_runs
WHERE workspace_id = sqlc.arg ('workspace_id')
ORDER BY created_at DESC ;

-- name: ListAgentRunsByWorkspaceNode :many
SELECT *
FROM agent_runs
WHERE workspace_id = sqlc.arg ('workspace_id')
AND workflow_node_id = sqlc.arg ('workflow_node_id')
ORDER BY created_at DESC ;

-- name: GetLatestAgentRunByWorkspaceNode :one
SELECT *
FROM agent_runs
WHERE workspace_id = sqlc.arg ('workspace_id')
AND workflow_node_id = sqlc.arg ('workflow_node_id')
ORDER BY created_at DESC
LIMIT 1 ;

-- name: BackfillAgentRunsWorkspaceForThread :exec
UPDATE agent_runs
SET workspace_id = sqlc.arg ('workspace_id')
WHERE thread_id = sqlc.arg ('thread_id')
AND (workspace_id IS NULL OR workspace_id = '') ;

-- name: UpdateAgentRunWorkspaceForTest :exec
UPDATE agent_runs
SET workspace_id = sqlc.arg ('workspace_id')
WHERE id = sqlc.arg ('id') ;

-- name: UpdateAgentRunStarted :exec
UPDATE agent_runs
SET status = 'running',
temporal_run_id = sqlc.narg ('temporal_run_id')
WHERE id = sqlc.arg ('id') ;

-- name: UpdateAgentRunCheckpoint :exec
UPDATE agent_runs
SET result_head_entry_id = sqlc.narg ('result_head_entry_id')
WHERE id = sqlc.arg ('id') ;

-- name: UpdateAgentRunWorkflowResult :exec
UPDATE agent_runs
SET workflow_result_status = sqlc.narg ('workflow_result_status'),
workflow_result_json = sqlc.narg ('workflow_result_json')
WHERE id = sqlc.arg ('id') ;

-- name: CompleteAgentRun :exec
UPDATE agent_runs
SET status = 'complete',
result_head_entry_id = sqlc.narg ('result_head_entry_id'),
completed_at = CURRENT_TIMESTAMP,
error_message = NULL
WHERE id = sqlc.arg ('id') ;

-- name: FailAgentRun :exec
UPDATE agent_runs
SET status = 'failed',
error_message = sqlc.arg ('error_message'),
completed_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: FailAgentRunIfRunning :one
UPDATE agent_runs
SET status = 'failed',
error_message = sqlc.arg ('error_message'),
completed_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id')
AND status = 'running'
RETURNING * ;
