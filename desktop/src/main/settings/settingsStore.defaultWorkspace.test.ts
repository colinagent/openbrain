import assert from 'node:assert/strict';
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';

import {
  getSystemSettingsPath,
  getSettingsRoot,
  getUserSettingsPath,
  ensureSettingsInitialized,
  loadSystemSettings,
  loadUserSettings,
} from './settingsStore';

test('loadUserSettings ignores the retired Desktop defaultWorkspace', async (t) => {
  const settingsRoot = await mkdtemp(path.join(os.tmpdir(), 'openbrain-settings-'));
  t.after(async () => {
    await rm(settingsRoot, { recursive: true, force: true });
  });
  await writeFile(
    getUserSettingsPath(settingsRoot),
    JSON.stringify({
      version: 1,
      defaultWorkspace: '/desktop/override',
      recentWorkspaces: { local: [], remote: {} },
    }),
    'utf8',
  );

  const settings = await loadUserSettings(settingsRoot);
  assert.equal(Object.hasOwn(settings, 'defaultWorkspace'), false);
});

test('loadSystemSettings ignores the retired Desktop defaultDirectory', async (t) => {
  const settingsRoot = await mkdtemp(path.join(os.tmpdir(), 'openbrain-settings-'));
  t.after(async () => {
    await rm(settingsRoot, { recursive: true, force: true });
  });
  await writeFile(
    getSystemSettingsPath(settingsRoot),
    JSON.stringify({
      version: 1,
      defaultDirectory: '/desktop/override',
    }),
    'utf8',
  );

  const settings = await loadSystemSettings(settingsRoot);
  assert.equal(Object.hasOwn(settings, 'defaultDirectory'), false);
});

test('ensureSettingsInitialized removes retired workspace settings from disk', async (t) => {
  const homeDir = await mkdtemp(path.join(os.tmpdir(), 'openbrain-settings-home-'));
  t.after(async () => {
    await rm(homeDir, { recursive: true, force: true });
  });
  const settingsRoot = getSettingsRoot(homeDir);
  await mkdir(settingsRoot, { recursive: true });
  await writeFile(
    getUserSettingsPath(settingsRoot),
    '{\n  "version": 1,\n  // `defaultWorkspace` is retired.\n  "defaultWorkspace": "/desktop/override",\n  "openBrain": { "provider": "local" }\n}\n',
    'utf8',
  );
  await writeFile(
    getSystemSettingsPath(settingsRoot),
    '{\n  "version": 1,\n  "defaultDirectory": "/desktop/override",\n  "logging": { "enabled": true }\n}\n',
    'utf8',
  );

  await ensureSettingsInitialized(homeDir);

  const userOnDisk = await readFile(getUserSettingsPath(settingsRoot), 'utf8');
  const systemOnDisk = await readFile(getSystemSettingsPath(settingsRoot), 'utf8');
  assert.doesNotMatch(userOnDisk, /defaultWorkspace/);
  assert.match(userOnDisk, /"provider": "local"/);
  assert.doesNotMatch(systemOnDisk, /defaultDirectory/);
  assert.match(systemOnDisk, /"enabled": true/);
});

test('ensureSettingsInitialized preserves malformed settings for manual recovery', async (t) => {
  const homeDir = await mkdtemp(path.join(os.tmpdir(), 'openbrain-settings-home-'));
  t.after(async () => {
    await rm(homeDir, { recursive: true, force: true });
  });
  const settingsRoot = getSettingsRoot(homeDir);
  await mkdir(settingsRoot, { recursive: true });
  const malformed = '{ "version": 1, "defaultWorkspace": ';
  await writeFile(getUserSettingsPath(settingsRoot), malformed, 'utf8');

  await ensureSettingsInitialized(homeDir);

  assert.equal(await readFile(getUserSettingsPath(settingsRoot), 'utf8'), malformed);
});
