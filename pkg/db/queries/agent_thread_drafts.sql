-- name: GetAgentThreadDraft :one
SELECT content
FROM agent_thread_drafts
WHERE
    user_email = sqlc.arg('user_email')
    AND thread_id = sqlc.arg('thread_id');

-- name: UpsertAgentThreadDraft :exec
INSERT INTO agent_thread_drafts (
    user_email, thread_id, content, operation_order, updated_at
)
VALUES (
    sqlc.arg('user_email'),
    sqlc.arg('thread_id'),
    sqlc.arg('content'),
    sqlc.arg('operation_order'),
    CURRENT_TIMESTAMP
)
ON CONFLICT (user_email, thread_id) DO UPDATE SET
content = excluded.content,
operation_order = excluded.operation_order,
updated_at = CURRENT_TIMESTAMP
WHERE excluded.operation_order > agent_thread_drafts.operation_order ;

-- name: ClearAgentThreadDraft :exec
INSERT INTO agent_thread_drafts (user_email,
thread_id,
content,
operation_order,
updated_at)
VALUES (
sqlc.arg ('user_email'),
sqlc.arg ('thread_id'),
'',
sqlc.arg ('operation_order'),
CURRENT_TIMESTAMP
)
ON CONFLICT (user_email, thread_id) DO UPDATE SET
content = '',
operation_order = excluded.operation_order,
updated_at = CURRENT_TIMESTAMP
WHERE excluded.operation_order > agent_thread_drafts.operation_order ;
