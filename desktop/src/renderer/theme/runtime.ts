import { ThemeDefinition, defaultThemeId as fallbackDefaultThemeId, themes as fallbackThemes } from './presets';
import { tokensToCssVariables } from './tokens';

const themeStyleId = 'openbrain-theme-tokens';
const THEME_SNAPSHOT_KEY = 'openbrain-theme-snapshot';
const THEME_SNAPSHOT_VERSION = 3;

export type ThemeSnapshot = {
  version: number;
  themeId: string;
  label: string;
  scheme: 'dark' | 'light';
  css: string;
};

let runtimeThemes: ThemeDefinition[] = fallbackThemes;
let runtimeDefaultThemeId: string = fallbackDefaultThemeId;
let runtimeCurrentThemeId: string | null = null;
let runtimeCurrentSnapshot: ThemeSnapshot | null = null;

function buildThemeCss(theme: ThemeDefinition): string {
  return `:root {\n${tokensToCssVariables(theme.tokens)}\n}`;
}

function ensureStyleElement(): HTMLStyleElement | null {
  if (typeof document === 'undefined') {
    return null;
  }
  let style = document.getElementById(themeStyleId) as HTMLStyleElement | null;
  if (!style) {
    style = document.createElement('style');
    style.id = themeStyleId;
    document.head.appendChild(style);
  }
  return style;
}

function applyThemeCss(css: string, scheme: 'dark' | 'light') {
  const style = ensureStyleElement();
  if (style) {
    style.textContent = css;
  }
  if (typeof document !== 'undefined') {
    document.documentElement.style.colorScheme = scheme;
    document.documentElement.dataset.colorScheme = scheme;
  }
}

function cacheThemeSnapshot(snapshot: ThemeSnapshot) {
  try {
    localStorage.setItem(THEME_SNAPSHOT_KEY, JSON.stringify(snapshot));
  } catch {
    // localStorage may be unavailable
  }
}

function isValidSnapshot(input: unknown): input is ThemeSnapshot {
  if (!input || typeof input !== 'object') {
    return false;
  }
  const snapshot = input as Record<string, unknown>;
  if (snapshot.version !== THEME_SNAPSHOT_VERSION) {
    return false;
  }
  if (typeof snapshot.themeId !== 'string' || !snapshot.themeId.trim()) {
    return false;
  }
  if (typeof snapshot.label !== 'string' || !snapshot.label.trim()) {
    return false;
  }
  if (snapshot.scheme !== 'dark' && snapshot.scheme !== 'light') {
    return false;
  }
  if (typeof snapshot.css !== 'string' || !snapshot.css.trim()) {
    return false;
  }
  return true;
}

function createThemeSnapshot(theme: ThemeDefinition): ThemeSnapshot {
  return {
    version: THEME_SNAPSHOT_VERSION,
    themeId: theme.id,
    label: theme.label,
    scheme: theme.scheme,
    css: buildThemeCss(theme),
  };
}

function resolveTheme(themeId?: string): ThemeDefinition {
  return getThemeById(themeId || '')
    || (runtimeCurrentThemeId ? getThemeById(runtimeCurrentThemeId) : undefined)
    || getThemeById(runtimeDefaultThemeId)
    || runtimeThemes[0]
    || fallbackThemes[0];
}

export function applyThemeSnapshot(snapshot: ThemeSnapshot, persist = true): ThemeSnapshot {
  applyThemeCss(snapshot.css, snapshot.scheme);
  runtimeCurrentThemeId = snapshot.themeId;
  runtimeCurrentSnapshot = snapshot;
  if (persist) {
    cacheThemeSnapshot(snapshot);
  }
  return snapshot;
}

export function restoreThemeSnapshot(): boolean {
  try {
    const raw = localStorage.getItem(THEME_SNAPSHOT_KEY);
    if (!raw) {
      return false;
    }
    const parsed = JSON.parse(raw) as unknown;
    if (!isValidSnapshot(parsed)) {
      return false;
    }
    applyThemeSnapshot(parsed, false);
    return true;
  } catch {
    return false;
  }
}

export function setThemeRuntimeDefinitions(nextThemes: ThemeDefinition[] | null, nextDefaultThemeId?: string) {
  runtimeThemes = nextThemes && nextThemes.length ? nextThemes : fallbackThemes;
  if (nextDefaultThemeId && runtimeThemes.some((theme) => theme.id === nextDefaultThemeId)) {
    runtimeDefaultThemeId = nextDefaultThemeId;
  } else if (!runtimeThemes.some((theme) => theme.id === runtimeDefaultThemeId)) {
    runtimeDefaultThemeId = runtimeThemes[0]?.id || fallbackDefaultThemeId;
  }
}

export function activateTheme(themeId?: string): ThemeDefinition {
  const theme = resolveTheme(themeId);
  runtimeCurrentThemeId = theme.id;
  const snapshot = createThemeSnapshot(theme);
  applyThemeSnapshot(snapshot, true);
  return theme;
}

export function reapplyCurrentThemeSnapshot(): boolean {
  if (runtimeCurrentSnapshot) {
    applyThemeSnapshot(runtimeCurrentSnapshot, false);
    return true;
  }
  if (!runtimeCurrentThemeId) {
    return false;
  }
  activateTheme(runtimeCurrentThemeId);
  return true;
}

export function getThemeById(themeId: string): ThemeDefinition | undefined {
  return runtimeThemes.find((theme) => theme.id === themeId);
}

export function getAvailableThemes(): ThemeDefinition[] {
  return runtimeThemes;
}

export function getCurrentThemeId(): string | null {
  return runtimeCurrentThemeId;
}

export function getCurrentThemeSnapshot(): ThemeSnapshot | null {
  return runtimeCurrentSnapshot;
}

export function getDefaultThemeId(): string {
  return runtimeDefaultThemeId;
}
