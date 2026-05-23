import { Context } from '@temporalio/activity';
import {
	createAgentSession,
	SessionManager,
	AuthStorage,
	ModelRegistry,
} from '@mariozechner/pi-coding-agent';
import type {
	ConversationRunFailure,
	ConversationRunInput,
	ConversationRunResult,
	EventEnvelope,
} from './types.js';
import {
	buildCheckpoint,
	buildRunResult,
	drainBestEffort,
	durableCallbackAttemptTimeoutMS,
	liveCallbackDrainTimeoutMS,
	liveCallbackTimeoutMS,
	loadSnapshot,
	materializeSnapshot,
	postEnvelope,
	postEnvelopeWithRetry,
	snapshotFetchTimeoutMS,
} from './conversation.js';

function logLiveCallbackDrop(error: unknown, envelope: EventEnvelope): void {
	const message = error instanceof Error ? error.message : String(error);
	console.warn(`dropped live callback ${envelope.event_type}: ${message}`);
}

export async function RunConversationTurn(
	input: ConversationRunInput,
): Promise<ConversationRunResult> {
	Context.current().heartbeat();
	const snapshot = await loadSnapshot(input, {
		timeoutMS: snapshotFetchTimeoutMS,
	});
	Context.current().heartbeat();
	const sessionManager = await materializeSnapshot(input, snapshot);
	const authStorage = AuthStorage.create(process.env.PI_AUTH_PATH || undefined);
	const modelRegistry = ModelRegistry.create(authStorage);
	const provider = process.env.PI_MODEL_PROVIDER || 'openai-codex';
	const modelId = process.env.PI_MODEL_ID || 'gpt-5.5';
	const model = modelRegistry.find(provider, modelId);

	const { session } = await createAgentSession({
		cwd: input.cwd,
		sessionManager,
		authStorage,
		modelRegistry,
		model: model ?? undefined,
		thinkingLevel: input.thinking_level as any,
	});
	session.setAutoCompactionEnabled(false);

	const metadata: Record<string, unknown> = {
		trigger: input.trigger,
		started_at: new Date().toISOString(),
		turns: 0,
	};

	let liveCallbackQueue = Promise.resolve();
	let durableCallbackQueue = Promise.resolve();
	const enqueueLiveCallback = (envelope: EventEnvelope) => {
		liveCallbackQueue = liveCallbackQueue
			.then(() =>
				postEnvelope(input.callback_endpoint, envelope, {
					attempts: 1,
					initialBackoffMS: 0,
					timeoutMS: liveCallbackTimeoutMS,
				}),
			)
			.catch((error) => logLiveCallbackDrop(error, envelope));
		return liveCallbackQueue;
	};
	const enqueueDurableCallback = (envelope: EventEnvelope) => {
		durableCallbackQueue = durableCallbackQueue.then(async () => {
			Context.current().heartbeat();
			await postEnvelopeWithRetry(input.callback_endpoint, envelope, {
				attempts: 8,
				initialBackoffMS: 500,
				timeoutMS: durableCallbackAttemptTimeoutMS,
			});
			Context.current().heartbeat();
		});
		return durableCallbackQueue;
	};

	const heartbeatInterval = setInterval(() => {
		Context.current().heartbeat();
	}, 30_000);

	const unsubscribe = session.subscribe((event: any) => {
		enqueueLiveCallback({
			workspace_id: input.workspace_id,
			session_id: input.session_id,
			run_id: input.run_id,
			thread_id: input.thread_id,
			event_type: event.type,
			payload_json: JSON.stringify(event),
			event_key: `${input.run_id}:live:${event.type}:${metadata.turns}:${event.sequence ?? ''}`,
		});

		if (event.type === 'turn_end') {
			metadata.turns = Number(metadata.turns) + 1;
			const checkpoint = buildCheckpoint(
				input,
				snapshot,
				sessionManager,
				event.turnIndex,
			);
			enqueueDurableCallback({
				workspace_id: input.workspace_id,
				session_id: input.session_id,
				run_id: input.run_id,
				thread_id: input.thread_id,
				event_type: 'checkpoint',
				payload_json: JSON.stringify(checkpoint),
				event_key: checkpoint.event_key,
			});
		}
	});

	let promptError: unknown;
	try {
		const prompt = input.context
			? `${input.context.trim()}\n\n---\n\n${input.prompt}`
			: input.prompt;
		await session.prompt(prompt);
	} catch (error) {
		promptError = error;
	}

	try {
		await durableCallbackQueue;

		if (promptError) {
			const failure: ConversationRunFailure = {
				workspace_id: input.workspace_id,
				session_id: input.session_id,
				run_id: input.run_id,
				thread_id: input.thread_id,
				head_entry_id: sessionManager.getLeafId() ?? undefined,
				session_path: sessionManager.getSessionFile() ?? '',
				artifact_root: input.artifact_root,
				error_message:
					promptError instanceof Error
						? promptError.message
						: String(promptError),
				event_key: `${input.run_id}:run_failed`,
			};

			await enqueueDurableCallback({
				workspace_id: input.workspace_id,
				session_id: input.session_id,
				run_id: input.run_id,
				thread_id: input.thread_id,
				event_type: 'run_failed',
				payload_json: JSON.stringify(failure),
				event_key: failure.event_key,
			});
			await durableCallbackQueue;
			throw promptError;
		}

		const result = buildRunResult(input, sessionManager, {
			...metadata,
			completed_at: new Date().toISOString(),
		});

		await enqueueDurableCallback({
			workspace_id: input.workspace_id,
			session_id: input.session_id,
			run_id: input.run_id,
			thread_id: input.thread_id,
			event_type: 'run_complete',
			payload_json: JSON.stringify(result),
			event_key: result.event_key,
		});
		await durableCallbackQueue;
		return result;
	} finally {
		await drainBestEffort(liveCallbackQueue, liveCallbackDrainTimeoutMS, () =>
			console.warn(
				`abandoning live callback drain after ${liveCallbackDrainTimeoutMS}ms`,
			),
		);
		clearInterval(heartbeatInterval);
		unsubscribe();
		session.dispose();
	}
}
