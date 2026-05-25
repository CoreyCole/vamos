import assert from "node:assert/strict";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { test } from "node:test";
import { writeReadyMarker } from "./worker.js";

test("writeReadyMarker writes structured identity JSON when configured", async () => {
  const dir = await mkdtemp(join(tmpdir(), "agents-ts-worker-"));
  try {
    const marker = join(dir, "nested", "ts-worker.ready");
    process.env.VAMOS_TS_WORKER_READY_FILE = marker;
    process.env.VAMOS_WORKSPACE_SLUG = "foo";
    process.env.VAMOS_DEFAULT_CWD = "/tmp/checkout";
    process.env.TEMPORAL_ADDR = "127.0.0.1:7234";
    process.env.VAMOS_TS_WORKER_TASK_QUEUE = "agents-ts";

    await writeReadyMarker();

    const identity = JSON.parse(await readFile(marker, "utf8"));
    assert.equal(identity.version, 1);
    assert.equal(identity.pid, process.pid);
    assert.ok(
      !Number.isNaN(Date.parse(identity.started_at)),
      `timestamp ${identity.started_at} should parse`,
    );
    assert.equal(identity.workspace_slug, "foo");
    assert.equal(identity.checkout_path, "/tmp/checkout");
    assert.equal(identity.temporal_address, "127.0.0.1:7234");
    assert.equal(identity.task_queue, "agents-ts");
    assert.equal(identity.ready_marker, marker);
  } finally {
    delete process.env.VAMOS_TS_WORKER_READY_FILE;
    delete process.env.VAMOS_WORKSPACE_SLUG;
    delete process.env.VAMOS_DEFAULT_CWD;
    delete process.env.TEMPORAL_ADDR;
    delete process.env.VAMOS_TS_WORKER_TASK_QUEUE;
    await rm(dir, { recursive: true, force: true });
  }
});

test("writeReadyMarker is a no-op without marker path", async () => {
  delete process.env.VAMOS_TS_WORKER_READY_FILE;
  await writeReadyMarker();
});
