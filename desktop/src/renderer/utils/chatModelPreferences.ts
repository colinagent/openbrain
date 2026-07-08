import type { ModelEntry, ModelServiceTier, ModelsConfig } from '../types/electron';
import {
  DEFAULT_UI_CHAT_THINKING_LEVEL,
  normalizeUiChatThinkingLevel,
} from '../../main/shared/chatThinking';
import { normalizeThinkingLevelForModel, type ThinkingLevel } from './chatThinking';

export type ResolvedChatModelPreference = {
  thinkingLevel: ThinkingLevel;
  contextWindow: number | null;
  contextWindowOptions: number[];
  serviceTier: ModelServiceTier | null;
};

export const FALLBACK_CONTEXT_WINDOW = 200_000;

function normalizePositiveInteger(value: unknown): number | null {
  return typeof value === 'number' && Number.isInteger(value) && value > 0 ? value : null;
}

function normalizeServiceTier(value: unknown): ModelServiceTier | null {
  if (typeof value !== 'string') {
    return null;
  }
  switch (value.trim().toLowerCase()) {
    case 'priority':
      return 'priority';
    case 'flex':
      return 'flex';
    default:
      return null;
  }
}

export function modelSupportsPriorityServiceTier(model: Pick<ModelEntry, 'serviceTiers'> | null): boolean {
  if (!model || !Array.isArray(model.serviceTiers)) {
    return false;
  }
  return model.serviceTiers.some((tier) => normalizeServiceTier(tier) === 'priority');
}

export function getModelContextWindowOptions(model: Pick<ModelEntry, 'contextWindows'> | null): number[] {
  if (!model) {
    return [];
  }
  const seen = new Set<number>();
  const out: number[] = [];
  for (const raw of Array.isArray(model.contextWindows) ? model.contextWindows : []) {
    const value = normalizePositiveInteger(raw);
    if (!value || seen.has(value)) {
      continue;
    }
    seen.add(value);
    out.push(value);
  }
  return out.sort((a, b) => a - b);
}

export function getDefaultContextWindowForModel(model: Pick<ModelEntry, 'contextWindows' | 'defaultContextWindow'> | null): number | null {
  const options = getModelContextWindowOptions(model);
  if (options.length === 0) {
    return model ? FALLBACK_CONTEXT_WINDOW : null;
  }
  const defaultContextWindow = normalizePositiveInteger(model?.defaultContextWindow);
  if (defaultContextWindow && options.includes(defaultContextWindow)) {
    return defaultContextWindow;
  }
  return options[options.length - 1] || null;
}

function defaultThinkingLevelForModel(config: ModelsConfig, model: ModelEntry | null): ThinkingLevel {
  if (!model) {
    return DEFAULT_UI_CHAT_THINKING_LEVEL;
  }
  const auto = config.strategies?.auto;
  if (auto?.defaultChatModelID === model.key) {
    return normalizeUiChatThinkingLevel(auto.defaultChatThinkingLevel);
  }
  return DEFAULT_UI_CHAT_THINKING_LEVEL;
}

export function resolveChatModelPreference(config: ModelsConfig, model: ModelEntry | null): ResolvedChatModelPreference {
  const modelPreference = model ? config.modelPreferences?.[model.key] : undefined;
  const contextWindowOptions = getModelContextWindowOptions(model);
  const preferredContextWindow = normalizePositiveInteger(modelPreference?.contextWindow);
  const contextWindow = preferredContextWindow && contextWindowOptions.includes(preferredContextWindow)
    ? preferredContextWindow
    : getDefaultContextWindowForModel(model);
  const rawThinkingLevel = modelPreference?.thinkingLevel || defaultThinkingLevelForModel(config, model);
  const serviceTier = modelSupportsPriorityServiceTier(model) && normalizeServiceTier(modelPreference?.serviceTier) === 'priority'
    ? 'priority'
    : null;

  return {
    thinkingLevel: normalizeThinkingLevelForModel(model, rawThinkingLevel),
    contextWindow,
    contextWindowOptions,
    serviceTier,
  };
}

export function formatContextWindowOption(value: number | null | undefined): string {
  const normalized = normalizePositiveInteger(value);
  if (!normalized) {
    return '';
  }
  if (normalized >= 1_000_000 && normalized % 1_000_000 === 0) {
    return `${normalized / 1_000_000}M`;
  }
  if (normalized >= 1000 && normalized % 1000 === 0) {
    return `${normalized / 1000}K`;
  }
  return normalized.toLocaleString();
}
