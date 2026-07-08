import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const styles = readFileSync(path.join(__dirname, '../../styles/index.css'), 'utf8');
const documentOutlineSource = readFileSync(path.join(__dirname, 'DocumentOutline.tsx'), 'utf8');
const markdownEditorSource = readFileSync(path.join(__dirname, 'MarkdownEditor.tsx'), 'utf8');

test('markdown outline toggle wrap stacks icons vertically', () => {
  assert.match(
    styles,
    /\.op-md-outline-toggle-wrap\s*\{[^}]*flex-direction:\s*column;/m,
  );
  assert.match(
    styles,
    /\.op-md-outline-toggle-wrap\s*\{[^}]*align-items:\s*center;/m,
  );
  assert.match(
    styles,
    /\.op-md-outline-toggle-wrap\s*\{[^}]*justify-content:\s*flex-end;/m,
  );
});

test('DocumentOutline shows pin only when collapsed', () => {
  assert.match(documentOutlineSource, /pinEnabled\?: boolean;/);
  assert.match(documentOutlineSource, /\{!expanded && pinEnabled && \(/);
  assert.match(documentOutlineSource, /op-md-outline-pin icon-gutter-btn-sm icon-button-inline/);
});

test('DocumentOutline keeps the pin control when there are no heading entries', () => {
  assert.match(documentOutlineSource, /const hasEntries = treeEntries\.length > 0;/);
  assert.match(documentOutlineSource, /if \(!hasEntries && !pinEnabled\) \{\s*return null;\s*\}/s);
  assert.match(documentOutlineSource, /\{hasEntries && outlineToggleEnabled && \(/);
  assert.match(documentOutlineSource, /\{expanded && hasEntries && \(/);
});

test('DocumentOutline can hide the outline toggle while keeping pin controls', () => {
  assert.match(documentOutlineSource, /outlineToggleEnabled\?: boolean;/);
  assert.match(documentOutlineSource, /outlineToggleEnabled = true/);
  assert.match(documentOutlineSource, /\{hasEntries && outlineToggleEnabled && \(/);
});

test('MarkdownEditor does not render a standalone pin button', () => {
  assert.doesNotMatch(markdownEditorSource, /op-md-editor-pin-btn/);
  assert.doesNotMatch(markdownEditorSource, /pinButtonRightInset/);
});

test('compact markdown editor removes left scroller padding', () => {
  assert.match(markdownEditorSource, /compact\?: boolean;/);
  assert.match(markdownEditorSource, /compact = false/);
  assert.match(markdownEditorSource, /is-compact/);
  assert.match(styles, /\.op-markdown-editor\.is-compact \.cm-scroller\s*\{[\s\S]*padding-left:\s*0;/m);
});
