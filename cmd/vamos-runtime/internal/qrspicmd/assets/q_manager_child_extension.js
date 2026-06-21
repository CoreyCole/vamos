import { spawn } from "node:child_process";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname } from "node:path";

async function writeJson(path, value) {
  if (!path) return;
  await mkdir(dirname(path), { recursive: true });
  await writeFile(path, `${JSON.stringify(value, null, 2)}\n`, "utf8");
}

async function readJson(path) {
  if (!path) return null;
  try {
    return JSON.parse(await readFile(path, "utf8"));
  } catch (error) {
    if (error && error.code === "ENOENT") return null;
    return null;
  }
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

function yamlBool(value) {
  return value ? "true" : "false";
}

function hasValue(value) {
  return value !== undefined && value !== null && String(value) !== "";
}

function resultStatus(validation) {
  return validation?.result || validation?.childResult || validation || {};
}

function shouldWakeManager(validation) {
  if (process.env.Q_MANAGER_WAKE_MODE === "raw-agent-end") return true;
  if (!validation) return false;
  return (
    validation.validated === true ||
    validation.managerNeeded === true ||
    validation.retryExhausted === true
  );
}

function wakeReason(status, validation) {
  if (hasValue(validation?.reason)) return validation.reason;
  if (validation?.retryExhausted) return "retry exhausted";
  if (status.status === "needs_human") return "result requested human input";
  if (status.status === "blocked") return "child reported blocked";
  if (status.status === "error") return "result error";
  if (validation?.validated) return "validated child result";
  if (process.env.Q_MANAGER_WAKE_MODE === "raw-agent-end")
    return "raw agent_end";
  return "manager needed";
}

function managerGuidance(status, validation) {
  if (validation?.retryExhausted || status.status === "invalid_result") {
    return {
      mode: "deterministic_recovery_first",
      instruction:
        "Inspect child output/artifacts. If stage work is durable and the correct graph result is deterministic, steer the same child or use CLI correction/continue to emit/apply the canonical result. Ask human only when intent, safety, product judgment, or missing facts make recovery non-deterministic.",
    };
  }
  if (status.status === "needs_human") {
    return {
      mode: "ask_human_only_when_required",
      instruction:
        "Summarize the child question to the human only if the answer requires product/scope/safety judgment; otherwise inspect artifacts/session and steer the child with deterministic recovery context.",
    };
  }
  if (status.status === "blocked" || status.status === "error") {
    return {
      mode: "diagnose_before_human",
      instruction:
        "Inspect blocker artifact/session and determine if the manager can unblock by steering, rerunning safe commands, fixing workspace mechanics, or continuing the graph. Ask human only if judgment or external authority is truly required.",
    };
  }
  return null;
}

function wakeMessage(validation) {
  const status = resultStatus(validation);
  const stage =
    status.stage ||
    validation?.stage ||
    process.env.Q_MANAGER_STAGE ||
    "unknown";
  const stateFile = process.env.Q_MANAGER_STATE_FILE || "";
  const command = `vamos qrspi continue --state-file ${stateFile}`;
  const validated = validation?.validated === true;
  const managerNeeded =
    validation?.managerNeeded === true ||
    validation?.retryExhausted === true ||
    ["needs_human", "blocked", "error", "invalid_result"].includes(
      status.status || "",
    );
  const retryExhausted = validation?.retryExhausted === true;
  const guidance = managerGuidance(status, validation);
  const lines = [
    "```yaml",
    "q_manager_child_wake:",
    `  validated: ${yamlBool(validated)}`,
    `  manager_needed: ${yamlBool(managerNeeded)}`,
    `  retry_exhausted: ${yamlBool(retryExhausted)}`,
    `  stage: ${yamlString(stage)}`,
    `  status: ${yamlString(status.status || (retryExhausted ? "invalid_result" : ""))}`,
    `  outcome: ${yamlString(status.outcome || "")}`,
  ];
  if (
    hasValue(status.artifact || status.primaryArtifact || validation?.artifact)
  ) {
    lines.push(
      `  artifact: ${yamlString(status.artifact || status.primaryArtifact || validation.artifact)}`,
    );
  }
  if (hasValue(validation?.attempt))
    lines.push(`  attempt: ${validation.attempt}`);
  if (hasValue(validation?.retryLimit))
    lines.push(`  retry_limit: ${validation.retryLimit}`);
  lines.push(`  state_file: ${yamlString(stateFile)}`);
  lines.push(`  reason: ${yamlString(wakeReason(status, validation))}`);
  if (guidance) {
    lines.push("  manager_guidance:");
    lines.push(`    mode: ${yamlString(guidance.mode)}`);
    lines.push(`    instruction: ${yamlString(guidance.instruction)}`);
  }
  lines.push("  next:");
  lines.push("    steps:");
  lines.push('      - action: "run_command"');
  lines.push(`        param: ${yamlString(command)}`);
  lines.push("```");
  return lines.join("\n");
}

async function wakeParent(pane, text) {
  if (!pane) return;
  const buffer = `q-manager-wake-${process.env.Q_MANAGER_CHILD_ID || Date.now()}`;
  await tmux(["set-buffer", "-b", buffer, text]);
  await tmux(["paste-buffer", "-p", "-r", "-b", buffer, "-t", pane]);
  await tmux(["send-keys", "-t", pane, "Enter"]);
}

export default function qManagerChildExtension(pi) {
  const key = Symbol.for("vamos.q_manager_child_extension.loaded");
  if (globalThis[key]) return;
  globalThis[key] = true;

  pi.on("agent_end", async () => {
    const validation = await readJson(
      process.env.Q_MANAGER_VALIDATED_STATUS_PATH || "",
    );
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
      wakeMode: process.env.Q_MANAGER_WAKE_MODE || "validated-only",
      validationStatusPath: process.env.Q_MANAGER_VALIDATED_STATUS_PATH || "",
      managerWakeSuppressed: !shouldWakeManager(validation),
    };
    await writeJson(process.env.Q_MANAGER_STATUS_PATH, status);
    await touch(process.env.Q_MANAGER_DONE_PATH);
    if (shouldWakeManager(validation)) {
      await wakeParent(
        process.env.Q_MANAGER_PARENT_PANE,
        wakeMessage(validation),
      );
    }
  });
}
