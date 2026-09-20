-- name: InsertAgentThreadEntry :exec
INSERT INTO agent_thread_entries (
    id,
    thread_id,
    parent_entry_id,
    author_kind,
    author_name,
    author_initial,
    author_slug,
    author_email,
    avatar_bg,
    body
)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('thread_id'),
    sqlc.narg('parent_entry_id'),
    sqlc.arg('author_kind'),
    sqlc.arg('author_name'),
    sqlc.arg('author_initial'),
    sqlc.arg('author_slug'),
    sqlc.arg('author_email'),
    sqlc.arg('avatar_bg'),
    sqlc.arg('body')
);

-- name: GetAgentThreadEntry :one
SELECT
    id,
    thread_id,
    parent_entry_id,
    author_kind,
    author_name,
    author_initial,
    author_slug,
    author_email,
    avatar_bg,
    body,
    created_at
FROM agent_thread_entries
WHERE id = sqlc.arg('id');

-- name: ListAgentThreadEntriesByParent :many
SELECT
    id,
    thread_id,
    parent_entry_id,
    author_kind,
    author_name,
    author_initial,
    author_slug,
    author_email,
    avatar_bg,
    body,
    created_at
FROM agent_thread_entries
WHERE
    thread_id = sqlc.arg('thread_id')
    AND parent_entry_id = sqlc.arg('parent_entry_id')
ORDER BY created_at ASC, id ASC;

-- name: ListAgentThreadEntriesByThread :many
SELECT
    id,
    thread_id,
    parent_entry_id,
    author_kind,
    author_name,
    author_initial,
    author_slug,
    author_email,
    avatar_bg,
    body,
    created_at
FROM agent_thread_entries
WHERE
    thread_id = sqlc.arg('thread_id')
    AND parent_entry_id IS NOT NULL
ORDER BY created_at ASC, id ASC;
