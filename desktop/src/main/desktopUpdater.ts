import { app } from 'electron';
import { autoUpdater } from 'electron-updater';

import {
  DesktopUpdateController,
  type DesktopUpdateState,
} from './desktopUpdateController';

let controller: DesktopUpdateController | null = null;
const DESKTOP_UPDATE_POLL_INTERVAL_MS = 10 * 60 * 1000;
export const DEFAULT_DESKTOP_UPDATE_URL = 'https://download.op-agent.com/desktop/latest';

export type { DesktopUpdateState } from './desktopUpdateController';

export function getDesktopUpdateController(): DesktopUpdateController {
  if (!controller) {
    controller = new DesktopUpdateController({
      appIsPackaged: app.isPackaged,
      currentVersion: app.getVersion(),
      updater: autoUpdater,
      pollIntervalMs: DESKTOP_UPDATE_POLL_INTERVAL_MS,
      feedURL: process.env.OPENBRAIN_DESKTOP_UPDATE_URL || DEFAULT_DESKTOP_UPDATE_URL,
    });
  }
  return controller;
}

export function startDesktopAutoUpdate() {
  getDesktopUpdateController().start();
}

export function getDesktopUpdateSnapshot(): DesktopUpdateState {
  return getDesktopUpdateController().getSnapshot();
}
