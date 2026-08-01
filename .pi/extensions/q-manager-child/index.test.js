import assert from "node:assert/strict";
import { closeSync, openSync } from "node:fs";
import { mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { parseDocument } from "yaml";

import extension, {
  DIAGNOSTIC,
  deriveLaunchNonce,
  evidencePath,
  messageID,
} from "./index.js";

const managedKeys = [
  "HERMES_SESSION_ID",
  "PI_SESSION_ID",
  "VAMOS_PLAN_DIR",
  "VAMOS_HERMES_HANDOFF_FD",
  "VAMOS_MANAGER_WAKE_GATEWAY_URL",
  "VAMOS_MANAGER_WAKE_INGRESS_TOKEN",
];

function restore(values) {
  for (const [key, value] of Object.entries(values))
    value === undefined ? delete process.env[key] : (process.env[key] = value);
}

async function managed(run) {
  const old = Object.fromEntries(
    managedKeys.map((key) => [key, process.env[key]]),
  );
  const root = await mkdtemp(join(tmpdir(), "opaque-handoff-"));
  const handoffPath = join(root, "handoff.bin");
  const fd = openSync(handoffPath, "w+");
  Object.assign(process.env, {
    HERMES_SESSION_ID: "opaque-hermes-session",
    PI_SESSION_ID: "pi-session-test-v1",
    VAMOS_PLAN_DIR: join(root, "plan"),
    VAMOS_HERMES_HANDOFF_FD: String(fd),
    VAMOS_MANAGER_WAKE_GATEWAY_URL: "https://must-not-be-used.invalid",
    VAMOS_MANAGER_WAKE_INGRESS_TOKEN: "must-not-be-used",
  });
  try {
    await run({ handoffPath });
  } finally {
    closeSync(fd);
    restore(old);
  }
}

function harness(branch) {
  const handlers = new Map();
  const custom = [];
  const pi = {
    on: (name, handler) => handlers.set(name, handler),
    appendEntry: (type, data) => custom.push({ type, data }),
  };
  extension(pi);
  return {
    handlers,
    custom,
    ctx: { sessionManager: { getBranch: () => branch } },
  };
}

function decodeFrame(bytes) {
  const size = bytes.readUInt32BE(0);
  assert.equal(size, bytes.length - 4);
  return JSON.parse(bytes.subarray(4).toString("utf8"));
}

test("cross-language launch nonce vector is frozen", () => {
  assert.equal(
    deriveLaunchNonce("pi-session-test-v1"),
    "ec19312204686d442e83eacc3ae23898ebae285fa94566561940517083ea7a35",
  );
});

test("unmanaged extension is inert", () => {
  const old = Object.fromEntries(
    managedKeys.map((key) => [key, process.env[key]]),
  );
  for (const key of managedKeys) delete process.env[key];
  try {
    assert.equal(harness([]).handlers.size, 0);
  } finally {
    restore(old);
  }
});

test("active lifecycle publishes opaque evidence before ID-only handoff", async () =>
  managed(async ({ handoffPath }) => {
    const rawResponse = "```yaml\noutcome: handoff\nnext: successor\n```";
    const state = harness([
      {
        id: "older",
        type: "message",
        message: {
          role: "assistant",
          content: [{ type: "text", text: "old" }],
        },
      },
      {
        id: "terminal",
        type: "message",
        message: {
          role: "assistant",
          content: [{ type: "text", text: rawResponse }],
        },
      },
    ]);
    await state.handlers.get("agent_settled")({}, state.ctx);
    const id = messageID("pi-session-test-v1", "terminal");
    const target = evidencePath(
      { plan: process.env.VAMOS_PLAN_DIR, piSessionID: "pi-session-test-v1" },
      id,
    );
    const document = parseDocument(await readFile(target, "utf8"));
    assert.equal(document.get("hermes_session_id"), "opaque-hermes-session");
    assert.equal(document.get("manager_thread_id"), undefined);
    assert.equal(document.get("raw_response"), rawResponse);
    assert.deepEqual(decodeFrame(await readFile(handoffPath)), {
      version: 1,
      launch_nonce: deriveLaunchNonce("pi-session-test-v1"),
      pi_session_id: "pi-session-test-v1",
      message_id: id,
    });
    assert.equal(state.custom.length, 0);
  }));

test("missing stable assistant identity records a bounded diagnostic", async () =>
  managed(async () => {
    const state = harness([]);
    await state.handlers.get("agent_settled")({}, state.ctx);
    assert.deepEqual(state.custom, [
      { type: DIAGNOSTIC, data: { code: "assistant_settlement_unavailable" } },
    ]);
  }));
