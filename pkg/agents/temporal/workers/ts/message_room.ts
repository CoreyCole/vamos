import { randomUUID } from "node:crypto";
import { defineTool, type ToolDefinition } from "@mariozechner/pi-coding-agent";
import { fetchWithTimeout } from "./conversation.js";
import type { ConversationRunInput } from "./types.js";

export interface MessageRoomToolContext {
  enqueueEndpoint: string;
  fromAgentID: string;
  originThreadID: string;
}

export function enqueueEndpointFromCallback(callbackEndpoint: string): string {
  const trimmed = callbackEndpoint.trim();
  if (trimmed.endsWith("/internal/agent-chat/events")) {
    return `${trimmed.slice(0, -"/internal/agent-chat/events".length)}/internal/agent-chat/enqueue`;
  }
  return trimmed.replace(/\/?$/, "") + "/internal/agent-chat/enqueue";
}

export function messageRoomContextFromRun(
  input: ConversationRunInput,
): MessageRoomToolContext {
  return {
    enqueueEndpoint: enqueueEndpointFromCallback(input.callback_endpoint),
    fromAgentID: input.room?.from_agent_id?.trim() ?? "",
    originThreadID: input.thread_id,
  };
}

function internalHeaders(): Record<string, string> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  const token = process.env.VAMOS_INTERNAL_TOKEN;
  if (token) {
    headers["X-Vamos-Internal-Token"] = token;
  }
  return headers;
}

export function messageRoomTool(ctx: MessageRoomToolContext): ToolDefinition {
  return defineTool({
    name: "message_room",
    label: "Message room",
    description:
      "Send markdown to another bot (roster slug) or a plan group (thoughts-relative plan directory). " +
      "Never a thread UUID, bot-home URL, or thoughts/agents path. " +
      "Same-pair replies stay on this pairwise thread. " +
      "Does not post into a bot home.",
    parameters: {
      type: "object",
      properties: {
        to: {
          type: "string",
          description:
            "Destination roster slug or thoughts-relative plan directory",
        },
        body: { type: "string", description: "Markdown body" },
      },
      required: ["to", "body"],
    } as never,
    async execute(_toolCallId, params: { to: string; body: string }) {
      const to = params.to.trim();
      const body = params.body.trim();
      if (!to || !body) {
        throw new Error("to and body are required");
      }
      const op_id = randomUUID();
      const payload = {
        to,
        body,
        op_id,
        from_agent_id: ctx.fromAgentID,
        thread_id: ctx.originThreadID,
        from_kind: "agent",
      };
      const response = await fetchWithTimeout(ctx.enqueueEndpoint, {
        method: "POST",
        headers: internalHeaders(),
        body: JSON.stringify(payload),
        timeoutMS: 15_000,
        operation: "message_room enqueue",
      });
      if (!response.ok) {
        const text = await response.text().catch(() => "");
        throw new Error(
          `message_room enqueue failed: ${response.status} ${text}`.trim(),
        );
      }
      return {
        content: [
          {
            type: "text" as const,
            text: `enqueued to ${to}`,
          },
        ],
        details: { to, op_id },
      };
    },
  });
}

export function messageRoomTools(
  ctx: MessageRoomToolContext,
): ToolDefinition[] {
  return [messageRoomTool(ctx)];
}
