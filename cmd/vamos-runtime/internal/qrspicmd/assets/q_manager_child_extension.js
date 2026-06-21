import { spawn } from "node:child_process";
import { mkdir, writeFile } from "node:fs/promises";
import { dirname } from "node:path";

async function writeJson(path, value) {
  if (!path) return;
  await mkdir(dirname(path), { recursive: true });
  await writeFile(path, `${JSON.stringify(value, null, 2)}\n`, "utf8");
}

async function touch(path) {
  if (!path) return;
  await mkdir(dirname(path), { recursive: true });
  await writeFile(path, "", "utf8");
}

function tmux(args) {
  return new Promise((resolve, reject) => {
    const child = spawn("tmux", args, { stdio: "ignore" });
    child.on("error", reject);
    child.on("exit", (code) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(new Error(`tmux ${args[0]} exited ${code}`));
    });
  });
}

function yamlString(value) {
  return JSON.stringify(value || "");
}

function wakeMessage() {
  const stage = process.env.Q_MANAGER_STAGE || "unknown";
  const stateFile = process.env.Q_MANAGER_STATE_FILE || "";
  const command = `vamos qrspi continue --state-file ${stateFile}`;
  return [
    "q_manager_child_wake:",
    `  stage: ${yamlString(stage)}`,
    `  state_file: ${yamlString(stateFile)}`,
    "  next:",
    "    steps:",
    '      - action: "run_command"',
    `        param: ${yamlString(command)}`,
  ].join("\n");
}

async function wakeParent(pane, text) {
  if (!pane) return;
  const buffer = `q-manager-wake-${process.env.Q_MANAGER_CHILD_ID || Date.now()}`;
  await tmux(["set-buffer", "-b", buffer, text]);
  await tmux(["paste-buffer", "-b", buffer, "-t", pane]);
  await tmux(["send-keys", "-t", pane, "Enter"]);
}

export default function qManagerChildExtension(pi) {
  const key = Symbol.for("vamos.q_manager_child_extension.loaded");
  if (globalThis[key]) return;
  globalThis[key] = true;

  pi.on("agent_end", async () => {
    const status = {
      event: "agent_end",
      stage: process.env.Q_MANAGER_STAGE || "",
      childId: process.env.Q_MANAGER_CHILD_ID || "",
      stateFile: process.env.Q_MANAGER_STATE_FILE || "",
      planDir: process.env.Q_MANAGER_PLAN_DIR || "",
      sessionId:
        process.env.Q_MANAGER_SESSION_ID || process.env.SESSION_ID || "",
      sessionDir:
        process.env.Q_MANAGER_SESSION_DIR || process.env.SESSION_DIR || "",
      finishedAt: new Date().toISOString(),
      wakeTarget: process.env.Q_MANAGER_PARENT_PANE || "",
    };
    await writeJson(process.env.Q_MANAGER_STATUS_PATH, status);
    await touch(process.env.Q_MANAGER_DONE_PATH);
    await wakeParent(process.env.Q_MANAGER_PARENT_PANE, wakeMessage());
  });
}
