import { NativeConnection, Worker } from '@temporalio/worker';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname } from 'node:path';
import { RunConversationTurn } from './activities.js';

export async function writeReadyMarker(): Promise<void> {
	const marker = process.env.VAMOS_TS_WORKER_READY_FILE;
	if (!marker) return;
	await mkdir(dirname(marker), { recursive: true });
	await writeFile(
		marker,
		`${process.pid}\n${new Date().toISOString()}\n`,
		'utf8',
	);
}

async function run(): Promise<void> {
	const address = process.env.TEMPORAL_ADDR || 'localhost:7233';
	console.log(`[ts-worker] Connecting to Temporal at ${address}`);

	const connection = await NativeConnection.connect({ address });

	const worker = await Worker.create({
		connection,
		taskQueue: 'agents-ts',
		activities: { RunConversationTurn },
	});

	await writeReadyMarker();
	console.log('[ts-worker] Started, polling task queue: agents-ts');
	await worker.run();
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
	run().catch((err) => {
		console.error('[ts-worker] Fatal error:', err);
		process.exit(1);
	});
}
