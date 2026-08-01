import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { YAMLMap, parseDocument } from "yaml";

import { buildSettlementEvidenceV1 } from "./opaque-settlement-capture.js";

const fixtures = JSON.parse(
  await readFile(
    new URL("./fixtures/opaque-settlement-v1-fixtures.json", import.meta.url),
    "utf8",
  ),
);

function parsed(bytes) {
  const document = parseDocument(bytes.toString("utf8"), {
    uniqueKeys: true,
    merge: false,
    prettyErrors: false,
  });
  assert.deepEqual(document.errors, []);
  assert.ok(document.contents instanceof YAMLMap);
  return document;
}

test("opaque v1 fixtures preserve exact identities and raw response", () => {
  for (const fixture of fixtures.cases) {
    const first = buildSettlementEvidenceV1(
      fixtures.identity,
      fixture.raw_response,
    );
    const second = buildSettlementEvidenceV1(
      fixtures.identity,
      fixture.raw_response,
    );
    assert.deepEqual(first, second, fixture.name);
    const document = parsed(first);
    assert.equal(document.get("version"), 1, fixture.name);
    assert.equal(
      document.get("hermes_session_id"),
      fixtures.identity.hermesSessionID,
      fixture.name,
    );
    assert.equal(
      document.get("pi_session_id"),
      fixtures.identity.piSessionID,
      fixture.name,
    );
    assert.equal(
      document.get("message_id"),
      fixtures.identity.messageID,
      fixture.name,
    );
    assert.equal(
      document.get("raw_response"),
      fixture.raw_response,
      fixture.name,
    );
    assert.equal(document.get("manager_thread_id"), undefined, fixture.name);
  }
});

test("v1 lifecycle-looking child fields remain copied non-authoritative text", () => {
  const fixture = fixtures.cases.find(
    ({ name }) => name === "opaque lifecycle-looking mapping",
  );
  const document = parsed(
    buildSettlementEvidenceV1(fixtures.identity, fixture.raw_response),
  );
  assert.equal(document.get("outcome"), "handoff");
  assert.equal(document.get("next"), "verify");
  assert.equal(document.get("complete"), false);
  assert.equal(document.get("raw_response"), fixture.raw_response);
});

test("v1 rejects invalid identities and reserves both routing field names", () => {
  for (const identity of [
    { ...fixtures.identity, hermesSessionID: "bad\u0000id" },
    { ...fixtures.identity, piSessionID: "bad/path" },
    { ...fixtures.identity, messageID: "bad:message" },
  ]) {
    assert.throws(() => buildSettlementEvidenceV1(identity, "raw"));
  }
  const document = parsed(
    buildSettlementEvidenceV1(
      fixtures.identity,
      "```yaml\nhermes_session_id: child-route\nmanager_thread_id: legacy-route\n```",
    ),
  );
  assert.equal(
    document.get("hermes_session_id"),
    fixtures.identity.hermesSessionID,
  );
  assert.equal(document.get("manager_thread_id"), undefined);
});

test("active legacy caller does not import or invoke the additive v1 builder", async () => {
  const source = await readFile(new URL("./index.js", import.meta.url), "utf8");
  assert.doesNotMatch(source, /buildSettlementEvidenceV1/);
  assert.match(source, /buildSettlementEvidence\(/);
  assert.match(source, /manager_thread_id/);
});
