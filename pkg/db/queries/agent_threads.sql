-- name: CreateAgentThread :one
INSERT INTO agent_threads (
    id,
    user_email,
    title,
    cwd,
    lineage_id,
    project_id,
    plan_dir_rel,
    head_entry_id,
    parent_thread_id,
    forked_from_entry_id,
    pi_session_id
)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('user_email'),
    sqlc.arg('title'),
    sqlc.arg('cwd'),
    sqlc.arg('lineage_id'),
    sqlc.arg('project_id'),
    sqlc.narg('plan_dir_rel'),
    sqlc.narg('head_entry_id'),
    sqlc.narg('parent_thread_id'),
    sqlc.narg('forked_from_entry_id'),
    sqlc.arg('pi_session_id')
)
RETURNING
id,
user_email,
title,
cwd,
lineage_id,
project_id,
plan_dir_rel,
head_entry_id,
parent_thread_id,
forked_from_entry_id,
pi_session_id,
agent_slug,
room_kind,
pair_agent_slug_a,
pair_agent_slug_b,
created_at,
updated_at,
archived_at ;

-- name: GetAgentThread :one
SELECT
id,
user_email,
title,
cwd,
lineage_id,
project_id,
plan_dir_rel,
head_entry_id,
parent_thread_id,
forked_from_entry_id,
pi_session_id,
agent_slug,
room_kind,
pair_agent_slug_a,
pair_agent_slug_b,
created_at,
updated_at,
archived_at
FROM agent_threads
WHERE id = sqlc.arg ('id')
AND archived_at IS NULL ;

-- name: GetSharedAgentThread :one
SELECT
id,
user_email,
title,
cwd,
lineage_id,
project_id,
plan_dir_rel,
head_entry_id,
parent_thread_id,
forked_from_entry_id,
pi_session_id,
agent_slug,
room_kind,
pair_agent_slug_a,
pair_agent_slug_b,
created_at,
updated_at,
archived_at
FROM agent_threads
WHERE id = sqlc.arg ('id')
AND archived_at IS NULL ;

-- name: ListSharedAgentThreadsWithWorkspace :many
SELECT
t.id,
t.user_email,
t.title,
t.cwd,
t.lineage_id,
t.project_id,
t.plan_dir_rel,
t.head_entry_id,
t.parent_thread_id,
t.forked_from_entry_id,
t.pi_session_id,
t.agent_slug,
t.room_kind,
t.pair_agent_slug_a,
t.pair_agent_slug_b,
t.created_at,
t.updated_at,
t.archived_at,
atw.workspace_id AS primary_workspace_id,
w.root_doc_path AS workspace_root_doc_path
FROM agent_threads t
LEFT JOIN agent_thread_workspaces atw
ON atw.thread_id = t.id AND atw.is_primary = 1
LEFT JOIN workspaces w
ON w.id = atw.workspace_id AND w.archived_at IS NULL
WHERE t.archived_at IS NULL
ORDER BY t.updated_at DESC ;

-- name: ListSharedAgentThreadsByPlanDir :many
-- Dual-read: prefer agent_threads.plan_dir_rel FK; fallback to plan-owned
-- session plan_dir for not-yet-backfilled (NULL) threads only.
SELECT DISTINCT
t.id,
t.user_email,
t.title,
t.cwd,
t.lineage_id,
t.project_id,
t.plan_dir_rel,
t.head_entry_id,
t.parent_thread_id,
t.forked_from_entry_id,
t.pi_session_id,
t.agent_slug,
t.room_kind,
t.pair_agent_slug_a,
t.pair_agent_slug_b,
t.created_at,
t.updated_at,
t.archived_at
FROM agent_threads t
LEFT JOIN agent_sessions s
ON s.projected_thread_id = t.id
AND s.identity_kind = 'plan_owned'
WHERE t.archived_at IS NULL
AND (
t.plan_dir_rel = sqlc.arg ('plan_dir_rel')
OR (
t.plan_dir_rel IS NULL
AND s.plan_dir = sqlc.arg ('plan_dir')
)
)
ORDER BY t.updated_at DESC ;

-- name: GetAgentThreadForUser :one
SELECT
id,
user_email,
title,
cwd,
lineage_id,
project_id,
plan_dir_rel,
head_entry_id,
parent_thread_id,
forked_from_entry_id,
pi_session_id,
agent_slug,
room_kind,
pair_agent_slug_a,
pair_agent_slug_b,
created_at,
updated_at,
archived_at
FROM agent_threads
WHERE id = sqlc.arg ('id')
AND archived_at IS NULL
AND (
user_email = sqlc.arg ('user_email')
OR room_kind IN ('bot_home', 'plan', 'pairwise')
) ;

-- name: ListAgentThreads :many
SELECT
id,
user_email,
title,
cwd,
lineage_id,
project_id,
plan_dir_rel,
head_entry_id,
parent_thread_id,
forked_from_entry_id,
pi_session_id,
agent_slug,
room_kind,
pair_agent_slug_a,
pair_agent_slug_b,
created_at,
updated_at,
archived_at
FROM agent_threads
WHERE archived_at IS NULL
AND (
user_email = sqlc.arg ('user_email')
OR room_kind IN ('bot_home', 'plan', 'pairwise')
)
ORDER BY updated_at DESC
LIMIT sqlc.arg ('limit') ;

-- name: UpdateAgentThreadHead :exec
UPDATE agent_threads
SET head_entry_id = sqlc.narg ('head_entry_id'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: UpdateAgentThreadTitle :exec
UPDATE agent_threads
SET title = sqlc.arg ('title'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: UpdateAgentThreadCwd :exec
UPDATE agent_threads
SET cwd = sqlc.arg ('cwd'),
plan_dir_rel = COALESCE (sqlc.narg ('plan_dir_rel'), plan_dir_rel),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: UpdateAgentThreadProject :exec
UPDATE agent_threads
SET project_id = sqlc.arg ('project_id'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: ListAgentThreadsByWorkspace :many
SELECT
t.id,
t.user_email,
t.title,
t.cwd,
t.lineage_id,
t.project_id,
t.plan_dir_rel,
t.head_entry_id,
t.parent_thread_id,
t.forked_from_entry_id,
t.pi_session_id,
t.agent_slug,
t.room_kind,
t.pair_agent_slug_a,
t.pair_agent_slug_b,
t.created_at,
t.updated_at,
t.archived_at
FROM agent_threads t
JOIN agent_thread_workspaces atw ON atw.thread_id = t.id
WHERE atw.workspace_id = sqlc.arg ('workspace_id')
AND atw.is_primary = 1
AND t.archived_at IS NULL
ORDER BY t.updated_at DESC ;

-- name: ListAgentThreadsForUserWithWorkspace :many
SELECT
t.id,
t.user_email,
t.title,
t.cwd,
t.lineage_id,
t.project_id,
t.plan_dir_rel,
t.head_entry_id,
t.parent_thread_id,
t.forked_from_entry_id,
t.pi_session_id,
t.agent_slug,
t.room_kind,
t.pair_agent_slug_a,
t.pair_agent_slug_b,
t.created_at,
t.updated_at,
t.archived_at,
atw.workspace_id AS primary_workspace_id,
w.root_doc_path AS workspace_root_doc_path
FROM agent_threads t
LEFT JOIN agent_thread_workspaces atw
ON atw.thread_id = t.id AND atw.is_primary = 1
LEFT JOIN workspaces w
ON w.id = atw.workspace_id
AND w.archived_at IS NULL
WHERE t.archived_at IS NULL
AND (
t.user_email = sqlc.arg ('user_email')
OR t.room_kind IN ('bot_home', 'plan', 'pairwise')
)
ORDER BY t.updated_at DESC ;

-- name: GetAgentThreadForWorkspaceUser :one
SELECT
t.id,
t.user_email,
t.title,
t.cwd,
t.lineage_id,
t.project_id,
t.plan_dir_rel,
t.head_entry_id,
t.parent_thread_id,
t.forked_from_entry_id,
t.pi_session_id,
t.agent_slug,
t.room_kind,
t.pair_agent_slug_a,
t.pair_agent_slug_b,
t.created_at,
t.updated_at,
t.archived_at
FROM agent_threads AS t
JOIN agent_thread_workspaces atw
ON atw.thread_id = t.id
AND atw.workspace_id = sqlc.arg ('workspace_id')
AND atw.is_primary = 1
JOIN workspaces AS w
ON w.id = atw.workspace_id
WHERE t.id = sqlc.arg ('thread_id')
AND t.archived_at IS NULL
AND w.archived_at IS NULL
AND (
w.user_email = sqlc.arg ('user_email')
OR t.room_kind IN ('bot_home', 'plan', 'pairwise')
) ;


-- name: ListAgentThreadsByPlanDirRel :many
-- Plan-home children: FK only (freeform NULL excluded).
SELECT
id,
user_email,
title,
cwd,
lineage_id,
project_id,
plan_dir_rel,
head_entry_id,
parent_thread_id,
forked_from_entry_id,
pi_session_id,
agent_slug,
room_kind,
pair_agent_slug_a,
pair_agent_slug_b,
created_at,
updated_at,
archived_at
FROM agent_threads
WHERE plan_dir_rel = sqlc.arg ('plan_dir_rel')
AND archived_at IS NULL
ORDER BY updated_at DESC ;

-- name: GetMostRecentAgentThreadByPlanDirRel :one
-- Plan-home most-recent chat via idx_agent_threads_plan_updated.
SELECT
id,
user_email,
title,
cwd,
lineage_id,
project_id,
plan_dir_rel,
head_entry_id,
parent_thread_id,
forked_from_entry_id,
pi_session_id,
agent_slug,
room_kind,
pair_agent_slug_a,
pair_agent_slug_b,
created_at,
updated_at,
archived_at
FROM agent_threads
WHERE plan_dir_rel = sqlc.arg ('plan_dir_rel')
AND archived_at IS NULL
ORDER BY updated_at DESC
LIMIT 1 ;

-- name: ListAgentThreadsForPlanDirBackfill :many
SELECT
t.id,
t.cwd,
t.plan_dir_rel,
s.plan_dir AS session_plan_dir
FROM agent_threads t
LEFT JOIN agent_sessions s
ON s.projected_thread_id = t.id
AND s.identity_kind = 'plan_owned'
AND s.plan_dir IS NOT NULL
WHERE t.archived_at IS NULL
AND t.plan_dir_rel IS NULL ;

-- name: SetAgentThreadPlanDirRel :exec
-- Backfill helper: do not bump updated_at (preserve most-recent ordering).
UPDATE agent_threads
SET plan_dir_rel = sqlc.arg ('plan_dir_rel')
WHERE id = sqlc.arg ('id')
AND plan_dir_rel IS NULL ;

-- name: ListAgentThreadsByAgentSlug :many
-- Top-level live bot-scoped conversations (room_kind is a scope tag).
SELECT
id,
user_email,
title,
cwd,
lineage_id,
project_id,
plan_dir_rel,
head_entry_id,
parent_thread_id,
forked_from_entry_id,
pi_session_id,
agent_slug,
room_kind,
pair_agent_slug_a,
pair_agent_slug_b,
created_at,
updated_at,
archived_at
FROM agent_threads
WHERE agent_slug = sqlc.arg ('agent_slug')
AND room_kind = 'bot_home'
AND archived_at IS NULL
AND parent_thread_id IS NULL
ORDER BY updated_at DESC ;

-- name: GetBotHomeThreadBySlug :one
SELECT
id,
user_email,
title,
cwd,
lineage_id,
project_id,
plan_dir_rel,
head_entry_id,
parent_thread_id,
forked_from_entry_id,
pi_session_id,
agent_slug,
room_kind,
pair_agent_slug_a,
pair_agent_slug_b,
created_at,
updated_at,
archived_at
FROM agent_threads
WHERE agent_slug = sqlc.arg ('agent_slug')
AND room_kind = 'bot_home'
AND archived_at IS NULL
LIMIT 1 ;

-- name: BindAgentThreadBotHome :exec
UPDATE agent_threads
SET agent_slug = sqlc.arg ('agent_slug'),
room_kind = 'bot_home',
cwd = sqlc.arg ('cwd'),
title = sqlc.arg ('title'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: BindAgentThreadPlan :exec
UPDATE agent_threads
SET agent_slug = sqlc.narg ('agent_slug'),
room_kind = 'plan',
cwd = sqlc.arg ('cwd'),
title = sqlc.arg ('title'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: BindAgentThreadPairwise :exec
UPDATE agent_threads
SET pair_agent_slug_a = sqlc.arg ('pair_agent_slug_a'),
pair_agent_slug_b = sqlc.arg ('pair_agent_slug_b'),
room_kind = 'pairwise',
cwd = sqlc.arg ('cwd'),
title = sqlc.arg ('title'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: GetPairwiseThread :one
SELECT
id,
user_email,
title,
cwd,
lineage_id,
project_id,
plan_dir_rel,
head_entry_id,
parent_thread_id,
forked_from_entry_id,
pi_session_id,
agent_slug,
room_kind,
pair_agent_slug_a,
pair_agent_slug_b,
created_at,
updated_at,
archived_at
FROM agent_threads
WHERE pair_agent_slug_a = sqlc.arg ('pair_agent_slug_a')
AND pair_agent_slug_b = sqlc.arg ('pair_agent_slug_b')
AND room_kind = 'pairwise'
AND archived_at IS NULL
LIMIT 1 ;

-- name: ListAgentThreadsByParentThreadID :many
SELECT
id,
user_email,
title,
cwd,
lineage_id,
project_id,
plan_dir_rel,
head_entry_id,
parent_thread_id,
forked_from_entry_id,
pi_session_id,
agent_slug,
room_kind,
pair_agent_slug_a,
pair_agent_slug_b,
created_at,
updated_at,
archived_at
FROM agent_threads
WHERE parent_thread_id = sqlc.arg ('parent_thread_id')
AND archived_at IS NULL
ORDER BY created_at ASC ;

-- name: SetAgentThreadRoomKind :exec
UPDATE agent_threads
SET room_kind = sqlc.arg ('room_kind'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id') ;

-- name: SetAgentThreadPiSessionID :exec
UPDATE agent_threads
SET pi_session_id = sqlc.arg ('pi_session_id'),
updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg ('id')
AND pi_session_id = '' ;
