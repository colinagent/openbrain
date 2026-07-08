import assert from 'node:assert/strict';
import test from 'node:test';

import { parseTableCellInlineMarkdown } from './tableCellInlineMarkdown.ts';

test('parses strong, emphasis, strikethrough, highlight, code, and line breaks in table cells', () => {
  assert.deepEqual(parseTableCellInlineMarkdown('before **bold** *em* ~~gone~~ ==mark== `code`<br>after'), [
    { type: 'text', text: 'before ' },
    { type: 'strong', children: [{ type: 'text', text: 'bold' }] },
    { type: 'text', text: ' ' },
    { type: 'emphasis', children: [{ type: 'text', text: 'em' }] },
    { type: 'text', text: ' ' },
    { type: 'strikethrough', children: [{ type: 'text', text: 'gone' }] },
    { type: 'text', text: ' ' },
    { type: 'highlight', children: [{ type: 'text', text: 'mark' }] },
    { type: 'text', text: ' ' },
    { type: 'code', text: 'code' },
    { type: 'lineBreak' },
    { type: 'text', text: 'after' },
  ]);
});

test('treats escaped markers as literal text outside code spans', () => {
  assert.deepEqual(parseTableCellInlineMarkdown('\\*\\*bold\\*\\* and \\=\\=mark\\=\\= and \\<br> and \\|'), [
    { type: 'text', text: '**bold** and ==mark== and <br> and |' },
  ]);
});

test('treats escaped square brackets as literal text in table cells', () => {
  assert.deepEqual(parseTableCellInlineMarkdown('\\[ 待填 \\]'), [
    { type: 'text', text: '[ 待填 ]' },
  ]);
});

test('does not parse markdown markers inside code spans', () => {
  assert.deepEqual(parseTableCellInlineMarkdown('`**bold** ==mark== | <br>`'), [
    { type: 'code', text: '**bold** ==mark== | <br>' },
  ]);
});

test('does not parse empty table highlight markers', () => {
  assert.deepEqual(parseTableCellInlineMarkdown('==== and == =='), [
    { type: 'text', text: '==== and == ==' },
  ]);
});
