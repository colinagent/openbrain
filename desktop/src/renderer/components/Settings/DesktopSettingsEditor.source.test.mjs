import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const rendererRoot = path.resolve(import.meta.dirname, '../..');
const editorSource = readFileSync(path.join(import.meta.dirname, './DesktopSettingsEditor.tsx'), 'utf8');
const appSource = readFileSync(path.join(rendererRoot, 'App.tsx'), 'utf8');
const appStoreSource = readFileSync(path.join(rendererRoot, 'store/appStore.ts'), 'utf8');
const hookSource = readFileSync(path.join(rendererRoot, 'hooks/usePreventSleepWhileAgentRunning.ts'), 'utf8');

test('DesktopSettingsEditor binds idleSleepPolicy to system settings', () => {
  assert.match(editorSource, /idleSleepPolicy/);
  assert.match(editorSource, /window\.electronAPI\?\.settings\?\.set/);
  assert.match(editorSource, /system:\s*\{\s*power:\s*\{\s*idleSleepPolicy:/);
  assert.match(editorSource, /type="radio"/);
});

test('Desktop settings are registered as a singleton editor with policy hook', () => {
  assert.match(appSource, /DesktopSettingsEditor/);
  assert.match(appSource, /id: 'desktop-settings'/);
  assert.match(appSource, /usePreventSleepWhileAgentRunning/);
  assert.match(appSource, /openDesktopSettingsTab/);
  assert.match(appStoreSource, /openDesktopSettingsTab: \(\) => \{/);
  assert.match(hookSource, /whileAgentRunning/);
  assert.match(hookSource, /idleSleepPolicy !== 'whileAgentRunning'/);
});
