import assert from "node:assert/strict";
import { test } from "node:test";
import { requireModel, turnModelSelector } from "./pi_model.js";

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

test("requireModel returns the model", () => {
  const model = { id: "grok-4.6" };
  assert.equal(requireModel(model, "xai", "grok-4.6"), model);
});

test("requireModel throws when missing", () => {
  assert.throws(
    () => requireModel(undefined, "openai-codex", "gpt-5.5"),
    /PI model not found: openai-codex\/gpt-5.5/,
  );
});
