import assert from "node:assert/strict";
import { mkdtemp, readFile, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import extension, {
  BRIDGE,
  CONSUMED,
  DIAGNOSTIC,
  PENDING,
  publish,
  serializeSettlement,
} from "./index.js";

function harness(branch = []) {
  const handlers = new Map();
  let sequence = branch.length;
  const pi = {
    on: (name, handler) => handlers.set(name, handler),
    appendEntry(customType, data) {
      branch.push({
        id: `custom-${++sequence}`,
        type: "custom",
        customType,
        data,
      });
    },
  };
  const ctx = {
    sessionManager: {
      getBranch: () => branch,
      getEntry: (id) => branch.find((entry) => entry.id === id),
    },
  };
  extension(pi);
  return { handlers, ctx, branch };
}

function restoreEnvironment(old) {
  for (const [key, value] of Object.entries(old)) {
    if (value === undefined) delete process.env[key];
    else process.env[key] = value;
  }
}

async function managed(fn) {
  const keys = [
    "PI_SESSION_ID",
    "VAMOS_PLAN_DIR",
    "VAMOS_HERMES_THREAD_ID",
    "VAMOS_THOUGHTS_ROOT",
  ];
  const old = Object.fromEntries(keys.map((key) => [key, process.env[key]]));
  const root = await mkdtemp(join(tmpdir(), "q-manager-child-thoughts-"));
  Object.assign(process.env, {
    PI_SESSION_ID: "session-1",
    VAMOS_PLAN_DIR: join(root, "CoreyCole", "plans", "example"),
    VAMOS_HERMES_THREAD_ID: "thread-1",
    VAMOS_THOUGHTS_ROOT: root,
  });
  try {
    await fn(process.env.VAMOS_PLAN_DIR);
  } finally {
    restoreEnvironment(old);
  }
}

async function bridge(handlers, ctx, message) {
  await handlers.get("turn_end")({ turnIndex: 1, message }, ctx);
}

test("ordinary Pi loads remain inert without managed identity", () => {
  const saved = Object.fromEntries(
    ["PI_SESSION_ID", "VAMOS_PLAN_DIR", "VAMOS_HERMES_THREAD_ID"].map((key) => [
      key,
      process.env[key],
    ]),
  );
  delete process.env.PI_SESSION_ID;
  delete process.env.VAMOS_PLAN_DIR;
  delete process.env.VAMOS_HERMES_THREAD_ID;
  try {
    assert.equal(harness().handlers.size, 0);
  } finally {
    restoreEnvironment(saved);
  }
});

test("managed boundary rejects unsafe identities before hooks register", async () =>
  managed(async () => {
    const unsafe = [
      ["PI_SESSION_ID", "../session", /unsafe session path component/],
      [
        "VAMOS_HERMES_THREAD_ID",
        "thread/child",
        /unsafe manager thread path component/,
      ],
      [
        "VAMOS_PLAN_DIR",
        "/tmp/escape",
        /settlement plan must be thoughts-relative/,
      ],
    ];
    for (const [key, value, error] of unsafe) {
      const saved = process.env[key];
      process.env[key] = value;
      try {
        assert.throws(() => harness(), error);
      } finally {
        if (saved === undefined) delete process.env[key];
        else process.env[key] = saved;
      }
    }
  }));

test("turn_end bridges the exact persisted assistant object", async () =>
  managed(async () => {
    const persisted = {
      role: "assistant",
      content: [{ type: "text", text: "```yaml\na: 1\n```\n" }],
    };
    const { handlers, ctx, branch } = harness([
      { id: "assistant-1", type: "message", message: persisted },
    ]);
    await bridge(handlers, ctx, { ...persisted });
    assert.equal(branch.at(-1).customType, DIAGNOSTIC);
    await bridge(handlers, ctx, persisted);
    assert.deepEqual(branch.at(-1).data, {
      assistant_entry_id: "assistant-1",
      turn: 1,
    });
  }));

test("settlement serializes opaque text fences with JavaScript exact bytes", async () =>
  managed(async () => {
    const bytes = serializeSettlement(
      {
        session: "session-1",
        plan: process.env.VAMOS_PLAN_DIR,
        thread: "thread-1",
      },
      "assistant-1",
      [
        {
          type: "text",
          text: "prefix```yaml nope\n```YAML\na: café 🌰\r\n```\r\n",
        },
      ],
    );
    assert.equal(
      bytes.toString(),
      '{"version":1,"kind":"opaque_pi_settlement","session":"session-1","plan":"CoreyCole/plans/example","manager_thread":"thread-1","final_entry_id":"assistant-1","fences":["a: café 🌰\\r\\n"]}',
    );
  }));

test("later steering skips tool-only leaves, settles the final text leaf once, and adds no capabilities", async () =>
  managed(async (plan) => {
    const toolLeaf = {
      role: "assistant",
      content: [{ type: "toolCall", name: "not-settlement-evidence" }],
    };
    const finalLeaf = {
      role: "assistant",
      content: [{ type: "text", text: "```yaml\nfinal: true\n```" }],
    };
    const state = harness([
      { id: "assistant-tool", type: "message", message: toolLeaf },
      { id: "assistant-final", type: "message", message: finalLeaf },
    ]);
    assert.deepEqual([...state.handlers.keys()].sort(), [
      "agent_settled",
      "session_before_compact",
      "session_before_tree",
      "turn_end",
    ]);
    await bridge(state.handlers, state.ctx, toolLeaf);
    assert.equal(
      state.branch.filter((entry) => entry.customType === BRIDGE).length,
      0,
    );
    await bridge(state.handlers, state.ctx, finalLeaf);
    await state.handlers.get("agent_settled")({}, state.ctx);
    await state.handlers.get("agent_settled")({}, state.ctx);
    assert.equal(
      state.branch.filter((entry) => entry.customType === CONSUMED).length,
      1,
    );
    const settlement = JSON.parse(
      await readFile(
        join(
          plan,
          ".vamos/sessions/pi/session-1/settlements/assistant-final.json",
        ),
        "utf8",
      ),
    );
    assert.deepEqual(settlement.fences, ["final: true\n"]);
  }));

test("agent_settled persists base64 before safe publication and consumes after it", async () =>
  managed(async (plan) => {
    const message = {
      role: "assistant",
      content: [{ type: "text", text: "```yml\nvalue: on\n```" }],
    };
    const { handlers, ctx, branch } = harness([
      { id: "assistant-1", type: "message", message },
    ]);
    await bridge(handlers, ctx, message);
    await handlers.get("agent_settled")({}, ctx);
    const pending = branch.find((entry) => entry.customType === PENDING);
    assert.ok(pending);
    assert.equal(branch.at(-1).customType, CONSUMED);
    const bytes = Buffer.from(pending.data.bytes_base64, "base64");
    assert.equal(
      await readFile(
        join(plan, ".vamos/sessions/pi/session-1/settlements/assistant-1.json"),
        "utf8",
      ),
      bytes.toString(),
    );
    assert.match(bytes.toString(), /"fences":\["value: on\\n"\]/);
  }));

test("restart recovers after link publication but before consume from exact pending bytes", async () =>
  managed(async (plan) => {
    const message = {
      role: "assistant",
      content: [{ type: "text", text: "```yaml\nx: 1\n```\n" }],
    };
    const initial = [
      { id: "assistant-1", type: "message", message },
      {
        id: "bridge-1",
        type: "custom",
        customType: BRIDGE,
        data: { assistant_entry_id: "assistant-1", turn: 1 },
      },
    ];
    const bytes = serializeSettlement(
      { session: "session-1", plan, thread: "thread-1" },
      "assistant-1",
      message.content,
    );
    initial.push({
      id: "pending-1",
      type: "custom",
      customType: PENDING,
      data: {
        bridge_id: "bridge-1",
        session: "session-1",
        final_entry_id: "assistant-1",
        bytes_base64: bytes.toString("base64"),
      },
    });
    await publish(plan, "session-1", "assistant-1", bytes);
    const restarted = harness(initial);
    await restarted.handlers.get("agent_settled")({}, restarted.ctx);
    assert.equal(
      restarted.branch.filter((entry) => entry.customType === PENDING).length,
      1,
    );
    assert.equal(
      restarted.branch.filter((entry) => entry.customType === CONSUMED).length,
      1,
    );
    assert.equal(
      await readFile(
        join(plan, ".vamos/sessions/pi/session-1/settlements/assistant-1.json"),
        "utf8",
      ),
      bytes.toString(),
    );
  }));

test("safe publisher is no-replace and equal-byte idempotent", async () => {
  const plan = await mkdtemp(join(tmpdir(), "q-manager-child-publish-"));
  const bytes = Buffer.from("{}", "utf8");
  const path = await publish(plan, "session-1", "entry-1", bytes);
  assert.equal((await stat(path)).isFile(), true);
  assert.equal(await publish(plan, "session-1", "entry-1", bytes), path);
  await assert.rejects(
    () => publish(plan, "session-1", "entry-1", Buffer.from("{ }")),
    /immutable opaque settlement identity conflict/,
  );
});

test("all compaction is cancelled as nonsemantic telemetry and summary cancellation remains narrow", async () =>
  managed(async () => {
    const { handlers, ctx, branch } = harness();
    for (const reason of ["manual", "threshold", "overflow"]) {
      assert.deepEqual(
        await handlers.get("session_before_compact")({ reason }, ctx),
        { cancel: true },
      );
    }
    assert.deepEqual(
      branch.map((entry) => entry.data),
      [
        { code: "context_compaction_cancelled", source: "manual" },
        { code: "context_compaction_cancelled", source: "threshold" },
        { code: "context_compaction_cancelled", source: "overflow" },
      ],
    );
    assert.deepEqual(
      await handlers.get("session_before_tree")(
        { preparation: { userWantsSummary: true } },
        ctx,
      ),
      { cancel: true },
    );
    assert.equal(
      await handlers.get("session_before_tree")(
        { preparation: { userWantsSummary: false } },
        ctx,
      ),
      undefined,
    );
  }));
