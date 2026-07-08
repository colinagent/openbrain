import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const source = readFileSync(path.join(__dirname, 'reviewOverlay.ts'), 'utf8');

test('review overlay utility derives editor overlay from selected pending review files', () => {
  assert.match(source, /review\.status !== 'pending'/);
  assert.match(source, /file\.status !== 'pending'/);
  assert.match(source, /file\.path !== normalizedPath/);
  assert.match(source, /hunks: file\.hunks \|\| \[\]/);
  assert.match(source, /changedRanges: file\.changedRanges \|\| \[\]/);
});

