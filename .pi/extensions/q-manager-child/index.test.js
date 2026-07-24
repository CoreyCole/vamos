import assert from "node:assert/strict";
import test from "node:test";

import qManagerChildExtension, {
  classifyChildInput,
  managedResumeControl,
  managedResumeInstruction,
} from "./index.js";

test("input classification keeps ordinary chat quiet and resumes only on exact control", () => {
  assert.deepEqual(classifyChildInput({ text: "hello" }, true), {
    interaction: "managed",
    managedResume: false,
  });
  assert.deepEqual(
    classifyChildInput({ source: "extension", text: "steer" }, false),
    {
      interaction: "managed",
      managedResume: false,
    },
  );
  assert.deepEqual(classifyChildInput({ text: "vamos" }, false), {
    interaction: "chat",
    managedResume: false,
  });
  assert.deepEqual(
    classifyChildInput({ text: `${managedResumeControl} now` }, false),
    {
      interaction: "chat",
      managedResume: false,
    },
  );
  assert.deepEqual(classifyChildInput({ text: managedResumeControl }, false), {
    interaction: "managed",
    managedResume: true,
  });
  assert.match(managedResumeInstruction(), /result init/);
});

test("manager-child sends exact managed-resume instruction", () => {
  const handlers = new Map();
  const pi = {
    on(event, handler) {
      handlers.set(event, handler);
    },
    messages: [],
    sendUserMessage(message) {
      this.messages.push(message);
    },
  };

  qManagerChildExtension(pi);
  const input = handlers.get("input");
  assert.ok(input, "input handler registered");
  assert.deepEqual(input({ source: "user", text: "stage prompt" }), {
    action: "continue",
  });
  assert.deepEqual(input({ source: "user", text: "vamos" }), {
    action: "continue",
  });
  assert.equal(pi.messages.length, 0);
  assert.deepEqual(input({ source: "user", text: managedResumeControl }), {
    action: "continue",
  });
  assert.deepEqual(pi.messages, [managedResumeInstruction()]);
  assert.ok(handlers.get("agent_end"), "agent_end handler registered");
  assert.ok(handlers.get("agent_settled"), "agent_settled handler registered");
});
