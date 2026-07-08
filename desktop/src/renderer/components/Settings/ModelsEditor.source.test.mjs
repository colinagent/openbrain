import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const source = readFileSync(path.join(import.meta.dirname, './ModelsEditor.tsx'), 'utf8');

test('ModelsEditor surfaces device-code sign-in failures after browser authorization', () => {
  assert.match(source, /const deviceCodeError = useAuthStore\(\(state\) => state\.deviceCodeError\)/);
  assert.match(source, /if \(deviceCodeError\) \{[\s\S]*setNotice\(\{ tone: 'error', text: deviceCodeError \}\)/);
});
