// Types shared between Go and the Agent Chat TS worker.

export interface ConversationSnapshotHeader {
	session_id: string;
	parent_session_id?: string;
	cwd: string;
}

export interface ConversationSnapshotEntry {
	lineage_id: string;
	entry_id: string;
	parent_entry_id?: string;
	entry_type: string;
	timestamp: string;
	origin_order: number;
	payload_json: string;
}

export interface ConversationSnapshot {
	header: ConversationSnapshotHeader;
	lineage_id: string;
	head_entry_id?: string;
	entries: ConversationSnapshotEntry[];
}

export interface ConversationSnapshotRef {
	lineage_id: string;
	head_entry_id?: string;
	session_path?: string;
}

export interface ConversationRunInput {
	workspace_id: string;
	session_id?: string;
	run_id: string;
	thread_id: string;
	trigger: 'send' | 'resume' | 'fork';
	prompt: string;
	context?: string;
	cwd: string;
	artifact_root: string;
	thinking_level: string;
	callback_endpoint: string;
	snapshot_loader_endpoint?: string;
	snapshot_ref: ConversationSnapshotRef;
}

export interface ConversationCheckpoint {
	workspace_id?: string;
	session_id?: string;
	run_id: string;
	thread_id: string;
	head_entry_id?: string;
	turn_index: number;
	header: ConversationSnapshotHeader;
	new_entries: ConversationSnapshotEntry[];
	event_key?: string;
}

export interface EventEnvelope {
	workspace_id?: string;
	session_id?: string;
	run_id: string;
	thread_id: string;
	event_type: string;
	payload_json: string;
	event_key?: string;
}

export interface ConversationRunResult {
	workspace_id?: string;
	session_id?: string;
	run_id: string;
	thread_id: string;
	head_entry_id?: string;
	session_path: string;
	artifact_root: string;
	metadata_json: string;
	event_key?: string;
}

export interface ConversationRunFailure {
	workspace_id?: string;
	session_id?: string;
	run_id: string;
	thread_id: string;
	head_entry_id?: string;
	session_path: string;
	artifact_root: string;
	error_message: string;
	event_key?: string;
}
