-- name: CreateDocumentComment :one
INSERT INTO document_comments (
    id,
    workspace_root,
    workspace_id,
    doc_path,
    user_email,
    comment_text,
    selected_text,
    section_hint,
    heading_hint,
    start_line,
    start_column,
    end_line,
    end_column
)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('workspace_root'),
    sqlc.narg('workspace_id'),
    sqlc.arg('doc_path'),
    sqlc.arg('user_email'),
    sqlc.arg('comment_text'),
    sqlc.arg('selected_text'),
    sqlc.narg('section_hint'),
    sqlc.narg('heading_hint'),
    sqlc.arg('start_line'),
    sqlc.arg('start_column'),
    sqlc.arg('end_line'),
    sqlc.arg('end_column')
)
RETURNING * ;

-- name: GetDocumentComment :one
SELECT *
FROM document_comments
WHERE id = sqlc.arg ('id')
AND deleted_at IS NULL
LIMIT 1 ;

-- name: ListDocumentComments :many
SELECT *
FROM document_comments
WHERE doc_path = sqlc.arg ('doc_path')
AND deleted_at IS NULL
AND (CAST (sqlc.arg ('include_resolved') AS INTEGER) = 1 OR resolved = 0)
ORDER BY resolved ASC, created_at DESC ;

-- name: ListWorkspaceDocumentComments :many
SELECT *
FROM document_comments
WHERE workspace_root = sqlc.arg ('workspace_root')
AND deleted_at IS NULL
AND (CAST (sqlc.arg ('include_resolved') AS INTEGER) = 1 OR resolved = 0)
ORDER BY doc_path ASC,
created_at DESC ;

-- name: CountUnresolvedWorkspaceComments :one
SELECT count (*)
FROM document_comments
WHERE workspace_root = sqlc.arg ('workspace_root')
AND resolved = 0
AND deleted_at IS NULL ;

-- name: CreateDocumentCommentReply :one
INSERT INTO document_comment_replies (
id,
comment_id,
user_email,
actor_type,
reply_text
)
VALUES (
sqlc.arg ('id'),
sqlc.arg ('comment_id'),
sqlc.arg ('user_email'),
sqlc.arg ('actor_type'),
sqlc.arg ('reply_text')
)
RETURNING * ;

-- name: ListDocumentCommentReplies :many
SELECT *
FROM document_comment_replies
WHERE comment_id = sqlc.arg ('comment_id')
AND deleted_at IS NULL
ORDER BY created_at ASC ;

-- name: ResolveDocumentComment :exec
UPDATE document_comments
SET resolved = 1,
resolved_by = sqlc.arg ('resolved_by'),
resolved_actor_type = sqlc.arg ('resolved_actor_type'),
resolved_at = CURRENT_TIMESTAMP,
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id')
AND deleted_at IS NULL ;

-- name: ReopenDocumentComment :exec
UPDATE document_comments
SET resolved = 0,
resolved_by = NULL,
resolved_actor_type = NULL,
resolved_at = NULL,
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id')
AND deleted_at IS NULL ;

-- name: SoftDeleteDocumentComment :exec
UPDATE document_comments
SET deleted_at = CURRENT_TIMESTAMP,
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id')
AND deleted_at IS NULL ;
