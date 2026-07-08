import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const rendererRoot = path.resolve(import.meta.dirname, '../..');
const editorSource = readFileSync(path.join(import.meta.dirname, './OpenBrainSettingsEditor.tsx'), 'utf8');
const appSource = readFileSync(path.join(rendererRoot, 'App.tsx'), 'utf8');
const appStoreSource = readFileSync(path.join(rendererRoot, 'store/appStore.ts'), 'utf8');

test('OpenBrainSettingsEditor exposes cloud default and all supported local GBrain modes', () => {
  assert.match(editorSource, /provider: raw\.provider === 'local' \? 'local' : 'cloud'/);
  assert.match(editorSource, /window\.electronAPI\?\.openBrain\?\.setProvider/);
  assert.match(editorSource, /engine: event\.target\.value === 'postgres' \? 'postgres' : 'pglite'/);
  assert.match(editorSource, /databasePath/);
  assert.match(editorSource, /databaseUrl/);
  assert.match(editorSource, /remoteMcpUrl/);
  assert.match(editorSource, /remoteMcpClientID/);
  assert.match(editorSource, /remoteMcpClientSecret/);
  assert.match(editorSource, /remoteMcpClientSecretEnvVar/);
  assert.match(editorSource, /cliPath/);
  assert.match(editorSource, /startLogin/);
  assert.match(editorSource, /deviceCodeError/);
  assert.match(editorSource, /setNotice\(\{ tone: 'error', text: deviceCodeError \}\)/);
});

test('OpenBrain settings are registered as a singleton editor outside the Home menu', () => {
  assert.match(appSource, /OpenBrainSettingsEditor/);
  assert.match(appSource, /id: 'openbrain-settings'/);
  assert.doesNotMatch(appSource, /t\('menu:openBrain'\)/);
  assert.match(appStoreSource, /openOpenBrainSettingsTab: \(\) => \{/);
  assert.match(appStoreSource, /rendererI18n\.t\('shell:tab\.openBrainSettings'\)/);
});
