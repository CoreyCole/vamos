-- name: InsertSystemSnapshot :one
INSERT INTO system_snapshots (
    boot_id, captured_at, cpu_percent,
    mem_used_bytes, mem_total_bytes, mem_used_percent,
    swap_used_bytes, swap_total_bytes,
    disk_used_bytes, disk_total_bytes,
    load_avg_1, load_avg_5, load_avg_15
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: InsertSnapshotProcess :exec
INSERT INTO system_snapshot_processes (snapshot_id, pid, user, mem_mb, cpu_percent, command)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetDistinctBootIDs :many
SELECT DISTINCT boot_id,
    CAST(MIN(captured_at) AS TEXT) as first_seen,
    CAST(MAX(captured_at) AS TEXT) as last_seen,
    COUNT(*) as snapshot_count
FROM system_snapshots
GROUP BY boot_id
ORDER BY MAX(captured_at) DESC;

-- name: GetSnapshotsByBootID :many
SELECT * FROM system_snapshots
WHERE boot_id = ?
ORDER BY captured_at DESC;

-- name: GetSnapshotProcesses :many
SELECT * FROM system_snapshot_processes
WHERE snapshot_id = ?
ORDER BY mem_mb DESC;

-- name: GetLatestSnapshotByBootID :one
SELECT * FROM system_snapshots
WHERE boot_id = ?
ORDER BY captured_at DESC
LIMIT 1;

-- name: DeleteSnapshotProcessesByBootIDs :exec
DELETE FROM system_snapshot_processes
WHERE snapshot_id IN (
    SELECT id FROM system_snapshots
    WHERE boot_id NOT IN (/*SLICE:boot_ids*/sqlc.slice('boot_ids'))
);

-- name: DeleteSnapshotsByExcludedBootIDs :exec
DELETE FROM system_snapshots
WHERE boot_id NOT IN (/*SLICE:boot_ids*/sqlc.slice('boot_ids'));

-- name: GetSystemSnapshotCount :one
SELECT COUNT(*) FROM system_snapshots;

-- name: DeleteOldestSnapshotProcesses :exec
DELETE FROM system_snapshot_processes
WHERE snapshot_id IN (
    SELECT id FROM system_snapshots
    ORDER BY captured_at ASC
    LIMIT ?
);

-- name: DeleteOldestSnapshots :exec
DELETE FROM system_snapshots
WHERE id IN (
    SELECT id FROM system_snapshots
    ORDER BY captured_at ASC
    LIMIT ?
);
