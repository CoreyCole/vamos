import assert from 'node:assert/strict';
import test from 'node:test';
import {
	drainBestEffort,
	fetchWithTimeout,
	isDurableConversationEvent,
	loadSnapshot,
	postEnvelope,
	postEnvelopeWithRetry,
} from './conversation.js';
import type { ConversationRunInput, EventEnvelope } from './types.js';

const envelope: EventEnvelope = {
	workspace_id: 'w1',
	session_id: 's1',
	run_id: 'r1',
	thread_id: 't1',
	event_type: 'run_complete',
	payload_json: '{}',
	event_key: 'r1:run_complete',
};

function installFetch(fn: typeof fetch): () => void {
	const original = globalThis.fetch;
	globalThis.fetch = fn;
	return () => {
		globalThis.fetch = original;
	};
}

test('isDurableConversationEvent classifies lifecycle events only', () => {
	assert.equal(isDurableConversationEvent('checkpoint'), true);
	assert.equal(isDurableConversationEvent('run_complete'), true);
	assert.equal(isDurableConversationEvent('run_failed'), true);
	assert.equal(isDurableConversationEvent('message_update'), false);
});

test('fetchWithTimeout aborts a never-resolving fetch', async () => {
	let sawSignal = false;
	const restore = installFetch(
		(_input: RequestInfo | URL, init?: RequestInit) =>
			new Promise<Response>((_resolve, reject) => {
				const signal = init?.signal as AbortSignal | undefined;
				sawSignal = !!signal;
				signal?.addEventListener('abort', () => reject(signal.reason));
			}) as Promise<Response>,
	);
	try {
		await assert.rejects(
			fetchWithTimeout('http://localhost/hang', {
				method: 'GET',
				timeoutMS: 5,
				operation: 'test operation',
			}),
			/test operation timed out after 5ms/,
		);
		assert.equal(sawSignal, true);
	} finally {
		restore();
	}
});

test('drainBestEffort returns after timeout without awaiting the original promise', async () => {
	let timedOut = false;
	await drainBestEffort(new Promise(() => undefined), 5, () => {
		timedOut = true;
	});
	assert.equal(timedOut, true);
});

test('live post timeout is caught as a dropped post by caller seam', async () => {
	const restore = installFetch(
		(_input: RequestInfo | URL, init?: RequestInit) =>
			new Promise<Response>((_resolve, reject) => {
				const signal = init?.signal as AbortSignal | undefined;
				signal?.addEventListener('abort', () => reject(signal.reason));
			}) as Promise<Response>,
	);
	try {
		await assert.rejects(
			postEnvelope('http://localhost/internal/agent-chat/events', envelope, {
				timeoutMS: 5,
			}),
			/post envelope run_complete timed out after 5ms/,
		);
	} finally {
		restore();
	}
});

test('postEnvelope sends internal token header when configured', async () => {
	const previous = process.env.VAMOS_INTERNAL_TOKEN;
	process.env.VAMOS_INTERNAL_TOKEN = 'secret-token';
	let token = '';
	const restore = installFetch(
		async (_input: RequestInfo | URL, init?: RequestInit) => {
			const headers = new Headers(init?.headers);
			token = headers.get('X-Vamos-Internal-Token') ?? '';
			return new Response('', { status: 202 });
		},
	);
	try {
		await postEnvelope('http://localhost/internal/agent-chat/events', envelope);
		assert.equal(token, 'secret-token');
	} finally {
		restore();
		if (previous === undefined) {
			delete process.env.VAMOS_INTERNAL_TOKEN;
		} else {
			process.env.VAMOS_INTERNAL_TOKEN = previous;
		}
	}
});

test('loadSnapshot sends internal token header when configured', async () => {
	const previous = process.env.VAMOS_INTERNAL_TOKEN;
	process.env.VAMOS_INTERNAL_TOKEN = 'secret-token';
	let token = '';
	let runID = '';
	const restore = installFetch(
		async (input: RequestInfo | URL, init?: RequestInit) => {
			const url = new URL(input.toString());
			runID = url.searchParams.get('run_id') ?? '';
			const headers = new Headers(init?.headers);
			token = headers.get('X-Vamos-Internal-Token') ?? '';
			return Response.json({
				header: { session_id: 's1', cwd: '/tmp/project' },
				lineage_id: 'lineage-1',
				entries: [],
			});
		},
	);
	const input: ConversationRunInput = {
		workspace_id: 'w1',
		session_id: 's1',
		run_id: 'r1',
		thread_id: 't1',
		trigger: 'send',
		prompt: 'hello',
		cwd: '/tmp/project',
		artifact_root: '/tmp/project',
		thinking_level: '',
		callback_endpoint: 'http://localhost/internal/agent-chat/events',
		snapshot_loader_endpoint: 'http://localhost/internal/agent-chat/snapshots',
		snapshot_ref: { lineage_id: 'lineage-1' },
	};
	try {
		const snapshot = await loadSnapshot(input);
		assert.equal(token, 'secret-token');
		assert.equal(runID, 'r1');
		assert.equal(snapshot.lineage_id, 'lineage-1');
	} finally {
		restore();
		if (previous === undefined) {
			delete process.env.VAMOS_INTERNAL_TOKEN;
		} else {
			process.env.VAMOS_INTERNAL_TOKEN = previous;
		}
	}
});

test('postEnvelope throws on non-2xx with bounded response body', async () => {
	const restore = installFetch(
		async () =>
			new Response('x'.repeat(32), {
				status: 503,
				statusText: 'Service Unavailable',
			}),
	);
	try {
		await assert.rejects(
			postEnvelope('http://localhost/internal/agent-chat/events', envelope, {
				maxResponseBodyBytes: 8,
			}),
			/503 Service Unavailable: xxxxxxxx…/,
		);
	} finally {
		restore();
	}
});

test('postEnvelopeWithRetry retries transient durable post failures', async () => {
	let calls = 0;
	const restore = installFetch(async () => {
		calls++;
		if (calls < 3) {
			return new Response('nope', { status: 500, statusText: 'Boom' });
		}
		return new Response('', { status: 202 });
	});
	try {
		await postEnvelopeWithRetry(
			'http://localhost/internal/agent-chat/events',
			envelope,
			{
				attempts: 3,
				initialBackoffMS: 1,
			},
		);
		assert.equal(calls, 3);
	} finally {
		restore();
	}
});

test('postEnvelopeWithRetry advances after timed-out attempts', async () => {
	let calls = 0;
	const restore = installFetch(
		(_input: RequestInfo | URL, init?: RequestInit) => {
			calls++;
			if (calls < 3) {
				return new Promise<Response>((_resolve, reject) => {
					const signal = init?.signal as AbortSignal | undefined;
					signal?.addEventListener('abort', () => reject(signal.reason));
				}) as Promise<Response>;
			}
			return Promise.resolve(new Response('', { status: 202 }));
		},
	);
	try {
		await postEnvelopeWithRetry(
			'http://localhost/internal/agent-chat/events',
			envelope,
			{
				attempts: 3,
				initialBackoffMS: 1,
				timeoutMS: 5,
			},
		);
		assert.equal(calls, 3);
	} finally {
		restore();
	}
});

test('loadSnapshot passes an abort signal and timeout', async () => {
	let sawSignal = false;
	const restore = installFetch(
		async (_input: RequestInfo | URL, init?: RequestInit) => {
			sawSignal = !!init?.signal;
			return Response.json({
				header: { session_id: 's1', cwd: '/tmp/project' },
				lineage_id: 'lineage-1',
				entries: [],
			});
		},
	);
	const input: ConversationRunInput = {
		workspace_id: 'w1',
		session_id: 's1',
		run_id: 'r1',
		thread_id: 't1',
		trigger: 'send',
		prompt: 'hello',
		cwd: '/tmp/project',
		artifact_root: '/tmp/project',
		thinking_level: '',
		callback_endpoint: 'http://localhost/internal/agent-chat/events',
		snapshot_loader_endpoint: 'http://localhost/internal/agent-chat/snapshots',
		snapshot_ref: { lineage_id: 'lineage-1' },
	};
	try {
		await loadSnapshot(input, { timeoutMS: 5 });
		assert.equal(sawSignal, true);
	} finally {
		restore();
	}
});

test('postEnvelopeWithRetry stops after configured attempts', async () => {
	let calls = 0;
	const restore = installFetch(async () => {
		calls++;
		return new Response('still down', { status: 503, statusText: 'Down' });
	});
	try {
		await assert.rejects(
			postEnvelopeWithRetry(
				'http://localhost/internal/agent-chat/events',
				envelope,
				{
					attempts: 2,
					initialBackoffMS: 1,
				},
			),
			/still down/,
		);
		assert.equal(calls, 2);
	} finally {
		restore();
	}
});
