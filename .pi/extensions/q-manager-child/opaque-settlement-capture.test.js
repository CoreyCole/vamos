import assert from "node:assert/strict";
import test from "node:test";

import {
  captureOpaqueSettlementEvidence,
  captureOpaqueYamlFences,
  projectPersistedAssistantText,
} from "./opaque-settlement-capture.js";

test("projects only persisted text parts in their stored order", () => {
  const content = [
    { type: "text", text: "first" },
    { type: "thinking", thinking: "excluded" },
    { type: "toolCall", name: "excluded" },
    { type: "text", text: " second" },
    { type: "text", text: "" },
  ];
  assert.equal(projectPersistedAssistantText(content), "first second");
  assert.deepEqual(captureOpaqueSettlementEvidence(content), {
    rawResponse: "first second",
    fencedYamlBlocks: [],
  });
});

const cases = [
  ["no fence and unfenced YAML", "outcome: handoff\nnext: implement\n", []],
  [
    "one empty fence",
    "```yaml\n```\n",
    [{ language: "yaml", raw: "```yaml\n```\n" }],
  ],
  [
    "multiple mixed case fences",
    "```YAML\na: 1\n```\ntext\n```yMl\nb: 2\n```",
    [
      { language: "YAML", raw: "```YAML\na: 1\n```\n" },
      { language: "yMl", raw: "```yMl\nb: 2\n```" },
    ],
  ],
  [
    "crlf and delimiter whitespace",
    "```yml \t\r\na: café 🌰\r\n``` \t\r\n",
    [{ language: "yml", raw: "```yml \t\r\na: café 🌰\r\n``` \t\r\n" }],
  ],
  [
    "exact run only",
    "````yaml\na\n```\n````\n",
    [{ language: "yaml", raw: "````yaml\na\n```\n````\n" }],
  ],
  [
    "longer and shorter runs do not close",
    "```yaml\na\n````\nb\n``\nc\n```\n",
    [{ language: "yaml", raw: "```yaml\na\n````\nb\n``\nc\n```\n" }],
  ],
  ["attributes rejected", "```yaml title=x\na\n```\n```yml {x}\nb\n```\n", []],
  [
    "non yaml and inline backticks excluded",
    "```json\na\n```\ninline ```yaml nope\n",
    [],
  ],
  ["unclosed fence", "```yaml\na: 1\n", []],
  [
    "malformed contradictory unknown yaml remains opaque",
    "```yaml\na: [\noutcome: complete\noutcome: handoff\nunknown: ☃\n```\n",
    [
      {
        language: "yaml",
        raw: "```yaml\na: [\noutcome: complete\noutcome: handoff\nunknown: ☃\n```\n",
      },
    ],
  ],
  [
    "trailing no newline and ascii whitespace only",
    "```yaml\t\na\n```\t ",
    [{ language: "yaml", raw: "```yaml\t\na\n```\t " }],
  ],
];

for (const [name, raw, want] of cases) {
  test(`captures lexical fence: ${name}`, () => {
    const got = captureOpaqueYamlFences(raw);
    assert.deepEqual(got, want);
    for (let i = 0; i < got.length; i++) assert.ok(raw.includes(got[i].raw));
  });
}
