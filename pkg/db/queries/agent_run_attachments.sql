-- name: CreateAgentRunAttachment :one
INSERT INTO agent_run_attachments (
    id, run_id, thread_id, path, basename, position
)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING * ;

-- name: ListAgentRunAttachmentsForRun :many
SELECT * FROM agent_run_attachments
WHERE run_id = ?
ORDER BY position ASC, created_at ASC ;

-- name: ListAgentRunAttachmentsForThread :many
SELECT * FROM agent_run_attachments
WHERE thread_id = ?
ORDER BY created_at ASC, position ASC ;
