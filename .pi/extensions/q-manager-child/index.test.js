import assert from "node:assert/strict";
import { mkdtemp, readFile, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import extension, {
  BRIDGE,
  CONSUMED,
  DIAGNOSTIC,
  LIMIT_CONSUMED,
  LIMIT_PENDING,
  PENDING,
  publish,
  yaml,
} from "./index.js";

function harness(branch = []) {
  const handlers = new Map();
  let sequence = branch.length;
  const pi = {
    on: (name, fn) => handlers.set(name, fn),
    appendEntry(type, data) {
      branch.push({
        type: "custom",
        customType: type,
        data,
        id: `custom-${++sequence}`,
      });
    },
  };
  const ctx = {
    signal: new AbortController().signal,
    sessionManager: {
      getBranch: () => branch,
      getEntry: (id) => branch.find((entry) => entry.id === id),
    },
  };
  extension(pi);
  return { handlers, ctx, branch };
}
async function managed(fn) {
  const old = Object.fromEntries(
    [
      "PI_SESSION_ID",
      "VAMOS_PLAN_DIR",
      "VAMOS_HERMES_THREAD_ID",
      "VAMOS_THOUGHTS_ROOT",
    ].map((key) => [key, process.env[key]]),
  );
  const root = await mkdtemp(join(tmpdir(), "q-manager-child-thoughts-"));
  const plan = join(root, "CoreyCole", "plans", "example");
  Object.assign(process.env, {
    PI_SESSION_ID: "session-1",
    VAMOS_PLAN_DIR: plan,
    VAMOS_HERMES_THREAD_ID: "thread-1",
    VAMOS_THOUGHTS_ROOT: root,
  });
  try {
    await fn(plan);
  } finally {
    Object.assign(process.env, old);
  }
}
async function bridge(handlers, ctx, message, run = 1) {
  await handlers.get("agent_start")({}, ctx);
  await handlers.get("turn_end")({ turnIndex: run, message, run_id: run }, ctx);
}

test("ordinary Pi loads are inert without managed identity", () => {
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
    Object.assign(process.env, saved);
  }
});

test("managed boundary rejects unsafe filesystem identities", async () => {
  const saved = Object.fromEntries(
    ["PI_SESSION_ID", "VAMOS_PLAN_DIR", "VAMOS_HERMES_THREAD_ID"].map((key) => [
      key,
      process.env[key],
    ]),
  );
  Object.assign(process.env, {
    PI_SESSION_ID: "../session",
    VAMOS_PLAN_DIR: "/tmp/plan",
    VAMOS_HERMES_THREAD_ID: "thread-1",
  });
  try {
    assert.throws(() => harness(), /unsafe session path component/);
  } finally {
    Object.assign(process.env, saved);
  }
});

test("managed boundary rejects empty, ancestor, traversal, and outside plan roots before hooks append", async () => {
  const saved = Object.fromEntries(
    [
      "PI_SESSION_ID",
      "VAMOS_PLAN_DIR",
      "VAMOS_HERMES_THREAD_ID",
      "VAMOS_THOUGHTS_ROOT",
    ].map((key) => [key, process.env[key]]),
  );
  const root = await mkdtemp(join(tmpdir(), "q-manager-child-root-"));
  for (const [plan, thoughtsRoot] of [
    [root, root], // ancestor/equal root
    [`${root}/../escape`, root], // traversal input
    [join(root, "plan"), ""], // empty root
    [join(root, "plan"), join(root, "other")], // outside root
  ]) {
    Object.assign(process.env, {
      PI_SESSION_ID: "session-1",
      VAMOS_PLAN_DIR: plan,
      VAMOS_HERMES_THREAD_ID: "thread-1",
      VAMOS_THOUGHTS_ROOT: thoughtsRoot,
    });
    assert.throws(() => harness(), /checkpoint plan must|VAMOS_THOUGHTS_ROOT/);
  }
  Object.assign(process.env, saved);
});

test("Task 2 canonical checkpoint bytes use yaml.v3 field order and block scalars", () => {
  assert.equal(
    yaml({
      version: 2,
      session: "session-1",
      plan: "thoughts/me/plan",
      manager_thread: "thread-1",
      final_entry_id: "entry-1",
      outcome: "handoff",
      next: "none",
      created_at: "2026-07-29T12:00:00Z",
      raw_response: `outcome: handoff
next: review`,
      diagnostics: ["missing lifecycle value"],
    }),
    `version: 2
session: session-1
plan: thoughts/me/plan
manager_thread: thread-1
final_entry_id: entry-1
outcome: handoff
next: none
created_at: 2026-07-29T12:00:00Z
raw_response: |-
    outcome: handoff
    next: review
diagnostics:
    - missing lifecycle value
`,
  );
});

test("canonical encoder preserves arbitrary metadata and ambiguous/multiline fields", () => {
  const bytes = yaml({
    version: 2,
    session: "session-1",
    plan: "CoreyCole/plans/example",
    manager_thread: "thread-1",
    final_entry_id: "entry-1",
    outcome: "complete",
    next: "none",
    created_at: "2026-07-29T12:00:00Z",
    raw_yaml: "state: complete\nvalue: on\n",
    intent_metadata: { alpha: "on", nested: { note: "a\nb" }, retries: 2 },
  });
  assert.match(bytes, /raw_yaml: \|\n/);
  assert.match(bytes, /alpha: "on"/);
  assert.match(bytes, /nested:\n        note: \|-\n/);
  assert.match(bytes, /retries: 2/);
});

test("publication is no-replace, equal-byte idempotent, and leaves a regular checkpoint", async () => {
  const plan = await mkdtemp(join(tmpdir(), "q-manager-child-"));
  const bytes = Buffer.from("version: 2\\n");
  const first = await publish(plan, "session-1", "entry-1", bytes);
  assert.equal((await stat(first)).isFile(), true);
  assert.equal(await publish(plan, "session-1", "entry-1", bytes), first);
  await assert.rejects(
    () =>
      publish(
        plan,
        "session-1",
        "entry-1",
        Buffer.from("version: 2\\nsummary: conflict\\n"),
      ),
    /immutable checkpoint identity conflict/,
  );
});

test("agent_end alone creates no checkpoint; exact turn bridge settles once", async () =>
  managed(async (plan) => {
    const message = {
      role: "assistant",
      content: [{ type: "text", text: "outcome: handoff\nnext: review" }],
    };
    const { handlers, ctx, branch } = harness([
      { id: "assistant-1", type: "message", message },
    ]);
    assert.equal(handlers.has("agent_end"), false);
    await bridge(handlers, ctx, message);
    await handlers.get("agent_settled")({ run_id: 1 }, ctx);
    const bytes = await readFile(
      join(plan, ".vamos/sessions/pi/session-1/checkpoints/assistant-1.yaml"),
      "utf8",
    );
    assert.match(bytes, /final_entry_id: assistant-1/);
    assert.match(bytes, /outcome: handoff/);
    assert.match(bytes, /next: none/);
    assert.equal(
      branch.filter((entry) => entry.customType === PENDING).length,
      1,
    );
    assert.equal(
      branch.filter((entry) => entry.customType === CONSUMED).length,
      1,
    );
    await handlers.get("agent_settled")({ run_id: 1 }, ctx);
    assert.equal(
      branch.filter((entry) => entry.customType === CONSUMED).length,
      1,
    );
  }));

test("only the persisted assistant object can be bridged; tool results and provider IDs do not matter", async () =>
  managed(async () => {
    const { handlers, ctx, branch } = harness([
      {
        id: "assistant-1",
        type: "message",
        message: { role: "assistant", providerResponseId: undefined },
      },
    ]);
    await handlers.get("turn_end")(
      {
        turnIndex: 1,
        message: { role: "assistant" },
        toolResults: [{ toolCallId: "x" }],
      },
      ctx,
    );
    assert.equal(branch.at(-1).customType, DIAGNOSTIC);
  }));

test("pending bytes survive restart/re-observation and are consumed only after equal-byte publication", async () =>
  managed(async (plan) => {
    const message = {
      role: "assistant",
      content: [{ type: "text", text: "outcome: complete" }],
    };
    const initial = [
      { id: "assistant-1", type: "message", message },
      {
        id: "bridge-1",
        type: "custom",
        customType: BRIDGE,
        data: { assistant_entry_id: "assistant-1", active_run: 1 },
      },
    ];
    const first = harness(initial);
    await first.handlers.get("agent_settled")({ run_id: 1 }, first.ctx);
    const pending = first.branch.find((entry) => entry.customType === PENDING);
    first.branch.splice(
      first.branch.findIndex((entry) => entry.customType === CONSUMED),
      1,
    );
    const restarted = harness(first.branch);
    await restarted.handlers.get("agent_settled")({ run_id: 1 }, restarted.ctx);
    assert.equal(
      restarted.branch.filter((entry) => entry.customType === CONSUMED).length,
      1,
    );
    assert.equal(
      await readFile(
        join(plan, ".vamos/sessions/pi/session-1/checkpoints/assistant-1.yaml"),
        "utf8",
      ),
      pending.data.bytes,
    );
  }));

test("manual is diagnostic-only, tree summary is cancelled, and automatic markers bind the settling bridge/run", async () =>
  managed(async () => {
    const one = {
        role: "assistant",
        content: [{ type: "text", text: "outcome: complete" }],
      },
      two = {
        role: "assistant",
        content: [{ type: "text", text: "outcome: handoff" }],
      };
    const { handlers, ctx, branch } = harness([
      { id: "a1", type: "message", message: one },
      { id: "a2", type: "message", message: two },
    ]);
    await bridge(handlers, ctx, one, 1);
    assert.deepEqual(
      await handlers.get("session_before_compact")(
        { reason: "threshold" },
        ctx,
      ),
      { cancel: true },
    );
    assert.equal(branch.at(-1).customType, LIMIT_PENDING);
    assert.deepEqual(
      await handlers.get("session_before_compact")({ reason: "manual" }, ctx),
      { cancel: true },
    );
    assert.equal(branch.at(-1).data.code, "context_limit_after_settlement");
    await handlers.get("agent_settled")({ run_id: 1 }, ctx);
    assert.equal(branch.at(-1).customType, CONSUMED);
    assert.ok(branch.some((entry) => entry.customType === LIMIT_CONSUMED));
    await bridge(handlers, ctx, two, 2);
    await handlers.get("agent_settled")({ run_id: 2 }, ctx);
    assert.equal(
      branch.filter((entry) => entry.customType === PENDING).length,
      2,
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

test("settled checkpoints preserve the closed outcome domain and child intent fields", async () =>
  managed(async () => {
    for (const [index, outcome] of [
      "complete",
      "handoff",
      "needs_human",
      "blocked",
      "error",
    ].entries()) {
      const message = {
        role: "assistant",
        content: [
          {
            type: "text",
            text: `result:\n  decision: ${outcome}\nsummary: ${outcome}\nartifacts:\n  - thoughts/me/${outcome}.md\nowner_note: retained\nnested:\n  retry: 2`,
          },
        ],
      };
      const { handlers, ctx, branch } = harness([
        { id: `assistant-${index}`, type: "message", message },
      ]);
      await bridge(handlers, ctx, message, index);
      await handlers.get("agent_settled")({}, ctx);
      const pending = branch.find((entry) => entry.customType === PENDING);
      assert.equal(pending.data.payload.outcome, outcome);
      assert.equal(pending.data.payload.summary, outcome);
      assert.deepEqual(pending.data.payload.artifacts, [
        `thoughts/me/${outcome}.md`,
      ]);
      assert.match(
        pending.data.payload.raw_yaml,
        new RegExp(`decision: ${outcome}`),
      );
      assert.deepEqual(pending.data.payload.intent_metadata, {
        owner_note: "retained",
        nested: { retry: 2 },
      });
    }
  }));
