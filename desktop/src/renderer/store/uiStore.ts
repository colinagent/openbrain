import { create } from 'zustand';
import { defaultThemeId as fallbackDefaultThemeId, ThemeDefinition, themes as fallbackThemes } from '../theme/presets';
import type { SidebarView } from '../components/Sidebar/sidebarTabs';
import {
  DEFAULT_UI_CHAT_THINKING_LEVEL,
  normalizeUiChatThinkingLevel,
  type UiChatThinkingLevel,
} from '../../main/shared/chatThinking';
import {
  DEFAULT_EDITOR_COMPLETION_SETTINGS,
  normalizeEditorCompletionSettings,
  type EditorCompletionSettings,
} from '../../main/shared/editorCompletion';
import {
  activateTheme,
  getAvailableThemes,
  getCurrentThemeId,
  getCurrentThemeSnapshot,
  getDefaultThemeId,
  getThemeById,
} from '../theme/runtime';

type UiState = {
  availableThemes: ThemeDefinition[];
  themeId: string;
  theme: ThemeDefinition;
  themeReady: boolean;
  showLineNumbers: boolean;
  chatThinkingLevel: UiChatThinkingLevel;
  completion: EditorCompletionSettings;
  sidebarView: SidebarView;
  blockingModalIds: string[];
  hasBlockingModal: boolean;
  setThemeId: (themeId: string) => void;
  setShowLineNumbers: (showLineNumbers: boolean) => void;
  setChatThinkingLevel: (chatThinkingLevel: UiChatThinkingLevel) => void;
  setCompletion: (completion: unknown) => void;
  setSidebarView: (view: SidebarView) => void;
  registerBlockingModal: (id: string) => void;
  unregisterBlockingModal: (id: string) => void;
  hydrateThemeState: (themes: ThemeDefinition[], activeTheme: ThemeDefinition) => void;
  syncThemeState: () => void;
};

function normalizeShowLineNumbers(value: unknown): boolean {
  return typeof value === 'boolean' ? value : true;
}

function resolveFallbackTheme(themes: ThemeDefinition[]): ThemeDefinition {
  return themes[0] || fallbackThemes.find((theme) => theme.id === fallbackDefaultThemeId) || fallbackThemes[0];
}

function resolveRuntimeTheme(themes: ThemeDefinition[]): { themeId: string; theme: ThemeDefinition } {
  const fallback = resolveFallbackTheme(themes);
  const runtimeThemeId = getCurrentThemeId() || getDefaultThemeId();
  const currentTheme = getThemeById(runtimeThemeId);
  if (currentTheme) {
    return {
      themeId: currentTheme.id,
      theme: currentTheme,
    };
  }
  const snapshot = getCurrentThemeSnapshot();
  if (snapshot && snapshot.themeId === runtimeThemeId) {
    return {
      themeId: runtimeThemeId,
      theme: {
        ...fallback,
        id: snapshot.themeId,
        label: snapshot.label,
        scheme: snapshot.scheme,
      },
    };
  }
  return {
    themeId: fallback.id,
    theme: fallback,
  };
}

function normalizeBlockingModalId(id: string): string {
  return id.trim();
}

export const useUiStore = create<UiState>((set) => ({
  availableThemes: getAvailableThemes(),
  themeId: (() => {
    const available = getAvailableThemes();
    return resolveRuntimeTheme(available).themeId;
  })(),
  theme: (() => {
    const available = getAvailableThemes();
    return resolveRuntimeTheme(available).theme;
  })(),
  themeReady: false,
  showLineNumbers: true,
  chatThinkingLevel: DEFAULT_UI_CHAT_THINKING_LEVEL,
  completion: DEFAULT_EDITOR_COMPLETION_SETTINGS,
  sidebarView: 'workspace',
  blockingModalIds: [],
  hasBlockingModal: false,
  setThemeId: (themeId) => {
    const theme = activateTheme(themeId);
    set({
      availableThemes: getAvailableThemes(),
      themeId: theme.id,
      theme,
      themeReady: true,
    });
  },
  setShowLineNumbers: (showLineNumbers) => {
    set({
      showLineNumbers: normalizeShowLineNumbers(showLineNumbers),
    });
  },
  setChatThinkingLevel: (chatThinkingLevel) => {
    set({
      chatThinkingLevel: normalizeUiChatThinkingLevel(chatThinkingLevel),
    });
  },
  setCompletion: (completion) => {
    set({
      completion: normalizeEditorCompletionSettings(completion),
    });
  },
  setSidebarView: (view) => {
    set({ sidebarView: view });
  },
  registerBlockingModal: (id) => set((state) => {
    const normalizedId = normalizeBlockingModalId(id);
    if (!normalizedId || state.blockingModalIds.includes(normalizedId)) {
      return {};
    }
    const blockingModalIds = [...state.blockingModalIds, normalizedId];
    return {
      blockingModalIds,
      hasBlockingModal: blockingModalIds.length > 0,
    };
  }),
  unregisterBlockingModal: (id) => set((state) => {
    const normalizedId = normalizeBlockingModalId(id);
    if (!normalizedId || !state.blockingModalIds.includes(normalizedId)) {
      return {};
    }
    const blockingModalIds = state.blockingModalIds.filter((existingId) => existingId !== normalizedId);
    return {
      blockingModalIds,
      hasBlockingModal: blockingModalIds.length > 0,
    };
  }),
  hydrateThemeState: (themes, activeTheme) => {
    set({
      availableThemes: themes,
      themeId: activeTheme.id,
      theme: activeTheme,
      themeReady: true,
    });
  },
  syncThemeState: () => {
    const available = getAvailableThemes();
    const current = resolveRuntimeTheme(available);
    set({
      availableThemes: available,
      themeId: current.themeId,
      theme: current.theme,
      themeReady: true,
    });
  },
}));
