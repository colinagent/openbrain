import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const sourcePath = new URL('./inlineFormat.ts', import.meta.url);
const sourceText = await readFile(sourcePath, 'utf8');
const transpiled = ts.transpileModule(sourceText, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
}).outputText;
const sourceModule = await import(
  `data:text/javascript;base64,${Buffer.from(transpiled, 'utf8').toString('base64')}`
);
const { applyInlineFormat, clearInlineFormatting } = sourceModule;

test('wraps a selected range with highlight markers', () => {
  assert.deepEqual(
    applyInlineFormat('hello', { anchor: 0, head: 5 }, 'highlight'),
    { text: '==hello==', anchor: 2, head: 7 }
  );
});

test('inserts paired markers and places the cursor between them for empty selections', () => {
  assert.deepEqual(
    applyInlineFormat('hello', { anchor: 2, head: 2 }, 'highlight'),
    { text: 'he====llo', anchor: 4, head: 4 }
  );
});

test('removes markers when the selected range includes the whole formatted source', () => {
  assert.deepEqual(
    applyInlineFormat('==hello==', { anchor: 0, head: 9 }, 'highlight'),
    { text: 'hello', anchor: 0, head: 5 }
  );
});

test('removes markers adjacent to the selected content', () => {
  assert.deepEqual(
    applyInlineFormat('==hello==', { anchor: 2, head: 7 }, 'highlight'),
    { text: 'hello', anchor: 0, head: 5 }
  );
});

test('preserves reversed selection direction when wrapping', () => {
  assert.deepEqual(
    applyInlineFormat('hello', { anchor: 5, head: 0 }, 'bold'),
    { text: '**hello**', anchor: 7, head: 2 }
  );
});

test('clears supported formatting around the selected content', () => {
  assert.deepEqual(
    clearInlineFormatting('**hello**', { anchor: 2, head: 7 }),
    { text: 'hello', anchor: 0, head: 5 }
  );
});

test('clears selected formatting source', () => {
  assert.deepEqual(
    clearInlineFormatting('`hello`', { anchor: 0, head: 7 }),
    { text: 'hello', anchor: 0, head: 5 }
  );
});

test('returns null when there is no supported formatting to clear', () => {
  assert.equal(clearInlineFormatting('hello', { anchor: 0, head: 5 }), null);
});
