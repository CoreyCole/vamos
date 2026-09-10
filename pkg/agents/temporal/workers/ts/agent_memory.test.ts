import assert from "node:assert/strict";
import test from "node:test";
import { agentMemoryTools, agentMemoryToolsEnabled } from "./agent_memory.js";

test("agent-memory tools register only for bot-home rooms", () => {
  assert.equal(agentMemoryToolsEnabled("bot_home"), true);
  assert.equal(agentMemoryToolsEnabled("plan"), false);
  assert.equal(agentMemoryToolsEnabled("pairwise"), false);
  assert.equal(agentMemoryToolsEnabled(undefined), false);
  const names = agentMemoryTools().map((tool) => tool.name);
  assert.deepEqual(names, ["memory_write", "memory_search"]);
  assert.equal(
    names.some((name) => name.includes("sqlite") || name.includes("session")),
    false,
  );
});
