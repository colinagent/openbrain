export const DEFAULT_UI_CHAT_THINKING_LEVEL = 'off';

export type UiChatThinkingLevel = string;

export function normalizeUiChatThinkingLevel(value: unknown): UiChatThinkingLevel {
  if (typeof value !== 'string') {
    return DEFAULT_UI_CHAT_THINKING_LEVEL;
  }
  const normalized = value.trim();
  return normalized || DEFAULT_UI_CHAT_THINKING_LEVEL;
}
