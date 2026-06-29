-- name: ListItems :many
SELECT id, title, completed, created_at, updated_at
FROM items
ORDER BY created_at DESC, id DESC;

-- name: CountItems :one
SELECT count(*) FROM items;

-- name: CreateItem :one
INSERT INTO items (title)
VALUES (?)
RETURNING id, title, completed, created_at, updated_at ;

-- name: ToggleItem :one
UPDATE items
SET completed = CASE completed WHEN 0 THEN 1 ELSE 0 END,
updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING id, title, completed, created_at, updated_at ;

-- name: DeleteItem :exec
DELETE FROM items
WHERE id = ? ;
