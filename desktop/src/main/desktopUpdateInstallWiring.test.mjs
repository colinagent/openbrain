import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const mainSource = readFileSync(
  path.join(__dirname, 'main.ts'),
  'utf8',
);

test('desktop update install IPC routes through the main-process install request', () => {
  assert.match(mainSource, /ipcMain\.handle\('desktopUpdate:getState'/);
  assert.match(mainSource, /ipcMain\.handle\('desktopUpdate:install'/);
  assert.match(mainSource, /return requestDesktopUpdateInstall\(\);/);
});

test('desktop update install requests window prepare-close instead of calling quitAndInstall directly', () => {
  assert.match(mainSource, /win\.webContents\.send\('window:prepareClose'\)/);
  assert.match(mainSource, /requestWindowPrepareClose\(record\.win\)/);
});

test('desktop update install only finalizes after awaited windows close', () => {
  assert.match(mainSource, /desktopUpdateInstallCoordinator\.markWindowClosed\(win\.id\)/);
  assert.match(mainSource, /maybeFinalizeDesktopUpdateInstall\(\)/);
});
