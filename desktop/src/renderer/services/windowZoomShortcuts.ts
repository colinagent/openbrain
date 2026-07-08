import {
  DEFAULT_WINDOW_ZOOM_LEVEL,
  normalizeSteppedWindowZoomLevel,
  stepWindowZoomLevel,
} from '../../main/shared/windowZoom';

export type WindowZoomShortcutCommand = 'zoomIn' | 'zoomOut' | 'zoomReset';

type KeyboardShortcutEvent = Pick<KeyboardEvent,
  'altKey' | 'code' | 'ctrlKey' | 'key' | 'metaKey' | 'shiftKey'
>;

export type WindowZoomLevelListener = (level: number) => void;

let currentWindowZoomLevel = DEFAULT_WINDOW_ZOOM_LEVEL;
let installedKeydownHandler: ((event: KeyboardEvent) => void) | null = null;
const windowZoomListeners = new Set<WindowZoomLevelListener>();

function notifyWindowZoomListeners() {
  for (const listener of windowZoomListeners) {
    listener(currentWindowZoomLevel);
  }
}

function isMacPlatform(platform: NodeJS.Platform | string | undefined): boolean {
  if (platform) {
    return platform === 'darwin';
  }
  const navigatorPlatform = typeof navigator !== 'undefined' ? navigator.platform.toLowerCase() : '';
  const userAgent = typeof navigator !== 'undefined' ? navigator.userAgent.toLowerCase() : '';
  return navigatorPlatform.includes('mac') || userAgent.includes('mac os');
}

function hasPrimaryModifier(event: KeyboardShortcutEvent, platform?: NodeJS.Platform | string): boolean {
  return isMacPlatform(platform) ? event.metaKey : event.ctrlKey;
}

export function resolveWindowZoomShortcutCommand(
  event: KeyboardShortcutEvent,
  platform?: NodeJS.Platform | string,
): WindowZoomShortcutCommand | null {
  if (event.altKey || !hasPrimaryModifier(event, platform)) {
    return null;
  }

  const key = event.key.toLowerCase();
  const code = event.code;

  if (code === 'Equal' || code === 'NumpadAdd' || key === '=' || key === '+') {
    return 'zoomIn';
  }
  if (code === 'Minus' || code === 'NumpadSubtract' || key === '-' || key === '_') {
    return 'zoomOut';
  }
  if (!event.shiftKey && (code === 'Digit0' || code === 'Numpad0' || key === '0')) {
    return 'zoomReset';
  }
  return null;
}

export function setCurrentWindowZoomLevel(level: unknown): number {
  const nextLevel = normalizeSteppedWindowZoomLevel(level);
  if (nextLevel === currentWindowZoomLevel) {
    return currentWindowZoomLevel;
  }
  currentWindowZoomLevel = nextLevel;
  notifyWindowZoomListeners();
  return currentWindowZoomLevel;
}

export function getCurrentWindowZoomLevel(): number {
  return currentWindowZoomLevel;
}

export function subscribeWindowZoomLevel(listener: WindowZoomLevelListener): () => void {
  windowZoomListeners.add(listener);
  return () => {
    windowZoomListeners.delete(listener);
  };
}

export function getNextWindowZoomLevel(command: WindowZoomShortcutCommand): number {
  if (command === 'zoomReset') {
    return DEFAULT_WINDOW_ZOOM_LEVEL;
  }
  return stepWindowZoomLevel(currentWindowZoomLevel, command === 'zoomIn' ? 1 : -1);
}

export function applyWindowZoomLevel(level: number): number {
  const nextLevel = setCurrentWindowZoomLevel(level);
  const electronAPI = typeof window !== 'undefined' ? window.electronAPI : undefined;
  electronAPI?.window?.setZoomLevel?.(nextLevel);
  void electronAPI?.settings?.set?.({
    ui: {
      zoomLevel: nextLevel,
    },
  }).catch((error: unknown) => {
    console.warn('[windowZoom] failed to persist zoom level:', error);
  });
  return nextLevel;
}

export function applyWindowZoomCommand(command: WindowZoomShortcutCommand): number {
  const nextZoomLevel = getNextWindowZoomLevel(command);
  if (nextZoomLevel === currentWindowZoomLevel) {
    return currentWindowZoomLevel;
  }
  return applyWindowZoomLevel(nextZoomLevel);
}

export function installWindowZoomShortcuts() {
  if (installedKeydownHandler || typeof document === 'undefined') {
    return;
  }

  installedKeydownHandler = (event: KeyboardEvent) => {
    const command = resolveWindowZoomShortcutCommand(event, window.electronAPI?.platform);
    if (!command) {
      return;
    }

    event.preventDefault();
    event.stopPropagation();

    applyWindowZoomCommand(command);
  };

  document.addEventListener('keydown', installedKeydownHandler, true);
}

export function disposeWindowZoomShortcuts() {
  if (!installedKeydownHandler || typeof document === 'undefined') {
    installedKeydownHandler = null;
    return;
  }
  document.removeEventListener('keydown', installedKeydownHandler, true);
  installedKeydownHandler = null;
}
