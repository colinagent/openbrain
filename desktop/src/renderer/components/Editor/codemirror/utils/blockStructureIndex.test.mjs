import assert from 'node:assert/strict';
import test from 'node:test';

import { EditorState } from '@codemirror/state';

import {
  buildBlockStructureIndex,
  findIndexedFenceBlockAtPos,
  findIndexedImageBlockBySourceRange,
  findIndexedTableBlockAtPos,
} from './blockStructureIndex.ts';

test('indexes frontmatter, fenced blocks, tables, standalone images, and indented code blocks', () => {
  const doc = [
    '---',
    'title: demo',
    '---',
    '',
    '```shell',
    'bash: pwd',
    '```',
    '',
    '```notes',
    'planning',
    '```',
    '',
    '```text',
    'hello',
    '```',
    '',
    '```mermaid',
    'graph TD',
    '```',
    '',
    '| a | b |',
    '| --- | --- |',
    '| 1 | 2 |',
    '',
    '![demo](./assets/demo.png){width=30%}',
    '',
    '    const x = 1;',
    '    console.log(x);',
  ].join('\n');
  const state = EditorState.create({ doc });
  const index = buildBlockStructureIndex(state);

  assert.deepEqual(index.frontmatter, {
    from: 0,
    to: state.doc.line(3).to,
    endLineNumber: 3,
  });
  assert.equal(index.fences.length, 4);
  assert.equal(index.tables.length, 1);
  assert.equal(index.images.length, 1);
  assert.equal(index.indentedCodeBlocks.length, 1);

  assert.equal(index.fences[0].language, 'shell');
  assert.equal(index.fences[1].language, 'notes');
  assert.equal(index.fences[2].language, 'text');
  assert.equal(index.fences[3].language, 'mermaid');

  assert.equal(index.tables[0].startLineNumber, 21);
  assert.equal(index.tables[0].endLineNumber, 23);

  assert.equal(index.images[0].lineNumber, 25);
  assert.equal(index.images[0].url, './assets/demo.png');
  assert.equal(index.images[0].widthPercent, 30);

  assert.equal(index.indentedCodeBlocks[0].startLineNumber, 27);
  assert.equal(index.indentedCodeBlocks[0].endLineNumber, 28);
});

test('does not index standalone images or tables inside fences or frontmatter', () => {
  const doc = [
    '---',
    'hero: ![frontmatter](./hero.png)',
    '---',
    '',
    '```shell',
    '![inside](./tool.png)',
    '| not | a | table |',
    '| --- | --- | --- |',
    '```',
    '',
    'plain | text',
    'still | text',
  ].join('\n');
  const state = EditorState.create({ doc });
  const index = buildBlockStructureIndex(state);

  assert.equal(index.images.length, 0);
  assert.equal(index.tables.length, 0);
  assert.equal(index.fences.length, 1);
});

test('does not treat ordinary pipe-separated text as a table', () => {
  const doc = [
    'foo | bar',
    'baz | qux',
    '',
    '| real | table |',
    '| --- | --- |',
    '| 1 | 2 |',
  ].join('\n');
  const state = EditorState.create({ doc });
  const index = buildBlockStructureIndex(state);

  assert.equal(index.tables.length, 1);
  assert.equal(index.tables[0].startLineNumber, 4);
});

test('supports point lookups for indexed blocks', () => {
  const doc = [
    '```shell',
    'bash: pwd',
    '```',
    '',
    '| a | b |',
    '| --- | --- |',
    '| 1 | 2 |',
    '',
    '![demo](./assets/demo.png)',
  ].join('\n');
  const state = EditorState.create({ doc });
  const index = buildBlockStructureIndex(state);
  const tablePos = doc.indexOf('| 1 | 2 |');
  const fencePos = doc.indexOf('bash: pwd');
  const imageFrom = doc.indexOf('![demo]');
  const imageTo = imageFrom + '![demo](./assets/demo.png)'.length;

  assert.equal(findIndexedFenceBlockAtPos(index, fencePos)?.startLineNumber, 1);
  assert.equal(findIndexedTableBlockAtPos(index, tablePos)?.startLineNumber, 5);
  assert.equal(findIndexedImageBlockBySourceRange(index, imageFrom, imageTo)?.lineNumber, 9);
});
