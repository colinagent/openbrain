import React from 'react';
import ReactDOM from 'react-dom/client';
import { I18nextProvider } from 'react-i18next';
import App from './App';
import { normalizeDisplayLocale } from '../main/i18n/locales';
import { initRendererI18n, rendererI18n, setRendererI18nLocale } from '../main/i18n/renderer';
import { MarkdownPdfExportRoot } from './components/PdfExport/MarkdownPdfExportRoot';
import './styles/index.css';
import './utils/scrollbarObserver';
import '@fontsource/jetbrains-mono/400.css';
import '@fontsource/jetbrains-mono/600.css';
import 'katex/dist/katex.min.css';
import 'pdfjs-dist/web/pdf_viewer.css';
import { buildThemeDefinitionsFromConfig, buildMarkdownThemesFromConfig, buildCodeThemesFromConfig } from './theme';
import { defaultThemeId } from './theme/presets';
import {
  activateTheme,
  getAvailableThemes,
  getDefaultThemeId,
  reapplyCurrentThemeSnapshot,
  restoreThemeSnapshot,
  setThemeRuntimeDefinitions,
} from './theme/runtime';
import { useUiStore } from './store/uiStore';
import { useAppStore } from './store/appStore';
import { useFileExcludeStore } from './store/fileExcludeStore';
import {
  DEFAULT_MARKDOWN_CONTENT_WIDTH,
  DEFAULT_MARKDOWN_TEXT_OFFSET,
  isMarkdownContentWidthDragLocked,
  isMarkdownTextOffsetDragLocked,
  normalizeMarkdownContentWidth,
  normalizeMarkdownTextOffset,
  setMarkdownContentWidthCssVar,
  setMarkdownTextOffsetCssVar,
} from './utils/markdownTextOffset';
import { normalizeUiChatThinkingLevel } from '../main/shared/chatThinking';
import { normalizeEditorCompletionSettings } from '../main/shared/editorCompletion';
import { DEFAULT_WINDOW_ZOOM_LEVEL, normalizeSteppedWindowZoomLevel } from '../main/shared/windowZoom';
import {
  disposeWindowZoomShortcuts,
  installWindowZoomShortcuts,
  setCurrentWindowZoomLevel,
} from './services/windowZoomShortcuts';

type UiSettings = {
  chatThinkingLevel?: unknown;
  showLineNumbers?: boolean;
  fontSize?: number;
  sidebarFontSize?: number;
  markdownContentWidth?: number;
  markdownTextOffset?: number;
  markdownOutlineWidth?: number;
  titlebarFontSize?: number;
  statusbarFontSize?: number;
  tabBarHeight?: number;
  tabBarFontSize?: number;
  fontFamily?: string;
  zoomLevel?: number;
  completion?: unknown;
  markdownFontSize?: {
    heading1?: number;
    heading2?: number;
    heading3?: number;
    heading4?: number;
  };
};

type BootstrapSettings = {
  ui?: UiSettings & {
    themeId?: string;
    displayLocale?: string;
  };
  editor?: unknown;
  theme?: unknown;
  markdownThemes?: unknown;
  codeThemes?: unknown;
};

type HotModuleApi = {
  dispose: (callback: () => void) => void;
};

const DEFAULT_UI_FONT_SIZE = 13;
const ROOT_VIEW = new URLSearchParams(window.location.search).get('view') || '';
const IS_MARKDOWN_PDF_EXPORT_VIEW = ROOT_VIEW === 'markdown-pdf-export';

function normalizeNumber(value: unknown, fallback: number) {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function normalizeFontSize(value: unknown, fallback: number) {
  const normalized = normalizeNumber(value, fallback);
  return normalized > 0 ? normalized : fallback;
}

const DEFAULT_TABBAR_HEIGHT = 32;

const DEFAULT_MARKDOWN_HEADING_SIZES = {
  heading1: 1.8,
  heading2: 1.5,
  heading3: 1.25,
  heading4: 1.1,
};

function normalizeMarkdownHeadingSize(value: unknown, fallback: number): number {
  const n = typeof value === 'number' && Number.isFinite(value) ? value : fallback;
  return n > 0 ? n : fallback;
}

function normalizeShowLineNumbers(value: unknown): boolean {
  return typeof value === 'boolean' ? value : true;
}

function getThemeSignature(settings?: BootstrapSettings): string {
  return JSON.stringify({
    themeId: settings?.ui?.themeId ?? null,
    theme: settings?.theme ?? null,
    markdownThemes: settings?.markdownThemes ?? null,
    codeThemes: settings?.codeThemes ?? null,
  });
}

let lastThemeSignature = '';

function applyUiSettings(ui?: UiSettings) {
  const baseFontSize = normalizeFontSize(ui?.fontSize, DEFAULT_UI_FONT_SIZE);
  const sidebarFontSize = normalizeFontSize(ui?.sidebarFontSize, baseFontSize);
  const titlebarFontSize = normalizeFontSize(ui?.titlebarFontSize, baseFontSize);
  const statusbarFontSize = normalizeFontSize(ui?.statusbarFontSize, baseFontSize);
  const tabBarHeight = normalizeNumber(ui?.tabBarHeight, DEFAULT_TABBAR_HEIGHT);
  const tabBarFontSize = normalizeFontSize(ui?.tabBarFontSize, baseFontSize);
  const zoomLevel = IS_MARKDOWN_PDF_EXPORT_VIEW
    ? DEFAULT_WINDOW_ZOOM_LEVEL
    : normalizeSteppedWindowZoomLevel(ui?.zoomLevel);
  const markdownTextOffset = IS_MARKDOWN_PDF_EXPORT_VIEW
    ? DEFAULT_MARKDOWN_TEXT_OFFSET
    : normalizeMarkdownTextOffset(ui?.markdownTextOffset);
  const markdownContentWidth = IS_MARKDOWN_PDF_EXPORT_VIEW
    ? DEFAULT_MARKDOWN_CONTENT_WIDTH
    : normalizeMarkdownContentWidth(ui?.markdownContentWidth);

  const md = ui?.markdownFontSize;
  const h1 = normalizeMarkdownHeadingSize(md?.heading1, DEFAULT_MARKDOWN_HEADING_SIZES.heading1);
  const h2 = normalizeMarkdownHeadingSize(md?.heading2, DEFAULT_MARKDOWN_HEADING_SIZES.heading2);
  const h3 = normalizeMarkdownHeadingSize(md?.heading3, DEFAULT_MARKDOWN_HEADING_SIZES.heading3);
  const h4 = normalizeMarkdownHeadingSize(md?.heading4, DEFAULT_MARKDOWN_HEADING_SIZES.heading4);

  const root = document.documentElement;
  root.style.fontSize = `${baseFontSize}px`;
  root.style.setProperty('--op-ui-font-size', `${baseFontSize}px`);
  root.style.setProperty('--op-ui-sidebar-font-size', `${sidebarFontSize}px`);
  root.style.setProperty('--op-ui-titlebar-font-size', `${titlebarFontSize}px`);
  root.style.setProperty('--op-ui-statusbar-font-size', `${statusbarFontSize}px`);
  root.style.setProperty('--op-ui-tabbar-height', `${tabBarHeight}px`);
  root.style.setProperty('--op-ui-tabbar-font-size', `${tabBarFontSize}px`);
  root.style.setProperty('--op-ui-markdown-heading1-size', `${h1}em`);
  root.style.setProperty('--op-ui-markdown-heading2-size', `${h2}em`);
  root.style.setProperty('--op-ui-markdown-heading3-size', `${h3}em`);
  root.style.setProperty('--op-ui-markdown-heading4-size', `${h4}em`);
  if (!isMarkdownTextOffsetDragLocked()) {
    setMarkdownTextOffsetCssVar(markdownTextOffset);
  }
  if (!isMarkdownContentWidthDragLocked()) {
    setMarkdownContentWidthCssVar(markdownContentWidth);
  }
  if (typeof ui?.fontFamily === 'string' && ui.fontFamily.trim()) {
    root.style.setProperty('--op-ui-font-family', ui.fontFamily.trim());
  } else {
    root.style.removeProperty('--op-ui-font-family');
  }

  setCurrentWindowZoomLevel(zoomLevel);
  window.electronAPI?.window?.setZoomLevel?.(zoomLevel);
}

function applySettingsTheme(settings?: BootstrapSettings) {
  useFileExcludeStore.getState().setFromSettings(settings);
  const markdownThemeDefs = buildMarkdownThemesFromConfig(settings?.markdownThemes) || [];
  const codeThemeDefs = buildCodeThemesFromConfig(settings?.codeThemes) || [];
  const themeDefs = buildThemeDefinitionsFromConfig(settings?.theme, markdownThemeDefs, codeThemeDefs);
  setThemeRuntimeDefinitions(themeDefs, defaultThemeId);
  const activeTheme = activateTheme(settings?.ui?.themeId);
  useUiStore.getState().hydrateThemeState(getAvailableThemes(), activeTheme);
  useUiStore.getState().setShowLineNumbers(normalizeShowLineNumbers(settings?.ui?.showLineNumbers));
  useUiStore.getState().setChatThinkingLevel(normalizeUiChatThinkingLevel(settings?.ui?.chatThinkingLevel));
  useUiStore.getState().setCompletion(normalizeEditorCompletionSettings(settings?.ui?.completion));
  applyUiSettings(settings?.ui);
  applyDisplayLocale(settings);
  lastThemeSignature = getThemeSignature(settings);
}

function applyFallbackTheme() {
  useFileExcludeStore.getState().setFromSettings(undefined);
  setThemeRuntimeDefinitions(null, defaultThemeId);
  const activeTheme = activateTheme(getDefaultThemeId());
  useUiStore.getState().hydrateThemeState(getAvailableThemes(), activeTheme);
  useUiStore.getState().setShowLineNumbers(true);
  useUiStore.getState().setChatThinkingLevel('off');
  useUiStore.getState().setCompletion(undefined);
  applyUiSettings();
  lastThemeSignature = '';
}

async function applyDisplayLocale(settings?: BootstrapSettings) {
  await setRendererI18nLocale(
    normalizeDisplayLocale(settings?.ui?.displayLocale, navigator.language),
  );
}

function applySettingsUiOnly(settings?: BootstrapSettings) {
  useFileExcludeStore.getState().setFromSettings(settings);
  useUiStore.getState().setShowLineNumbers(normalizeShowLineNumbers(settings?.ui?.showLineNumbers));
  useUiStore.getState().setChatThinkingLevel(normalizeUiChatThinkingLevel(settings?.ui?.chatThinkingLevel));
  useUiStore.getState().setCompletion(normalizeEditorCompletionSettings(settings?.ui?.completion));
  applyUiSettings(settings?.ui);
  applyDisplayLocale(settings);
}

function syncFromCurrentThemeState(): boolean {
  if (!reapplyCurrentThemeSnapshot()) {
    return false;
  }
  useUiStore.getState().syncThemeState();
  return true;
}

async function bootstrap() {
  try {
    const settings = await window.electronAPI?.settings.get() as BootstrapSettings | undefined;
    await applyDisplayLocale(settings);
    applySettingsTheme(settings);
  } catch {
    if (!syncFromCurrentThemeState()) {
      applyFallbackTheme();
    }
    await applyDisplayLocale(undefined);
  } finally {
    if (!IS_MARKDOWN_PDF_EXPORT_VIEW) {
      installWindowZoomShortcuts();
    }
  }
}

// Synchronous: restore last-known resolved snapshot before async bootstrap.
if (restoreThemeSnapshot()) {
  useUiStore.getState().syncThemeState();
}

const handleVisibilityChange = async () => {
  if (document.visibilityState !== 'visible') return;
  try {
    const settings = await window.electronAPI?.settings.get() as BootstrapSettings | undefined;
    applySettingsTheme(settings);
  } catch {
    if (!syncFromCurrentThemeState()) {
      applyFallbackTheme();
    }
  }
};
document.addEventListener('visibilitychange', handleVisibilityChange);

const disposeSettingsChanged = window.electronAPI?.settings?.onChanged?.((settings) => {
  const nextSettings = settings as BootstrapSettings;
  const nextThemeSignature = getThemeSignature(nextSettings);
  if (nextThemeSignature !== lastThemeSignature) {
    applySettingsTheme(nextSettings);
    return;
  }
  applySettingsUiOnly(nextSettings);
});

let cleanedUpThemeListeners = false;
const disposePrepareClose = window.electronAPI?.window?.onPrepareClose?.(() => {
  void useAppStore.getState().flushDirtyTabs()
    .catch((error) => {
      console.error('Failed to flush dirty tabs before window close:', error);
    })
    .finally(() => {
      void window.electronAPI?.window?.readyToClose?.().catch((error) => {
        console.error('Failed to acknowledge window close:', error);
      });
    });
});

const cleanupThemeListeners = () => {
  if (cleanedUpThemeListeners) {
    return;
  }
  cleanedUpThemeListeners = true;
  document.removeEventListener('visibilitychange', handleVisibilityChange);
  disposeSettingsChanged?.();
  disposePrepareClose?.();
  disposeWindowZoomShortcuts();
};

window.addEventListener('beforeunload', cleanupThemeListeners, { once: true });
const hotModule = (import.meta as ImportMeta & { hot?: HotModuleApi }).hot;
if (hotModule) {
  hotModule.dispose(cleanupThemeListeners);
}

const rootElement = document.getElementById('root')!;

void (async () => {
  await initRendererI18n(undefined, navigator.language);
  ReactDOM.createRoot(rootElement).render(
    <React.StrictMode>
      <I18nextProvider i18n={rendererI18n}>
        {IS_MARKDOWN_PDF_EXPORT_VIEW ? <MarkdownPdfExportRoot /> : <App />}
      </I18nextProvider>
    </React.StrictMode>,
  );
  void bootstrap();
})();
