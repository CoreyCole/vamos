-- name: CreateExternalAgentSession :one
INSERT INTO external_agent_sessions (
    id,
    provider,
    external_session_id,
    transcript_path,
    cwd,
    model,
    title
)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('provider'),
    sqlc.narg('external_session_id'),
    sqlc.narg('transcript_path'),
    sqlc.narg('cwd'),
    sqlc.narg('model'),
    sqlc.narg('title')
)
RETURNING * ;

-- name: LinkExternalAgentSession :one
INSERT INTO chat_session_external_links (
id,
chat_session_id,
external_agent_session_id,
link_mode
)
VALUES (
sqlc.arg ('id'),
sqlc.arg ('chat_session_id'),
sqlc.arg ('external_agent_session_id'),
sqlc.arg ('link_mode')
)
RETURNING * ;
