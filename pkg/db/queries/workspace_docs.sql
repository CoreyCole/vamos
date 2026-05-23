-- name: ResolveWorkspaceForDocPath :one
WITH input AS (
    SELECT
        CAST(sqlc.arg('doc_path') AS TEXT) AS doc_path,
        CAST(sqlc.arg('user_email') AS TEXT) AS user_email
)

SELECT w.*, d.rel_path
FROM input
JOIN workspace_docs d ON d.doc_path = input.doc_path
JOIN workspaces w ON w.id = d.workspace_id
WHERE
    d.deleted_at IS NULL
    AND w.archived_at IS NULL
ORDER BY
    CASE WHEN w.user_email = input.user_email THEN 0 ELSE 1 END,
    w.updated_at DESC,
    w.created_at DESC,
    w.id ASC
LIMIT 1;

-- name: ListWorkspaceDocs :many
SELECT * FROM workspace_docs
WHERE
    workspace_id = sqlc.arg('workspace_id')
    AND deleted_at IS NULL
ORDER BY rel_path ASC;

-- name: GetWorkspaceDoc :one
SELECT * FROM workspace_docs
WHERE
    workspace_id = sqlc.arg('workspace_id')
    AND doc_path = sqlc.arg('doc_path');

-- name: UpsertWorkspaceDoc :exec
INSERT INTO workspace_docs (
    workspace_id,
    doc_path,
    rel_path,
    kind,
    title,
    size_bytes,
    mtime_unix,
    content_hash,
    deleted_at,
    updated_at
)
VALUES (
    sqlc.arg('workspace_id'),
    sqlc.arg('doc_path'),
    sqlc.arg('rel_path'),
    sqlc.arg('kind'),
    sqlc.arg('title'),
    sqlc.arg('size_bytes'),
    sqlc.arg('mtime_unix'),
    sqlc.narg('content_hash'),
    NULL,
    CURRENT_TIMESTAMP
)
ON CONFLICT (workspace_id, doc_path) DO UPDATE SET
rel_path = excluded.rel_path,
kind = excluded.kind,
title = excluded.title,
size_bytes = excluded.size_bytes,
mtime_unix = excluded.mtime_unix,
content_hash = excluded.content_hash,
deleted_at = NULL,
updated_at = CURRENT_TIMESTAMP ;

-- name: MarkWorkspaceDocDeleted :exec
UPDATE workspace_docs
SET deleted_at = CURRENT_TIMESTAMP,
updated_at = CURRENT_TIMESTAMP
WHERE workspace_id = sqlc.arg ('workspace_id')
AND doc_path = sqlc.arg ('doc_path')
AND deleted_at IS NULL ;
