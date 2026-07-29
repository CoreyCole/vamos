import assert from "node:assert/strict";
import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import extension from "./index.js";

function harness() {
  const handlers = new Map();
  extension({ on: (name, handler) => handlers.set(name, handler) });
  return handlers;
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
    await fn();
  } finally {
    Object.assign(process.env, old);
  }
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
    assert.equal(harness().size, 0);
  } finally {
    Object.assign(process.env, saved);
  }
});

test("managed identity remains validated while semantic settlement hooks are disabled", async () =>
  managed(async () => {
    assert.equal(harness().size, 0);
  }));

test("managed boundary rejects unsafe filesystem identities before hooks register", () => {
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
