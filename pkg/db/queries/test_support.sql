-- Test/support queries for assertions that intentionally inspect persistence shape.
-- Keep production code on service/query helpers; use these only from tests.

-- name: TestSupportCountPrimaryThreadWorkspaceAssociations :one
SELECT COUNT(*)
FROM agent_thread_workspaces
WHERE
    thread_id = sqlc.arg('thread_id')
    AND is_primary = 1;

-- name: TestSupportCountRelatedThreadWorkspaceAssociation :one
SELECT COUNT(*)
FROM agent_thread_workspaces
WHERE
    thread_id = sqlc.arg('thread_id')
    AND workspace_id = sqlc.arg('workspace_id')
    AND is_primary = 0
    AND role = 'related';

-- name: TestSupportGetAgentSessionWorkspaceID :one
SELECT attached_workspace_id
FROM agent_sessions
WHERE id = sqlc.arg('id');

-- name: TestSupportCountChatSessionEvents :one
SELECT COUNT(*)
FROM chat_session_events
WHERE session_id = sqlc.arg('session_id');

-- name: TestSupportCountAgentSessions :one
SELECT COUNT(*)
FROM agent_sessions;

-- name: TestSupportCountAgentSessionsByPath :one
SELECT COUNT(*)
FROM agent_sessions
WHERE artifact_path = sqlc.arg('artifact_path');

-- name: TestSupportCountAgentEntries :one
SELECT COUNT(*)
FROM agent_entries;

-- name: TestSupportCountWorkspaces :one
SELECT COUNT(*)
FROM workspaces;

-- name: TestSupportCountWorkspacesByRootDocPath :one
SELECT COUNT(*)
FROM workspaces
WHERE root_doc_path = sqlc.arg('root_doc_path');

-- name: TestSupportCountPrimaryThreadWorkspaceAssociationsByWorkspace :one
SELECT COUNT(*)
FROM agent_thread_workspaces
WHERE
    workspace_id = sqlc.arg('workspace_id')
    AND is_primary = 1;
