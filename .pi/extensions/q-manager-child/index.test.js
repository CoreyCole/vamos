import assert from "node:assert/strict";
import test from "node:test";

import qManagerChildExtension from "./index.js";

test("manager-child marks only initial and extension input as managed work", () => {
  const handlers = new Map();
  const pi = {
    on(event, handler) {
      handlers.set(event, handler);
    },
    sendUserMessage() {},
  };

  qManagerChildExtension(pi);
  const input = handlers.get("input");
  assert.ok(input, "input handler registered");
  assert.deepEqual(input({ source: "user", streamingBehavior: true }), {
    action: "continue",
  });
  assert.deepEqual(input({ source: "extension", streamingBehavior: false }), {
    action: "continue",
  });
  assert.deepEqual(input({ source: "user", streamingBehavior: true }), {
    action: "continue",
  });
  assert.ok(handlers.get("agent_end"), "agent_end handler registered");
  assert.ok(handlers.get("agent_settled"), "agent_settled handler registered");
});
