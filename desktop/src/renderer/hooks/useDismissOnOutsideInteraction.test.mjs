import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const source = readFileSync(path.join(__dirname, 'useDismissOnOutsideInteraction.ts'), 'utf8');

test('dismiss hook listens on window capture for pointer and mouse down', () => {
  assert.match(source, /window\.addEventListener\('pointerdown', handlePointerDown, true\)/);
  assert.match(source, /window\.addEventListener\('mousedown', handlePointerDown, true\)/);
  assert.doesNotMatch(source, /document\.addEventListener\('mousedown'/);
});

test('dismiss hook closes on window blur and Escape', () => {
  assert.match(source, /window\.addEventListener\('blur', handleBlur\)/);
  assert.match(source, /event\.key !== 'Escape'/);
  assert.match(source, /window\.addEventListener\('keydown', handleKeyDown\)/);
});

test('dismiss hook skips targets inside provided refs', () => {
  assert.match(source, /ref\.current\?\.contains\(target\)/);
  assert.match(source, /insideRefsRef\.current/);
});
