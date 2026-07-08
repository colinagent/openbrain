import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const sourcePath = new URL('./markdownHighlight.ts', import.meta.url);
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
const { findMarkdownHighlightRanges } = sourceModule;

test('finds simple markdown highlight ranges', () => {
  assert.deepEqual(findMarkdownHighlightRanges('==highlight=='), [
    { from: 0, to: 13 },
  ]);
});

test('finds multiple highlight ranges on one line', () => {
  assert.deepEqual(findMarkdownHighlightRanges('a ==one== b ==two=='), [
    { from: 2, to: 9 },
    { from: 12, to: 19 },
  ]);
});

test('skips empty and whitespace-only highlight ranges', () => {
  assert.deepEqual(findMarkdownHighlightRanges('==== == =='), []);
});

test('skips markers that are escaped or part of longer equals runs', () => {
  assert.deepEqual(findMarkdownHighlightRanges('\\==no== ===no=='), []);
});

test('skips candidate ranges that overlap excluded markdown syntax', () => {
  assert.deepEqual(
    findMarkdownHighlightRanges('==`code`==', [{ from: 2, to: 8 }]),
    []
  );
});

test('continues after excluded ranges and finds later highlights', () => {
  assert.deepEqual(
    findMarkdownHighlightRanges('`==no==` ==yes==', [{ from: 0, to: 8 }]),
    [{ from: 9, to: 16 }]
  );
});
