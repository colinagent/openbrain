import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const rowSource = readFileSync(path.join(__dirname, 'FileTreeRow.tsx'), 'utf8');
const stylesSource = readFileSync(path.join(__dirname, '../../styles/index.css'), 'utf8');

test('file tree row renders multi-selection as its own visual state', () => {
  assert.match(rowSource, /multiSelected\?: boolean/);
  assert.match(rowSource, /multiSelected = false/);
  assert.match(rowSource, /multiSelected \? 'multi-selected' : ''/);
  assert.doesNotMatch(
    stylesSource,
    /\.file-tree-item:not\(\.selected\):hover::before\s*\{[^}]*background-color:\s*var\(--color-hover-bg\);/m,
  );
  assert.match(stylesSource, /\.file-tree-item\.multi-selected::before/);
  assert.match(
    stylesSource,
    /\.file-tree-item\.multi-selected::before\s*\{[^}]*background-color:\s*var\(--color-hover-bg\);/m,
  );
  assert.doesNotMatch(stylesSource, /\.sidebar-hover-area:hover \.file-tree-item\.multi-selected/);
  assert.doesNotMatch(stylesSource, /\.file-tree-item\.multi-selected\s*\{[^}]*color:/m);
  assert.doesNotMatch(stylesSource, /\.file-tree-item\.selected::after/);
  assert.match(stylesSource, /\.file-tree-item\.context-menu-target::before/);
  assert.match(stylesSource, /\.file-tree-item\.external-drop-target::before/);
});
