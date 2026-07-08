import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const repoRoot = process.cwd().endsWith('desktop')
  ? process.cwd()
  : path.join(process.cwd(), 'desktop');

const remoteRuntimeStateSource = readFileSync(
  path.join(repoRoot, 'src/main/remote/remoteRuntimeState.ts'),
  'utf8',
);
const configSyncSource = readFileSync(
  path.join(repoRoot, 'src/main/configSync.ts'),
  'utf8',
);

test('remote runtime reads defaultWorkspace from canonical settings path with legacy fallback', () => {
  assert.match(remoteRuntimeStateSource, /\.openbrain\\\\configs\\\\settings\\\\user\.jsonc/);
  assert.match(remoteRuntimeStateSource, /\.openbrain\/configs\/settings\/user\.jsonc/);
  assert.match(remoteRuntimeStateSource, /\.openbrain\\\\settings\\\\user\.jsonc/);
  assert.match(remoteRuntimeStateSource, /\.openbrain\/settings\/user\.jsonc/);
  assert.match(remoteRuntimeStateSource, /Remote machine controls its own defaultWorkspace/);
});

test('OpenBrain settings are machine-local and not part of runtime config sync', () => {
  assert.match(configSyncSource, /new Set\(\['auth\.json', 'models\.json', 'nodes\.json', 'profile\.json'\]\)/);
  assert.doesNotMatch(configSyncSource, /user\.jsonc/);
  assert.doesNotMatch(configSyncSource, /settings\.json/);
});
