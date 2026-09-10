import assert from "node:assert/strict";
import test from "node:test";
import {
  enqueueEndpointFromCallback,
  messageRoomContextFromRun,
  messageRoomTool,
  messageRoomTools,
} from "./message_room.js";
import type { ConversationRunInput } from "./types.js";

function installFetch(fn: typeof fetch): () => void {
  const original = globalThis.fetch;
  globalThis.fetch = fn;
  return () => {
    globalThis.fetch = original;
  };
}

test("message_room tool POSTs to, body, op_id, from_agent_id", async () => {
  const previous = process.env.VAMOS_INTERNAL_TOKEN;
  process.env.VAMOS_INTERNAL_TOKEN = "secret-token";
  let posted: { url: string; body: string; headers: Headers } | undefined;
  const restore = installFetch(async (input, init) => {
    posted = {
      url: String(input),
      body: String(init?.body ?? ""),
      headers: new Headers(init?.headers),
    };
    return new Response(JSON.stringify({ ok: true }), { status: 200 });
  });
  try {
    const tool = messageRoomTool({
      enqueueEndpoint: "http://localhost/internal/agent-chat/enqueue",
      fromAgentID: "agent-alpha",
      originThreadID: "thread-plan",
    });
    assert.equal(tool.name, "message_room");
    assert.equal(
      tool.description?.includes("http") ||
        tool.description?.includes("localhost"),
      false,
    );
    const result = await tool.execute(
      "call-1",
      {
        to: "beta",
        body: "hello pairwise",
      },
      undefined,
      undefined,
      {} as never,
    );
    assert.ok(posted);
    assert.equal(posted.url, "http://localhost/internal/agent-chat/enqueue");
    assert.equal(posted.headers.get("X-Vamos-Internal-Token"), "secret-token");
    const parsed = JSON.parse(posted.body) as Record<string, string>;
    assert.equal(parsed.to, "beta");
    assert.equal(parsed.body, "hello pairwise");
    assert.equal(parsed.from_agent_id, "agent-alpha");
    assert.ok(parsed.op_id);
    assert.equal(parsed.from_kind, "agent");
    assert.equal(parsed.thread_id, "thread-plan");
    assert.equal(result.content[0].type, "text");
  } finally {
    restore();
    if (previous === undefined) {
      delete process.env.VAMOS_INTERNAL_TOKEN;
    } else {
      process.env.VAMOS_INTERNAL_TOKEN = previous;
    }
  }
});

test("enqueue endpoint is derived from events callback", () => {
  assert.equal(
    enqueueEndpointFromCallback(
      "http://127.0.0.1:4200/internal/agent-chat/events",
    ),
    "http://127.0.0.1:4200/internal/agent-chat/enqueue",
  );
  const input = {
    callback_endpoint: "http://host/internal/agent-chat/events",
    thread_id: "t1",
    room: { kind: "plan", from_agent_id: "id-1" },
  } as ConversationRunInput;
  const ctx = messageRoomContextFromRun(input);
  assert.equal(ctx.fromAgentID, "id-1");
  assert.equal(ctx.originThreadID, "t1");
  assert.equal(
    messageRoomTools(ctx)
      .map((tool) => tool.name)
      .join(","),
    "message_room",
  );
});
