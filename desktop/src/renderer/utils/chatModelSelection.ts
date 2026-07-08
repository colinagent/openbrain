import type { ModelEntry, ModelsConfig } from '../types/electron';

export type ChatModelSource = 'explicit';

export type ChatModelSelection = {
  effectiveModelKey: string | null;
  effectiveModel: ModelEntry | null;
  source: ChatModelSource | null;
};

export type DefaultChatModelSelection = {
  modelKey: string | null;
  model: ModelEntry | null;
  errorMessage: string | null;
};

export const DEFAULT_CHAT_MODEL_REQUIRED_MESSAGE = 'Default Chat Model is not configured. Open Models > Policies and set Default Chat Model to an enabled chat model.';

function normalizeSelectionModelKey(value: string | null | undefined): string {
  return (value || '').trim();
}

function findEnabledModel(models: ModelEntry[], key: string): ModelEntry | null {
  const normalized = normalizeSelectionModelKey(key);
  if (!normalized) {
    return null;
  }
  return models.find((model) => model.enabled && model.key === normalized) || null;
}

export function resolveChatModelSelection(
  models: ModelEntry[],
  explicitModelKey: string | null | undefined,
): ChatModelSelection {
  const normalizedExplicitModelKey = normalizeSelectionModelKey(explicitModelKey);
  const explicitModel = findEnabledModel(models, normalizedExplicitModelKey);
  if (normalizedExplicitModelKey) {
    return {
      effectiveModelKey: normalizedExplicitModelKey,
      effectiveModel: explicitModel,
      source: 'explicit',
    };
  }

  return {
    effectiveModelKey: null,
    effectiveModel: null,
    source: null,
  };
}

export function resolveDefaultChatModelSelection(config: ModelsConfig): DefaultChatModelSelection {
  const modelKey = normalizeSelectionModelKey(config.strategies?.auto?.defaultChatModelID);
  if (!modelKey) {
    return {
      modelKey: null,
      model: null,
      errorMessage: DEFAULT_CHAT_MODEL_REQUIRED_MESSAGE,
    };
  }
  const model = findEnabledModel(config.models, modelKey);
  if (!model) {
    return {
      modelKey: null,
      model: null,
      errorMessage: `Default Chat Model "${modelKey}" is not available. Open Models > Policies and set Default Chat Model to an enabled chat model.`,
    };
  }
  return {
    modelKey: model.key,
    model,
    errorMessage: null,
  };
}
