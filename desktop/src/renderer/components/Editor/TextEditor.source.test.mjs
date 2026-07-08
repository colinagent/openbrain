import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const textEditorSource = readFileSync(path.join(__dirname, 'TextEditor.tsx'), 'utf8');
const appSource = readFileSync(path.join(__dirname, '../../App.tsx'), 'utf8');
const styles = readFileSync(path.join(__dirname, '../../styles/index.css'), 'utf8');

test('TextEditor exposes the same file pin action as markdown text files', () => {
  assert.match(textEditorSource, /pinEnabled\?: boolean;/);
  assert.match(textEditorSource, /pinned\?: boolean;/);
  assert.match(textEditorSource, /onPinToggle\?: \(\) => void;/);
  assert.match(textEditorSource, /\{pinEnabled && \(/);
  assert.match(textEditorSource, /op-text-editor-pin icon-gutter-btn-sm icon-button-inline/);
  assert.match(textEditorSource, /<PinIcon className="h-3\.5 w-3\.5" \/>/);
});

test('App wires text editor tabs to pinned tab state', () => {
  const textBranchStart = appSource.indexOf("if (tab.editorId === 'text') {");
  const imageBranchStart = appSource.indexOf("if (tab.editorId === 'image') {", textBranchStart);
  assert.notEqual(textBranchStart, -1);
  assert.notEqual(imageBranchStart, -1);
  const textBranch = appSource.slice(textBranchStart, imageBranchStart);
  assert.match(textBranch, /pinEnabled=\{isPinnable\}/);
  assert.match(textBranch, /pinned=\{isPinnedTab\}/);
  assert.match(textBranch, /onPinToggle=\{\(\) => handlePinnedFileToggle\(tab\.id\)\}/);
});

test('TextEditor pin uses the compact editor icon style', () => {
  assert.match(styles, /\.op-text-editor-pin\s*\{[\s\S]*position:\s*absolute;[\s\S]*right:\s*8px;/m);
  assert.match(styles, /\.op-md-outline-pin,\s*\.op-text-editor-pin\s*\{[\s\S]*color:\s*var\(--color-secondary-text\);/m);
});

test('TextEditor keeps the CodeMirror instance stable across content edits', () => {
  const createEffectStart = textEditorSource.indexOf('useEffect(() => {\n    if (!containerRef.current) return;');
  const nextEffectStart = textEditorSource.indexOf('  useEffect(() => {\n    if (!editorFocusRequest)', createEffectStart);
  assert.notEqual(createEffectStart, -1);
  assert.notEqual(nextEffectStart, -1);

  const createEffect = textEditorSource.slice(createEffectStart, nextEffectStart);
  const deps = createEffect.match(/\}, \[([^\]]+)\]\);/s)?.[1] || '';
  assert.doesNotMatch(deps, /\bfileContent\b/);
  assert.doesNotMatch(deps, /\bboundTab\b/);
  assert.doesNotMatch(deps, /\bpendingRevealTarget\b/);
  assert.match(textEditorSource, /editorRef\.current\.setContent\(fileContent\)/);
});
