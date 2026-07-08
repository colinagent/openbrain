export const MIN_WINDOW_ZOOM_LEVEL = -8;
export const MAX_WINDOW_ZOOM_LEVEL = 8;
export const DEFAULT_WINDOW_ZOOM_LEVEL = 0;
export const WINDOW_ZOOM_PERCENT_STEP = 10;

export function clampWindowZoomLevel(level: number): number {
  return Math.min(MAX_WINDOW_ZOOM_LEVEL, Math.max(MIN_WINDOW_ZOOM_LEVEL, level));
}

export function normalizeWindowZoomLevel(value: unknown, fallback = DEFAULT_WINDOW_ZOOM_LEVEL): number {
  const level = typeof value === 'number' && Number.isFinite(value)
    ? value
    : typeof fallback === 'number' && Number.isFinite(fallback)
      ? fallback
      : DEFAULT_WINDOW_ZOOM_LEVEL;
  return clampWindowZoomLevel(level);
}

/** Electron follows Chromium's convention: zoomFactor = 1.2 ^ zoomLevel. */
export function zoomLevelToZoomFactor(zoomLevel = DEFAULT_WINDOW_ZOOM_LEVEL): number {
  return 1.2 ** normalizeWindowZoomLevel(zoomLevel);
}

export function windowZoomLevelToPercent(zoomLevel = DEFAULT_WINDOW_ZOOM_LEVEL): number {
  return Math.round(zoomLevelToZoomFactor(zoomLevel) * 100);
}

export const MIN_WINDOW_ZOOM_PERCENT = Math.ceil(
  windowZoomLevelToPercent(MIN_WINDOW_ZOOM_LEVEL) / WINDOW_ZOOM_PERCENT_STEP
) * WINDOW_ZOOM_PERCENT_STEP;
export const MAX_WINDOW_ZOOM_PERCENT = Math.round(
  windowZoomLevelToPercent(MAX_WINDOW_ZOOM_LEVEL) / WINDOW_ZOOM_PERCENT_STEP
) * WINDOW_ZOOM_PERCENT_STEP;

export function clampWindowZoomPercent(percent: number): number {
  if (!Number.isFinite(percent)) {
    return 100;
  }
  return Math.min(MAX_WINDOW_ZOOM_PERCENT, Math.max(MIN_WINDOW_ZOOM_PERCENT, Math.round(percent)));
}

export function windowZoomPercentToLevel(percent: number): number {
  const clampedPercent = clampWindowZoomPercent(percent);
  return clampWindowZoomLevel(Math.log(clampedPercent / 100) / Math.log(1.2));
}

export function normalizeSteppedWindowZoomLevel(
  value: unknown,
  fallback = DEFAULT_WINDOW_ZOOM_LEVEL
): number {
  const level = normalizeWindowZoomLevel(value, fallback);
  const snappedPercent = Math.round(windowZoomLevelToPercent(level) / WINDOW_ZOOM_PERCENT_STEP)
    * WINDOW_ZOOM_PERCENT_STEP;
  return windowZoomPercentToLevel(snappedPercent);
}

export function stepWindowZoomLevel(value: unknown, delta: 1 | -1): number {
  const currentPercent = windowZoomLevelToPercent(normalizeWindowZoomLevel(value));
  const nextPercent = delta > 0
    ? Math.floor(currentPercent / WINDOW_ZOOM_PERCENT_STEP) * WINDOW_ZOOM_PERCENT_STEP + WINDOW_ZOOM_PERCENT_STEP
    : Math.ceil(currentPercent / WINDOW_ZOOM_PERCENT_STEP) * WINDOW_ZOOM_PERCENT_STEP - WINDOW_ZOOM_PERCENT_STEP;
  return windowZoomPercentToLevel(clampWindowZoomPercent(nextPercent));
}

export function formatWindowZoomPercent(zoomLevel = DEFAULT_WINDOW_ZOOM_LEVEL): string {
  return `${windowZoomLevelToPercent(zoomLevel)}%`;
}
