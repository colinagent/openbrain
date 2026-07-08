import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildThreadLinkTarget,
  isValidThreadID,
  parseThreadLinkTarget,
} from './threadLink.ts';

test('buildThreadLinkTarget formats a valid thread target', () => {
  assert.equal(buildThreadLinkTarget('thread-123'), 'thread:thread-123');
  assert.equal(buildThreadLinkTarget('20260624T153012Z-3f9ac1'), 'thread:20260624T153012Z-3f9ac1');
});

test('buildThreadLinkTarget rejects invalid thread ids', () => {
  assert.equal(buildThreadLinkTarget('abc'), '');
});

test('parseThreadLinkTarget parses a valid thread target', () => {
  assert.deepEqual(parseThreadLinkTarget('thread:thread-123'), { threadID: 'thread-123' });
  assert.deepEqual(parseThreadLinkTarget('thread:20260624T153012Z-3f9ac1'), {
    threadID: '20260624T153012Z-3f9ac1',
  });
});

test('parseThreadLinkTarget rejects invalid targets', () => {
  assert.equal(parseThreadLinkTarget('thread:abc'), null);
  assert.equal(parseThreadLinkTarget('/tmp/file.md'), null);
});

test('isValidThreadID accepts canonical and legacy thread ids', () => {
  assert.equal(isValidThreadID('thread-123'), true);
  assert.equal(isValidThreadID('20260624T153012Z-3f9ac1'), true);
  assert.equal(isValidThreadID('abc'), false);
  assert.equal(isValidThreadID('20260624T153012Z-xyz123'), false);
});
