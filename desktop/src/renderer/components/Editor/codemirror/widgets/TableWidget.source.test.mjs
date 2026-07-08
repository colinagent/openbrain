import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const source = readFileSync(path.resolve(import.meta.dirname, './TableWidget.ts'), 'utf8');

test('table cell edits are committed before autosave relies on editor content', () => {
  assert.match(source, /const scheduleTableCommitToSource = \(\) => \{/);
  assert.match(source, /pendingTableCommitTimer = window\.setTimeout\(\(\) => \{[\s\S]*commitTableToSource\(\);[\s\S]*\}, 300\);/);
  assert.match(source, /element\.addEventListener\('input',[\s\S]*syncCellRawTextFromDom\(target\);[\s\S]*scheduleTableCommitToSource\(\);[\s\S]*\);/);
  assert.match(source, /element\.addEventListener\('focusout',[\s\S]*exitCellEditMode\(target\);[\s\S]*commitTableToSource\(\);[\s\S]*\);/);
  assert.match(source, /e\.key\.toLowerCase\(\) === 's'[\s\S]*commitTableToSource\(\);/);
});

test('table cell autosave commit waits until IME composition completes', () => {
  assert.match(source, /let isCellComposing = false;/);
  assert.match(source, /event instanceof InputEvent && event\.isComposing/);
  assert.match(source, /element\.addEventListener\('compositionstart'/);
  assert.match(source, /element\.addEventListener\('compositionend'[\s\S]*scheduleTableCommitToSource\(\);/);
});
