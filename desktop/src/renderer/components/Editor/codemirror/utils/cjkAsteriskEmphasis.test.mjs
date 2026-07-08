import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const sourcePath = new URL('./cjkAsteriskEmphasis.ts', import.meta.url);
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
const { findCjkAsteriskEmphasisRanges } = sourceModule;

test('finds unresolved strong emphasis around Chinese quotes after Han text', () => {
  const input = '而是**“大了但空”**。';

  assert.deepEqual(findCjkAsteriskEmphasisRanges(input), [
    { from: 2, to: 12, markerLength: 2 },
  ]);
});

test('finds unresolved emphasis around Chinese quotes after Han text', () => {
  const input = '而是*“大了但空”*。';

  assert.deepEqual(findCjkAsteriskEmphasisRanges(input), [
    { from: 2, to: 10, markerLength: 1 },
  ]);
});

test('skips ranges already parsed by the default markdown syntax tree', () => {
  const input = '**《封神第三部》大概率会是一部“西岐伐纣”的终章电影。**';

  assert.deepEqual(
    findCjkAsteriskEmphasisRanges(input, [{ from: 0, to: input.length }]),
    []
  );
});

test('does not treat plain bracket text as emphasis', () => {
  assert.deepEqual(findCjkAsteriskEmphasisRanges('- [ ] and - [x].'), []);
});

test('skips candidate ranges when the caller marks them as excluded', () => {
  const input = '`**“源码”**`';

  assert.deepEqual(
    findCjkAsteriskEmphasisRanges(input, [{ from: 0, to: input.length }]),
    []
  );
});
