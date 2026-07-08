import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const appStoreSourcePath = new URL('./appStore.ts', import.meta.url);
const appSource = await readFile(appStoreSourcePath, 'utf8');
const appComponentSourcePath = new URL('../App.tsx', import.meta.url);
const appComponentSource = await readFile(appComponentSourcePath, 'utf8');
const markdownEditorSourcePath = new URL('../components/Editor/MarkdownEditor.tsx', import.meta.url);
const markdownEditorSource = await readFile(markdownEditorSourcePath, 'utf8');
const textEditorSourcePath = new URL('../components/Editor/TextEditor.tsx', import.meta.url);
const textEditorSource = await readFile(textEditorSourcePath, 'utf8');

test('openFile focus is explicit and defaults to no editor focus', () => {
  assert.match(appSource, /focusEditor\?: boolean/);
  assert.match(appSource, /editorFocusRequest: nextEditorFocusRequest\(\s*state,\s*existing\.id,\s*options\?\.focusEditor === true\s*\)/s);
  assert.match(appSource, /editorFocusRequest: nextEditorFocusRequest\(\s*state,\s*nextTab\.id,\s*options\?\.focusEditor === true\s*\)/s);
  assert.match(appComponentSource, /autoFocus=\{options\?\.autoFocus \?\? false\}/);
});

test('editors consume one-shot focus requests instead of focusing on tab activation', () => {
  assert.match(appSource, /consumeEditorFocusRequest/);
  assert.match(markdownEditorSource, /focusEditorIfRequested/);
  assert.match(textEditorSource, /focusEditorIfRequested/);
});
