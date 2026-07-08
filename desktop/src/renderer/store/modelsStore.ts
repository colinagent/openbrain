import { create } from 'zustand';
import type { ModelAPI, ModelEntry, ModelPreference, ModelsConfig, ModelStrategies, ProviderEntry } from '../types/electron';
import {
  resolveModelReasoningControl,
  type ModelReasoningControl,
} from '../../main/shared/modelReasoning';
import {
  OPENBRAIN_PROVIDER_KEY,
  buildModelKey,
  formatModelProviderLabel,
  normalizeModelKey,
  normalizeProviderKey,
} from '../../shared/modelKeys';

type ModelsState = {
  config: ModelsConfig;
  loading: boolean;
  error: string | null;
  load: () => Promise<void>;
  refreshFromOpenBrain: () => Promise<void>;
  setModelPreference: (key: string, patch: Partial<ModelPreference>) => Promise<void>;
  toggleEnabled: (key: string) => Promise<void>;
  setDefault: (key: string) => Promise<void>;
  setDefaultChatModel: (key: string) => Promise<void>;
  addProviderModel: (entry: {
    provider: string;
    providerLabel?: string;
    id: string;
    label?: string;
    api?: ModelAPI;
    reasoning?: boolean;
    reasoningControl?: ModelReasoningControl;
    reasoningLevels?: string[];
    baseUrl?: string;
    apiKey?: string;
  }) => Promise<void>;
  updateProvider: (provider: string, patch: { providerLabel?: string; baseUrl?: string; apiKey?: string }) => Promise<void>;
  removeProvider: (provider: string) => Promise<void>;
  removeProviderModel: (key: string) => Promise<void>;
  updateProviderModel: (key: string, patch: {
    label?: string;
    api?: ModelAPI;
    reasoningControl?: ModelReasoningControl;
    baseUrl?: string;
    apiKey?: string;
  }) => Promise<void>;
};

function isManagedProvider(config: ModelsConfig, providerKey: string | null | undefined): boolean {
  const key = normalizeProviderKey(providerKey);
  return Boolean(key && (key === OPENBRAIN_PROVIDER_KEY || config.providers[key]?.managed === true));
}

function buildProvidersFromModels(models: ModelEntry[], existingProviders: Record<string, ProviderEntry> = {}): Record<string, ProviderEntry> {
  const grouped = new Map<string, ModelEntry[]>();
  for (const model of models) {
    const bucket = grouped.get(model.provider);
    if (bucket) {
      bucket.push(model);
    } else {
      grouped.set(model.provider, [model]);
    }
  }

  const providers: Record<string, ProviderEntry> = {};
  for (const [providerKey, providerModels] of grouped.entries()) {
    const providerLabel = formatModelProviderLabel(
      providerKey,
      providerModels.find((model) => normalizeOptional(model.providerLabel))?.providerLabel,
    ) || undefined;
    const commonApi = providerModels.length > 0 && providerModels.every((model) => model.api === providerModels[0]?.api)
      ? providerModels[0]?.api
      : undefined;
    const isManaged = providerKey === OPENBRAIN_PROVIDER_KEY || existingProviders[providerKey]?.managed === true;
    const commonBaseUrl = isManaged
      ? undefined
      : commonOptionalValue(providerModels.map((model) => model.baseUrl));
    const commonApiKey = isManaged
      ? undefined
      : commonOptionalValue(providerModels.map((model) => model.apiKey));
    providers[providerKey] = {
      label: providerLabel,
      managed: isManaged || undefined,
      api: isManaged ? undefined : commonApi,
      baseUrl: commonBaseUrl,
      apiKey: commonApiKey,
      models: providerModels.map((model) => ({
        id: model.id,
        label: model.label,
        enabled: model.enabled,
        api:
          isManaged || !commonApi || model.api !== commonApi
            ? model.api
            : undefined,
        baseUrl:
          isManaged
            ? undefined
            : (commonBaseUrl && normalizeOptional(model.baseUrl) === commonBaseUrl
                ? undefined
                : normalizeOptional(model.baseUrl)),
        apiKey:
          isManaged
            ? undefined
            : (commonApiKey && normalizeOptional(model.apiKey) === commonApiKey
                ? undefined
                : normalizeOptional(model.apiKey)),
        reasoning: model.reasoning,
        reasoningControl: model.reasoningControl,
        reasoningLevels: model.reasoningLevels,
        contextWindow: model.contextWindow,
        contextWindows: model.contextWindows,
        defaultContextWindow: model.defaultContextWindow,
        serviceTiers: model.serviceTiers,
        maxOutputTokens: model.maxOutputTokens,
        updatedAt: model.updatedAt,
      })),
    };
  }
  return providers;
}

function buildConfig(
  models: ModelEntry[],
  defaultModelKey: string | null,
  updatedAt: number,
  strategies?: ModelStrategies,
  existingProviders: Record<string, ProviderEntry> = {},
  modelPreferences?: Record<string, ModelPreference>,
): ModelsConfig {
  return {
    version: 5,
    defaultModelKey,
    providers: buildProvidersFromModels(models, existingProviders),
    models,
    strategies,
    modelPreferences: normalizeModelPreferences(modelPreferences, models),
    updatedAt,
  };
}

const emptyConfig: ModelsConfig = buildConfig([
], null, 0);

function getDefaultChatModelKey(config: ModelsConfig): string {
  return normalizeModelKey(config.strategies?.auto?.defaultChatModelID);
}

function isDefaultChatModel(config: ModelsConfig, modelKey: string): boolean {
  return Boolean(modelKey && getDefaultChatModelKey(config) === modelKey);
}

function providerHasDefaultChatModel(config: ModelsConfig, providerKey: string): boolean {
  const defaultChatModelKey = getDefaultChatModelKey(config);
  if (!defaultChatModelKey) {
    return false;
  }
  return config.models.some((model) => model.key === defaultChatModelKey && model.provider === providerKey);
}

function setDefaultChatModelInStrategies(strategies: ModelStrategies | undefined, modelKey: string): ModelStrategies {
  const currentAuto = strategies?.auto || {};
  return {
    auto: {
      ...currentAuto,
      defaultChatModelID: modelKey,
    },
  };
}

let loadModelsPromise: Promise<void> | null = null;

function normalizeId(value: string | undefined | null): string {
  return (value || '').trim();
}

function normalizeOptional(value: string | undefined | null): string | undefined {
  const trimmed = (value || '').trim();
  return trimmed || undefined;
}

function normalizeReasoningLevels(value: string[] | undefined | null): string[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  const seen = new Set<string>();
  const levels: string[] = [];
  for (const raw of value) {
    const level = (raw || '').trim().toLowerCase();
    if (!level || seen.has(level)) {
      continue;
    }
    seen.add(level);
    levels.push(level);
  }
  return levels.length > 0 ? levels : undefined;
}

function normalizePositiveInt(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isInteger(value) && value > 0 ? value : undefined;
}

function normalizeModelPreference(value: Partial<ModelPreference> | null | undefined): ModelPreference | undefined {
  if (!value || typeof value !== 'object') {
    return undefined;
  }
  const thinkingLevel = normalizeOptional(value.thinkingLevel);
  const contextWindow = normalizePositiveInt(value.contextWindow);
  const rawServiceTier = (value as { serviceTier?: unknown }).serviceTier;
  const serviceTier = rawServiceTier === 'priority' || rawServiceTier === 'flex' || rawServiceTier === null
    ? rawServiceTier
    : undefined;
  const preference: ModelPreference = {
    ...(thinkingLevel ? { thinkingLevel } : {}),
    ...(contextWindow ? { contextWindow } : {}),
    ...(serviceTier !== undefined ? { serviceTier } : {}),
  };
  return Object.keys(preference).length > 0 ? preference : undefined;
}

function normalizeModelPreferences(
  value: Record<string, ModelPreference> | undefined,
  models: ModelEntry[],
): Record<string, ModelPreference> | undefined {
  if (!value || typeof value !== 'object') {
    return undefined;
  }
  const modelKeys = new Set(models.map((model) => model.key));
  const out: Record<string, ModelPreference> = {};
  for (const [rawKey, rawPreference] of Object.entries(value)) {
    const key = normalizeModelKey(rawKey);
    if (!key || !modelKeys.has(key)) {
      continue;
    }
    const preference = normalizeModelPreference(rawPreference);
    if (preference) {
      out[key] = preference;
    }
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

function commonOptionalValue(values: Array<string | undefined>): string | undefined {
  let common: string | undefined;
  let initialized = false;
  for (const value of values) {
    const normalized = normalizeOptional(value);
    if (!initialized) {
      common = normalized;
      initialized = true;
      continue;
    }
    if (normalized !== common) {
      return undefined;
    }
  }
  return initialized ? common : undefined;
}

function pickDefault(models: ModelEntry[], preferred?: string | null): string | null {
  const preferredKey = normalizeModelKey(preferred);
  if (preferredKey) {
    const match = models.find((m) => m.enabled && m.key === preferredKey);
    if (match) {
      return match.key;
    }
  }
  const firstEnabled = models.find((m) => m.enabled);
  if (firstEnabled) {
    return firstEnabled.key;
  }
  return null;
}

async function persistConfig(config: ModelsConfig): Promise<ModelsConfig> {
  if (!window.electronAPI?.models?.set) {
    return config;
  }
  const saved = await window.electronAPI.models.set(config);
  return saved || config;
}

export const useModelsStore = create<ModelsState>((set, get) => ({
  config: emptyConfig,
  loading: false,
  error: null,

  load: async () => {
    const modelsApi = window.electronAPI?.models;
    if (!modelsApi?.get) {
      return;
    }
    if (loadModelsPromise) {
      return loadModelsPromise;
    }
    set({ loading: true, error: null });
    loadModelsPromise = (async () => {
      try {
        const config = await modelsApi.get();
        set({ config: config || emptyConfig });
      } catch (err) {
        set({ error: (err as Error).message || 'Failed to load models' });
      } finally {
        set({ loading: false });
        loadModelsPromise = null;
      }
    })();
    return loadModelsPromise;
  },

  refreshFromOpenBrain: async () => {
    if (!window.electronAPI?.models?.refreshFromOpenBrain) {
      return;
    }
    set({ loading: true, error: null });
    try {
      const result = await window.electronAPI.models.refreshFromOpenBrain();
      if (!result?.success || !result.config) {
        set({ error: result?.error || 'Failed to refresh models' });
        return;
      }
      set({ config: result.config });
    } catch (err) {
      set({ error: (err as Error).message || 'Failed to refresh models' });
    } finally {
      set({ loading: false });
    }
  },

  setModelPreference: async (key: string, patch: Partial<ModelPreference>) => {
    const modelKey = normalizeModelKey(key);
    if (!modelKey) {
      return;
    }
    const current = get().config;
    if (!current.models.some((model) => model.key === modelKey)) {
      return;
    }
    const currentPreference = current.modelPreferences?.[modelKey] || {};
    const nextPreference = normalizeModelPreference({
      ...currentPreference,
      ...patch,
    });
    const modelPreferences = { ...(current.modelPreferences || {}) };
    if (nextPreference) {
      modelPreferences[modelKey] = nextPreference;
    } else {
      delete modelPreferences[modelKey];
    }
    const next = {
      ...current,
      modelPreferences: normalizeModelPreferences(modelPreferences, current.models),
      updatedAt: Date.now(),
    };
    set({ config: next });
    try {
      set({ config: await persistConfig(next) });
    } catch (error) {
      set({ config: current });
      throw error;
    }
  },

  toggleEnabled: async (key: string) => {
    const modelKey = normalizeModelKey(key);
    if (!modelKey) {
      return;
    }
    const current = get().config;
    const target = current.models.find((m) => m.key === modelKey);
    if (!target) {
      return;
    }
    if (target.enabled && isDefaultChatModel(current, modelKey)) {
      throw new Error('Default Chat Model cannot be disabled. Set a different Default Chat Model first.');
    }
    const models = current.models.map((m) =>
      m.key === modelKey ? { ...m, enabled: !m.enabled, updatedAt: Date.now() } : m,
    );
    const defaultModelKey = pickDefault(models, current.defaultModelKey);
    const next = buildConfig(models, defaultModelKey, Date.now(), current.strategies, current.providers, current.modelPreferences);
    set({ config: next });
    set({ config: await persistConfig(next) });
  },

  setDefault: async (key: string) => {
    const modelKey = normalizeModelKey(key);
    if (!modelKey) {
      return;
    }
    const current = get().config;
    const target = current.models.find((m) => m.key === modelKey);
    if (!target) {
      return;
    }
    const models = current.models.map((m) =>
      m.key === modelKey ? { ...m, enabled: true, updatedAt: Date.now() } : m,
    );
    const next = buildConfig(models, modelKey, Date.now(), current.strategies, current.providers, current.modelPreferences);
    set({ config: next });
    set({ config: await persistConfig(next) });
  },

  setDefaultChatModel: async (key: string) => {
    const modelKey = normalizeModelKey(key);
    if (!modelKey) {
      return;
    }
    const current = get().config;
    const target = current.models.find((m) => m.key === modelKey);
    if (!target) {
      return;
    }
    const now = Date.now();
    const models = current.models.map((m) =>
      m.key === modelKey ? { ...m, enabled: true, updatedAt: now } : m,
    );
    const strategies = setDefaultChatModelInStrategies(current.strategies, modelKey);
    const next = buildConfig(models, current.defaultModelKey, now, strategies, current.providers, current.modelPreferences);
    set({ config: next });
    set({ config: await persistConfig(next) });
  },

  addProviderModel: async ({ provider, providerLabel, id, label, api, reasoning, reasoningControl, reasoningLevels, baseUrl, apiKey }) => {
    const providerKey = normalizeProviderKey(provider);
    const modelId = normalizeId(id);
    const modelLabel = normalizeOptional(label);
    const requestedProviderLabel = normalizeOptional(providerLabel);
    const requestedBaseUrl = normalizeOptional(baseUrl);
    const requestedApiKey = normalizeOptional(apiKey);
    const modelReasoningLevels = normalizeReasoningLevels(reasoningLevels);
    const modelApi =
      api === 'openai-responses' || api === 'anthropic-messages' || api === 'gemini-native'
        ? api
        : 'openai-completions';
    const modelReasoning = reasoning === true || modelReasoningLevels !== undefined;
    const modelReasoningControl = modelReasoning
      ? resolveModelReasoningControl({
          reasoning: modelReasoning,
          reasoningLevels: modelReasoningLevels,
          reasoningControl,
        })
      : undefined;
    const current = get().config;
    if (!providerKey || isManagedProvider(current, providerKey) || !modelId) {
      return;
    }
    const now = Date.now();
    const modelKey = buildModelKey(providerKey, modelId);
    if (current.models.some((model) => model.key === modelKey)) {
      return;
    }
    const existingProvider = current.providers[providerKey];
    const inheritedBaseUrl = normalizeOptional(existingProvider?.baseUrl);
    const inheritedApiKey = normalizeOptional(existingProvider?.apiKey);
    const resolvedBaseUrl = requestedBaseUrl || inheritedBaseUrl;
    const resolvedApiKey = requestedApiKey || inheritedApiKey;
    if (!resolvedBaseUrl || !resolvedApiKey) {
      return;
    }
    const normalizedProviderLabel = formatModelProviderLabel(
      providerKey,
      requestedProviderLabel || existingProvider?.label,
    ) || undefined;
    const entry: ModelEntry = {
      key: modelKey,
      id: modelId,
      label: modelLabel,
      enabled: true,
      provider: providerKey,
      providerLabel: normalizedProviderLabel,
      api: modelApi,
      reasoning: modelReasoning,
      reasoningControl: modelReasoningControl,
      reasoningLevels: modelReasoningLevels,
      baseUrl: resolvedBaseUrl,
      apiKey: resolvedApiKey,
      updatedAt: now,
    };
    const models = [...current.models, entry];
    const defaultModelKey = current.defaultModelKey || modelKey;
    const next = buildConfig(models, defaultModelKey, now, current.strategies, current.providers, current.modelPreferences);
    set({ config: next });
    set({ config: await persistConfig(next) });
  },

  updateProvider: async (provider: string, patch: { providerLabel?: string; baseUrl?: string; apiKey?: string }) => {
    const providerKey = normalizeProviderKey(provider);
    const current = get().config;
    if (!providerKey || isManagedProvider(current, providerKey)) {
      return;
    }
    const existingProvider = current.providers[providerKey];
    if (!existingProvider) {
      return;
    }
    const baseUrl = normalizeOptional(patch.baseUrl) || normalizeOptional(existingProvider.baseUrl);
    const apiKey = normalizeOptional(patch.apiKey) || normalizeOptional(existingProvider.apiKey);
    if (!baseUrl || !apiKey) {
      return;
    }
    const now = Date.now();
    const normalizedProviderLabel = formatModelProviderLabel(
      providerKey,
      patch.providerLabel ?? existingProvider.label,
    ) || undefined;
    const models = current.models.map((model) =>
      model.provider === providerKey
        ? {
            ...model,
            providerLabel: normalizedProviderLabel,
            baseUrl,
            apiKey,
            updatedAt: now,
          }
        : model,
    );
    const next = buildConfig(models, current.defaultModelKey, now, current.strategies, current.providers, current.modelPreferences);
    set({ config: next });
    set({ config: await persistConfig(next) });
  },

  removeProvider: async (provider: string) => {
    const providerKey = normalizeProviderKey(provider);
    const current = get().config;
    if (!providerKey || isManagedProvider(current, providerKey)) {
      return;
    }
    if (!current.providers[providerKey]) {
      return;
    }
    if (providerHasDefaultChatModel(current, providerKey)) {
      throw new Error('Default Chat Model cannot be removed. Set a different Default Chat Model first.');
    }
    const models = current.models.filter((model) => model.provider !== providerKey);
    const defaultModelKey = pickDefault(models, current.defaultModelKey);
    const next = buildConfig(models, defaultModelKey, Date.now(), current.strategies, current.providers, current.modelPreferences);
    set({ config: next });
    set({ config: await persistConfig(next) });
  },

  removeProviderModel: async (key: string) => {
    const modelKey = normalizeModelKey(key);
    if (!modelKey) {
      return;
    }
    const current = get().config;
    const target = current.models.find((m) => m.key === modelKey);
    if (!target || isManagedProvider(current, target.provider)) {
      return;
    }
    if (isDefaultChatModel(current, modelKey)) {
      throw new Error('Default Chat Model cannot be removed. Set a different Default Chat Model first.');
    }
    const models = current.models.filter((m) => m.key !== modelKey);
    const defaultModelKey = pickDefault(models, current.defaultModelKey);
    const next = buildConfig(models, defaultModelKey, Date.now(), current.strategies, current.providers, current.modelPreferences);
    set({ config: next });
    set({ config: await persistConfig(next) });
  },

  updateProviderModel: async (key: string, patch) => {
    const modelKey = normalizeModelKey(key);
    if (!modelKey) {
      return;
    }
    const current = get().config;
    const target = current.models.find((m) => m.key === modelKey);
    if (!target || isManagedProvider(current, target.provider)) {
      return;
    }
    const now = Date.now();
    const nextApi = patch.api === 'openai-responses' || patch.api === 'anthropic-messages' || patch.api === 'gemini-native'
      ? patch.api
      : patch.api === 'openai-completions'
        ? patch.api
        : undefined;
    const models = current.models.map((m) =>
      m.key === modelKey
        ? {
            ...m,
            label: patch.label !== undefined ? normalizeOptional(patch.label) : m.label,
            api: nextApi || m.api,
            reasoningControl: patch.reasoningControl !== undefined
              ? resolveModelReasoningControl({
                  reasoning: m.reasoning,
                  reasoningLevels: m.reasoningLevels,
                  reasoningControl: patch.reasoningControl,
                })
              : m.reasoningControl,
            baseUrl: patch.baseUrl !== undefined ? normalizeOptional(patch.baseUrl) || m.baseUrl : m.baseUrl,
            apiKey: patch.apiKey !== undefined ? normalizeOptional(patch.apiKey) || m.apiKey : m.apiKey,
            updatedAt: now,
          }
        : m,
    );
    const next = buildConfig(models, current.defaultModelKey, now, current.strategies, current.providers, current.modelPreferences);
    set({ config: next });
    set({ config: await persistConfig(next) });
  },
}));
