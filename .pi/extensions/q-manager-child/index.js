import { randomUUID } from "node:crypto";
import { link, mkdir, open, readFile, rm } from "node:fs/promises";
import { dirname, join, relative, resolve } from "node:path";

const BRIDGE = "q-manager-child/assistant-bridge";
const PENDING = "q-manager-child/settlement-pending";
const CONSUMED = "q-manager-child/settlement-consumed";
const LIMIT_PENDING = "q-manager-child/context-limit-pending";
const LIMIT_CONSUMED = "q-manager-child/context-limit-consumed";
const DIAGNOSTIC = "q-manager-child/diagnostic";

const custom = (entry, kind) =>
  entry?.type === "custom" && entry.customType === kind;
const data = (entry) => entry?.data ?? entry?.content ?? {};
const entries = (ctx) => ctx.sessionManager.getBranch();
const append = (pi, type, value) => pi.appendEntry(type, value);
const now = () => new Date().toISOString().replace(/\.\d{3}Z$/, "Z");
const text = (message) =>
  (message?.content ?? [])
    .filter((part) => part.type === "text")
    .map((part) => part.text)
    .join("");

// This is the yaml.v3 Marshal layout for the Task 2 PiCheckpoint struct. Keep
// field order, omission rules, map ordering, and scalar spelling byte-locked
// with CanonicalPiCheckpoint: ReadPiCheckpoint rejects noncanonical files.
function yamlString(value, indent = "") {
  if (value.includes("\n")) {
    const trailing = value.match(/\n+$/)?.[0].length ?? 0;
    const indicator = trailing > 1 ? "|+" : trailing === 1 ? "|" : "|-";
    const body = trailing ? value.slice(0, -trailing) : value;
    return `${indicator}\n${body
      .split("\n")
      .map((line) => `${indent}    ${line}`)
      .join("\n")}`;
  }
  if (/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$/.test(value)) return value;
  if (
    value === "" ||
    /^(?:null|~|true|false|yes|no|on|off|[-+]?(?:\d+|\.inf|\.nan))$/i.test(
      value,
    ) ||
    /[:#{}\[\],&*!|>'"%@`]/.test(value) ||
    /^\s|\s$/.test(value)
  )
    return JSON.stringify(value);
  return value;
}
function yamlValue(value, indent = "") {
  if (typeof value === "string") return yamlString(value, indent);
  if (typeof value === "number" || typeof value === "boolean")
    return String(value);
  if (value === null) return "null";
  throw new Error("intent_metadata contains an unsupported value");
}
function yamlMap(value, indent) {
  return Object.keys(value)
    .sort()
    .map((key) => {
      if (!/^[A-Za-z0-9_-]+$/.test(key))
        throw new Error("intent_metadata contains an unsafe key");
      const item = value[key];
      if (Array.isArray(item)) {
        if (!item.length) return `${indent}${key}: []`;
        return `${indent}${key}:\n${item.map((entry) => `${indent}    - ${yamlValue(entry, indent + "    ")}`).join("\n")}`;
      }
      if (item && typeof item === "object")
        return `${indent}${key}:\n${yamlMap(item, indent + "    ")}`;
      return `${indent}${key}: ${yamlValue(item, indent)}`;
    })
    .join("\n");
}
function yaml(checkpoint) {
  const fields = [
    "version",
    "session",
    "plan",
    "manager_thread",
    "final_entry_id",
    "outcome",
    "next",
    "created_at",
    "recommendation",
    "summary",
  ];
  let out = "";
  for (const field of fields)
    if (checkpoint[field] !== undefined && checkpoint[field] !== "")
      out += `${field}: ${typeof checkpoint[field] === "number" ? checkpoint[field] : yamlString(checkpoint[field])}\n`;
  for (const field of [
    "artifacts",
    "raw_response",
    "raw_yaml",
    "intent_metadata",
    "diagnostics",
  ]) {
    const value = checkpoint[field];
    if (Array.isArray(value) && value.length) {
      out += `${field}:\n${value.map((item) => `    - ${yamlString(item, "    ")}`).join("\n")}\n`;
    } else if (typeof value === "string" && value !== "")
      out += `${field}: ${yamlString(value)}\n`;
    else if (
      field === "intent_metadata" &&
      value &&
      typeof value === "object" &&
      Object.keys(value).length
    )
      out += `${field}:\n${yamlMap(value, "    ")}\n`;
  }
  return out;
}
function safeComponent(value, label) {
  if (!/^[A-Za-z0-9_-]+$/.test(value ?? ""))
    throw new Error(`unsafe ${label} path component`);
  return value;
}
function managedIdentity() {
  const session = process.env.PI_SESSION_ID,
    plan = process.env.VAMOS_PLAN_DIR,
    thread = process.env.VAMOS_HERMES_THREAD_ID;
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
      "VAMOS_THOUGHTS_ROOT is required for checkpoint containment",
    );
  for (const path of [root, plan]) {
    if (
      !path ||
      path.split(/[\\/]/).some((part) => part === "." || part === "..")
    )
      throw new Error(
        "checkpoint plan must be a contained thoughts-relative path",
      );
  }
  const value = relative(resolve(root), resolve(plan)).replaceAll("\\", "/");
  if (
    !value ||
    value.startsWith("/") ||
    value.split("/").some((part) => !part || part === "." || part === "..")
  )
    throw new Error("checkpoint plan must be thoughts-relative");
  return value;
}
async function syncDirectory(directory) {
  const handle = await open(directory, "r");
  try {
    await handle.sync();
  } finally {
    await handle.close();
  }
}
async function publish(plan, session, entry, bytes) {
  safeComponent(session, "session");
  safeComponent(entry, "final entry");
  const target = join(
    plan,
    ".vamos",
    "sessions",
    "pi",
    session,
    "checkpoints",
    `${entry}.yaml`,
  );
  const directory = dirname(target);
  await mkdir(directory, { recursive: true, mode: 0o700 });
  const temporary = join(
    directory,
    `.checkpoint-${process.pid}-${Date.now()}-${Math.random().toString(16).slice(2)}`,
  );
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
      throw new Error("immutable checkpoint identity conflict");
  } finally {
    await rm(temporary, { force: true });
  }
  return target;
}
function unconsumed(ctx, kind, consumed, key, value) {
  return [...entries(ctx)]
    .reverse()
    .find(
      (entry) =>
        custom(entry, kind) &&
        data(entry)[key] === value &&
        !entries(ctx).some(
          (used) => custom(used, consumed) && data(used)[key] === value,
        ),
    );
}
function bridgeFor(ctx) {
  return [...entries(ctx)]
    .reverse()
    .find(
      (entry) =>
        custom(entry, BRIDGE) &&
        !entries(ctx).some(
          (used) => custom(used, CONSUMED) && data(used).bridge_id === entry.id,
        ),
    );
}
function scalar(value) {
  value = value.trim().replace(/\s+#.*$/, "");
  if (/^['"].*['"]$/.test(value)) return value.slice(1, -1);
  if (value === "true") return true;
  if (value === "false") return false;
  if (value === "null" || value === "~") return null;
  if (/^-?\d+(?:\.\d+)?$/.test(value)) return Number(value);
  return value;
}
function intentSource(raw) {
  const fenced = [...raw.matchAll(/```(?:yaml|yml)\s*\n([\s\S]*?)```/gi)].map(
    (m) => m[1].trim(),
  );
  if (fenced.length) return fenced.join("\n---\n");
  const start = raw.search(/^(?:state|outcome|status|result|decision):/im);
  return start < 0 ? "" : raw.slice(start).trim();
}
function lifecycle(raw) {
  const source = intentSource(raw);
  const lines = source.split("\n");
  const fields = {};
  const metadata = {};
  const artifacts = [];
  let current;
  for (const line of lines) {
    if (!line.trim() || line.trim() === "---") continue;
    const item = line.match(/^\s*-\s+(.+)$/);
    if (item && current === "artifacts") {
      artifacts.push(scalar(item[1]));
      continue;
    }
    const match = line.match(/^(\s*)([A-Za-z_][A-Za-z0-9_-]*):\s*(.*)$/);
    if (!match) continue;
    const [, indent, key, value] = match;
    if (
      indent &&
      current === "result" &&
      ["state", "outcome", "status", "result", "decision"].includes(key)
    )
      fields[key] = scalar(value);
    else if (indent && metadata[current])
      metadata[current][key] = scalar(value);
    else if (!indent) {
      current = key;
      if (
        [
          "state",
          "outcome",
          "status",
          "result",
          "decision",
          "recommendation",
          "next",
          "summary",
          "explanation",
          "artifact",
          "artifacts",
        ].includes(key)
      ) {
        if (value) fields[key] = scalar(value);
      } else if (value) metadata[key] = scalar(value);
      else metadata[key] = {};
    }
  }
  const values = ["state", "outcome", "status", "result", "decision"]
    .filter((key) => typeof fields[key] === "string")
    .map((key) => fields[key]);
  const valid = ["complete", "handoff", "needs_human", "blocked", "error"];
  const diagnostics = [];
  if (values.length !== 1)
    diagnostics.push(
      values.length
        ? "duplicate or conflicting lifecycle values"
        : "missing lifecycle value",
    );
  else if (!valid.includes(values[0]))
    diagnostics.push(`unknown lifecycle value ${JSON.stringify(values[0])}`);
  if (fields.artifact) artifacts.push(fields.artifact);
  return {
    outcome: diagnostics.length ? "blocked" : values[0],
    recommendation:
      typeof fields.recommendation === "string"
        ? fields.recommendation
        : typeof fields.next === "string"
          ? fields.next
          : undefined,
    summary:
      typeof fields.summary === "string"
        ? fields.summary
        : typeof fields.explanation === "string"
          ? fields.explanation
          : undefined,
    artifacts: artifacts.length ? artifacts : undefined,
    raw_yaml: source || undefined,
    intent_metadata: Object.keys(metadata).length ? metadata : undefined,
    diagnostics,
  };
}
function checkpoint(identity, bridge, message, limit) {
  const intent = limit
    ? { outcome: "blocked", diagnostics: [`context_limit_${limit.source}`] }
    : lifecycle(text(message));
  return {
    version: 2,
    session: identity.session,
    plan: planRelative(identity.plan),
    manager_thread: identity.thread,
    final_entry_id: data(bridge).assistant_entry_id,
    outcome: intent.outcome,
    next: "none",
    created_at: now(),
    recommendation: intent.recommendation,
    summary: intent.summary,
    raw_response: text(message),
    raw_yaml: intent.raw_yaml,
    intent_metadata: intent.intent_metadata,
    artifacts: intent.artifacts,
    diagnostics: intent.diagnostics,
  };
}

export default function qManagerChildExtension(pi) {
  // Project-local extensions are loaded in ordinary Pi sessions too.  A child is
  // managed only when the launcher supplies all three immutable identities.
  const identity = managedIdentity();
  if (!identity) return;
  // Reject bad configuration before registering any lifecycle hooks that can
  // append session records or write a checkpoint.
  planRelative(identity.plan);
  let run;
  pi.on("agent_start", () => {
    // Pi events deliberately have no provider run ID. This UUID is persisted
    // with the bridge/marker, so restart and a later steer cannot share one.
    run = randomUUID();
  });
  pi.on("turn_end", async (event, ctx) => {
    const matches = entries(ctx).filter(
      (entry) =>
        entry.type === "message" &&
        entry.message === event.message &&
        entry.message?.role === "assistant",
    );
    if (matches.length !== 1)
      return append(pi, DIAGNOSTIC, {
        code: "assistant_bridge_unavailable",
        turn: event.turnIndex,
      });
    append(pi, BRIDGE, {
      assistant_entry_id: matches[0].id,
      turn: event.turnIndex,
      active_run: run ?? randomUUID(),
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
    )
      return append(pi, DIAGNOSTIC, {
        code: "assistant_bridge_invalid",
        bridge_id: bridge.id,
      });
    let pending = unconsumed(ctx, PENDING, CONSUMED, "bridge_id", bridge.id);
    let marker;
    if (!pending) {
      marker = [...entries(ctx)]
        .reverse()
        .find(
          (entry) =>
            custom(entry, LIMIT_PENDING) &&
            data(entry).assistant_entry_id ===
              data(bridge).assistant_entry_id &&
            data(entry).bridge_id === bridge.id &&
            data(entry).active_run === data(bridge).active_run &&
            !entries(ctx).some(
              (used) =>
                custom(used, LIMIT_CONSUMED) &&
                data(used).marker_id === entry.id,
            ),
        );
      const payload = checkpoint(
        identity,
        bridge,
        assistant.message,
        marker && data(marker),
      );
      const bytes = yaml(payload);
      append(pi, PENDING, {
        bridge_id: bridge.id,
        payload,
        bytes,
        marker_id: marker?.id,
      });
      pending = unconsumed(ctx, PENDING, CONSUMED, "bridge_id", bridge.id);
    }
    const record = data(pending);
    await publish(
      identity.plan,
      record.payload.session,
      record.payload.final_entry_id,
      Buffer.from(record.bytes),
    );
    if (record.marker_id)
      append(pi, LIMIT_CONSUMED, {
        marker_id: record.marker_id,
        bridge_id: bridge.id,
      });
    append(pi, CONSUMED, {
      bridge_id: bridge.id,
      pending_id: pending.id,
      final_entry_id: record.payload.final_entry_id,
    });
  });
  pi.on("session_before_compact", async (event, ctx) => {
    const bridge = [...entries(ctx)]
      .reverse()
      .find((entry) => custom(entry, BRIDGE));
    const source = event.reason ?? "manual";
    if (source === "manual") {
      // AgentSession.compact waits for settlement before this hook. Manual
      // compaction is therefore retrospective telemetry, never a checkpoint
      // gate or a pending marker.
      append(
        pi,
        DIAGNOSTIC,
        bridge
          ? {
              code: "context_limit_after_settlement",
              source,
              assistant_entry_id: data(bridge).assistant_entry_id,
            }
          : { code: "context_limit_unattributed", source },
      );
      return { cancel: true };
    }
    if (source !== "threshold" && source !== "overflow") {
      append(pi, DIAGNOSTIC, { code: "context_limit_unattributed", source });
      return undefined;
    }
    const active =
      bridge &&
      bridgeFor(ctx)?.id === bridge.id &&
      run === data(bridge).active_run &&
      (event.signal ?? ctx.signal);
    if (active)
      append(pi, LIMIT_PENDING, {
        source,
        bridge_id: bridge.id,
        assistant_entry_id: data(bridge).assistant_entry_id,
        active_run: data(bridge).active_run,
        marker: randomUUID(),
      });
    else
      append(
        pi,
        DIAGNOSTIC,
        !bridge
          ? { code: "context_limit_unattributed", source }
          : {
              code: "context_limit_after_settlement",
              source,
              assistant_entry_id: data(bridge).assistant_entry_id,
            },
      );
    return { cancel: true };
  });
  pi.on("session_before_tree", (event) =>
    event.preparation.userWantsSummary ? { cancel: true } : undefined,
  );
}
export {
  BRIDGE,
  PENDING,
  CONSUMED,
  LIMIT_PENDING,
  LIMIT_CONSUMED,
  DIAGNOSTIC,
  publish,
  yaml,
};
