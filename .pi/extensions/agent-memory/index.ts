import type { ExtensionAPI } from "@mariozechner/pi-coding-agent";

export default function agentMemory(pi: ExtensionAPI) {
  pi.registerTool({
    name: "memory_write",
    label: "Memory write",
    description:
      "Write MEMORY.md, USER.md, or skills/ under this bot home. File tools only. No sqlite.",
    parameters: {
      type: "object",
      properties: {
        path: { type: "string" },
        content: { type: "string" },
      },
      required: ["path", "content"],
    } as never,
    async execute(_id, params: { path: string; content: string }) {
      const { mkdir, writeFile } = await import("node:fs/promises");
      const { dirname, resolve, relative, sep } = await import("node:path");
      const root = resolve(process.cwd());
      const abs = resolve(root, params.path);
      const rel = relative(root, abs);
      if (rel.startsWith("..")) {
        throw new Error(`path escapes agent directory: ${params.path}`);
      }
      if (abs !== root && !abs.startsWith(root + sep)) {
        throw new Error(`path escapes agent directory: ${params.path}`);
      }
      await mkdir(dirname(abs), { recursive: true });
      await writeFile(abs, params.content, "utf8");
      return {
        content: [{ type: "text" as const, text: `wrote ${params.path}` }],
        details: { path: params.path },
      };
    },
  });
  pi.registerTool({
    name: "memory_search",
    label: "Memory search",
    description: "Grep MEMORY.md (or a listed file) under this bot home.",
    parameters: {
      type: "object",
      properties: {
        query: { type: "string" },
        path: { type: "string" },
      },
      required: ["query"],
    } as never,
    async execute(_id, params: { query: string; path?: string }) {
      const { readFile } = await import("node:fs/promises");
      const { join, resolve } = await import("node:path");
      const root = resolve(process.cwd());
      const target = params.path
        ? resolve(root, params.path)
        : join(root, "MEMORY.md");
      let text = "";
      try {
        text = await readFile(target, "utf8");
      } catch {
        return {
          content: [{ type: "text" as const, text: "no memory file" }],
          details: { hits: 0 },
        };
      }
      const hits = text
        .split("\n")
        .filter((line) =>
          line.toLowerCase().includes(params.query.toLowerCase()),
        );
      return {
        content: [
          {
            type: "text" as const,
            text: hits.length ? hits.join("\n") : "no matches",
          },
        ],
        details: { hits: hits.length },
      };
    },
  });
}
