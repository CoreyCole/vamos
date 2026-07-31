import assert from "node:assert/strict";
import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import extension, {
  ATTEMPT,
  DIAGNOSTIC,
  deliveryBody,
  evidencePath,
  messageID,
  publish,
  setDeliveryDependenciesForTest,
} from "./index.js";

function saveEnv() {
  return Object.fromEntries(
    [
      "VAMOS_MANAGER_WAKE_MANAGER_THREAD_ID",
      "VAMOS_MANAGER_WAKE_PI_SESSION_ID",
      "VAMOS_MANAGER_WAKE_GATEWAY_URL",
      "VAMOS_MANAGER_WAKE_INGRESS_TOKEN",
      "VAMOS_PLAN_DIR",
    ].map((k) => [k, process.env[k]]),
  );
}
function restore(values) {
  for (const [k, v] of Object.entries(values))
    v === undefined ? delete process.env[k] : (process.env[k] = v);
}
async function managed(run) {
  const old = saveEnv(),
    root = await mkdtemp(join(tmpdir(), "wake-"));
  Object.assign(process.env, {
    VAMOS_MANAGER_WAKE_MANAGER_THREAD_ID: "thread-1",
    VAMOS_MANAGER_WAKE_PI_SESSION_ID: "session-1",
    VAMOS_MANAGER_WAKE_GATEWAY_URL: "http://gateway.test/",
    VAMOS_MANAGER_WAKE_INGRESS_TOKEN: "secret",
    VAMOS_PLAN_DIR: join(root, "plan"),
  });
  try {
    await run();
  } finally {
    restore(old);
    setDeliveryDependenciesForTest({});
  }
}
function harness(branch) {
  const handlers = new Map(),
    entries = [...branch],
    custom = [];
  const pi = {
    on: (n, h) => handlers.set(n, h),
    appendEntry: (type, data) => custom.push({ type, data }),
  };
  extension(pi);
  return {
    handlers,
    ctx: { sessionManager: { getBranch: () => entries } },
    custom,
  };
}

test("deterministic ID uses only session and terminal entry", () =>
  assert.equal(
    messageID("s", "e"),
    "pi-settlement-v1-j7Qq1yfM1Wj5G4eYsMMFxzM5B0cSTdvoCtkCSfyVd1Y",
  ));
test("unmanaged extension is inert", () => {
  const old = saveEnv();
  for (const k of Object.keys(old)) delete process.env[k];
  try {
    assert.equal(harness([]).handlers.size, 0);
  } finally {
    restore(old);
  }
});
test("agent settlement publishes then posts terminal persisted assistant response", async () =>
  managed(async () => {
    const calls = [];
    setDeliveryDependenciesForTest({
      retries: 1,
      fetch: async (url, options) => {
        calls.push({ url, options });
        return { ok: true, status: 200 };
      },
    });
    const state = harness([
      {
        id: "early",
        type: "message",
        message: {
          role: "assistant",
          content: [{ type: "text", text: "old" }],
        },
      },
      {
        id: "final",
        type: "message",
        message: {
          role: "assistant",
          content: [{ type: "text", text: "```yaml\na: 1\n```" }],
        },
      },
    ]);
    await state.handlers.get("agent_settled")({}, state.ctx);
    const id = messageID("session-1", "final"),
      path = evidencePath(
        { plan: process.env.VAMOS_PLAN_DIR, piSessionID: "session-1" },
        id,
      );
    assert.equal((await deliveryBody(path)).message, "```yaml\na: 1\n```");
    assert.equal(calls.length, 1);
    assert.deepEqual(
      JSON.parse(calls[0].options.body),
      await deliveryBody(path),
    );
    assert.ok(state.custom.some((x) => x.type === ATTEMPT));
  }));
test("empty text is a settlement and duplicate/restarted hooks reuse path and body", async () =>
  managed(async () => {
    const bodies = [];
    setDeliveryDependenciesForTest({
      retries: 1,
      fetch: async (_u, o) => {
        bodies.push(o.body);
        return { ok: true, status: 200 };
      },
    });
    const branch = [
      {
        id: "final",
        type: "message",
        message: { role: "assistant", content: [] },
      },
    ];
    for (let i = 0; i < 2; i++) {
      const s = harness(branch);
      await s.handlers.get("agent_settled")({}, s.ctx);
    }
    assert.equal(bodies.length, 2);
    assert.equal(bodies[0], bodies[1]);
    assert.equal(JSON.parse(bodies[0]).message, "");
  }));
test("existing byte-different target is a recovery conflict and never posts", async () =>
  managed(async () => {
    const branch = [
        {
          id: "final",
          type: "message",
          message: {
            role: "assistant",
            content: [{ type: "text", text: "new" }],
          },
        },
      ],
      id = messageID("session-1", "final"),
      path = evidencePath(
        { plan: process.env.VAMOS_PLAN_DIR, piSessionID: "session-1" },
        id,
      );
    await publish(
      path,
      Buffer.from(
        "version: 1\nmanager_thread_id: thread-1\npi_session_id: session-1\nmessage_id: " +
          id +
          "\nraw_response: |-\n  old\n",
      ),
    );
    let posts = 0;
    setDeliveryDependenciesForTest({
      fetch: async () => {
        posts++;
        return { ok: true };
      },
    });
    const s = harness(branch);
    await s.handlers.get("agent_settled")({}, s.ctx);
    assert.equal(posts, 0);
    assert.equal(s.custom.at(-1).data.code, "assistant_settlement_conflict");
  }));
test("failed delivery retries fixed published bytes without leaking secrets", async () =>
  managed(async () => {
    const calls = [];
    setDeliveryDependenciesForTest({
      retries: 2,
      sleep: async () => {},
      fetch: async (_u, o) => {
        calls.push(o.body);
        throw new Error("lost ack");
      },
    });
    const s = harness([
      {
        id: "final",
        type: "message",
        message: {
          role: "assistant",
          content: [{ type: "text", text: "safe" }],
        },
      },
    ]);
    await s.handlers.get("agent_settled")({}, s.ctx);
    assert.equal(calls.length, 2);
    assert.equal(calls[0], calls[1]);
    const diagnostic = JSON.stringify(s.custom);
    assert.ok(
      !diagnostic.includes("secret") && !diagnostic.includes("gateway"),
    );
  }));
test("missing stable assistant identity reports recovery diagnostic", async () =>
  managed(async () => {
    const s = harness([]);
    await s.handlers.get("agent_settled")({}, s.ctx);
    assert.deepEqual(s.custom, [
      { type: DIAGNOSTIC, data: { code: "assistant_settlement_unavailable" } },
    ]);
  }));
