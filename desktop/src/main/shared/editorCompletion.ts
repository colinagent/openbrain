export type EditorCompletionMode = 'default' | 'custom' | 'off';

export type EditorCompletionSettings = {
  enabled: boolean;
  mode: EditorCompletionMode;
  customModelKey?: string | null;
};

export const DEFAULT_EDITOR_COMPLETION_SETTINGS: EditorCompletionSettings = {
  enabled: true,
  mode: 'default',
  customModelKey: null,
};

export function normalizeEditorCompletionMode(
  value: unknown,
): EditorCompletionMode {
  switch (value) {
    case 'default':
    case 'custom':
    case 'off':
      return value;
    case 'auto':
    case 'chat':
      return 'default';
    default:
      return DEFAULT_EDITOR_COMPLETION_SETTINGS.mode;
  }
}

export function normalizeEditorCompletionSettings(
  value: unknown,
): EditorCompletionSettings {
  if (!value || typeof value !== 'object') {
    return DEFAULT_EDITOR_COMPLETION_SETTINGS;
  }
  const raw = value as Record<string, unknown>;
  const customModelKey =
    typeof raw.customModelKey === 'string' && raw.customModelKey.trim()
      ? raw.customModelKey.trim()
      : null;
  return {
    enabled:
      typeof raw.enabled === 'boolean'
        ? raw.enabled
        : DEFAULT_EDITOR_COMPLETION_SETTINGS.enabled,
    mode: normalizeEditorCompletionMode(raw.mode),
    customModelKey,
  };
}
