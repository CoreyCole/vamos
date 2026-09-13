import assert from "node:assert/strict";
import { test } from "node:test";
import { resolveTurnModel, turnModelSelector } from "./pi_model.js";

test("turnModelSelector defaults to xai grok-4.6", () => {
  const got = turnModelSelector({});
  assert.equal(got.provider, "xai");
  assert.equal(got.modelId, "grok-4.6");
});

test("turnModelSelector honors env", () => {
  const got = turnModelSelector({
    PI_MODEL_PROVIDER: "openai-codex",
    PI_MODEL_ID: "gpt-5.5",
  });
  assert.equal(got.provider, "openai-codex");
  assert.equal(got.modelId, "gpt-5.5");
});

test("resolveTurnModel returns registry hit", () => {
  const model = { id: "grok-4.6", name: "Grok 4.6", provider: "xai" };
  const got = resolveTurnModel(
    {
      find: (provider, id) =>
        provider === "xai" && id === "grok-4.6" ? model : undefined,
    },
    "xai",
    "grok-4.6",
  );
  assert.equal(got, model);
});

test("resolveTurnModel clones grok-4 when grok-4.6 is missing", () => {
  const grok4 = {
    id: "grok-4",
    name: "Grok 4",
    provider: "xai",
    api: "openai-completions",
  };
  const got = resolveTurnModel(
    {
      find: (provider, id) =>
        provider === "xai" && id === "grok-4" ? grok4 : undefined,
    },
    "xai",
    "grok-4.6",
  );
  assert.equal(got.id, "grok-4.6");
  assert.equal(got.name, "Grok 4.6");
  assert.equal(got.provider, "xai");
  assert.equal(got.api, "openai-completions");
  assert.equal(grok4.id, "grok-4");
});

test("resolveTurnModel throws when missing", () => {
  assert.throws(
    () =>
      resolveTurnModel({ find: () => undefined }, "openai-codex", "gpt-5.5"),
    /PI model not found: openai-codex\/gpt-5.5/,
  );
});
