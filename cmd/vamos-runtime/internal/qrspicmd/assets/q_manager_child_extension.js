export default function qManagerChildExtension(pi) {
  const key = Symbol.for("vamos.q_manager_child_extension.loaded");
  if (globalThis[key]) return;
  globalThis[key] = true;

  pi.on("agent_end", async () => {});
}
