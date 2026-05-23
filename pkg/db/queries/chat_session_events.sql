-- name: EnsureChatSessionSequence :exec
INSERT INTO chat_session_sequences (session_id, next_seq)
VALUES (sqlc.arg('session_id'), 1)
ON CONFLICT (session_id) DO NOTHING ;

-- name: ReserveChatSessionSeq :one
UPDATE chat_session_sequences
SET next_seq = next_seq + 1
WHERE session_id = sqlc.arg ('session_id')
RETURNING next_seq - 1 AS seq ;

-- name: AppendChatSessionEvent :one
INSERT INTO chat_session_events (
session_id,
seq,
event_type,
actor_participant_id,
command_id,
run_id,
payload_json
)
VALUES (
sqlc.arg ('session_id'),
sqlc.arg ('seq'),
sqlc.arg ('event_type'),
sqlc.narg ('actor_participant_id'),
sqlc.narg ('command_id'),
sqlc.narg ('run_id'),
sqlc.arg ('payload_json')
)
RETURNING * ;

-- name: ListChatSessionEventsAfter :many
SELECT *
FROM chat_session_events
WHERE session_id = sqlc.arg ('session_id')
AND seq > sqlc.arg ('after_seq')
ORDER BY seq ASC
LIMIT sqlc.arg ('limit') ;

-- name: ListChatSessionEventsThrough :many
SELECT *
FROM chat_session_events
WHERE session_id = sqlc.arg ('session_id')
AND seq <= sqlc.arg ('through_seq')
ORDER BY seq ASC ;

-- name: UpsertChatSessionProjection :one
INSERT INTO chat_session_projections (
session_id,
last_seq,
messages_json,
runs_json,
participants_json,
artifacts_json,
topology_json
)
VALUES (
sqlc.arg ('session_id'),
sqlc.arg ('last_seq'),
sqlc.arg ('messages_json'),
sqlc.arg ('runs_json'),
sqlc.arg ('participants_json'),
sqlc.arg ('artifacts_json'),
sqlc.arg ('topology_json')
)
ON CONFLICT (session_id) DO UPDATE SET
last_seq = excluded.last_seq,
messages_json = excluded.messages_json,
runs_json = excluded.runs_json,
participants_json = excluded.participants_json,
artifacts_json = excluded.artifacts_json,
topology_json = excluded.topology_json,
updated_at = CURRENT_TIMESTAMP
RETURNING * ;

-- name: GetChatSessionProjection :one
SELECT *
FROM chat_session_projections
WHERE session_id = sqlc.arg ('session_id') ;

-- name: CreateChatSessionBaseline :one
INSERT INTO chat_session_baselines (
session_id,
parent_session_id,
forked_from_seq,
baseline_projection_version,
messages_json,
runs_json,
artifacts_json,
participants_json,
topology_json,
selected_state_json
)
VALUES (
sqlc.arg ('session_id'),
sqlc.arg ('parent_session_id'),
sqlc.arg ('forked_from_seq'),
sqlc.arg ('baseline_projection_version'),
sqlc.arg ('messages_json'),
sqlc.arg ('runs_json'),
sqlc.arg ('artifacts_json'),
sqlc.arg ('participants_json'),
sqlc.arg ('topology_json'),
sqlc.arg ('selected_state_json')
)
RETURNING * ;

-- name: GetChatSessionBaseline :one
SELECT *
FROM chat_session_baselines
WHERE session_id = sqlc.arg ('session_id') ;
