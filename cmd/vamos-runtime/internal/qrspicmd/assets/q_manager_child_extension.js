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

async function runChildComplete() {
  const stateFile = process.env.Q_MANAGER_STATE_FILE || "";
  const childID = process.env.Q_MANAGER_CHILD_ID || "";
  if (!stateFile || !childID) return null;
  return new Promise((resolve) => {
    const child = spawn(
      "vamos",
      [
        "qrspi",
        "child-complete",
        "--state-file",
        stateFile,
        "--child-id",
        childID,
        "--output",
        "json",
      ],
      { stdio: ["ignore", "pipe", "pipe"] },
    );
    let stdout = "";
    child.stdout.on("data", (chunk) => {
      stdout += chunk.toString();
    });
    child.on("exit", () => {
      try {
        resolve(JSON.parse(stdout));
      } catch {
        resolve(null);
      }
    });
    child.on("error", () => resolve(null));
  });
}

function shouldWakeManager(validation) {
  if (!validation) return false;
  return (
    validation.validated === true ||
    validation.managerNeeded === true ||
    validation.retryExhausted === true
  );
}

export default function qManagerChildExtension(pi) {
  const key = Symbol.for("vamos.q_manager_child_extension.loaded");
  if (globalThis[key]) return;
  globalThis[key] = true;

  pi.on("agent_end", async () => {
    const produced = await runChildComplete();
    const validation =
      produced ||
      (await readJson(process.env.Q_MANAGER_VALIDATED_STATUS_PATH || ""));
    const status = {
      event: "agent_end",
      stage: process.env.Q_MANAGER_STAGE || "",
      childId: process.env.Q_MANAGER_CHILD_ID || "",
      managerRunId: process.env.Q_MANAGER_MANAGER_RUN_ID || "",
      stateFile: process.env.Q_MANAGER_STATE_FILE || "",
      planDir: process.env.Q_MANAGER_PLAN_DIR || "",
      sessionId:
        process.env.Q_MANAGER_SESSION_ID || process.env.SESSION_ID || "",
      sessionDir:
        process.env.Q_MANAGER_SESSION_DIR || process.env.SESSION_DIR || "",
      sessionPath: process.env.SESSION_PATH || "",
      finishedAt: new Date().toISOString(),
      wakeTarget: process.env.Q_MANAGER_PARENT_PANE || "",
      wakeMode: process.env.Q_MANAGER_WAKE_MODE || "validated-only",
      validationStatusPath: process.env.Q_MANAGER_VALIDATED_STATUS_PATH || "",
      activeChildGeneration: validation?.childGeneration || "",
      helperProducedStatus: produced !== null,
      managerWakeSuppressed: !shouldWakeManager(validation),
      wakeDeliveryMode: validation?.wake?.mode || "",
      wakeDeliveryReason: validation?.wake?.reason || "",
    };
    await writeJson(process.env.Q_MANAGER_STATUS_PATH, status);
    await touch(process.env.Q_MANAGER_DONE_PATH);
  });
}
