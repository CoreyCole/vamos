-- Sessions table for authenticated users
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_email TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    last_accessed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Index for session lookup by ID
CREATE INDEX IF NOT EXISTS idx_sessions_id ON sessions (id);

-- Index for cleanup of expired sessions
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions (expires_at);

-- Auth attempts table for audit logging
CREATE TABLE IF NOT EXISTS auth_attempts (
id INTEGER PRIMARY KEY AUTOINCREMENT,
email TEXT NOT NULL,
success BOOLEAN NOT NULL,
error_message TEXT,
attempted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ;

-- Index for querying auth attempts by email
CREATE INDEX IF NOT EXISTS idx_auth_attempts_email ON auth_attempts (email) ;

-- Chat threads table
CREATE TABLE IF NOT EXISTS chat_threads (
id TEXT PRIMARY KEY,
user_email TEXT NOT NULL,
title TEXT NOT NULL DEFAULT 'New Chat',
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
archived_at DATETIME
) ;

-- Index for listing user's threads (excludes archived)
CREATE INDEX IF NOT EXISTS idx_chat_threads_user ON chat_threads (user_email,
updated_at DESC) WHERE archived_at IS NULL ;

-- Chat messages table
CREATE TABLE IF NOT EXISTS chat_messages (
id TEXT PRIMARY KEY,
thread_id TEXT NOT NULL,
role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
content TEXT NOT NULL,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
FOREIGN KEY (thread_id) REFERENCES chat_threads (id)
) ;

-- Index for fetching messages by thread
CREATE INDEX IF NOT EXISTS idx_chat_messages_thread ON chat_messages (thread_id,
created_at ASC) ;

-- User preferences table for theme settings (keyed by user_email)
CREATE TABLE IF NOT EXISTS user_preferences (
user_email TEXT PRIMARY KEY,
theme TEXT NOT NULL DEFAULT 'dark',
syntax_theme TEXT NOT NULL DEFAULT 'github-dark',
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ;

CREATE TABLE IF NOT EXISTS layout_preferences (
user_email TEXT NOT NULL,
page TEXT NOT NULL CHECK (page IN ('agent-chat', 'thoughts')),
view TEXT NOT NULL CHECK (view IN ('focus', 'split')),
viewport_class TEXT NOT NULL DEFAULT 'desktop-full'
CHECK (viewport_class IN ('mobile', 'desktop-half', 'desktop-full')),
config_json TEXT NOT NULL,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
PRIMARY KEY (user_email, page, view, viewport_class)
) ;

CREATE INDEX IF NOT EXISTS idx_layout_preferences_user
ON layout_preferences (user_email, page, view, viewport_class) ;

CREATE TABLE IF NOT EXISTS user_chat_selections (
user_email TEXT NOT NULL,
scope TEXT NOT NULL DEFAULT 'global' CHECK (scope IN ('global',
'freeform',
'workspace')),
scope_id TEXT NOT NULL DEFAULT '',
workspace_id TEXT NOT NULL,
thread_id TEXT,
run_id TEXT,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
PRIMARY KEY (user_email, scope, scope_id)
) ;

CREATE INDEX IF NOT EXISTS idx_user_chat_selections_user_updated
ON user_chat_selections (user_email, updated_at DESC) ;

-- ============================================================
-- System Health Metrics History
-- ============================================================

-- Periodic snapshots of system metrics (captured every 2 minutes)
CREATE TABLE IF NOT EXISTS system_snapshots (
id INTEGER PRIMARY KEY AUTOINCREMENT,
boot_id TEXT NOT NULL,
captured_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
cpu_percent REAL NOT NULL DEFAULT 0,
mem_used_bytes INTEGER NOT NULL DEFAULT 0,
mem_total_bytes INTEGER NOT NULL DEFAULT 0,
mem_used_percent REAL NOT NULL DEFAULT 0,
swap_used_bytes INTEGER NOT NULL DEFAULT 0,
swap_total_bytes INTEGER NOT NULL DEFAULT 0,
disk_used_bytes INTEGER NOT NULL DEFAULT 0,
disk_total_bytes INTEGER NOT NULL DEFAULT 0,
load_avg_1 REAL NOT NULL DEFAULT 0,
load_avg_5 REAL NOT NULL DEFAULT 0,
load_avg_15 REAL NOT NULL DEFAULT 0
) ;

CREATE INDEX IF NOT EXISTS idx_system_snapshots_boot_id ON system_snapshots (boot_id) ;
CREATE INDEX IF NOT EXISTS idx_system_snapshots_captured_at ON system_snapshots (captured_at) ;

-- Top processes by memory at each snapshot
CREATE TABLE IF NOT EXISTS system_snapshot_processes (
id INTEGER PRIMARY KEY AUTOINCREMENT,
snapshot_id INTEGER NOT NULL REFERENCES system_snapshots (id),
pid INTEGER NOT NULL,
user TEXT NOT NULL,
mem_mb REAL NOT NULL DEFAULT 0,
cpu_percent REAL NOT NULL DEFAULT 0,
command TEXT NOT NULL
) ;

CREATE INDEX IF NOT EXISTS idx_snapshot_processes_snapshot_id ON system_snapshot_processes (snapshot_id) ;

CREATE TABLE IF NOT EXISTS workspaces (
id TEXT PRIMARY KEY,
user_email TEXT NOT NULL,
title TEXT NOT NULL DEFAULT 'New Workspace',
root_doc_path TEXT NOT NULL,
cwd TEXT,
-- Generic workflow runtime accepts host/library workflow IDs beyond the
-- initial freeform/QRSPI pair. Existing SQLite DBs with the old CHECK need
-- a table rebuild by a future migration runner; schema.sql is bootstrap-only.
workflow_type TEXT NOT NULL DEFAULT 'freeform',
workflow_state_json TEXT,
source TEXT NOT NULL DEFAULT 'web'
CHECK (source IN ('web', 'terminal', 'imported')),
selected_thread_id TEXT,
selected_doc_path TEXT,
current_session_id TEXT REFERENCES chat_sessions (id),
current_branch_id TEXT,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
archived_at DATETIME
) ;

CREATE INDEX IF NOT EXISTS idx_workspaces_user_updated
ON workspaces (user_email, updated_at DESC)
WHERE archived_at IS NULL ;

CREATE INDEX IF NOT EXISTS idx_workspaces_root_doc_path
ON workspaces (root_doc_path)
WHERE archived_at IS NULL ;

CREATE TABLE IF NOT EXISTS plan_workspaces (
plan_dir_rel TEXT PRIMARY KEY,
project_id TEXT NOT NULL DEFAULT '',
plan_dir TEXT NOT NULL,
label TEXT NOT NULL,
workspace_slug TEXT NOT NULL DEFAULT '',
impl_workspace_path TEXT,
impl_workspace_url TEXT,
impl_workspace_discovered_at DATETIME,
artifact_updated_at DATETIME NOT NULL,
qrspi_lifecycle TEXT NOT NULL DEFAULT 'question'
CHECK (qrspi_lifecycle IN ('question',
'research',
'design',
'outline',
'review_outline',
'plan',
'review_plan',
'workspace',
'implement',
'review_implementation',
'verify',
'merged', 'closed')),
qrspi_lifecycle_updated_at DATETIME,
qrspi_closed_reason TEXT NOT NULL DEFAULT '',
discovered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
last_discovered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
archived_at DATETIME
) ;

CREATE INDEX IF NOT EXISTS idx_plan_workspaces_active_activity
ON plan_workspaces (artifact_updated_at DESC, plan_dir_rel)
WHERE archived_at IS NULL ;

CREATE INDEX IF NOT EXISTS idx_plan_workspaces_project_active_activity
ON plan_workspaces (project_id, artifact_updated_at DESC, plan_dir_rel)
WHERE archived_at IS NULL ;

CREATE INDEX IF NOT EXISTS idx_plan_workspaces_lifecycle_activity
ON plan_workspaces (qrspi_lifecycle, artifact_updated_at DESC, plan_dir_rel)
WHERE archived_at IS NULL ;

CREATE INDEX IF NOT EXISTS idx_plan_workspaces_project_lifecycle_activity
ON plan_workspaces (project_id, qrspi_lifecycle, artifact_updated_at DESC, plan_dir_rel)
WHERE archived_at IS NULL ;

CREATE UNIQUE INDEX IF NOT EXISTS idx_plan_workspaces_active_slug
ON plan_workspaces (workspace_slug)
WHERE archived_at IS NULL AND workspace_slug <> '' ;

CREATE TABLE IF NOT EXISTS impl_workspaces (
workspace_slug TEXT PRIMARY KEY,
checkout_path TEXT NOT NULL,
display_name TEXT NOT NULL,
host TEXT NOT NULL DEFAULT '',
url TEXT NOT NULL DEFAULT '',
plan_dir_rel TEXT REFERENCES plan_workspaces (plan_dir_rel),
plan_dir TEXT,
status TEXT NOT NULL DEFAULT 'active'
CHECK (status IN ('active', 'cleaned_up', 'merged')),
branch TEXT,
commit_hash TEXT,
trunk_branch TEXT,
top_branch TEXT,
bottom_branch TEXT,
bottom_parent_branch TEXT,
base_branch TEXT,
ahead_count INTEGER NOT NULL DEFAULT 0,
behind_count INTEGER NOT NULL DEFAULT 0,
merged_at DATETIME,
cleaned_up_at DATETIME,
merge_evidence TEXT,
cleanup_proof_kind TEXT NOT NULL DEFAULT 'unknown'
CHECK (cleanup_proof_kind IN ('ancestor', 'patch_equivalent', 'cached', 'unknown')),
cleanup_proof_source_ref TEXT,
cleanup_proof_target_commit TEXT,
cleanup_proof_at DATETIME,
cleanup_risk_reason TEXT,
env_last_repaired_at DATETIME,
env_last_error TEXT,
git_detail TEXT,
discovered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
last_discovered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ;

CREATE INDEX IF NOT EXISTS idx_impl_workspaces_status_updated
ON impl_workspaces (status, updated_at DESC) ;

CREATE INDEX IF NOT EXISTS idx_impl_workspaces_plan_dir_rel
ON impl_workspaces (plan_dir_rel)
WHERE plan_dir_rel IS NOT NULL ;

CREATE UNIQUE INDEX IF NOT EXISTS idx_impl_workspaces_checkout_path
ON impl_workspaces (checkout_path) ;

CREATE TABLE IF NOT EXISTS release_queue_items (
id TEXT PRIMARY KEY,
definition_id TEXT NOT NULL DEFAULT 'default',
definition_version TEXT NOT NULL DEFAULT 'v1',
workflow_id TEXT NOT NULL,
workflow_version TEXT NOT NULL DEFAULT 'v1',
flow_id TEXT NOT NULL,
source_slug TEXT NOT NULL DEFAULT '',
target_lane TEXT NOT NULL DEFAULT '',
expected_source_commit TEXT NOT NULL DEFAULT '',
expected_target_commit TEXT NOT NULL DEFAULT '',
status TEXT NOT NULL CHECK (status IN ('pending',
'running',
'succeeded',
'failed',
'canceled')),
current_node_id TEXT NOT NULL DEFAULT '',
actor_email TEXT NOT NULL DEFAULT '',
error_message TEXT NOT NULL DEFAULT '',
payload_json TEXT NOT NULL DEFAULT '{}',
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
started_at DATETIME,
finished_at DATETIME,
updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ;

CREATE INDEX IF NOT EXISTS idx_release_queue_items_active
ON release_queue_items (status, created_at ASC)
WHERE status IN ('pending', 'running') ;

CREATE INDEX IF NOT EXISTS idx_release_queue_items_history
ON release_queue_items (finished_at DESC, created_at DESC)
WHERE status IN ('succeeded', 'failed', 'canceled') ;

CREATE TABLE IF NOT EXISTS release_queue_events (
id INTEGER PRIMARY KEY AUTOINCREMENT,
item_id TEXT NOT NULL REFERENCES release_queue_items (id) ON DELETE CASCADE,
level TEXT NOT NULL CHECK (level IN ('debug', 'info', 'warn', 'error')),
node_id TEXT NOT NULL DEFAULT '',
message TEXT NOT NULL,
payload_json TEXT NOT NULL DEFAULT '{}',
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ;

CREATE INDEX IF NOT EXISTS idx_release_queue_events_item_created
ON release_queue_events (item_id, created_at ASC, id ASC) ;

CREATE TABLE IF NOT EXISTS workspace_error_events (
id INTEGER PRIMARY KEY AUTOINCREMENT,
workspace_slug TEXT NOT NULL,
source TEXT NOT NULL CHECK (source IN ('switch', 'manager', 'log')),
severity TEXT NOT NULL CHECK (severity IN ('warn', 'error')),
message TEXT NOT NULL,
detail TEXT NOT NULL DEFAULT '',
dedupe_key TEXT NOT NULL,
occurrence_count INTEGER NOT NULL DEFAULT 1,
payload_json TEXT NOT NULL DEFAULT '{}',
first_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ;

CREATE UNIQUE INDEX IF NOT EXISTS idx_workspace_error_events_dedupe
ON workspace_error_events (dedupe_key) ;

CREATE INDEX IF NOT EXISTS idx_workspace_error_events_recent
ON workspace_error_events (last_seen_at DESC, id DESC) ;

CREATE INDEX IF NOT EXISTS idx_workspace_error_events_workspace_recent
ON workspace_error_events (workspace_slug, last_seen_at DESC, id DESC) ;

CREATE TABLE IF NOT EXISTS agent_threads (
id TEXT PRIMARY KEY,
user_email TEXT NOT NULL,
title TEXT NOT NULL DEFAULT 'New Chat',
cwd TEXT NOT NULL,
lineage_id TEXT NOT NULL,
head_entry_id TEXT,
parent_thread_id TEXT REFERENCES agent_threads (id),
forked_from_entry_id TEXT,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
archived_at DATETIME
) ;

CREATE INDEX IF NOT EXISTS idx_agent_threads_user_updated
ON agent_threads (user_email, updated_at DESC)
WHERE archived_at IS NULL ;

CREATE TABLE IF NOT EXISTS agent_thread_workspaces (
thread_id TEXT NOT NULL REFERENCES agent_threads (id) ON DELETE CASCADE,
workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
is_primary INTEGER NOT NULL DEFAULT 0,
role TEXT NOT NULL DEFAULT 'related'
CHECK (role IN ('primary', 'related')),
adopted_from TEXT NOT NULL DEFAULT '',
adopted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
PRIMARY KEY (thread_id, workspace_id)
) ;

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_thread_workspaces_primary
ON agent_thread_workspaces (thread_id)
WHERE is_primary = 1 ;

CREATE INDEX IF NOT EXISTS idx_agent_thread_workspaces_workspace
ON agent_thread_workspaces (workspace_id, thread_id) ;

CREATE TABLE IF NOT EXISTS agent_sessions (
id TEXT PRIMARY KEY,
workspace_id TEXT REFERENCES workspaces (id),
thread_id TEXT REFERENCES agent_threads (id),
user_email TEXT,
source TEXT NOT NULL CHECK (source IN ('terminal', 'web', 'adopted')),
session_path TEXT,
session_id TEXT,
parent_session_id TEXT,
cwd TEXT,
status TEXT NOT NULL DEFAULT 'pending'
CHECK (status IN ('pending',
'importing',
'imported',
'unassigned',
'ambiguous',
'diverged',
'failed')),
inferred_workspace_id TEXT,
inferred_plan_dir TEXT,
imported_head_entry_id TEXT,
last_imported_at DATETIME,
last_error TEXT,
metadata_json TEXT,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ;

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_sessions_path
ON agent_sessions (session_path)
WHERE session_path IS NOT NULL ;

CREATE INDEX IF NOT EXISTS idx_agent_sessions_user_plan_updated
ON agent_sessions (user_email, updated_at DESC)
WHERE inferred_plan_dir IS NOT NULL ;

CREATE INDEX IF NOT EXISTS idx_agent_sessions_user_plan_dir_updated
ON agent_sessions (user_email, inferred_plan_dir, updated_at DESC)
WHERE inferred_plan_dir IS NOT NULL ;

CREATE INDEX IF NOT EXISTS idx_agent_sessions_user_workspace_plan_dir_updated
ON agent_sessions (user_email, workspace_id, inferred_plan_dir, updated_at DESC)
WHERE inferred_plan_dir IS NOT NULL ;

CREATE TABLE IF NOT EXISTS agent_entries (
lineage_id TEXT NOT NULL,
entry_id TEXT NOT NULL,
parent_entry_id TEXT,
entry_type TEXT NOT NULL,
origin_order INTEGER NOT NULL,
payload_json TEXT NOT NULL,
origin_thread_id TEXT NOT NULL REFERENCES agent_threads (id),
origin_run_id TEXT,
origin_session_id TEXT REFERENCES agent_sessions (id),
session_timestamp DATETIME NOT NULL,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
PRIMARY KEY (lineage_id, entry_id)
) ;

CREATE INDEX IF NOT EXISTS idx_agent_entries_parent
ON agent_entries (lineage_id, parent_entry_id) ;

CREATE INDEX IF NOT EXISTS idx_agent_entries_origin_run
ON agent_entries (origin_run_id)
WHERE origin_run_id IS NOT NULL ;

CREATE TABLE IF NOT EXISTS agent_runs (
id TEXT PRIMARY KEY,
workspace_id TEXT REFERENCES workspaces (id),
thread_id TEXT NOT NULL REFERENCES agent_threads (id),
session_id TEXT REFERENCES agent_sessions (id),
trigger TEXT NOT NULL CHECK (trigger IN ('send', 'resume', 'fork')),
status TEXT NOT NULL CHECK (status IN ('pending',
'running',
'complete',
'failed')),
prompt_text TEXT NOT NULL,
restore_head_entry_id TEXT,
result_head_entry_id TEXT,
workflow_id TEXT NOT NULL,
temporal_run_id TEXT,
workflow_node_id TEXT,
workflow_attempt INTEGER NOT NULL DEFAULT 0,
workflow_result_status TEXT,
workflow_result_json TEXT,
root_doc_path TEXT NOT NULL,
error_message TEXT,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
completed_at DATETIME
) ;

CREATE INDEX IF NOT EXISTS idx_agent_runs_thread_created
ON agent_runs (thread_id, created_at DESC) ;

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_runs_thread_running
ON agent_runs (thread_id)
WHERE status = 'running' ;

CREATE INDEX IF NOT EXISTS idx_agent_runs_workspace_node_created
ON agent_runs (workspace_id, workflow_node_id, created_at DESC)
WHERE workflow_node_id IS NOT NULL ;

CREATE TABLE IF NOT EXISTS agent_run_attachments (
id TEXT PRIMARY KEY,
run_id TEXT NOT NULL REFERENCES agent_runs (id) ON DELETE CASCADE,
thread_id TEXT NOT NULL REFERENCES agent_threads (id) ON DELETE CASCADE,
path TEXT NOT NULL,
basename TEXT NOT NULL,
position INTEGER NOT NULL DEFAULT 0,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ;

CREATE INDEX IF NOT EXISTS idx_agent_run_attachments_run
ON agent_run_attachments (run_id, position ASC) ;

CREATE INDEX IF NOT EXISTS idx_agent_run_attachments_thread
ON agent_run_attachments (thread_id, created_at ASC, position ASC) ;

CREATE TABLE IF NOT EXISTS chat_sessions (
id TEXT PRIMARY KEY,
workspace_id TEXT NOT NULL REFERENCES workspaces (id),
created_by_user_email TEXT NOT NULL,
parent_session_id TEXT REFERENCES chat_sessions (id),
forked_from_seq INTEGER,
branch_id TEXT NOT NULL,
workflow_id TEXT,
workflow_node_id TEXT,
workflow_attempt INTEGER NOT NULL DEFAULT 0,
topology_kind TEXT NOT NULL CHECK (topology_kind IN ('root', 'fork')),
current_projection_seq INTEGER NOT NULL DEFAULT 0,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
archived_at DATETIME
) ;

CREATE INDEX IF NOT EXISTS idx_chat_sessions_workspace_updated
ON chat_sessions (workspace_id, updated_at DESC)
WHERE archived_at IS NULL ;

CREATE TABLE IF NOT EXISTS chat_session_commands (
id TEXT PRIMARY KEY,
session_id TEXT NOT NULL REFERENCES chat_sessions (id),
idempotency_key TEXT NOT NULL,
command_type TEXT NOT NULL,
status TEXT NOT NULL CHECK (status IN ('submitted',
'accepted',
'rejected',
'applied',
'failed')),
actor_email TEXT NOT NULL,
payload_json TEXT NOT NULL,
outcome_json TEXT,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
UNIQUE (session_id, idempotency_key)
) ;

CREATE TABLE IF NOT EXISTS chat_session_sequences (
session_id TEXT PRIMARY KEY REFERENCES chat_sessions (id),
next_seq INTEGER NOT NULL DEFAULT 1
) ;

CREATE TABLE IF NOT EXISTS chat_session_events (
session_id TEXT NOT NULL REFERENCES chat_sessions (id),
seq INTEGER NOT NULL,
event_type TEXT NOT NULL,
actor_participant_id TEXT,
command_id TEXT REFERENCES chat_session_commands (id),
run_id TEXT REFERENCES agent_runs (id),
payload_json TEXT NOT NULL,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
PRIMARY KEY (session_id, seq)
) ;

CREATE INDEX IF NOT EXISTS idx_chat_events_session_seq
ON chat_session_events (session_id, seq) ;

CREATE TABLE IF NOT EXISTS chat_session_projections (
session_id TEXT PRIMARY KEY REFERENCES chat_sessions (id),
last_seq INTEGER NOT NULL,
messages_json TEXT NOT NULL,
runs_json TEXT NOT NULL,
participants_json TEXT NOT NULL,
artifacts_json TEXT NOT NULL,
topology_json TEXT NOT NULL,
updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ;

CREATE TABLE IF NOT EXISTS chat_session_baselines (
session_id TEXT PRIMARY KEY REFERENCES chat_sessions (id),
parent_session_id TEXT NOT NULL REFERENCES chat_sessions (id),
forked_from_seq INTEGER NOT NULL,
baseline_projection_version INTEGER NOT NULL DEFAULT 1,
messages_json TEXT NOT NULL,
runs_json TEXT NOT NULL,
artifacts_json TEXT NOT NULL,
participants_json TEXT NOT NULL,
topology_json TEXT NOT NULL,
selected_state_json TEXT NOT NULL,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ;

CREATE TABLE IF NOT EXISTS chat_annotations (
id TEXT PRIMARY KEY,
workspace_id TEXT NOT NULL REFERENCES workspaces (id),
session_id TEXT NOT NULL REFERENCES chat_sessions (id),
node_id TEXT NOT NULL,
event_seq INTEGER NOT NULL,
author_email TEXT NOT NULL,
body_markdown TEXT NOT NULL,
status TEXT NOT NULL CHECK (status IN ('open', 'resolved')),
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ;

CREATE INDEX IF NOT EXISTS idx_chat_annotations_anchor
ON chat_annotations (workspace_id, session_id, node_id, event_seq) ;

CREATE TABLE IF NOT EXISTS external_agent_sessions (
id TEXT PRIMARY KEY,
provider TEXT NOT NULL CHECK (provider IN ('pi')),
external_session_id TEXT,
transcript_path TEXT,
cwd TEXT,
model TEXT,
title TEXT,
discovered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
adopted_at DATETIME,
UNIQUE (provider, external_session_id)
) ;

CREATE TABLE IF NOT EXISTS chat_session_external_links (
id TEXT PRIMARY KEY,
chat_session_id TEXT NOT NULL REFERENCES chat_sessions (id),
external_agent_session_id TEXT NOT NULL REFERENCES external_agent_sessions (id),
link_mode TEXT NOT NULL CHECK (link_mode IN ('imported',
'observed',
'interactive',
'controlled',
'handoff')),
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
UNIQUE (chat_session_id, external_agent_session_id)
) ;

CREATE TABLE IF NOT EXISTS agent_surface_attachments (
id TEXT PRIMARY KEY,
chat_session_id TEXT NOT NULL REFERENCES chat_sessions (id),
run_id TEXT REFERENCES agent_runs (id),
surface_kind TEXT NOT NULL CHECK (surface_kind IN ('terminal',
'web',
'temporal_worker',
'api')),
surface_id TEXT,
user_email TEXT,
permission_mode TEXT NOT NULL CHECK (permission_mode IN ('observe',
'submit',
'control',
'own')),
owner_lease_expires_at DATETIME,
last_heartbeat_at DATETIME,
connected_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
disconnected_at DATETIME
) ;

CREATE INDEX IF NOT EXISTS idx_agent_surface_attachments_session
ON agent_surface_attachments (chat_session_id, run_id) ;

CREATE TABLE IF NOT EXISTS workspace_events (
id INTEGER PRIMARY KEY AUTOINCREMENT,
workspace_id TEXT NOT NULL REFERENCES workspaces (id),
event_type TEXT NOT NULL,
actor_email TEXT,
actor_type TEXT NOT NULL DEFAULT 'system'
CHECK (actor_type IN ('user', 'agent', 'system')),
thread_id TEXT REFERENCES agent_threads (id),
session_id TEXT REFERENCES agent_sessions (id),
run_id TEXT REFERENCES agent_runs (id),
doc_path TEXT,
comment_id TEXT,
payload_json TEXT,
event_key TEXT,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ;

CREATE INDEX IF NOT EXISTS idx_workspace_events_workspace_created
ON workspace_events (workspace_id, created_at ASC, id ASC) ;

CREATE UNIQUE INDEX IF NOT EXISTS idx_workspace_events_workspace_key
ON workspace_events (workspace_id, event_key)
WHERE event_key IS NOT NULL ;

CREATE TABLE IF NOT EXISTS workspace_docs (
workspace_id TEXT NOT NULL REFERENCES workspaces (id),
doc_path TEXT NOT NULL,
rel_path TEXT NOT NULL,
kind TEXT NOT NULL DEFAULT 'file' CHECK (kind IN ('file', 'dir')),
title TEXT NOT NULL DEFAULT '',
size_bytes INTEGER NOT NULL DEFAULT 0,
mtime_unix INTEGER NOT NULL DEFAULT 0,
content_hash TEXT,
deleted_at DATETIME,
updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
PRIMARY KEY (workspace_id, doc_path)
) ;

CREATE INDEX IF NOT EXISTS idx_workspace_docs_doc_path
ON workspace_docs (doc_path)
WHERE deleted_at IS NULL ;

CREATE INDEX IF NOT EXISTS idx_workspace_docs_workspace_rel_path
ON workspace_docs (workspace_id, rel_path)
WHERE deleted_at IS NULL ;

DROP TABLE IF EXISTS workspace_doc_comment_replies ;
DROP TABLE IF EXISTS workspace_doc_comments ;

CREATE TABLE IF NOT EXISTS document_comments (
id TEXT PRIMARY KEY,
workspace_root TEXT NOT NULL DEFAULT '',
workspace_id TEXT,
doc_path TEXT NOT NULL,
user_email TEXT NOT NULL,
comment_text TEXT NOT NULL,
selected_text TEXT NOT NULL,
section_hint TEXT,
heading_hint TEXT,
start_line INTEGER NOT NULL DEFAULT 0,
start_column INTEGER NOT NULL DEFAULT 0,
end_line INTEGER NOT NULL DEFAULT 0,
end_column INTEGER NOT NULL DEFAULT 0,
resolved BOOLEAN NOT NULL DEFAULT 0,
resolved_by TEXT,
resolved_actor_type TEXT,
resolved_at DATETIME,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at DATETIME,
deleted_at DATETIME
) ;

CREATE INDEX IF NOT EXISTS idx_document_comments_doc_active
ON document_comments (doc_path, resolved, created_at DESC)
WHERE deleted_at IS NULL ;

CREATE INDEX IF NOT EXISTS idx_document_comments_workspace_active
ON document_comments (workspace_root, resolved, doc_path, created_at DESC)
WHERE workspace_root != '' AND deleted_at IS NULL ;

CREATE TABLE IF NOT EXISTS document_comment_replies (
id TEXT PRIMARY KEY,
comment_id TEXT NOT NULL REFERENCES document_comments (id),
user_email TEXT NOT NULL,
actor_type TEXT NOT NULL DEFAULT 'user'
CHECK (actor_type IN ('user', 'agent', 'system')),
reply_text TEXT NOT NULL,
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at DATETIME,
deleted_at DATETIME
) ;

CREATE INDEX IF NOT EXISTS idx_document_comment_replies_comment
ON document_comment_replies (comment_id, created_at ASC)
WHERE deleted_at IS NULL ;
