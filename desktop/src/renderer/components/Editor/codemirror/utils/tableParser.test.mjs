import assert from 'node:assert/strict';
import test from 'node:test';

import { parseTableBlock, parseTableLine, serializeTable, splitTableCells } from './tableParser.ts';

test('parseTableLine ignores escaped pipes and pipes inside code spans', () => {
  const parsed = parseTableLine('| left \\| right | `code|span` | tail |');
  assert.ok(parsed);
  assert.deepEqual(parsed.pipes, [0, 16, 30, 37]);
  assert.equal(parsed.isSeparator, false);
});

test('splitTableCells preserves empty cells and ignores non-delimiting pipes', () => {
  assert.deepEqual(splitTableCells('| left \\| right |  | `code|span` |'), [
    'left \\| right',
    '',
    '`code|span`',
  ]);
});

test('parseTableBlock preserves raw cell markdown while splitting cells correctly', () => {
  const source = [
    '| a \\| b | `x|y` |',
    '| --- | --- |',
    '| **bold** | `code|span` and tail |',
  ].join('\n');

  assert.deepEqual(parseTableBlock(source), {
    headers: ['a \\| b', '`x|y`'],
    alignments: ['none', 'none'],
    rows: [['**bold**', '`code|span` and tail']],
  });
});

test('serializeTable keeps escaped pipes stable and does not escape pipes inside code spans', () => {
  const source = [
    '| a \\| b | `x|y` |',
    '| --- | --- |',
    '| **bold** | `code|span` and tail |',
  ].join('\n');
  const parsed = parseTableBlock(source);
  assert.ok(parsed);

  assert.equal(
    serializeTable(parsed.headers, parsed.rows, parsed.alignments),
    source
  );
});

test('serializeTable escapes plain pipes outside code spans', () => {
  assert.equal(
    serializeTable(['a | b', '`x|y`'], [['left | right', '`m|n`']], ['none', 'none']),
    [
      '| a \\| b | `x|y` |',
      '| --- | --- |',
      '| left \\| right | `m|n` |',
    ].join('\n')
  );
});

test('parseTableBlock normalizes ragged tables to a rectangular grid', () => {
  const source = [
    '|| 工具 | AI 对话存在哪 |',
    '| --- | --- |',
    '| ChatGPT | OpenAI 的服务器，你看不见摸不着 |',
    '| OpenBrain | 就是你文件夹里的 .md 文件 |件 |',
  ].join('\n');

  assert.deepEqual(parseTableBlock(source), {
    headers: ['', '工具', 'AI 对话存在哪'],
    alignments: ['none', 'none', 'none'],
    rows: [
      ['ChatGPT', 'OpenAI 的服务器，你看不见摸不着', ''],
      ['OpenBrain', '就是你文件夹里的 .md 文件', '件'],
    ],
  });
});

test('serializeTable pads ragged rows and separators instead of writing missing cells', () => {
  assert.equal(
    serializeTable(
      ['', '工具', 'AI 对话存在哪'],
      [
        ['ChatGPT', 'OpenAI 的服务器，你看不见摸不着'],
        ['OpenBrain', '就是你文件夹里的 .md 文件', '件'],
      ],
      ['none', 'none']
    ),
    [
      '|  | 工具 | AI 对话存在哪 |',
      '| --- | --- | --- |',
      '| ChatGPT | OpenAI 的服务器，你看不见摸不着 |  |',
      '| OpenBrain | 就是你文件夹里的 .md 文件 | 件 |',
    ].join('\n')
  );
});
