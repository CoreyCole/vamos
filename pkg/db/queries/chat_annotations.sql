-- name: CreateChatAnnotation :one
INSERT INTO chat_annotations (
    id,
    workspace_id,
    session_id,
    node_id,
    event_seq,
    author_email,
    body_markdown,
    status
)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('workspace_id'),
    sqlc.arg('session_id'),
    sqlc.arg('node_id'),
    sqlc.arg('event_seq'),
    sqlc.arg('author_email'),
    sqlc.arg('body_markdown'),
    sqlc.arg('status')
)
RETURNING * ;

-- name: ResolveChatAnnotation :exec
UPDATE chat_annotations
SET status = 'resolved', updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: ListChatAnnotationsBySession :many
SELECT *
FROM chat_annotations
WHERE session_id = sqlc.arg ('session_id')
ORDER BY created_at ASC ;

-- name: ListOpenChatAnnotationsByIDs :many
SELECT *
FROM chat_annotations
WHERE id IN (sqlc.slice ('ids'))
AND status = 'open'
ORDER BY created_at ASC ;
