import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const fixtureURL = new URL(
  "./fixtures/opaque-settlement-fixtures.json",
  import.meta.url,
);

test("opaque settlement fixtures retain exact JavaScript JSON bytes", async () => {
  const fixtures = JSON.parse(await readFile(fixtureURL, "utf8"));
  for (const fixture of fixtures.cases) {
    const bytes = Buffer.from(JSON.stringify(fixture.envelope));
    assert.equal(
      JSON.parse(bytes).raw_response,
      fixture.envelope.raw_response,
      fixture.name,
    );
    assert.deepEqual(
      JSON.parse(bytes).fenced_yaml_blocks,
      fixture.envelope.fenced_yaml_blocks,
      fixture.name,
    );
  }
});
