import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const sourcePath = path.join(import.meta.dirname, 'EditorContextMenu.tsx');
const source = readFileSync(sourcePath, 'utf8');

test('editor context menu exposes the format submenu and highlight command', () => {
  assert.match(source, /<span className="flex-1">Format<\/span>/);
  assert.match(source, /label: 'Highlight'/);
  assert.match(source, /toggleInlineFormat\(item\.format\)/);
});

test('editor context menu exposes clear formatting in the format submenu', () => {
  assert.match(source, /Clear formatting/);
  assert.match(source, /clearInlineFormatting\(\)/);
});

test('editor context menu exposes random id insertion in the insert submenu', () => {
  assert.match(source, /Random ID/);
  assert.match(source, /onInsertRandomID\?\.\(\)/);
});
