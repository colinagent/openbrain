import assert from 'node:assert/strict';
import test from 'node:test';

import { EditorState } from '@codemirror/state';
import { markdown, markdownLanguage } from '@codemirror/lang-markdown';
import {
  collectListContinuationLineInfo,
  getListContinuationInsert,
  getListItemContinuationIndentAt,
  getTaskListPrefixRange,
  parseListLinePrefix,
} from './listPrefix.ts';

test('parses task list prefix through the first content character', () => {
  const parsed = parseListLinePrefix('- [x] done');

  assert.ok(parsed);
  assert.deepEqual(getTaskListPrefixRange(parsed), { from: 0, to: 6 });
  assert.equal(parsed.taskMarkerFrom, 2);
  assert.equal(parsed.taskMarkerTo, 5);
  assert.equal(parsed.taskContentFrom, 6);
  assert.equal(parsed.taskChecked, true);
});

test('parses nested unchecked task list prefix', () => {
  const parsed = parseListLinePrefix('  - [ ] todo');

  assert.ok(parsed);
  assert.deepEqual(getTaskListPrefixRange(parsed), { from: 2, to: 8 });
  assert.equal(parsed.markerFrom, 2);
  assert.equal(parsed.taskMarkerFrom, 4);
  assert.equal(parsed.taskMarkerTo, 7);
  assert.equal(parsed.taskContentFrom, 8);
  assert.equal(parsed.taskChecked, false);
});

test('returns null task prefix range for plain unordered list items', () => {
  const parsed = parseListLinePrefix('- plain item');

  assert.ok(parsed);
  assert.equal(getTaskListPrefixRange(parsed), null);
  assert.equal(parsed.taskContentFrom, null);
});

test('continues ordered lists without preserving loose-list blank separators', () => {
  assert.equal(
    getListContinuationInsert('3. 创建Cloudflare worker'),
    '\n4. '
  );
});

test('continues nested ordered lists at the same source indent', () => {
  assert.equal(
    getListContinuationInsert('    9. nested item'),
    '\n    10. '
  );
});

test('continues task lists with an unchecked task marker', () => {
  assert.equal(
    getListContinuationInsert('- [x] done'),
    '\n- [ ] '
  );
});

test('does not continue empty list items', () => {
  assert.equal(getListContinuationInsert('4. '), null);
  assert.equal(getListContinuationInsert('- [ ] '), null);
});

test('collects list continuation metadata for lazy text and image lines', () => {
  const doc = [
    '1. 创建一个github仓库',
    '要部署到cloudflare上',
    'https://github.com/example/cblog',
    '![build](./build.png)',
    '',
    '3. 创建Cloudflare worker',
  ].join('\n');
  const state = EditorState.create({
    doc,
    extensions: [markdown({ base: markdownLanguage })],
  });

  const info = collectListContinuationLineInfo(state, 0, doc.length);

  assert.deepEqual(info.get(2), { depth: 0, markerTo: 3 });
  assert.deepEqual(info.get(3), { depth: 0, markerTo: 3 });
  assert.deepEqual(info.get(4), { depth: 0, markerTo: 3 });
  assert.equal(info.has(5), false);

  const imageFrom = doc.indexOf('![build]');
  const visibleSliceInfo = collectListContinuationLineInfo(
    state,
    imageFrom,
    imageFrom + '![build]'.length
  );
  assert.deepEqual([...visibleSliceInfo.keys()], [4]);
});

test('returns source indentation for new lines inside list items', () => {
  const doc = [
    '1. parent',
    '   continuation',
    '  - [x] nested task',
  ].join('\n');
  const state = EditorState.create({
    doc,
    extensions: [markdown({ base: markdownLanguage })],
  });

  assert.equal(getListItemContinuationIndentAt(state, doc.indexOf('parent')), '   ');
  assert.equal(getListItemContinuationIndentAt(state, doc.indexOf('continuation')), '   ');
  assert.equal(getListItemContinuationIndentAt(state, doc.indexOf('nested task')), '        ');
});
