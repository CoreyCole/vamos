import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const fixtureURL = new URL("./fixtures/opaque-settlement-fixtures.json", import.meta.url);

test("opaque settlement fixtures retain exact JavaScript JSON bytes", async () => {
  const fixtures = JSON.parse(await readFile(fixtureURL, "utf8"));
  for (const fixture of fixtures.cases) {
    assert.equal(
      Buffer.from(JSON.stringify(fixture.envelope)).toString("base64"),
      fixture.expected_json_base64,
      fixture.name,
    );
  }
});
