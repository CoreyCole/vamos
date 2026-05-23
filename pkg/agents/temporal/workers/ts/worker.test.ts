import assert from 'node:assert/strict';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { test } from 'node:test';
import { writeReadyMarker } from './worker.js';

test('writeReadyMarker writes pid and timestamp when configured', async () => {
	const dir = await mkdtemp(join(tmpdir(), 'agents-ts-worker-'));
	try {
		const marker = join(dir, 'nested', 'ts-worker.ready');
		process.env.VAMOS_TS_WORKER_READY_FILE = marker;

		await writeReadyMarker();

		const lines = (await readFile(marker, 'utf8')).trim().split('\n');
		assert.equal(lines[0], String(process.pid));
		assert.ok(
			!Number.isNaN(Date.parse(lines[1])),
			`timestamp ${lines[1]} should parse`,
		);
	} finally {
		delete process.env.VAMOS_TS_WORKER_READY_FILE;
		await rm(dir, { recursive: true, force: true });
	}
});

test('writeReadyMarker is a no-op without marker path', async () => {
	delete process.env.VAMOS_TS_WORKER_READY_FILE;
	await writeReadyMarker();
});
