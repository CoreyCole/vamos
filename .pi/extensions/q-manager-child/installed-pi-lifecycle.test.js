import assert from "node:assert/strict";
import { createServer } from "node:http";
import { mkdir, mkdtemp, readFile, readdir, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawn } from "node:child_process";
import test from "node:test";

const pi = process.env.PI_BIN ?? "pi";

function run(command, args, options) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, options);
    let stdout = "";
    let stderr = "";
    const timeout = setTimeout(() => child.kill("SIGTERM"), 20_000);
    child.stdin.end();
    child.stdout.on("data", (chunk) => (stdout += chunk));
    child.stderr.on("data", (chunk) => (stderr += chunk));
    child.on("error", reject);
    child.on("close", (code, signal) => {
      clearTimeout(timeout);
      resolve({ code, signal, stdout, stderr });
    });
  });
}

async function sessionEntries(directory) {
  const files = await readdir(directory, { recursive: true });
  const session = files.find((file) => file.endsWith(".jsonl"));
  assert.ok(session, "installed Pi persisted a session JSONL file");
  return (await readFile(join(directory, session), "utf8"))
    .trim()
    .split("\n")
    .map((line) => JSON.parse(line));
}

async function fixtureModel() {
  let request;
  const server = createServer(async (req, res) => {
    request = {
      method: req.method,
      url: req.url,
      body: await new Promise((resolve, reject) => {
        let body = "";
        req.setEncoding("utf8");
        req.on("data", (chunk) => (body += chunk));
        req.on("end", () => resolve(JSON.parse(body)));
        req.on("error", reject);
      }),
    };
    res.writeHead(200, { "content-type": "text/event-stream" });
    res.write(
      `data: ${JSON.stringify({
        object: "chat.completion.chunk",
        choices: [
          {
            index: 0,
            delta: { role: "assistant", content: "outcome: complete" },
            finish_reason: null,
          },
        ],
      })}\n\n`,
    );
    res.write(
      `data: ${JSON.stringify({
        object: "chat.completion.chunk",
        choices: [{ index: 0, delta: {}, finish_reason: "stop" }],
      })}\n\n`,
    );
    res.end("data: [DONE]\n\n");
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address();
  return {
    baseURL: `http://127.0.0.1:${port}/v1`,
    request: () => request,
    close: () =>
      new Promise((resolve, reject) =>
        server.close((error) => (error ? reject(error) : resolve())),
      ),
  };
}

test("installed managed Pi persists a text-only, no-YAML settlement without a provider response ID", async (t) => {
  const root = await mkdtemp(join(tmpdir(), "q-manager-child-installed-pi-"));
  const agentDir = join(root, "agent");
  const sessions = join(root, "sessions");
  const thoughts = join(root, "thoughts");
  const plan = join(thoughts, "CoreyCole", "plans", "installed-pi");
  const model = await fixtureModel();
  t.after(model.close);
  await mkdir(agentDir, { recursive: true });
  await writeFile(
    join(agentDir, "models.json"),
    JSON.stringify({
      providers: {
        fixture: {
          baseUrl: model.baseURL,
          api: "openai-completions",
          apiKey: "fixture-key",
          compat: {
            supportsDeveloperRole: false,
            supportsReasoningEffort: false,
            supportsUsageInStreaming: false,
            maxTokensField: "max_tokens",
          },
          models: [{ id: "fixture", contextWindow: 4096, maxTokens: 256 }],
        },
      },
    }),
  );

  const result = await run(
    pi,
    [
      "--approve",
      "--offline",
      "--no-context-files",
      "--no-skills",
      "--no-prompt-templates",
      "--no-themes",
      "--no-builtin-tools",
      "--session-dir",
      sessions,
      "--model",
      "fixture/fixture",
      "--print",
      "Reply with lifecycle-looking text.",
    ],
    {
      cwd: process.cwd(),
      env: {
        ...process.env,
        PI_CODING_AGENT_DIR: agentDir,
        PI_SESSION_ID: "installed-session",
        VAMOS_THOUGHTS_ROOT: thoughts,
        VAMOS_PLAN_DIR: plan,
        VAMOS_HERMES_THREAD_ID: "installed-thread",
      },
    },
  );
  assert.equal(result.code, 0, `installed Pi failed:\n${result.stderr}`);
  assert.match(result.stdout, /outcome: complete/);
  assert.equal(model.request().url, "/v1/chat/completions");

  const entries = await sessionEntries(sessions);
  assert.ok(entries.some((entry) => entry.message?.role === "assistant"));
  assert.ok(
    entries.some(
      (entry) =>
        entry.type === "custom" &&
        entry.customType === "q-manager-child/settlement-pending",
    ),
  );
  const settlement = JSON.parse(
    await readFile(
      join(
        plan,
        ".vamos",
        "sessions",
        "pi",
        "installed-session",
        "settlements",
        entries.find(
          (entry) =>
            entry.type === "custom" &&
            entry.customType === "q-manager-child/settlement-consumed",
        ).data.final_entry_id + ".json",
      ),
      "utf8",
    ),
  );
  assert.deepEqual(settlement, {
    version: 1,
    kind: "opaque_pi_settlement",
    session: "installed-session",
    plan: "CoreyCole/plans/installed-pi",
    manager_thread: "installed-thread",
    final_entry_id: settlement.final_entry_id,
    fences: [],
  });
});
