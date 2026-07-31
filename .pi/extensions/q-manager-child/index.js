import { createHash, randomUUID } from "node:crypto";
import { link, mkdir, open, readFile, rm, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { YAMLMap, Scalar, parseDocument } from "yaml";

import {
  buildSettlementEvidence,
  captureOpaqueSettlementEvidence,
  captureOpaqueYamlFences,
  projectPersistedAssistantText,
} from "./opaque-settlement-capture.js";

export {
  buildSettlementEvidence,
  captureOpaqueSettlementEvidence,
  captureOpaqueYamlFences,
  projectPersistedAssistantText,
};
export const DIAGNOSTIC = "q-manager-child/diagnostic";
export const ATTEMPT = "q-manager-child/manager-wake-attempt";

const wakeNames = [
  "VAMOS_MANAGER_WAKE_MANAGER_THREAD_ID",
  "VAMOS_MANAGER_WAKE_PI_SESSION_ID",
  "VAMOS_MANAGER_WAKE_GATEWAY_URL",
  "VAMOS_MANAGER_WAKE_INGRESS_TOKEN",
];
const defaultDependencies = {
  fetch: (...args) => fetch(...args),
  sleep: () => Promise.resolve(),
  retries: 3,
};
let dependencies = defaultDependencies;
export function setDeliveryDependenciesForTest(overrides) {
  dependencies = { ...defaultDependencies, ...overrides };
}

function safeComponent(value, label) {
  if (!/^[A-Za-z0-9_-]+$/.test(value ?? ""))
    throw new Error(`unsafe ${label} path component`);
  return value;
}

export function managedIdentity() {
  const [managerThreadID, piSessionID, gatewayURL, ingressToken] =
    wakeNames.map((name) => process.env[name]);
  const plan = process.env.VAMOS_PLAN_DIR;
  if (
    ![managerThreadID, piSessionID, gatewayURL, ingressToken, plan].every(
      Boolean,
    )
  )
    return undefined;
  return {
    managerThreadID,
    piSessionID: safeComponent(piSessionID, "Pi session"),
    gatewayURL: gatewayURL.replace(/\/$/, ""),
    ingressToken,
    plan,
  };
}

export function messageID(piSessionID, assistantEntryID) {
  return `pi-settlement-v1-${createHash("sha256").update(`${piSessionID}\0${assistantEntryID}`, "utf8").digest("base64url")}`;
}

function evidencePath(identity, id) {
  return join(
    identity.plan,
    ".vamos",
    "sessions",
    "pi",
    identity.piSessionID,
    "settlements",
    `${id}.yaml`,
  );
}

async function syncDirectory(directory) {
  const handle = await open(directory, "r");
  try {
    await handle.sync();
  } finally {
    await handle.close();
  }
}

/** No-replace publication: equal bytes reuse; different bytes never deliver. */
export async function publish(target, bytes) {
  const directory = dirname(target);
  await mkdir(directory, { recursive: true, mode: 0o700 });
  const temporary = join(directory, `.settlement-${randomUUID()}`);
  const file = await open(temporary, "wx", 0o600);
  try {
    await file.writeFile(bytes);
    await file.sync();
  } finally {
    await file.close();
  }
  try {
    await link(temporary, target);
    await syncDirectory(directory);
  } catch (error) {
    if (error.code !== "EEXIST") throw error;
    const existing = await readFile(target);
    if (Buffer.compare(existing, bytes))
      throw new Error("immutable manager-wake settlement conflict");
  } finally {
    await rm(temporary, { force: true });
  }
  return target;
}

function scalar(map, key) {
  const pair = map.items.find(
    (item) => item.key instanceof Scalar && item.key.value === key,
  );
  return pair?.value instanceof Scalar ? pair.value.value : undefined;
}

/** Reads the published record; delivery never observes a later assistant message. */
export async function deliveryBody(target) {
  const document = parseDocument(await readFile(target, "utf8"), {
    uniqueKeys: true,
    merge: false,
    prettyErrors: false,
  });
  if (
    document.errors.length ||
    document.warnings.length ||
    !(document.contents instanceof YAMLMap)
  )
    throw new Error("invalid published manager-wake evidence");
  const fields = Object.fromEntries(
    [
      "version",
      "manager_thread_id",
      "pi_session_id",
      "message_id",
      "raw_response",
    ].map((key) => [key, scalar(document.contents, key)]),
  );
  if (
    fields.version !== 1 ||
    ![fields.manager_thread_id, fields.pi_session_id, fields.message_id].every(
      (value) => typeof value === "string",
    ) ||
    typeof fields.raw_response !== "string"
  )
    throw new Error("invalid published manager-wake identity");
  return {
    version: 1,
    manager_thread_id: fields.manager_thread_id,
    pi_session_id: fields.pi_session_id,
    message_id: fields.message_id,
    message: fields.raw_response,
  };
}

async function recordAttempt(target, outcome) {
  // This is local diagnostics only, never an acknowledgement protocol.
  await writeFile(`${target}.attempt.json`, JSON.stringify(outcome), {
    mode: 0o600,
  });
}

export async function deliverPublished(identity, target, { appendEntry } = {}) {
  const body = await deliveryBody(target);
  let lastError;
  for (let attempt = 1; attempt <= dependencies.retries; attempt++) {
    try {
      const response = await dependencies.fetch(
        `${identity.gatewayURL}/vamos/manager-wake`,
        {
          method: "POST",
          headers: {
            Authorization: `Bearer ${identity.ingressToken}`,
            "Content-Type": "application/json",
          },
          body: JSON.stringify(body),
        },
      );
      if (!response.ok) throw new Error(`manager-wake HTTP ${response.status}`);
      await recordAttempt(target, { status: "success", attempt });
      appendEntry?.(ATTEMPT, {
        message_id: body.message_id,
        status: "success",
        attempt,
      });
      return body;
    } catch (error) {
      lastError = error;
      if (attempt < dependencies.retries) await dependencies.sleep(attempt);
    }
  }
  await recordAttempt(target, {
    status: "failed",
    attempts: dependencies.retries,
  });
  appendEntry?.(ATTEMPT, {
    message_id: body.message_id,
    status: "failed",
    attempts: dependencies.retries,
  });
  throw lastError;
}

function terminalAssistant(ctx) {
  return [...ctx.sessionManager.getBranch()]
    .reverse()
    .find(
      (entry) =>
        entry?.type === "message" && entry.message?.role === "assistant",
    );
}

export default function qManagerChildExtension(pi) {
  const identity = managedIdentity();
  if (!identity) return;
  pi.on("agent_settled", async (_event, ctx) => {
    const assistant = terminalAssistant(ctx);
    if (!assistant || typeof assistant.id !== "string" || !assistant.id) {
      pi.appendEntry(DIAGNOSTIC, { code: "assistant_settlement_unavailable" });
      return;
    }
    const id = messageID(identity.piSessionID, assistant.id);
    const target = evidencePath(identity, id);
    const rawResponse = projectPersistedAssistantText(
      assistant.message.content ?? [],
    );
    const bytes = buildSettlementEvidence(
      {
        managerThreadID: identity.managerThreadID,
        piSessionID: identity.piSessionID,
        messageID: id,
      },
      rawResponse,
    );
    try {
      await publish(target, bytes);
    } catch (error) {
      pi.appendEntry(DIAGNOSTIC, {
        code: "assistant_settlement_conflict",
        message_id: id,
      });
      return;
    }
    try {
      await deliverPublished(identity, target, pi);
    } catch {
      pi.appendEntry(DIAGNOSTIC, {
        code: "manager_wake_delivery_failed",
        message_id: id,
      });
    }
  });
  pi.on("session_before_compact", (event) => {
    pi.appendEntry(DIAGNOSTIC, {
      code: "context_compaction_cancelled",
      source: event.reason ?? "manual",
    });
    return { cancel: true };
  });
  pi.on("session_before_tree", (event) =>
    event.preparation.userWantsSummary ? { cancel: true } : undefined,
  );
}

export { evidencePath, safeComponent };
