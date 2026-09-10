-- name: CreateAgent :one
INSERT INTO agents (
    id,
    slug,
    name,
    label,
    description
)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('slug'),
    sqlc.arg('name'),
    sqlc.arg('label'),
    sqlc.arg('description')
)
RETURNING
id,
slug,
name,
label,
description,
created_at,
updated_at,
archived_at ;

-- name: GetAgentBySlug :one
SELECT
id,
slug,
name,
label,
description,
created_at,
updated_at,
archived_at
FROM agents
WHERE slug = sqlc.arg ('slug')
AND archived_at IS NULL ;

-- name: GetAgent :one
SELECT
id,
slug,
name,
label,
description,
created_at,
updated_at,
archived_at
FROM agents
WHERE id = sqlc.arg ('id')
AND archived_at IS NULL ;

-- name: ListAgents :many
SELECT
id,
slug,
name,
label,
description,
created_at,
updated_at,
archived_at
FROM agents
WHERE archived_at IS NULL
ORDER BY LOWER (name), slug ;
