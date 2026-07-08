import assert from 'node:assert/strict';
import test from 'node:test';

import { EditorState } from '@codemirror/state';
import { getBlockReplacementTo } from './blockReplacement.ts';

test('extends block replacement through the trailing newline when the block ends at line end', () => {
  const state = EditorState.create({
    doc: '![img](./assets/demo.png)\nnext line',
  });
  const line = state.doc.line(1);

  assert.equal(getBlockReplacementTo(state, line.from, line.to), line.to + 1);
});

test('keeps block replacement at EOF when there is no trailing newline to consume', () => {
  const state = EditorState.create({
    doc: '![img](./assets/demo.png)',
  });
  const line = state.doc.line(1);

  assert.equal(getBlockReplacementTo(state, line.from, line.to), line.to);
});

test('consumes only the image line newline when the next line has other content', () => {
  const state = EditorState.create({
    doc: '![img](./assets/demo.png)\nnext line',
  });
  const line = state.doc.line(1);
  const replacementTo = getBlockReplacementTo(state, line.from, line.to);

  assert.equal(replacementTo, state.doc.line(2).from);
  assert.equal(state.doc.sliceString(line.from, replacementTo), '![img](./assets/demo.png)\n');
});
