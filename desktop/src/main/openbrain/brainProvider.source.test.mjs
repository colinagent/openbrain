import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const source = readFileSync(
  path.resolve(import.meta.dirname, './brainProvider.ts'),
  'utf8',
);

test('getOpenBrainProviderStatus exposes cloud readiness fields', () => {
  assert.match(source, /githubConnected\?: boolean/);
  assert.match(source, /cloudReady\?: boolean/);
  assert.match(source, /export async function getOpenBrainProviderStatus/);
  assert.match(source, /cloudReady: githubConnected/);
  assert.match(source, /githubConnected: true,\s*cloudReady: true/);
});
