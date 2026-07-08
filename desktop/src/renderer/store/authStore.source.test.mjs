import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const source = readFileSync(
  path.resolve(import.meta.dirname, './authStore.ts'),
  'utf8',
);

test('auth store exposes a revision that changes when auth state is refreshed', () => {
  assert.match(source, /authRevision:\s*number/);
  assert.match(source, /authRevision:\s*0/);
  assert.match(source, /authRevision:\s*state\.authRevision \+ 1/);
});

test('device-code completion re-reads auth from main so stale signed-in UI can refresh', () => {
  assert.match(
    source,
    /onDeviceCodeComplete\?\.\(\(payload\) => \{[\s\S]*if \(payload\.success\) \{[\s\S]*authApi\.get\(\)[\s\S]*loggedIn:\s*true[\s\S]*authRevision:\s*state\.authRevision \+ 1/,
  );
});
