import { createHash, randomUUID } from "node:crypto";
import { writeSync } from "node:fs";
import { link, mkdir, open, readFile, rm } from "node:fs/promises";
import { dirname, isAbsolute, join } from "node:path";

import {
  buildSettlementEvidenceV1,
  projectPersistedAssistantText,
} from "./opaque-settlement-capture.js";

export const DIAGNOSTIC = "q-manager-child/diagnostic";

const managedNames = [
  "HERMES_SESSION_ID",
  "PI_SESSION_ID",
  "VAMOS_PLAN_DIR",
  "VAMOS_HERMES_HANDOFF_FD",
];
function safeComponent(value, label) {
  if (!/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/.test(value ?? ""))
    throw new Error(`unsafe ${label} path component`);
  return value;
}

export function deriveLaunchNonce(piSessionID) {
  return createHash("sha256")
    .update("vamos-hermes-launch-nonce-v1", "ascii")
    .update(Buffer.from([0]))
    .update(piSessionID, "utf8")
    .digest("hex");
}

export function managedIdentity() {
  const [hermesSessionID, piSessionID, plan, rawFD] = managedNames.map(
    (name) => process.env[name],
  );
  if (![hermesSessionID, piSessionID, plan, rawFD].every(Boolean))
    return undefined;
  if (
    new TextEncoder().encode(hermesSessionID).length > 1024 ||
    Array.from(hermesSessionID).some((character) => {
      const code = character.codePointAt(0);
      return code <= 0x1f || (code >= 0x7f && code <= 0x9f);
    })
  )
    throw new Error("invalid Hermes session ID");
  if (!isAbsolute(plan)) throw new Error("invalid plan directory");
  const handoffFD = Number(rawFD);
  if (!Number.isSafeInteger(handoffFD) || handoffFD < 3)
    throw new Error("invalid handoff descriptor");
  return {
    hermesSessionID,
    piSessionID: safeComponent(piSessionID, "Pi session"),
    plan,
    handoffFD,
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

export function writeHandoffFrame(identity, id) {
  const payload = Buffer.from(
    JSON.stringify({
      version: 1,
      launch_nonce: deriveLaunchNonce(identity.piSessionID),
      pi_session_id: identity.piSessionID,
      message_id: id,
    }),
    "utf8",
  );
  if (payload.length < 1 || payload.length > 4096)
    throw new Error("handoff frame exceeds size limit");
  const frame = Buffer.allocUnsafe(4 + payload.length);
  frame.writeUInt32BE(payload.length, 0);
  payload.copy(frame, 4);
  for (let offset = 0; offset < frame.length; ) {
    const written = writeSync(identity.handoffFD, frame, offset);
    if (written < 1) throw new Error("short handoff frame write");
    offset += written;
  }
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
    const bytes = buildSettlementEvidenceV1(
      {
        hermesSessionID: identity.hermesSessionID,
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
      writeHandoffFrame(identity, id);
    } catch {
      pi.appendEntry(DIAGNOSTIC, {
        code: "settlement_handoff_failed",
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
