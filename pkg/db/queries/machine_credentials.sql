-- name: CreateMachineCredential :one
INSERT INTO machine_credentials (
    id,
    name,
    secret_hash,
    default_actor_email,
    allowed_actor_emails_json,
    allowed_slugs_json,
    allowed_purposes_json,
    expires_at,
    created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id,
name,
secret_hash,
default_actor_email,
allowed_actor_emails_json,
allowed_slugs_json,
allowed_purposes_json,
expires_at,
revoked_at,
created_at,
last_used_at ;

-- name: GetMachineCredential :one
SELECT id,
name,
secret_hash,
default_actor_email,
allowed_actor_emails_json,
allowed_slugs_json,
allowed_purposes_json,
expires_at,
revoked_at,
created_at,
last_used_at
FROM machine_credentials
WHERE id = ? ;

-- name: UpdateMachineCredentialLastUsed :exec
UPDATE machine_credentials
SET last_used_at = ?
WHERE id = ? ;

-- name: RevokeMachineCredential :execrows
UPDATE machine_credentials
SET revoked_at = ?
WHERE id = ? AND revoked_at IS NULL ;

-- name: ListMachineCredentials :many
SELECT id,
name,
secret_hash,
default_actor_email,
allowed_actor_emails_json,
allowed_slugs_json,
allowed_purposes_json,
expires_at,
revoked_at,
created_at,
last_used_at
FROM machine_credentials
ORDER BY created_at DESC ;
