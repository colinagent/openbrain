import {
  DEFAULT_UI_CHAT_THINKING_LEVEL,
  normalizeUiChatThinkingLevel,
  type UiChatThinkingLevel,
} from '../../main/shared/chatThinking';
import { useUiStore } from '../store/uiStore';
import type { ModelEntry } from '../types/electron';

export type ThinkingLevel = UiChatThinkingLevel;
type ThinkingModel = Pick<ModelEntry, 'reasoning' | 'reasoningControl' | 'reasoningLevels'>;
export const UI_CHAT_THINKING_ON_LEVEL = 'on';

function normalizeSupportedThinkingLevel(value: unknown): ThinkingLevel | null {
  if (typeof value !== 'string') {
    return null;
  }
  const normalized = value.trim();
  if (!normalized || normalized === DEFAULT_UI_CHAT_THINKING_LEVEL) {
    return null;
  }
  return normalized;
}

export function getSupportedThinkingLevels(model: ThinkingModel | null): ThinkingLevel[] {
  if (!model?.reasoning) {
    return [];
  }
  if (model.reasoningControl === 'toggle') {
    return [UI_CHAT_THINKING_ON_LEVEL];
  }
  const reasoningLevels = model?.reasoningLevels;
  if (!Array.isArray(reasoningLevels) || reasoningLevels.length === 0) {
    return [];
  }

  const seen = new Set<ThinkingLevel>();
  const normalized: ThinkingLevel[] = [];
  for (const rawLevel of reasoningLevels) {
    const level = normalizeSupportedThinkingLevel(rawLevel);
    if (!level || seen.has(level)) {
      continue;
    }
    seen.add(level);
    normalized.push(level);
  }
  return normalized;
}

export function getThinkingPickerLevels(model: ThinkingModel | null): ThinkingLevel[] {
  return [DEFAULT_UI_CHAT_THINKING_LEVEL, ...getSupportedThinkingLevels(model)];
}

export function isThinkingLevelSupported(model: ThinkingModel | null, level: ThinkingLevel): boolean {
  const normalized = normalizeUiChatThinkingLevel(level);
  if (normalized === DEFAULT_UI_CHAT_THINKING_LEVEL) {
    return true;
  }
  return getSupportedThinkingLevels(model).includes(normalized);
}

export function normalizeThinkingLevelForModel(model: ThinkingModel | null, level: ThinkingLevel): ThinkingLevel {
  const normalized = normalizeUiChatThinkingLevel(level);
  return isThinkingLevelSupported(model, normalized) ? normalized : DEFAULT_UI_CHAT_THINKING_LEVEL;
}

export function getGlobalThinkingLevel(): ThinkingLevel {
  return normalizeUiChatThinkingLevel(useUiStore.getState().chatThinkingLevel);
}

export async function persistGlobalThinkingLevel(nextLevel: ThinkingLevel): Promise<ThinkingLevel> {
  const prevLevel = getGlobalThinkingLevel();
  const normalized = normalizeUiChatThinkingLevel(nextLevel);
  useUiStore.getState().setChatThinkingLevel(normalized);
  if (!window.electronAPI?.settings?.set) {
    return normalized;
  }
  try {
    const settings = await window.electronAPI.settings.set({
      ui: {
        chatThinkingLevel: normalized,
      },
    });
    const resolved = normalizeUiChatThinkingLevel(settings?.ui?.chatThinkingLevel);
    useUiStore.getState().setChatThinkingLevel(resolved);
    return resolved;
  } catch (error) {
    useUiStore.getState().setChatThinkingLevel(prevLevel);
    throw error;
  }
}
