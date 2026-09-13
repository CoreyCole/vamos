export type PiModelSelector = {
  provider: string;
  modelId: string;
};

export function turnModelSelector(
  env: NodeJS.ProcessEnv = process.env,
): PiModelSelector {
  return {
    provider: env.PI_MODEL_PROVIDER || "xai",
    modelId: env.PI_MODEL_ID || "grok-4.6",
  };
}

export function resolveTurnModel<T extends { id: string; name: string }>(
  registry: { find(provider: string, modelId: string): T | undefined },
  provider: string,
  modelId: string,
): T {
  const found = registry.find(provider, modelId);
  if (found) {
    return found;
  }
  // Worker catalog is pi-coding-agent 0.65.0; grok-4.6 is not built-in.
  // xAI accepts the grok-4.6 id on the same openai-completions endpoint as grok-4.
  if (provider === "xai" && modelId === "grok-4.6") {
    const grok4 = registry.find("xai", "grok-4");
    if (grok4) {
      return { ...grok4, id: "grok-4.6", name: "Grok 4.6" };
    }
  }
  throw new Error(`PI model not found: ${provider}/${modelId}`);
}
