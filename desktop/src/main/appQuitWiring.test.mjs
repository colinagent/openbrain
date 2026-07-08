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

test('app quit intent is remembered so macOS quit does not degrade into close-window only', () => {
  assert.match(mainSource, /let appQuitRequested = false;/);
  assert.match(mainSource, /app\.on\('before-quit', \(\) => \{\s*appQuitRequested = true;\s*\}\);/s);
  assert.match(mainSource, /app\.on\('will-quit', \(\) => \{\s*appQuitRequested = true;/s);
  assert.match(mainSource, /app\.on\('window-all-closed', \(\) => \{[\s\S]*if \(appQuitRequested\) \{\s*app\.quit\(\);\s*return;\s*\}/);
});
