import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const source = readFileSync(path.join(__dirname, 'resolveInlineCompletion.ts'), 'utf8');

test('inline completion resolver does not use Auto model strategies', () => {
  assert.doesNotMatch(source, /resolveAutoInlineCompletionTarget/);
  assert.doesNotMatch(source, /defaultInlineCompletionModelID/);
  assert.doesNotMatch(source, /defaultInlineCompletionThinkingLevel/);
  assert.match(source, /modelsConfig\.defaultModelKey/);
});
