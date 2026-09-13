import assert from "node:assert/strict";
import test from "node:test";
import {
  additionalSkillPathsForTurn,
  agentMemoryTools,
  agentMemoryToolsEnabled,
} from "./agent_memory.js";

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

import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

test("bot_home turn options include additionalSkillPaths when cwd/skills exists", async () => {
  const cwd = await mkdtemp(join(tmpdir(), "bot-home-skills-"));
  try {
    await mkdir(join(cwd, "skills"), { recursive: true });
    await writeFile(join(cwd, "skills", "note.md"), "# skill\n", "utf8");
    await writeFile(join(cwd, "MEMORY.md"), "# memory\n", "utf8");
    const paths = additionalSkillPathsForTurn("bot_home", cwd);
    assert.deepEqual(paths, [join(cwd, "skills")]);
    assert.equal(
      paths?.some((p) => p.endsWith("MEMORY.md") || p.includes("MEMORY")),
      false,
    );
  } finally {
    await rm(cwd, { recursive: true, force: true });
  }
});

test("additionalSkillPaths omitted when skills dir missing", async () => {
  const cwd = await mkdtemp(join(tmpdir(), "bot-home-noskills-"));
  try {
    assert.equal(additionalSkillPathsForTurn("bot_home", cwd), undefined);
  } finally {
    await rm(cwd, { recursive: true, force: true });
  }
});

test("additionalSkillPaths omitted when room_kind is not bot_home", async () => {
  const cwd = await mkdtemp(join(tmpdir(), "plan-skills-"));
  try {
    await mkdir(join(cwd, "skills"), { recursive: true });
    assert.equal(additionalSkillPathsForTurn("plan", cwd), undefined);
    assert.equal(additionalSkillPathsForTurn("pairwise", cwd), undefined);
    assert.equal(additionalSkillPathsForTurn(undefined, cwd), undefined);
    assert.equal(additionalSkillPathsForTurn("bot_home", ""), undefined);
  } finally {
    await rm(cwd, { recursive: true, force: true });
  }
});
