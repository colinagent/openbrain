import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const repoRoot = process.cwd().endsWith('desktop')
  ? process.cwd()
  : path.join(process.cwd(), 'desktop');
const configSyncSource = readFileSync(
  path.join(repoRoot, 'src/main/configSync.ts'),
  'utf8',
);

test('OpenBrain settings are machine-local and not part of runtime config sync', () => {
  assert.match(configSyncSource, /new Set\(\['auth\.json', 'models\.json', 'nodes\.json', 'profile\.json'\]\)/);
  assert.doesNotMatch(configSyncSource, /user\.jsonc/);
  assert.doesNotMatch(configSyncSource, /settings\.json/);
});
