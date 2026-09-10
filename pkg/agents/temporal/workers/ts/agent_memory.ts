import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname, join, relative, resolve, sep } from "node:path";
import { defineTool, type ToolDefinition } from "@mariozechner/pi-coding-agent";

export const BOT_HOME_KIND = "bot_home";

export function agentMemoryToolsEnabled(kind?: string): boolean {
  return kind === BOT_HOME_KIND;
}

function speakerRoot(cwd: string): string {
  return resolve(cwd);
}

function resolveUnderRoot(root: string, requested: string): string {
  const abs = resolve(root, requested);
  const rel = relative(root, abs);
  if (rel.startsWith("..")) {
    throw new Error(`path escapes agent directory: ${requested}`);
  }
  if (abs !== root && !abs.startsWith(root + sep)) {
    throw new Error(`path escapes agent directory: ${requested}`);
  }
  return abs;
}

export const memoryWriteTool: ToolDefinition = defineTool({
  name: "memory_write",
  label: "Memory write",
  description:
    "Write a standing memory file under this bot home (MEMORY.md, USER.md, or skills/). Prefer this over sqlite. Do not store Hermes product names or ~/.pi paths.",
  parameters: {
    type: "object",
    properties: {
      path: {
        type: "string",
        description:
          "Relative path such as MEMORY.md, USER.md, or skills/foo.md",
      },
      content: { type: "string", description: "File contents" },
    },
    required: ["path", "content"],
  } as never,
  async execute(_toolCallId, params: { path: string; content: string }) {
    const root = speakerRoot(process.cwd());
    const abs = resolveUnderRoot(root, params.path);
    await mkdir(dirname(abs), { recursive: true });
    await writeFile(abs, params.content, "utf8");
    return {
      content: [{ type: "text" as const, text: `wrote ${params.path}` }],
      details: { path: params.path },
    };
  },
});

export const memorySearchTool: ToolDefinition = defineTool({
  name: "memory_search",
  label: "Memory search",
  description:
    "Grep standing memory files under this bot home. Prefer Pi read/grep when the path is known.",
  parameters: {
    type: "object",
    properties: {
      query: { type: "string", description: "Substring to search" },
      path: {
        type: "string",
        description: "Optional relative file under the bot home",
      },
    },
    required: ["query"],
  } as never,
  async execute(_toolCallId, params: { query: string; path?: string }) {
    const root = speakerRoot(process.cwd());
    const target = params.path
      ? resolveUnderRoot(root, params.path)
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

export function agentMemoryTools(): ToolDefinition[] {
  return [memoryWriteTool, memorySearchTool];
}
