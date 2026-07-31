import assert from "node:assert/strict";
import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import extension, {
  deliveryBody,
  evidencePath,
  messageID,
  setDeliveryDependenciesForTest,
} from "./index.js";

test("lifecycle uses the last persisted assistant entry, not turn identity", async (t) => {
  const root = await mkdtemp(join(tmpdir(), "installed-wake-"));
  const keys = [
    "VAMOS_MANAGER_WAKE_MANAGER_THREAD_ID",
    "VAMOS_MANAGER_WAKE_PI_SESSION_ID",
    "VAMOS_MANAGER_WAKE_GATEWAY_URL",
    "VAMOS_MANAGER_WAKE_INGRESS_TOKEN",
    "VAMOS_PLAN_DIR",
  ];
  const old = Object.fromEntries(keys.map((k) => [k, process.env[k]]));
  Object.assign(process.env, {
    VAMOS_MANAGER_WAKE_MANAGER_THREAD_ID: "thread",
    VAMOS_MANAGER_WAKE_PI_SESSION_ID: "installed-session",
    VAMOS_MANAGER_WAKE_GATEWAY_URL: "http://fake",
    VAMOS_MANAGER_WAKE_INGRESS_TOKEN: "token",
    VAMOS_PLAN_DIR: join(root, "plan"),
  });
  t.after(() => {
    for (const [k, v] of Object.entries(old))
      v === undefined ? delete process.env[k] : (process.env[k] = v);
    setDeliveryDependenciesForTest({});
  });
  const branch = [
    {
      id: "first",
      type: "message",
      message: {
        role: "assistant",
        content: [{ type: "text", text: "first" }],
      },
    },
    {
      id: "last",
      type: "message",
      message: {
        role: "assistant",
        content: [{ type: "text", text: "text-only final" }],
      },
    },
  ];
  let handler, body;
  const pi = {
    on: (name, fn) => {
      if (name === "agent_settled") handler = fn;
    },
    appendEntry: () => {},
  };
  setDeliveryDependenciesForTest({
    retries: 1,
    fetch: async (_url, request) => {
      body = JSON.parse(request.body);
      return { ok: true, status: 200 };
    },
  });
  extension(pi);
  await handler({}, { sessionManager: { getBranch: () => branch } });
  const id = messageID("installed-session", "last");
  assert.equal(body.message_id, id);
  assert.equal(body.message, "text-only final");
  assert.deepEqual(
    await deliveryBody(
      evidencePath(
        { plan: process.env.VAMOS_PLAN_DIR, piSessionID: "installed-session" },
        id,
      ),
    ),
    body,
  );
});
