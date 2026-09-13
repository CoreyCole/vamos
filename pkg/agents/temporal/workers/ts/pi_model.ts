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

export function requireModel<T>(
  model: T | undefined,
  provider: string,
  modelId: string,
): T {
  if (!model) {
    throw new Error(`PI model not found: ${provider}/${modelId}`);
  }
  return model;
}
