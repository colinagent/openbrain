import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const source = readFileSync(path.join(__dirname, 'editorCompletion.ts'), 'utf8');

test('editor completion settings do not emit Auto mode', () => {
  assert.match(source, /export type EditorCompletionMode = 'default' \| 'custom' \| 'off';/);
  assert.match(source, /mode: 'default'/);
  assert.match(source, /case 'auto':\s+case 'chat':\s+return 'default';/m);
  assert.doesNotMatch(source, /mode: 'auto'/);
});
