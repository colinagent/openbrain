import assert from 'node:assert/strict';
import test from 'node:test';

import { serializeTableCellSelectionRows } from './tableSelectionCopy.ts';

const rows = [
  ['指标', '目标（3 个月）'],
  ['内测用户数', '100-200'],
  ['周活跃率', '> 40%'],
  ['用户自发分享内容数', '> 10 篇/帖子'],
  ['GitHub Star（如有开源）', '> 500'],
  ['核心反馈收集', '> 50 条有效 issue/建议'],
];

test('serializeTableCellSelectionRows copies a single visual row as tab-separated text', () => {
  assert.equal(
    serializeTableCellSelectionRows(rows, {
      anchorRow: 4,
      anchorCol: 0,
      currentRow: 4,
      currentCol: 1,
    }),
    'GitHub Star（如有开源）\t> 500',
  );
});

test('serializeTableCellSelectionRows copies a multi-row rectangle as newline-separated text', () => {
  assert.equal(
    serializeTableCellSelectionRows(rows, {
      anchorRow: 2,
      anchorCol: 0,
      currentRow: 4,
      currentCol: 1,
    }),
    [
      '周活跃率\t> 40%',
      '用户自发分享内容数\t> 10 篇/帖子',
      'GitHub Star（如有开源）\t> 500',
    ].join('\n'),
  );
});

test('serializeTableCellSelectionRows preserves empty cells inside the copied rectangle', () => {
  assert.equal(
    serializeTableCellSelectionRows([
      ['a', 'b'],
      ['1'],
      ['2', '3'],
    ], {
      anchorRow: 1,
      anchorCol: 0,
      currentRow: 2,
      currentCol: 1,
    }),
    '1\t\n2\t3',
  );
});

test('serializeTableCellSelectionRows handles reverse drag direction', () => {
  assert.equal(
    serializeTableCellSelectionRows(rows, {
      anchorRow: 4,
      anchorCol: 1,
      currentRow: 3,
      currentCol: 0,
    }),
    [
      '用户自发分享内容数\t> 10 篇/帖子',
      'GitHub Star（如有开源）\t> 500',
    ].join('\n'),
  );
});
