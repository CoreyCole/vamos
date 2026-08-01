import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("installed active lifecycle has no child transport authority", async () => {
  const source = await readFile(new URL("./index.js", import.meta.url), "utf8");
  const active = source.slice(source.indexOf("export default function"));
  assert.match(active, /buildSettlementEvidenceV1/);
  assert.match(active, /writeHandoffFrame/);
  assert.doesNotMatch(
    active,
    /deliverPublished|dependencies\.fetch|gatewayURL|ingressToken/,
  );
  assert.doesNotMatch(active, /managerThreadID|manager_thread_id/);
});
