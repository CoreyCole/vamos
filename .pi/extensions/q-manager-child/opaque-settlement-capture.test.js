import assert from "node:assert/strict";
import test from "node:test";
import { parseDocument } from "yaml";
import {
  buildSettlementEvidence,
  projectPersistedAssistantText,
} from "./opaque-settlement-capture.js";

const identity = {
  managerThreadID: "thread-1",
  piSessionID: "session-1",
  messageID: "pi-settlement-v1-test",
};
function built(raw) {
  return buildSettlementEvidence(identity, raw).toString();
}
function raw(bytes) {
  return parseDocument(bytes).get("raw_response");
}

test("projects only persisted text", () =>
  assert.equal(
    projectPersistedAssistantText([
      { type: "text", text: "a" },
      { type: "toolCall" },
      { type: "text", text: "b" },
    ]),
    "ab",
  ));
test("valid child mapping is sorted and system fields follow it", () => {
  const first = built("```yaml\nz: 1\na:\n  y: 2\n  b: 3\n```");
  assert.equal(first, built("```yaml\nz: 1\na:\n  y: 2\n  b: 3\n```"));
  assert.match(first, /^a:\n  b: 3\n  y: 2\nz: 1\nversion:/);
});
test("invalid or ambiguous candidate falls back to system-only mapping", () => {
  for (const candidate of [
    "plain",
    "```yaml\na: 1\n```\n```yml\nb: 2\n```",
    "```yaml\na: [\n```",
    "```yaml\n---\na: 1\n```",
    "```yaml\na: 1\n...\n```",
    "```yaml\na: 1\na: 2\n```",
    "```yaml\na: &x 1\nb: *x\n```",
    "```yaml\n<<: {a: 1}\n```",
    "```yaml\n[bad]: key\n```",
    "```yaml\nraw_response: bad\n```",
  ]) {
    const bytes = built(candidate);
    assert.ok(!bytes.includes("\na: ") && !bytes.includes("\nb: "));
    assert.equal(raw(bytes), candidate);
  }
});
test("literal raw response preserves terminal newlines, CRLF, and Unicode", () => {
  for (const value of ["none", "one\n", "many\n\n", "café 🌰\r\nnext\r\n"])
    assert.equal(raw(built(value)), value);
  assert.match(built("none"), /raw_response: \|-\n/);
  assert.match(built("one\n"), /raw_response: \|\n/);
  assert.match(built("many\n\n"), /raw_response: \|\+\n/);
});
