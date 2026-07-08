import React, { useEffect, useState } from 'react';

import {
  formatWindowZoomPercent,
  MAX_WINDOW_ZOOM_LEVEL,
  MIN_WINDOW_ZOOM_LEVEL,
} from '../../main/shared/windowZoom';
import {
  applyWindowZoomCommand,
  getCurrentWindowZoomLevel,
  subscribeWindowZoomLevel,
} from '../services/windowZoomShortcuts';
import { ZoomStatusControl } from './ZoomStatusControl';

function isMacPlatform(): boolean {
  const platform = typeof window !== 'undefined' ? window.electronAPI?.platform : undefined;
  if (platform) {
    return platform === 'darwin';
  }
  const navigatorPlatform = typeof navigator !== 'undefined' ? navigator.platform.toLowerCase() : '';
  const userAgent = typeof navigator !== 'undefined' ? navigator.userAgent.toLowerCase() : '';
  return navigatorPlatform.includes('mac') || userAgent.includes('mac os');
}

function getShortcutLabels() {
  if (isMacPlatform()) {
    return {
      zoomIn: '⌘=',
      zoomOut: '⌘-',
      zoomReset: '⌘0',
    };
  }
  return {
    zoomIn: 'Ctrl+=',
    zoomOut: 'Ctrl+-',
    zoomReset: 'Ctrl+0',
  };
}

export function WindowZoomStatusControl() {
  const [zoomLevel, setZoomLevel] = useState(() => getCurrentWindowZoomLevel());

  useEffect(() => subscribeWindowZoomLevel(setZoomLevel), []);

  const percent = formatWindowZoomPercent(zoomLevel);
  const shortcuts = getShortcutLabels();
  const canZoomOut = zoomLevel > MIN_WINDOW_ZOOM_LEVEL;
  const canZoomIn = zoomLevel < MAX_WINDOW_ZOOM_LEVEL;
  const title = `Window Zoom: ${percent}. Reset with ${shortcuts.zoomReset}.`;

  return (
    <ZoomStatusControl
      label="Window"
      percent={percent}
      title={title}
      canZoomOut={canZoomOut}
      canZoomIn={canZoomIn}
      onZoomOut={() => applyWindowZoomCommand('zoomOut')}
      onZoomIn={() => applyWindowZoomCommand('zoomIn')}
      onReset={() => applyWindowZoomCommand('zoomReset')}
    />
  );
}
