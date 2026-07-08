import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const sidebarSource = readFileSync(
  path.resolve(import.meta.dirname, './Sidebar.tsx'),
  'utf8',
);

const selectionStoreSource = readFileSync(
  path.resolve(import.meta.dirname, '../FileExplorer/fileTreeSelectionStore.ts'),
  'utf8',
);

test('Sidebar clears transient file tree selection when the cursor leaves the sidebar', () => {
  assert.match(sidebarSource, /useFileTreeSelectionStore/);
  assert.match(sidebarSource, /onMouseLeave=\{\(\) => \{\s*useFileTreeSelectionStore\.getState\(\)\.clearAllSelections\(\);\s*\}\}/m);
  assert.match(selectionStoreSource, /clearAllSelections: \(\) => void/);
  assert.match(selectionStoreSource, /selection: new Set\(\),\s*anchor: null,\s*dropTargetPath: null/s);
});
