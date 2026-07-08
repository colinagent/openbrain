import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const stylesPath = path.join(__dirname, '../../styles/index.css');
const stylesSource = readFileSync(stylesPath, 'utf8');

function readCssRule(selector) {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const match = stylesSource.match(new RegExp(`${escapedSelector}\\s*\\{([^}]*)\\}`, 'm'));
  assert.ok(match, `expected ${selector} CSS rule`);
  return match[1];
}

test('markdown PDF export uses a white paper background instead of the editor theme background', () => {
  const bodyRule = readCssRule('.op-pdf-export-body');
  const rootRule = readCssRule('.op-pdf-export-root');

  assert.match(bodyRule, /--op-pdf-paper-bg:\s*#ffffff;/);
  assert.match(bodyRule, /--color-editor-bg:\s*var\(--op-pdf-paper-bg\);/);
  assert.match(bodyRule, /background:\s*var\(--op-pdf-paper-bg\);/);
  assert.doesNotMatch(bodyRule, /background:\s*var\(--color-editor-bg\);/);
  assert.match(rootRule, /background:\s*var\(--op-pdf-paper-bg\);/);
  assert.doesNotMatch(rootRule, /background:\s*var\(--color-editor-bg\);/);
});

test('markdown PDF export overrides text tokens for dark themes', () => {
  const bodyRule = readCssRule('.op-pdf-export-body');
  const exportModeRule = readCssRule('.cm-editor.cm-export-mode');

  assert.match(bodyRule, /--op-pdf-paper-fg:\s*#1f2328;/);
  assert.match(bodyRule, /--color-editor-fg:\s*var\(--op-pdf-paper-fg\);/);
  assert.match(bodyRule, /--color-prime-text:\s*var\(--op-pdf-paper-fg\);/);
  assert.match(bodyRule, /--color-preview-heading1:\s*var\(--op-pdf-paper-fg\);/);
  assert.match(exportModeRule, /color:\s*var\(--op-pdf-paper-fg\);/);
});
