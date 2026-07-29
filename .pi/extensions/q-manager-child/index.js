import { relative, resolve } from "node:path";

export {
  captureOpaqueSettlementEvidence,
  captureOpaqueYamlFences,
  projectPersistedAssistantText,
} from "./opaque-settlement-capture.js";

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

export default function qManagerChildExtension() {
  const identity = managedIdentity();
  if (!identity) return;
  planRelative(identity.plan);
}

export { managedIdentity, planRelative, safeComponent };
