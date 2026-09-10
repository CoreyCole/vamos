-- name: InsertAgentThreadOp :execrows
INSERT INTO agent_thread_ops (
    thread_id,
    op_id,
    speaker_agent_id,
    from_kind,
    from_agent_id,
    from_user_email,
    body
)
VALUES (
    sqlc.arg('thread_id'),
    sqlc.arg('op_id'),
    sqlc.narg('speaker_agent_id'),
    sqlc.arg('from_kind'),
    sqlc.narg('from_agent_id'),
    sqlc.arg('from_user_email'),
    sqlc.arg('body')
)
ON CONFLICT (thread_id, op_id) DO NOTHING ;

-- name: GetAgentThreadOp :one
SELECT *
FROM agent_thread_ops
WHERE thread_id = sqlc.arg ('thread_id')
AND op_id = sqlc.arg ('op_id') ;

-- name: CountAgentThreadOps :one
SELECT COUNT (*)
FROM agent_thread_ops
WHERE thread_id = sqlc.arg ('thread_id') ;
