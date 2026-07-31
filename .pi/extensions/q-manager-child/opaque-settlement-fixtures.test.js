import assert from "node:assert/strict";
import test from "node:test";
import { buildSettlementEvidence } from "./opaque-settlement-capture.js";

test("fixture vectors produce deterministic immutable YAML", () => {
  const identity = {
    managerThreadID: "thread",
    piSessionID: "session",
    messageID: "pi-settlement-v1-fixture",
  };
  for (const raw of [
    "",
    "```yaml\na: true\n```",
    "```yml\na: [\n```",
    "text\r\n🌰\r\n",
  ]) {
    assert.deepEqual(
      buildSettlementEvidence(identity, raw),
      buildSettlementEvidence(identity, raw),
    );
  }
});
