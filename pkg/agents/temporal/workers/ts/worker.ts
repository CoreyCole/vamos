import { NativeConnection, Worker } from "@temporalio/worker";
import { mkdir, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { dirname } from "node:path";
import { RunConversationTurn } from "./activities.js";

function taskQueue(): string {
  return process.env.VAMOS_TS_WORKER_TASK_QUEUE || "agents-ts";
}

export async function writeReadyMarker(): Promise<void> {
  const marker = process.env.VAMOS_TS_WORKER_READY_FILE;
  if (!marker) return;
  await mkdir(dirname(marker), { recursive: true });
  await writeFile(
    marker,
    JSON.stringify(
      {
        version: 1,
        pid: process.pid,
        started_at: new Date().toISOString(),
        workspace_slug: process.env.VAMOS_WORKSPACE_SLUG || "",
        checkout_path: process.env.VAMOS_DEFAULT_CWD || "",
        temporal_address: process.env.TEMPORAL_ADDR || "localhost:7233",
        task_queue: taskQueue(),
        ready_marker: marker,
      },
      null,
      2,
    ) + "\n",
    "utf8",
  );
}

async function run(): Promise<void> {
  const address = process.env.TEMPORAL_ADDR || "localhost:7233";
  console.log(`[ts-worker] Connecting to Temporal at ${address}`);

  const connection = await NativeConnection.connect({ address });

  const queue = taskQueue();
  const worker = await Worker.create({
    connection,
    taskQueue: queue,
    activities: { RunConversationTurn },
  });

  await writeReadyMarker();
  console.log(`[ts-worker] Started, polling task queue: ${queue}`);
  await worker.run();
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  run().catch((err) => {
    console.error("[ts-worker] Fatal error:", err);
    process.exit(1);
  });
}
