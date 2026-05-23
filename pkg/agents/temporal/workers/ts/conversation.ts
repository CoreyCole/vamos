import { mkdir, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { SessionManager } from '@mariozechner/pi-coding-agent';
import type {
	ConversationCheckpoint,
	ConversationRunInput,
	ConversationRunResult,
	ConversationSnapshot,
	EventEnvelope,
} from './types.js';

function internalHeaders(): Record<string, string> {
	const headers: Record<string, string> = {
		'Content-Type': 'application/json',
	};
	const token = process.env.VAMOS_INTERNAL_TOKEN;
	if (token) {
		headers['X-Vamos-Internal-Token'] = token;
	}
	return headers;
}

export const snapshotFetchTimeoutMS = 15_000;
export const durableCallbackAttemptTimeoutMS = 10_000;
export const liveCallbackTimeoutMS = 1_500;
export const liveCallbackDrainTimeoutMS = 2_000;

export interface TimeoutFetchOptions extends RequestInit {
	timeoutMS: number;
	operation: string;
}

export interface DurablePostOptions {
	attempts?: number;
	initialBackoffMS?: number;
	maxResponseBodyBytes?: number;
	timeoutMS?: number;
}

export interface SnapshotLoadOptions {
	timeoutMS?: number;
}

const defaultDurablePostOptions: Required<DurablePostOptions> = {
	attempts: 3,
	initialBackoffMS: 250,
	maxResponseBodyBytes: 4096,
	timeoutMS: durableCallbackAttemptTimeoutMS,
};

async function boundedResponseBody(
	response: Response,
	maxBytes: number,
): Promise<string> {
	try {
		const text = await response.text();
		if (text.length <= maxBytes) {
			return text;
		}
		return `${text.slice(0, maxBytes)}…`;
	} catch {
		return '';
	}
}

export async function fetchWithTimeout(
	input: RequestInfo | URL,
	opts: TimeoutFetchOptions,
): Promise<Response> {
	const { timeoutMS, operation, signal, ...requestInit } = opts;
	const controller = new AbortController();
	let timeout: NodeJS.Timeout | undefined;

	const abortFromParent = () => {
		controller.abort(signal?.reason ?? new Error(`${operation} aborted`));
	};
	if (signal?.aborted) {
		abortFromParent();
	} else if (signal) {
		signal.addEventListener('abort', abortFromParent, { once: true });
	}

	timeout = setTimeout(() => {
		controller.abort(new Error(`${operation} timed out after ${timeoutMS}ms`));
	}, timeoutMS);

	try {
		return await fetch(input, {
			...requestInit,
			signal: controller.signal,
		});
	} catch (error) {
		if (controller.signal.aborted) {
			const reason = controller.signal.reason;
			if (reason instanceof Error) {
				throw reason;
			}
			throw new Error(`${operation} aborted`);
		}
		throw error;
	} finally {
		if (timeout) {
			clearTimeout(timeout);
		}
		if (signal) {
			signal.removeEventListener('abort', abortFromParent);
		}
	}
}

export function isDurableConversationEvent(eventType: string): boolean {
	return (
		eventType === 'checkpoint' ||
		eventType === 'run_complete' ||
		eventType === 'run_failed'
	);
}

export async function postEnvelope(
	endpoint: string,
	envelope: EventEnvelope,
	opts: DurablePostOptions = {},
): Promise<void> {
	const maxResponseBodyBytes =
		opts.maxResponseBodyBytes ?? defaultDurablePostOptions.maxResponseBodyBytes;
	const timeoutMS = opts.timeoutMS ?? defaultDurablePostOptions.timeoutMS;
	const response = await fetchWithTimeout(endpoint, {
		method: 'POST',
		headers: internalHeaders(),
		body: JSON.stringify(envelope),
		timeoutMS,
		operation: `post envelope ${envelope.event_type}`,
	});
	if (!response.ok) {
		const body = await boundedResponseBody(response, maxResponseBodyBytes);
		const suffix = body ? `: ${body}` : '';
		throw new Error(
			`post envelope ${envelope.event_type} failed: ${response.status} ${response.statusText}${suffix}`,
		);
	}
}

async function sleep(ms: number): Promise<void> {
	await new Promise((resolve) => setTimeout(resolve, ms));
}

export async function drainBestEffort(
	promise: Promise<unknown>,
	timeoutMS: number,
	onTimeout: () => void,
): Promise<void> {
	let timeout: NodeJS.Timeout | undefined;
	let timedOut = false;
	try {
		await Promise.race([
			promise.catch(() => undefined),
			new Promise<void>((resolve) => {
				timeout = setTimeout(() => {
					timedOut = true;
					onTimeout();
					resolve();
				}, timeoutMS);
			}),
		]);
	} finally {
		if (!timedOut && timeout) {
			clearTimeout(timeout);
		}
	}
}

export async function postEnvelopeWithRetry(
	endpoint: string,
	envelope: EventEnvelope,
	opts: DurablePostOptions = {},
): Promise<void> {
	const options = { ...defaultDurablePostOptions, ...opts };
	let lastError: unknown;
	for (let attempt = 1; attempt <= options.attempts; attempt++) {
		try {
			await postEnvelope(endpoint, envelope, options);
			return;
		} catch (error) {
			lastError = error;
			if (attempt === options.attempts) {
				break;
			}
			await sleep(options.initialBackoffMS * attempt);
		}
	}
	throw lastError instanceof Error
		? lastError
		: new Error(`post envelope ${envelope.event_type} failed`);
}

export async function loadSnapshot(
	input: ConversationRunInput,
	opts: SnapshotLoadOptions = {},
): Promise<ConversationSnapshot> {
	if (!input.snapshot_loader_endpoint) {
		throw new Error('ConversationRunInput requires snapshot_loader_endpoint');
	}

	const url = new URL(input.snapshot_loader_endpoint);
	url.searchParams.set('run_id', input.run_id);
	const response = await fetchWithTimeout(url, {
		method: 'GET',
		headers: internalHeaders(),
		timeoutMS: opts.timeoutMS ?? snapshotFetchTimeoutMS,
		operation: 'load conversation snapshot',
	});
	if (!response.ok) {
		throw new Error(
			`load snapshot failed: ${response.status} ${response.statusText}`,
		);
	}
	return (await response.json()) as ConversationSnapshot;
}

export async function materializeSnapshot(
	input: ConversationRunInput,
	snapshot: ConversationSnapshot,
): Promise<SessionManager> {
	const sessionDir = join(tmpdir(), 'agent-chat-sessions', input.run_id);
	const sessionFile = join(sessionDir, 'session.jsonl');

	await mkdir(sessionDir, { recursive: true });

	const header = {
		type: 'session',
		version: 3,
		id: snapshot.header.session_id || input.session_id || input.run_id,
		timestamp: new Date().toISOString(),
		cwd: snapshot.header.cwd,
		...(snapshot.header.parent_session_id
			? { parentSession: snapshot.header.parent_session_id }
			: {}),
	};

	const lines = [
		JSON.stringify(header),
		...snapshot.entries.map((entry) => entry.payload_json),
	];
	await writeFile(sessionFile, lines.join('\n') + '\n', 'utf8');

	return SessionManager.open(sessionFile, dirname(sessionFile));
}

export function buildCheckpoint(
	input: ConversationRunInput,
	snapshot: ConversationSnapshot,
	sessionManager: SessionManager,
	turnIndex: number,
): ConversationCheckpoint {
	const existingIds = new Set(snapshot.entries.map((entry) => entry.entry_id));
	const nextOriginOrder =
		snapshot.entries.length === 0
			? 0
			: Math.max(...snapshot.entries.map((entry) => entry.origin_order)) + 1;

	const newEntries = sessionManager
		.getEntries()
		.filter((entry) => !existingIds.has(entry.id))
		.map((entry, index) => ({
			lineage_id: snapshot.lineage_id,
			entry_id: entry.id,
			parent_entry_id: entry.parentId ?? undefined,
			entry_type: entry.type,
			timestamp: entry.timestamp,
			origin_order: nextOriginOrder + index,
			payload_json: JSON.stringify(entry),
		}));

	const header = sessionManager.getHeader();

	return {
		workspace_id: input.workspace_id,
		session_id: input.session_id,
		run_id: input.run_id,
		thread_id: input.thread_id,
		head_entry_id: sessionManager.getLeafId() ?? undefined,
		turn_index: turnIndex,
		header: {
			session_id: header?.id ?? snapshot.header.session_id,
			parent_session_id: header?.parentSession,
			cwd: header?.cwd ?? input.cwd,
		},
		new_entries: newEntries,
		event_key: `${input.run_id}:checkpoint:${turnIndex}`,
	};
}

export function buildRunResult(
	input: ConversationRunInput,
	sessionManager: SessionManager,
	metadata: Record<string, unknown>,
): ConversationRunResult {
	return {
		workspace_id: input.workspace_id,
		session_id: input.session_id,
		run_id: input.run_id,
		thread_id: input.thread_id,
		head_entry_id: sessionManager.getLeafId() ?? undefined,
		session_path: sessionManager.getSessionFile() ?? '',
		artifact_root: input.artifact_root,
		metadata_json: JSON.stringify(metadata),
		event_key: `${input.run_id}:run_complete`,
	};
}
