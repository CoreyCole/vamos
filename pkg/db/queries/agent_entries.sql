-- name: CreateAgentEntry :exec
INSERT INTO agent_entries (
    lineage_id,
    entry_id,
    parent_entry_id,
    entry_type,
    origin_order,
    payload_json,
    origin_thread_id,
    origin_run_id,
    origin_session_id,
    session_timestamp
)
VALUES (
    sqlc.arg('lineage_id'),
    sqlc.arg('entry_id'),
    sqlc.narg('parent_entry_id'),
    sqlc.arg('entry_type'),
    sqlc.arg('origin_order'),
    sqlc.arg('payload_json'),
    sqlc.arg('origin_thread_id'),
    sqlc.narg('origin_run_id'),
    sqlc.narg('origin_session_id'),
    sqlc.arg('session_timestamp')
);

-- name: GetAgentEntry :one
SELECT *
FROM agent_entries
WHERE
    lineage_id = sqlc.arg('lineage_id')
    AND entry_id = sqlc.arg('entry_id');

-- name: ListAgentEntryPath :many
WITH RECURSIVE ancestry AS (
    SELECT
        root.lineage_id,
        root.entry_id,
        root.parent_entry_id,
        root.entry_type,
        root.origin_order,
        root.payload_json,
        root.origin_thread_id,
        root.origin_run_id,
        root.origin_session_id,
        root.session_timestamp,
        root.created_at,
        0 AS depth
    FROM agent_entries AS root
    WHERE
        root.lineage_id = sqlc.arg('lineage_id')
        AND root.entry_id = sqlc.arg('head_entry_id')

    UNION ALL

    SELECT
        e.lineage_id,
        e.entry_id,
        e.parent_entry_id,
        e.entry_type,
        e.origin_order,
        e.payload_json,
        e.origin_thread_id,
        e.origin_run_id,
        e.origin_session_id,
        e.session_timestamp,
        e.created_at,
        ancestry.depth + 1 AS depth
    FROM agent_entries e
    JOIN ancestry
        ON
            e.lineage_id = ancestry.lineage_id
            AND e.entry_id = ancestry.parent_entry_id
)

SELECT
    lineage_id,
    entry_id,
    parent_entry_id,
    entry_type,
    origin_order,
    payload_json,
    origin_thread_id,
    origin_run_id,
    origin_session_id,
    session_timestamp,
    created_at
FROM ancestry
ORDER BY depth DESC, origin_order ASC;
