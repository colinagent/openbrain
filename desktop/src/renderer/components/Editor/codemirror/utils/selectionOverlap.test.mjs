import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const sourcePath = new URL('./selectionOverlap.ts', import.meta.url);
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
const { isSelectionOverlappingRange } = sourceModule;

test('reveals inline source only when the caret is inside the source range', () => {
  assert.equal(
    isSelectionOverlappingRange({ from: 2, to: 2, head: 2, empty: true }, 3, 8),
    false
  );
  assert.equal(
    isSelectionOverlappingRange({ from: 3, to: 3, head: 3, empty: true }, 3, 8),
    true
  );
  assert.equal(
    isSelectionOverlappingRange({ from: 8, to: 8, head: 8, empty: true }, 3, 8),
    true
  );
  assert.equal(
    isSelectionOverlappingRange({ from: 9, to: 9, head: 9, empty: true }, 3, 8),
    false
  );
});

test('reveals inline source when a non-empty selection intersects the source range', () => {
  assert.equal(
    isSelectionOverlappingRange({ from: 1, to: 3, head: 3, empty: false }, 3, 8),
    false
  );
  assert.equal(
    isSelectionOverlappingRange({ from: 1, to: 4, head: 4, empty: false }, 3, 8),
    true
  );
  assert.equal(
    isSelectionOverlappingRange({ from: 6, to: 9, head: 9, empty: false }, 3, 8),
    true
  );
});
