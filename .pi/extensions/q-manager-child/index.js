import { randomUUID } from "node:crypto";
import { link, mkdir, open, readFile, rm } from "node:fs/promises";
import { dirname, join, relative, resolve } from "node:path";

import { captureOpaqueSettlementEvidence } from "./opaque-settlement-capture.js";

export {
  captureOpaqueSettlementEvidence,
  captureOpaqueYamlFences,
  projectPersistedAssistantText,
} from "./opaque-settlement-capture.js";

export const BRIDGE = "q-manager-child/assistant-bridge";
export const PENDING = "q-manager-child/settlement-pending";
export const CONSUMED = "q-manager-child/settlement-consumed";
export const DIAGNOSTIC = "q-manager-child/diagnostic";

const custom = (entry, kind) =>
  entry?.type === "custom" && entry.customType === kind;
const data = (entry) => entry?.data ?? entry?.content ?? {};
const entries = (ctx) => ctx.sessionManager.getBranch();

function safeComponent(value, label) {
  if (!/^[A-Za-z0-9_-]+$/.test(value ?? ""))
    throw new Error(`unsafe ${label} path component`);
  return value;
}

function managedIdentity() {
  const session = process.env.PI_SESSION_ID;
  const plan = process.env.VAMOS_PLAN_DIR;
  const thread = process.env.VAMOS_HERMES_THREAD_ID;
  if (!session || !plan || !thread) return undefined;
  return {
    session: safeComponent(session, "session"),
    plan,
    thread: safeComponent(thread, "manager thread"),
  };
}

function planRelative(plan) {
  const root = process.env.VAMOS_THOUGHTS_ROOT;
  if (!root)
    throw new Error(
      "VAMOS_THOUGHTS_ROOT is required for settlement containment",
    );
  for (const path of [root, plan]) {
    if (
      !path ||
      path.split(/[\\/]/).some((part) => part === "." || part === "..")
    )
      throw new Error(
        "settlement plan must be a contained thoughts-relative path",
      );
  }
  const value = relative(resolve(root), resolve(plan)).replaceAll("\\", "/");
  if (
    !value ||
    value.startsWith("/") ||
    value.split("/").some((part) => !part || part === "." || part === "..")
  )
    throw new Error("settlement plan must be thoughts-relative");
  return value;
}

function fenceBody(raw) {
  const first = raw.indexOf("\n");
  const last = raw.lastIndexOf("\n", raw.length - 2);
  return first < 0 || last < first ? "" : raw.slice(first + 1, last + 1);
}

/** JavaScript owns these exact persisted JSON bytes. */
export function serializeSettlement(identity, assistantEntryID, content) {
  const evidence = captureOpaqueSettlementEvidence(content);
  const envelope = {
    version: 1,
    kind: "opaque_pi_settlement",
    session: identity.session,
    plan: planRelative(identity.plan),
    manager_thread: identity.thread,
    final_entry_id: assistantEntryID,
    fences: evidence.fencedYamlBlocks.map((block) => fenceBody(block.raw)),
  };
  return Buffer.from(JSON.stringify(envelope));
}

async function syncDirectory(directory) {
  const handle = await open(directory, "r");
  try {
    await handle.sync();
  } finally {
    await handle.close();
  }
}

/** Immutable, no-replace publication; equal bytes are a successful retry. */
export async function publish(plan, session, entry, bytes) {
  safeComponent(session, "session");
  safeComponent(entry, "final entry");
  const target = join(
    plan,
    ".vamos",
    "sessions",
    "pi",
    session,
    "settlements",
    `${entry}.json`,
  );
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
    try {
      await syncDirectory(directory);
    } catch (error) {
      await rm(target, { force: true });
      await syncDirectory(directory);
      throw error;
    }
  } catch (error) {
    if (error.code !== "EEXIST") throw error;
    const existing = await readFile(target);
    if (Buffer.compare(existing, bytes))
      throw new Error("immutable opaque settlement identity conflict");
  } finally {
    await rm(temporary, { force: true });
  }
  return target;
}

function bridgeFor(ctx) {
  const branch = entries(ctx);
  return [...branch]
    .reverse()
    .find(
      (entry) =>
        custom(entry, BRIDGE) &&
        !branch.some(
          (used) => custom(used, CONSUMED) && data(used).bridge_id === entry.id,
        ),
    );
}

function pendingFor(ctx, bridgeID) {
  const branch = entries(ctx);
  return [...branch]
    .reverse()
    .find(
      (entry) =>
        custom(entry, PENDING) &&
        data(entry).bridge_id === bridgeID &&
        !branch.some(
          (used) =>
            custom(used, CONSUMED) && data(used).pending_id === entry.id,
        ),
    );
}

export default function qManagerChildExtension(pi) {
  const identity = managedIdentity();
  if (!identity) return;
  planRelative(identity.plan);

  pi.on("turn_end", async (event, ctx) => {
    const matches = entries(ctx).filter(
      (entry) =>
        entry.type === "message" &&
        entry.message === event.message &&
        entry.message?.role === "assistant",
    );
    if (matches.length !== 1) {
      pi.appendEntry(DIAGNOSTIC, {
        code: "assistant_bridge_unavailable",
        turn: event.turnIndex,
      });
      return;
    }
    if (event.message.content?.some((part) => part?.type === "toolCall"))
      return;
    pi.appendEntry(BRIDGE, {
      assistant_entry_id: matches[0].id,
      turn: event.turnIndex,
    });
  });

  pi.on("agent_settled", async (_event, ctx) => {
    const bridge = bridgeFor(ctx);
    if (!bridge) return;
    const assistant = ctx.sessionManager.getEntry?.(
      data(bridge).assistant_entry_id,
    );
    if (
      !assistant ||
      assistant.type !== "message" ||
      assistant.message?.role !== "assistant"
    ) {
      pi.appendEntry(DIAGNOSTIC, {
        code: "assistant_bridge_invalid",
        bridge_id: bridge.id,
      });
      return;
    }
    let pending = pendingFor(ctx, bridge.id);
    if (!pending) {
      const bytes = serializeSettlement(
        identity,
        assistant.id,
        assistant.message.content ?? [],
      );
      pi.appendEntry(PENDING, {
        bridge_id: bridge.id,
        session: identity.session,
        final_entry_id: assistant.id,
        bytes_base64: bytes.toString("base64"),
      });
      pending = pendingFor(ctx, bridge.id);
    }
    const record = data(pending);
    await publish(
      identity.plan,
      record.session,
      record.final_entry_id,
      Buffer.from(record.bytes_base64, "base64"),
    );
    pi.appendEntry(CONSUMED, {
      bridge_id: bridge.id,
      pending_id: pending.id,
      final_entry_id: record.final_entry_id,
    });
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

export { managedIdentity, planRelative, safeComponent };
