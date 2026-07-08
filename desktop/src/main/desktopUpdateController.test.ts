import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import test from 'node:test';

import {
  DesktopUpdateController,
  type DesktopAutoUpdater,
} from './desktopUpdateController';

class FakeUpdater extends EventEmitter implements DesktopAutoUpdater {
  autoDownload = false;
  autoInstallOnAppQuit = true;
  allowPrerelease = false;
  checkCalls = 0;
  quitCalls = 0;
  feedURL: { provider: 'generic'; url: string } | null = null;

  setFeedURL(options: { provider: 'generic'; url: string }): void {
    this.feedURL = options;
  }

  async checkForUpdates(): Promise<unknown> {
    this.checkCalls += 1;
    return {};
  }

  quitAndInstall(): void {
    this.quitCalls += 1;
  }
}

test('DesktopUpdateController starts checking and transitions to ready after download', async () => {
  const updater = new FakeUpdater();
  const controller = new DesktopUpdateController({
    appIsPackaged: true,
    currentVersion: '0.1.4',
    updater,
    feedURL: 'https://github.com/colinagent/openbrain/releases/latest/download/',
    logger: {
      log: () => {},
      warn: () => {},
    },
  });

  controller.start();

  assert.equal(updater.autoDownload, true);
  assert.equal(updater.autoInstallOnAppQuit, false);
  assert.equal(updater.allowPrerelease, false);
  assert.deepEqual(updater.feedURL, {
    provider: 'generic',
    url: 'https://github.com/colinagent/openbrain/releases/latest/download',
  });
  assert.equal(updater.checkCalls, 1);
  assert.deepEqual(controller.getSnapshot(), {
    phase: 'checking',
    currentVersion: '0.1.4',
    targetVersion: null,
    error: undefined,
  });

  updater.emit('update-available', { version: '0.1.4' });
  assert.deepEqual(controller.getSnapshot(), {
    phase: 'downloading',
    currentVersion: '0.1.4',
    targetVersion: '0.1.4',
    error: undefined,
  });

  updater.emit('update-downloaded', { version: '0.1.4' });
  assert.deepEqual(controller.getSnapshot(), {
    phase: 'ready',
    currentVersion: '0.1.4',
    targetVersion: '0.1.4',
    error: undefined,
  });
});

test('DesktopUpdateController returns to idle when no update is available', async () => {
  const updater = new FakeUpdater();
  const controller = new DesktopUpdateController({
    appIsPackaged: true,
    currentVersion: '0.1.4',
    updater,
    logger: {
      log: () => {},
      warn: () => {},
    },
  });

  controller.start();
  updater.emit('update-not-available');

  assert.deepEqual(controller.getSnapshot(), {
    phase: 'idle',
    currentVersion: '0.1.4',
    targetVersion: null,
    error: undefined,
  });
});

test('DesktopUpdateController reports errors and only installs from ready state', async () => {
  const updater = new FakeUpdater();
  const controller = new DesktopUpdateController({
    appIsPackaged: true,
    currentVersion: '0.1.4',
    updater,
    logger: {
      log: () => {},
      warn: () => {},
    },
  });

  controller.start();
  const notReady = controller.beginInstall();
  assert.equal(notReady.success, false);

  updater.emit('update-available', { version: '0.1.4' });
  updater.emit('update-downloaded', { version: '0.1.4' });

  assert.deepEqual(controller.beginInstall(), { success: true });
  assert.deepEqual(controller.getSnapshot(), {
    phase: 'installing',
    currentVersion: '0.1.4',
    targetVersion: '0.1.4',
    error: undefined,
  });

  assert.deepEqual(controller.finalizeInstall(), { success: true });
  assert.equal(updater.quitCalls, 1);

  updater.emit('error', new Error('network failed'));
  assert.deepEqual(controller.getSnapshot(), {
    phase: 'error',
    currentVersion: '0.1.4',
    targetVersion: '0.1.4',
    error: 'network failed',
  });
});

test('DesktopUpdateController stays unsupported outside packaged builds', () => {
  const updater = new FakeUpdater();
  const controller = new DesktopUpdateController({
    appIsPackaged: false,
    currentVersion: '0.1.4',
    updater,
    logger: {
      log: () => {},
      warn: () => {},
    },
  });

  controller.start();

  assert.deepEqual(controller.getSnapshot(), {
    phase: 'unsupported',
    currentVersion: '0.1.4',
    targetVersion: null,
    error: undefined,
  });
  assert.equal(updater.checkCalls, 0);
});

test('DesktopUpdateController waitForStartupDecision resolves when update becomes ready', async () => {
  const updater = new FakeUpdater();
  const controller = new DesktopUpdateController({
    appIsPackaged: true,
    currentVersion: '0.1.4',
    updater,
    logger: {
      log: () => {},
      warn: () => {},
    },
  });

  controller.start();
  const decisionPromise = controller.waitForStartupDecision(1_000);

  updater.emit('update-available', { version: '0.1.4' });
  updater.emit('update-downloaded', { version: '0.1.4' });

  assert.deepEqual(await decisionPromise, {
    phase: 'ready',
    currentVersion: '0.1.4',
    targetVersion: '0.1.4',
    error: undefined,
  });
});

test('DesktopUpdateController polls for updates roughly every configured interval', async () => {
  const updater = new FakeUpdater();
  const controller = new DesktopUpdateController({
    appIsPackaged: true,
    currentVersion: '0.1.4',
    updater,
    pollIntervalMs: 20,
    logger: {
      log: () => {},
      warn: () => {},
    },
  });

  controller.start();
  assert.equal(updater.checkCalls, 1);
  updater.emit('update-not-available');

  await new Promise((resolve) => setTimeout(resolve, 35));
  assert.ok(updater.checkCalls >= 2);
});

test('DesktopUpdateController does not poll again while update is already ready', async () => {
  const updater = new FakeUpdater();
  const controller = new DesktopUpdateController({
    appIsPackaged: true,
    currentVersion: '0.1.4',
    updater,
    pollIntervalMs: 20,
    logger: {
      log: () => {},
      warn: () => {},
    },
  });

  controller.start();
  updater.emit('update-available', { version: '0.1.4' });
  updater.emit('update-downloaded', { version: '0.1.4' });
  const callsAfterReady = updater.checkCalls;

  await new Promise((resolve) => setTimeout(resolve, 35));
  assert.equal(updater.checkCalls, callsAfterReady);
});

test('DesktopUpdateController waitForStartupDecision returns current snapshot on timeout', async () => {
  const updater = new FakeUpdater();
  const controller = new DesktopUpdateController({
    appIsPackaged: true,
    currentVersion: '0.1.4',
    updater,
    logger: {
      log: () => {},
      warn: () => {},
    },
  });

  controller.start();
  updater.emit('update-available', { version: '0.1.4' });

  assert.deepEqual(await controller.waitForStartupDecision(5), {
    phase: 'downloading',
    currentVersion: '0.1.4',
    targetVersion: '0.1.4',
    error: undefined,
  });
});
