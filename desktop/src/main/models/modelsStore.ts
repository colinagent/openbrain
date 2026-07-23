import * as fs from 'fs/promises';
import * as path from 'path';
import {
  normalizeModelReasoningControl,
  resolveModelReasoningControl,
  type ModelReasoningControl,
} from '../shared/modelReasoning';
import { writeJsonFileAtomic } from '../shared/jsonFile';

const OPENBRAIN_PROVIDER_KEY = 'cloud';
const OPENBRAIN_PROVIDER_LABEL = 'Cloud';

function normalizeModelKey(value: string | null | undefined): string {
  return (value || '').trim();
}

function normalizeProviderKey(value: string | null | undefined): string {
  return (value || '').trim().toLowerCase();
}

function isValidProviderKey(value: string | null | undefined): boolean {
  const normalized = normalizeProviderKey(value);
  return /^[a-z0-9][a-z0-9._-]*$/.test(normalized);
}

function buildModelKey(providerKey: string | null | undefined, modelId: string | null | undefined): string {
  const normalizedID = (modelId || '').trim();
  if (!normalizedID) {
    return '';
  }
  const normalizedProviderKey = normalizeProviderKey(providerKey);
  if (!normalizedProviderKey) {
    return '';
  }
  return `${normalizedProviderKey}:${normalizedID}`;
}

function formatModelProviderLabel(
  providerKey: string | null | undefined,
  providerLabel?: string | null,
): string | null {
  const normalizedProviderKey = normalizeProviderKey(providerKey);
  if (normalizedProviderKey === OPENBRAIN_PROVIDER_KEY) {
    return OPENBRAIN_PROVIDER_LABEL;
  }
  const normalizedLabel = (providerLabel || '').trim();
  if (normalizedLabel) {
    return normalizedLabel;
  }
  if (!normalizedProviderKey) {
    return null;
  }
  return normalizedProviderKey;
}

export type ModelAPI = 'openai-completions' | 'openai-responses' | 'anthropic-messages' | 'gemini-native';
export type ModelServiceTier = 'priority' | 'flex';

export type ModelEntry = {
  key: string;
  id: string;
  label?: string;
  enabled: boolean;
  provider: string;
  providerLabel?: string;
  api: ModelAPI;
  reasoning: boolean;
  reasoningControl?: ModelReasoningControl;
  reasoningLevels?: string[];
  contextWindow?: number;
  contextWindows?: number[];
  defaultContextWindow?: number;
  serviceTiers?: ModelServiceTier[];
  maxOutputTokens?: number;
  baseUrl?: string;
  apiKey?: string;
  updatedAt?: number;
};

export type ProviderModelEntry = {
  id: string;
  label?: string;
  enabled: boolean;
  api?: ModelAPI;
  baseUrl?: string;
  apiKey?: string;
  reasoning: boolean;
  reasoningControl?: ModelReasoningControl;
  reasoningLevels?: string[];
  contextWindow?: number;
  contextWindows?: number[];
  defaultContextWindow?: number;
  serviceTiers?: ModelServiceTier[];
  maxOutputTokens?: number;
  updatedAt?: number;
};

export type ProviderEntry = {
  label?: string;
  api?: ModelAPI;
  baseUrl?: string;
  apiKey?: string;
  managed?: boolean;
  models: ProviderModelEntry[];
};

export type ModelAutoStrategy = {
  defaultChatModelID?: string;
  defaultChatThinkingLevel?: string;
  defaultInlineCompletionModelID?: string;
  defaultInlineCompletionThinkingLevel?: string;
};

export type ModelStrategies = {
  auto?: ModelAutoStrategy;
};

export type ModelsConfig = {
  version: number;
  defaultModelKey: string | null;
  providers: Record<string, ProviderEntry>;
  models: ModelEntry[];
  strategies?: ModelStrategies;
  modelPreferences?: Record<string, ModelPreference>;
  updatedAt: number;
};

export const CURRENT_VERSION = 5;

export type OpenBrainModelEntry = {
  id: string;
  label?: string;
  api: ModelAPI;
  reasoning: boolean;
  reasoningControl?: ModelReasoningControl;
  reasoningLevels?: string[];
  contextWindows?: number[];
  defaultContextWindow?: number;
  serviceTiers?: ModelServiceTier[];
  maxOutputTokens?: number;
};

export type ModelPreference = {
  thinkingLevel?: string;
  contextWindow?: number;
  serviceTier?: ModelServiceTier | null;
};

export type OpenBrainCatalog = {
  providerKey?: string;
  providerLabel?: string;
  models: OpenBrainModelEntry[];
  strategies?: ModelStrategies;
};

export type MergeOpenBrainOrgCatalogsOptions = {
  activeOrgID?: string | null;
  privateOnly?: boolean;
};

export type OpenBrainOrgCatalog = Required<Pick<OpenBrainCatalog, 'providerKey' | 'providerLabel'>> & {
  models: OpenBrainModelEntry[];
  strategies?: ModelStrategies;
};

type RawProviderModelEntry = Partial<ProviderModelEntry> & {
  key?: string;
};

type RawProviderEntry = Partial<ProviderEntry> & {
  models?: Array<RawProviderModelEntry | null | undefined>;
};

type RawModelsConfig = {
  version?: number;
  defaultModelKey?: string | null;
  providers?: Record<string, RawProviderEntry | null | undefined>;
  models?: Array<Partial<ModelEntry> | null | undefined>;
  strategies?: ModelStrategies | null;
  modelPreferences?: Record<string, Partial<ModelPreference> | null | undefined> | null;
  updatedAt?: number;
};

function createModelsConfigError(message: string): Error {
  return new Error(`Invalid models config: ${message}`);
}

function normalizeApi(value: string | undefined | null): ModelAPI {
  switch (value) {
    case 'openai-responses':
    case 'anthropic-messages':
    case 'gemini-native':
      return value;
    default:
      return 'openai-completions';
  }
}

function normalizeId(value: string | undefined | null): string {
  return (value || '').trim();
}

function normalizeOptional(value: string | undefined | null): string | undefined {
  const trimmed = (value || '').trim();
  return trimmed || undefined;
}

function normalizeServiceTier(value: unknown): ModelServiceTier | null {
  if (value === undefined || value === null) {
    return null;
  }
  if (typeof value !== 'string') {
    throw createModelsConfigError('serviceTier must be priority, flex, or null');
  }
  switch (value.trim().toLowerCase()) {
    case 'priority':
      return 'priority';
    case 'flex':
      return 'flex';
    default:
      throw createModelsConfigError('serviceTier must be priority, flex, or null');
  }
}

function normalizeOptionalServiceTier(value: unknown): ModelServiceTier | undefined {
  try {
    return normalizeServiceTier(value) || undefined;
  } catch {
    return undefined;
  }
}

function normalizeServiceTierArray(value: unknown): ModelServiceTier[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  const seen = new Set<ModelServiceTier>();
  const out: ModelServiceTier[] = [];
  for (const raw of value) {
    const normalized = normalizeOptionalServiceTier(raw);
    if (!normalized || seen.has(normalized)) {
      continue;
    }
    seen.add(normalized);
    out.push(normalized);
  }
  return out.length > 0 ? out : undefined;
}

function normalizePositiveInt(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isInteger(value) && value > 0 ? value : undefined;
}

function normalizePositiveIntArray(value: unknown): number[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  const seen = new Set<number>();
  const out: number[] = [];
  for (const raw of value) {
    const normalized = normalizePositiveInt(raw);
    if (!normalized || seen.has(normalized)) {
      continue;
    }
    seen.add(normalized);
    out.push(normalized);
  }
  out.sort((a, b) => a - b);
  return out.length > 0 ? out : undefined;
}

function normalizeModelPreference(value: Partial<ModelPreference> | null | undefined): ModelPreference | undefined {
  if (!value || typeof value !== 'object') {
    return undefined;
  }
  const thinkingLevel = normalizeOptional(value.thinkingLevel);
  const contextWindow = normalizePositiveInt(value.contextWindow);
  const serviceTier = Object.prototype.hasOwnProperty.call(value, 'serviceTier')
    ? normalizeServiceTier(value.serviceTier)
    : undefined;
  const preference: ModelPreference = {
    ...(thinkingLevel ? { thinkingLevel } : {}),
    ...(contextWindow ? { contextWindow } : {}),
    ...(serviceTier !== undefined ? { serviceTier } : {}),
  };
  return Object.keys(preference).length > 0 ? preference : undefined;
}

function normalizeModelPreferences(
  value: Record<string, Partial<ModelPreference> | null | undefined> | null | undefined,
  models?: ModelEntry[],
): Record<string, ModelPreference> | undefined {
  if (!value || typeof value !== 'object') {
    return undefined;
  }
  const allowed = models ? new Set(models.map((model) => model.key)) : null;
  const out: Record<string, ModelPreference> = {};
  for (const [rawKey, rawPreference] of Object.entries(value)) {
    const key = normalizeModelKey(rawKey);
    if (!key || (allowed && !allowed.has(key))) {
      continue;
    }
    const preference = normalizeModelPreference(rawPreference);
    if (preference) {
      out[key] = preference;
    }
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

function normalizeReasoningLevels(value: string[] | undefined | null): string[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  const seen = new Set<string>();
  const levels: string[] = [];
  for (const raw of value) {
    const level = (raw || '').trim();
    if (!level || seen.has(level)) {
      continue;
    }
    seen.add(level);
    levels.push(level);
  }
  return levels.length > 0 ? levels : undefined;
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

function normalizeModelAutoStrategy(value: unknown): ModelAutoStrategy | undefined {
  if (!value || typeof value !== 'object') {
    return undefined;
  }
  const raw = value as Record<string, unknown>;
  const defaultChatModelID = normalizeOptional(
    typeof raw.defaultChatModelID === 'string' ? raw.defaultChatModelID : undefined,
  );
  const defaultChatThinkingLevel = normalizeOptional(
    typeof raw.defaultChatThinkingLevel === 'string' ? raw.defaultChatThinkingLevel : undefined,
  );
  const defaultInlineCompletionModelID = normalizeOptional(
    typeof raw.defaultInlineCompletionModelID === 'string' ? raw.defaultInlineCompletionModelID : undefined,
  );
  const defaultInlineCompletionThinkingLevel = normalizeOptional(
    typeof raw.defaultInlineCompletionThinkingLevel === 'string' ? raw.defaultInlineCompletionThinkingLevel : undefined,
  );
  if (!defaultChatModelID && !defaultInlineCompletionModelID) {
    return undefined;
  }
  return {
    ...(defaultChatModelID ? { defaultChatModelID } : {}),
    ...(defaultChatThinkingLevel ? { defaultChatThinkingLevel } : {}),
    ...(defaultInlineCompletionModelID ? { defaultInlineCompletionModelID } : {}),
    ...(defaultInlineCompletionThinkingLevel ? { defaultInlineCompletionThinkingLevel } : {}),
  };
}

function normalizeModelStrategies(value: unknown): ModelStrategies | undefined {
  if (!value || typeof value !== 'object') {
    return undefined;
  }
  const raw = value as Record<string, unknown>;
  const auto = normalizeModelAutoStrategy(raw.auto);
  if (!auto) {
    return undefined;
  }
  return { auto };
}

function normalizeProviderLabel(
  providerKey: string,
  providerLabel: string | undefined | null,
): string | undefined {
  return formatModelProviderLabel(providerKey, providerLabel) || undefined;
}

function isManagedProviderKey(
  value: string | null | undefined,
  managedProviderKeys: Set<string> | undefined,
): boolean {
  const key = normalizeProviderKey(value);
  return Boolean(key && managedProviderKeys?.has(key));
}

function collectManagedProviderKeys(raw: RawModelsConfig): Set<string> {
  const managedProviderKeys = new Set<string>();
  for (const [providerKeyRaw, providerValue] of Object.entries((raw.providers || {}) as Record<string, RawProviderEntry | null | undefined>)) {
    const providerKey = normalizeProviderKey(providerKeyRaw);
    if (providerKey && (providerValue?.managed === true || providerKey === OPENBRAIN_PROVIDER_KEY)) {
      managedProviderKeys.add(providerKey);
    }
  }
  for (const item of (raw.models || []) as Array<Partial<ModelEntry> | null | undefined>) {
    if (!item || typeof item !== 'object') {
      continue;
    }
    const providerKey = normalizeProviderKey(item.provider);
    if (providerKey === OPENBRAIN_PROVIDER_KEY) {
      managedProviderKeys.add(providerKey);
    }
  }
  return managedProviderKeys;
}

function normalizeFlatEntry(
  raw: Partial<ModelEntry> | null | undefined,
  managedProviderKeys: Set<string> = new Set(),
): ModelEntry {
  if (!raw || typeof raw !== 'object') {
    throw createModelsConfigError('model entry must be an object');
  }
  const rawID = normalizeId(raw.id);
  if (!rawID) {
    throw createModelsConfigError('model entry id is required');
  }

  const key = normalizeModelKey(raw.key);
  if (!key) {
    throw createModelsConfigError(`model ${rawID} key is required`);
  }

  const provider = normalizeProviderKey(raw.provider);
  if (!provider) {
    throw createModelsConfigError(`model ${rawID} provider is required`);
  }
  if (provider !== OPENBRAIN_PROVIDER_KEY && !isValidProviderKey(provider)) {
    throw createModelsConfigError(`model ${rawID} provider key is invalid`);
  }

  const isManagedProvider = isManagedProviderKey(provider, managedProviderKeys);
  const id = rawID;
  const expectedKey = buildModelKey(provider, id);
  if (key !== expectedKey) {
    throw createModelsConfigError(`model ${id} key must be ${expectedKey}`);
  }

  const label = normalizeOptional(raw.label);
  const enabled = typeof raw.enabled === 'boolean' ? raw.enabled : true;
  const api = normalizeApi(raw.api);
  const reasoningLevels = normalizeReasoningLevels(raw.reasoningLevels);
  const reasoning = raw.reasoning === true || reasoningLevels !== undefined;
  const reasoningControl = reasoning
    ? resolveModelReasoningControl({
        reasoning,
        reasoningLevels,
        reasoningControl: raw.reasoningControl,
      })
    : undefined;
  const providerLabel = normalizeProviderLabel(provider, raw.providerLabel);
  const updatedAt = typeof raw.updatedAt === 'number' ? raw.updatedAt : undefined;
  const contextWindow = normalizePositiveInt(raw.contextWindow);
  const normalizedContextWindows = normalizePositiveIntArray(raw.contextWindows);
  const contextWindows = normalizedContextWindows && contextWindow
    ? normalizedContextWindows.filter((value) => value <= contextWindow)
    : normalizedContextWindows;
  const rawDefaultContextWindow = normalizePositiveInt(raw.defaultContextWindow);
  const defaultContextWindow = rawDefaultContextWindow && contextWindows?.includes(rawDefaultContextWindow)
    ? rawDefaultContextWindow
    : undefined;
  const serviceTiers = normalizeServiceTierArray(raw.serviceTiers);
  const maxOutputTokens = normalizePositiveInt(raw.maxOutputTokens);

  const baseUrl = isManagedProvider ? undefined : normalizeOptional(raw.baseUrl);
  const apiKey = isManagedProvider ? undefined : normalizeOptional(raw.apiKey);
  if (!isManagedProvider) {
    if (!baseUrl) {
      throw createModelsConfigError(`provider model ${key} baseUrl is required`);
    }
    if (!apiKey) {
      throw createModelsConfigError(`provider model ${key} apiKey is required`);
    }
  }

  return {
    key: expectedKey,
    id,
    label,
    enabled,
    provider,
    providerLabel,
    api,
    reasoning,
    reasoningControl,
    reasoningLevels,
    contextWindow,
    contextWindows,
    defaultContextWindow,
    serviceTiers,
    maxOutputTokens,
    baseUrl,
    apiKey,
    updatedAt,
  };
}

function normalizeFlatModels(
  modelsRaw: Array<Partial<ModelEntry> | null | undefined>,
  managedProviderKeys: Set<string> = new Set(),
): ModelEntry[] {
  const deduped = new Map<string, ModelEntry>();
  for (const item of modelsRaw) {
    const normalized = normalizeFlatEntry(item, managedProviderKeys);
    if (deduped.has(normalized.key)) {
      throw createModelsConfigError(`duplicate model key ${normalized.key}`);
    }
    deduped.set(normalized.key, normalized);
  }
  return Array.from(deduped.values());
}

function buildProvidersFromFlatModels(models: ModelEntry[], managedProviderKeys: Set<string> = new Set()): Record<string, ProviderEntry> {
  const groups = new Map<string, ModelEntry[]>();
  for (const model of models) {
    const existing = groups.get(model.provider);
    if (existing) {
      existing.push(model);
    } else {
      groups.set(model.provider, [model]);
    }
  }

  const providers: Record<string, ProviderEntry> = {};
  for (const [providerKey, providerModels] of groups.entries()) {
    const providerLabel = normalizeProviderLabel(
      providerKey,
      providerModels.find((model) => normalizeOptional(model.providerLabel))?.providerLabel,
    );

    const commonApi = providerModels.length > 0 && providerModels.every((model) => model.api === providerModels[0]?.api)
      ? providerModels[0]?.api
      : undefined;
    const isManagedProvider = isManagedProviderKey(providerKey, managedProviderKeys);
    const commonBaseUrl = isManagedProvider
      ? undefined
      : commonOptionalValue(providerModels.map((model) => model.baseUrl));
    const commonAPIKey = isManagedProvider
      ? undefined
      : commonOptionalValue(providerModels.map((model) => model.apiKey));

    providers[providerKey] = {
      label: providerLabel,
      managed: isManagedProvider || undefined,
      api: isManagedProvider ? undefined : commonApi,
      baseUrl: commonBaseUrl,
      apiKey: commonAPIKey,
      models: providerModels.map((model) => ({
        id: model.id,
        label: model.label,
        enabled: model.enabled,
        api:
          isManagedProvider || !commonApi || model.api !== commonApi
            ? model.api
            : undefined,
        baseUrl:
          isManagedProvider
            ? undefined
            : (commonBaseUrl && normalizeOptional(model.baseUrl) === commonBaseUrl
                ? undefined
                : normalizeOptional(model.baseUrl)),
        apiKey:
          isManagedProvider
            ? undefined
            : (commonAPIKey && normalizeOptional(model.apiKey) === commonAPIKey
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

function flattenProvidersToModels(
  providersRaw: Record<string, RawProviderEntry | null | undefined>,
  managedProviderKeys: Set<string> = new Set(),
): ModelEntry[] {
  const models: ModelEntry[] = [];
  for (const [providerKeyRaw, providerValue] of Object.entries(providersRaw)) {
    if (!providerValue || typeof providerValue !== 'object') {
      throw createModelsConfigError(`provider ${providerKeyRaw} must be an object`);
    }
    const providerKey = normalizeProviderKey(providerKeyRaw);
    if (!providerKey) {
      throw createModelsConfigError('provider key is required');
    }
    if (providerKey !== OPENBRAIN_PROVIDER_KEY && !isValidProviderKey(providerKey)) {
      throw createModelsConfigError(`provider ${providerKey} key is invalid`);
    }

    const isManagedProvider = providerValue.managed === true || isManagedProviderKey(providerKey, managedProviderKeys);
    if (isManagedProvider) {
      managedProviderKeys.add(providerKey);
    }
    const providerLabel = normalizeProviderLabel(providerKey, providerValue.label);
    const providerBaseUrl = isManagedProvider ? undefined : normalizeOptional(providerValue.baseUrl);
    const providerApiKey = isManagedProvider ? undefined : normalizeOptional(providerValue.apiKey);
    const providerApi = normalizeOptional(providerValue.api);
    const rawProviderModels = Array.isArray(providerValue.models) ? providerValue.models : [];
    if (rawProviderModels.length === 0) {
      throw createModelsConfigError(`provider ${providerKey} models must not be empty`);
    }

    for (const rawModel of rawProviderModels) {
      if (!rawModel || typeof rawModel !== 'object') {
        throw createModelsConfigError(`provider ${providerKey} model entry must be an object`);
      }
      const rawID = normalizeId(rawModel.id);
      if (!rawID) {
        throw createModelsConfigError(`provider ${providerKey} model id is required`);
      }
      const id = rawID;
      const expectedKey = buildModelKey(providerKey, id);
      const explicitKey = normalizeModelKey(rawModel.key);
      if (explicitKey && explicitKey !== expectedKey) {
        throw createModelsConfigError(`provider ${providerKey} model ${id} key must be ${expectedKey}`);
      }
      const modelApi = normalizeApi(normalizeOptional(rawModel.api) || providerApi);
      const reasoningLevels = normalizeReasoningLevels(rawModel.reasoningLevels);
      const reasoning = rawModel.reasoning === true || reasoningLevels !== undefined;
      const reasoningControl = reasoning
        ? resolveModelReasoningControl({
            reasoning,
            reasoningLevels,
            reasoningControl: rawModel.reasoningControl,
          })
        : undefined;
      const enabled = typeof rawModel.enabled === 'boolean' ? rawModel.enabled : true;
      const modelBaseUrl = isManagedProvider ? undefined : normalizeOptional(rawModel.baseUrl) || providerBaseUrl;
      const modelApiKey = isManagedProvider ? undefined : normalizeOptional(rawModel.apiKey) || providerApiKey;
      const contextWindow = normalizePositiveInt(rawModel.contextWindow);
      const normalizedContextWindows = normalizePositiveIntArray(rawModel.contextWindows);
      const contextWindows = normalizedContextWindows && contextWindow
        ? normalizedContextWindows.filter((value) => value <= contextWindow)
        : normalizedContextWindows;
      const rawDefaultContextWindow = normalizePositiveInt(rawModel.defaultContextWindow);
      const defaultContextWindow = rawDefaultContextWindow && contextWindows?.includes(rawDefaultContextWindow)
        ? rawDefaultContextWindow
        : undefined;
      const serviceTiers = normalizeServiceTierArray(rawModel.serviceTiers);
      const maxOutputTokens = normalizePositiveInt(rawModel.maxOutputTokens);
      if (!isManagedProvider) {
        if (!modelBaseUrl) {
          throw createModelsConfigError(`provider ${providerKey} model ${id} baseUrl is required`);
        }
        if (!modelApiKey) {
          throw createModelsConfigError(`provider ${providerKey} model ${id} apiKey is required`);
        }
      }
      models.push({
        key: expectedKey,
        id,
        label: normalizeOptional(rawModel.label),
        enabled,
        provider: providerKey,
        providerLabel,
        api: modelApi,
        reasoning,
        reasoningControl,
        reasoningLevels,
        contextWindow,
        contextWindows,
        defaultContextWindow,
        serviceTiers,
        maxOutputTokens,
        baseUrl: modelBaseUrl,
        apiKey: modelApiKey,
        updatedAt: typeof rawModel.updatedAt === 'number' ? rawModel.updatedAt : undefined,
      });
    }
  }
  return normalizeFlatModels(models, managedProviderKeys);
}

function pickDefault(
  models: ModelEntry[],
  preferred?: string | null,
): string | null {
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

function assertDefaultChatModelEnabled(strategies: ModelStrategies | undefined, models: ModelEntry[]): void {
  const defaultChatModelID = normalizeModelKey(strategies?.auto?.defaultChatModelID);
  if (!defaultChatModelID) {
    return;
  }
  const match = models.find((model) => model.key === defaultChatModelID);
  if (!match || !match.enabled) {
    throw createModelsConfigError(`Default Chat Model ${defaultChatModelID} must reference an enabled model`);
  }
}

export function normalizeModelsConfig(raw: Partial<ModelsConfig> | RawModelsConfig | null | undefined): ModelsConfig {
  const rawConfig = (raw || {}) as RawModelsConfig;
  const version = typeof rawConfig.version === 'number' ? rawConfig.version : NaN;
  if (version !== CURRENT_VERSION) {
    throw createModelsConfigError(`version must be ${CURRENT_VERSION}`);
  }

  const defaultModelKey = normalizeModelKey(rawConfig.defaultModelKey);
  const hasFlatModels = Array.isArray(rawConfig.models) && rawConfig.models.length > 0;
  const managedProviderKeys = collectManagedProviderKeys(rawConfig);
  const models = hasFlatModels
    ? normalizeFlatModels((rawConfig.models || []) as Array<Partial<ModelEntry> | null | undefined>, managedProviderKeys)
    : flattenProvidersToModels((rawConfig.providers || {}) as Record<string, RawProviderEntry | null | undefined>, managedProviderKeys);

  const strategies = normalizeModelStrategies(rawConfig.strategies);
  assertDefaultChatModelEnabled(strategies, models);
  const modelPreferences = normalizeModelPreferences(rawConfig.modelPreferences, models);
  if (models.length === 0) {
    return {
      version,
      defaultModelKey: null,
      providers: {},
      models: [],
      strategies,
      modelPreferences,
      updatedAt: typeof rawConfig.updatedAt === 'number' ? rawConfig.updatedAt : 0,
    };
  }

  const providers = buildProvidersFromFlatModels(models, managedProviderKeys);
  const pickedDefaultModelKey = pickDefault(models, defaultModelKey);
  return {
    version,
    defaultModelKey: pickedDefaultModelKey,
    providers,
    models,
    strategies,
    modelPreferences,
    updatedAt: typeof rawConfig.updatedAt === 'number' ? rawConfig.updatedAt : 0,
  };
}

function serializeModelsConfig(config: ModelsConfig): Omit<ModelsConfig, 'models'> {
  return {
    version: config.version,
    defaultModelKey: config.defaultModelKey,
    providers: config.providers,
    strategies: normalizeModelStrategies(config.strategies),
    modelPreferences: normalizeModelPreferences(config.modelPreferences, config.models),
    updatedAt: config.updatedAt,
  };
}

export function getModelsConfigDir(homeDir: string): string {
  return path.join(homeDir, '.openbrain', 'configs', 'user');
}

export function getModelsConfigPath(homeDir: string): string {
  return path.join(getModelsConfigDir(homeDir), 'models.json');
}

export function createEmptyModelsConfig(): ModelsConfig {
  return {
    version: CURRENT_VERSION,
    defaultModelKey: null,
    providers: {},
    models: [],
    strategies: undefined,
    modelPreferences: undefined,
    updatedAt: 0,
  };
}

export async function loadModelsConfig(homeDir: string): Promise<ModelsConfig> {
  const configPath = getModelsConfigPath(homeDir);
  try {
    const data = await fs.readFile(configPath, 'utf8');
    const parsed = JSON.parse(data) as RawModelsConfig;
    return normalizeModelsConfig(parsed);
  } catch (error) {
    if ((error as NodeJS.ErrnoException)?.code === 'ENOENT') {
      return createEmptyModelsConfig();
    }
    throw error;
  }
}

export async function saveModelsConfig(homeDir: string, config: ModelsConfig): Promise<ModelsConfig> {
  const configDir = getModelsConfigDir(homeDir);
  const configPath = getModelsConfigPath(homeDir);
  const now = Date.now();
  const normalized = normalizeModelsConfig({
    ...config,
    version: CURRENT_VERSION,
    updatedAt: now,
  });
  await fs.mkdir(configDir, { recursive: true });
  await writeJsonFileAtomic(configPath, serializeModelsConfig(normalized));
  return normalized;
}

function strategyForProvider(strategies: ModelStrategies | undefined, providerKey: string): ModelStrategies | undefined {
  const auto = normalizeModelStrategies(strategies)?.auto;
  if (!auto) {
    return undefined;
  }
  return {
    auto: {
      ...(auto.defaultChatModelID ? {
        defaultChatModelID: buildModelKey(providerKey, auto.defaultChatModelID),
      } : {}),
      ...(auto.defaultChatThinkingLevel ? { defaultChatThinkingLevel: auto.defaultChatThinkingLevel } : {}),
      ...(auto.defaultInlineCompletionModelID ? {
        defaultInlineCompletionModelID: buildModelKey(providerKey, auto.defaultInlineCompletionModelID),
      } : {}),
      ...(auto.defaultInlineCompletionThinkingLevel ? {
        defaultInlineCompletionThinkingLevel: auto.defaultInlineCompletionThinkingLevel,
      } : {}),
    },
  };
}

function strategyModelExists(
  strategy: ModelStrategies | undefined,
  models: ModelEntry[],
  field: 'defaultChatModelID' | 'defaultInlineCompletionModelID',
): boolean {
  const key = normalizeModelKey(strategy?.auto?.[field]);
  return Boolean(key && models.some((model) => model.enabled && model.key === key));
}

function normalizePrivateStrategies(
  strategy: ModelStrategies | undefined,
  models: ModelEntry[],
): ModelStrategies | undefined {
  const auto = normalizeModelStrategies(strategy)?.auto;
  if (!auto) {
    return undefined;
  }
  const defaultChatModelID = strategyModelExists(strategy, models, 'defaultChatModelID')
    ? auto?.defaultChatModelID
    : undefined;
  const defaultInlineCompletionModelID = strategyModelExists(strategy, models, 'defaultInlineCompletionModelID')
    ? auto?.defaultInlineCompletionModelID
    : undefined;
  if (!defaultChatModelID && !defaultInlineCompletionModelID) {
    return undefined;
  }
  return {
    auto: {
      ...(defaultChatModelID ? { defaultChatModelID } : {}),
      ...(defaultChatModelID && auto?.defaultChatThinkingLevel ? {
        defaultChatThinkingLevel: auto.defaultChatThinkingLevel,
      } : {}),
      ...(defaultInlineCompletionModelID ? { defaultInlineCompletionModelID } : {}),
      ...(defaultInlineCompletionModelID && auto?.defaultInlineCompletionThinkingLevel ? {
        defaultInlineCompletionThinkingLevel: auto.defaultInlineCompletionThinkingLevel,
      } : {}),
    },
  };
}

function normalizeOpenBrainOrgCatalog(catalog: OpenBrainCatalog): OpenBrainOrgCatalog {
  const providerKey = normalizeProviderKey(catalog.providerKey) || OPENBRAIN_PROVIDER_KEY;
  return {
    providerKey,
    providerLabel: formatModelProviderLabel(providerKey, catalog.providerLabel) || providerKey,
    models: catalog.models || [],
    strategies: catalog.strategies,
  };
}

export function mergeOpenBrainOrgCatalogs(
  config: ModelsConfig,
  catalogs: OpenBrainCatalog[],
  now = Date.now(),
  options: MergeOpenBrainOrgCatalogsOptions = {},
): ModelsConfig {
  const normalized = normalizeModelsConfig(config);
  const orgCatalogs = catalogs.map(normalizeOpenBrainOrgCatalog);
  const managedProviderKeys = new Set(orgCatalogs.map((catalog) => catalog.providerKey));
  const previousManagedProviderKeys = new Set(
    Object.entries(normalized.providers)
      .filter(([providerKey, provider]) => provider.managed === true || providerKey === OPENBRAIN_PROVIDER_KEY)
      .map(([providerKey]) => providerKey),
  );
  const providerLabels = new Map(orgCatalogs.map((catalog) => [catalog.providerKey, catalog.providerLabel] as const));
  const remoteModelsByProvider = new Map<string, Map<string, OpenBrainModelEntry>>();
  for (const orgCatalog of orgCatalogs) {
    const remoteModels = new Map<string, OpenBrainModelEntry>();
    for (const model of orgCatalog.models) {
      const id = normalizeId(model.id);
      if (!id) {
        continue;
      }
      remoteModels.set(id, {
        id,
        label: normalizeOptional(model.label),
        api: normalizeApi(model.api),
        reasoning: model.reasoning === true,
        reasoningControl: resolveModelReasoningControl({
          reasoning: model.reasoning === true,
          reasoningLevels: normalizeReasoningLevels(model.reasoningLevels),
          reasoningControl: model.reasoningControl,
        }),
        reasoningLevels: normalizeReasoningLevels(model.reasoningLevels),
        contextWindows: normalizePositiveIntArray(model.contextWindows),
        defaultContextWindow: normalizePositiveInt(model.defaultContextWindow),
        serviceTiers: normalizeServiceTierArray(model.serviceTiers),
        maxOutputTokens: normalizePositiveInt(model.maxOutputTokens),
      });
    }
    remoteModelsByProvider.set(orgCatalog.providerKey, remoteModels);
  }

  const modelsByKey = new Map<string, ModelEntry>();
  let changed = false;
  for (const model of normalized.models) {
    const normalizedProvider = model.provider;
    if (previousManagedProviderKeys.has(normalizedProvider) && !managedProviderKeys.has(normalizedProvider)) {
      changed = true;
      continue;
    }
    if (!managedProviderKeys.has(normalizedProvider)) {
      modelsByKey.set(model.key, model);
      continue;
    }
    const providerLabel = providerLabels.get(normalizedProvider) || model.providerLabel;
    const remoteModels = remoteModelsByProvider.get(normalizedProvider) || new Map<string, OpenBrainModelEntry>();
    const remoteModel = remoteModels.get(model.id);
    if (!remoteModel) {
      changed = true;
      continue;
    }
    const cleaned = normalizeFlatEntry({
      key: buildModelKey(normalizedProvider, remoteModel.id),
      id: remoteModel.id,
      enabled: model.enabled,
      provider: normalizedProvider,
      providerLabel,
      label: remoteModel.label || model.label,
      api: remoteModel.api,
      reasoning: remoteModel.reasoning,
      reasoningControl: remoteModel.reasoningControl,
      reasoningLevels: remoteModel.reasoningLevels,
      contextWindows: remoteModel.contextWindows,
      defaultContextWindow: remoteModel.defaultContextWindow,
      serviceTiers: remoteModel.serviceTiers,
      maxOutputTokens: remoteModel.maxOutputTokens,
      baseUrl: undefined,
      apiKey: undefined,
      updatedAt: model.updatedAt,
    }, managedProviderKeys);
    if (
      cleaned.label !== model.label ||
      cleaned.key !== model.key ||
      cleaned.api !== model.api ||
      cleaned.provider !== model.provider ||
      cleaned.providerLabel !== model.providerLabel ||
      cleaned.reasoning !== model.reasoning ||
      cleaned.reasoningControl !== model.reasoningControl ||
      JSON.stringify(cleaned.reasoningLevels || []) !== JSON.stringify(model.reasoningLevels || []) ||
      cleaned.contextWindow !== model.contextWindow ||
      JSON.stringify(cleaned.contextWindows || []) !== JSON.stringify(model.contextWindows || []) ||
      cleaned.defaultContextWindow !== model.defaultContextWindow ||
      JSON.stringify(cleaned.serviceTiers || []) !== JSON.stringify(model.serviceTiers || []) ||
      cleaned.maxOutputTokens !== model.maxOutputTokens ||
      cleaned.baseUrl !== model.baseUrl ||
      cleaned.apiKey !== model.apiKey
    ) {
      changed = true;
    }
    modelsByKey.set(cleaned.key, cleaned);
  }

  for (const orgCatalog of orgCatalogs) {
    const remoteModels = remoteModelsByProvider.get(orgCatalog.providerKey) || new Map<string, OpenBrainModelEntry>();
    for (const remoteModel of remoteModels.values()) {
      const key = buildModelKey(orgCatalog.providerKey, remoteModel.id);
      if (modelsByKey.has(key)) {
        continue;
      }
      modelsByKey.set(key, {
        key,
        id: remoteModel.id,
        label: remoteModel.label,
        enabled: true,
        provider: orgCatalog.providerKey,
        providerLabel: orgCatalog.providerLabel,
        api: remoteModel.api,
        reasoning: remoteModel.reasoning,
        reasoningControl: remoteModel.reasoningControl,
        reasoningLevels: remoteModel.reasoningLevels,
        contextWindows: remoteModel.contextWindows,
        defaultContextWindow: remoteModel.defaultContextWindow,
        serviceTiers: remoteModel.serviceTiers,
        maxOutputTokens: remoteModel.maxOutputTokens,
        updatedAt: now,
      });
      changed = true;
    }
  }

  const activeOrgID = normalizeProviderKey(options.activeOrgID);
  const activeCatalog = activeOrgID ? orgCatalogs.find((catalog) => catalog.providerKey === activeOrgID) : undefined;
  const defaultCatalog = orgCatalogs.find((catalog) => catalog.providerKey === OPENBRAIN_PROVIDER_KEY);
  const strategyCatalog = activeCatalog || defaultCatalog;
  const rawStrategies = strategyCatalog ? strategyForProvider(strategyCatalog.strategies, strategyCatalog.providerKey) : undefined;
  const nextModels = Array.from(modelsByKey.values());
  const nextStrategies = options.privateOnly && strategyCatalog
    ? normalizePrivateStrategies(rawStrategies, nextModels)
    : rawStrategies;
  if (JSON.stringify(nextStrategies || null) !== JSON.stringify(normalized.strategies || null)) {
    changed = true;
  }

  const currentDefault = nextModels.find((model) => model.key === normalized.defaultModelKey) || null;
  const activeDefault = activeCatalog
    ? nextModels.find((model) => model.enabled && model.provider === activeCatalog.providerKey) || null
    : null;
  const shouldPreferActiveDefault = Boolean(
    activeDefault
      && (options.privateOnly || !currentDefault || managedProviderKeys.has(currentDefault.provider)),
  );
  const nextDefaultModelKey = shouldPreferActiveDefault
    ? activeDefault?.key || null
    : normalized.defaultModelKey;
  if ((nextDefaultModelKey || null) !== (normalized.defaultModelKey || null)) {
    changed = true;
  }

  if (!changed) {
    return normalized;
  }

  return normalizeModelsConfig({
    ...normalized,
    defaultModelKey: nextDefaultModelKey,
    providers: buildProvidersFromFlatModels(nextModels, managedProviderKeys),
    models: nextModels,
    strategies: nextStrategies,
    updatedAt: now,
  });
}

export function mergeOpenBrainModels(config: ModelsConfig, catalog: OpenBrainCatalog, now = Date.now()): ModelsConfig {
  return mergeOpenBrainOrgCatalogs(config, [catalog], now);
}
