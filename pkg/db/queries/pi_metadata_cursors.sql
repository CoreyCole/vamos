-- name: GetPiMetadataCursor :one
SELECT *
FROM pi_metadata_cursors
WHERE source_path = sqlc.arg('source_path');

-- name: UpsertPiMetadataCursorAdvanced :one
INSERT INTO pi_metadata_cursors (
    source_path,
    source_identity,
    byte_offset,
    last_event_id,
    last_event_time,
    status,
    last_error
) VALUES (
    sqlc.arg('source_path'),
    sqlc.narg('source_identity'),
    sqlc.arg('byte_offset'),
    sqlc.narg('last_event_id'),
    sqlc.narg('last_event_time'),
    'ok',
    NULL
)
ON CONFLICT (source_path) DO UPDATE SET
source_identity = excluded.source_identity,
byte_offset = excluded.byte_offset,
last_event_id = excluded.last_event_id,
last_event_time = excluded.last_event_time,
status = 'ok',
last_error = NULL,
updated_at = CURRENT_TIMESTAMP
RETURNING * ;

-- name: MarkPiMetadataCursorFailed :exec
INSERT INTO pi_metadata_cursors (
source_path,
source_identity,
byte_offset,
status,
last_error
) VALUES (
sqlc.arg ('source_path'),
sqlc.narg ('source_identity'),
sqlc.arg ('byte_offset'),
'failed',
sqlc.narg ('last_error')
)
ON CONFLICT (source_path) DO UPDATE SET
source_identity = excluded.source_identity,
byte_offset = excluded.byte_offset,
status = 'failed',
last_error = excluded.last_error,
updated_at = CURRENT_TIMESTAMP ;
