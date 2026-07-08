import assert from 'node:assert/strict';
import test from 'node:test';

import {
  isMatchingFenceCloser,
  parseFenceLine,
} from './markdownFences.ts';

test('parses generic fenced code openers', () => {
  assert.deepEqual(parseFenceLine('  ````ts title'), {
    indent: '  ',
    char: '`',
    width: 4,
    info: 'ts title',
  });
  assert.deepEqual(parseFenceLine('~~~'), {
    indent: '',
    char: '~',
    width: 3,
    info: '',
  });
});

test('matches fence closers by marker kind and minimum width', () => {
  const opener = parseFenceLine('````markdown');
  assert.ok(opener);

  assert.equal(isMatchingFenceCloser(opener, '````'), true);
  assert.equal(isMatchingFenceCloser(opener, '`````'), true);
  assert.equal(isMatchingFenceCloser(opener, '```'), false);
  assert.equal(isMatchingFenceCloser(opener, '~~~~'), false);
  assert.equal(isMatchingFenceCloser(opener, '```` js'), false);
});
