import assert from 'node:assert/strict';
import test from 'node:test';

import {
  isCanonicalChatMarkdownContent,
  normalizeCanonicalChatFrontmatter,
  parseCanonicalChatFrontmatter,
} from './frontmatterParser';

test('recognizes canonical chat markdown frontmatter', () => {
  const content = [
    '---',
    'thread: thread-123',
    'title: "Hello"',
    '---',
    '',
    'body',
  ].join('\n');

  assert.deepEqual(parseCanonicalChatFrontmatter(content), {
    threadID: 'thread-123',
    title: 'Hello',
  });
  assert.equal(isCanonicalChatMarkdownContent(content), true);
});

test('parses canonical parent thread frontmatter when present', () => {
  const content = [
    '---',
    'thread: thread-child',
    'title: "Child"',
    'parent_thread: thread-parent',
    '---',
  ].join('\n');

  assert.deepEqual(parseCanonicalChatFrontmatter(content), {
    threadID: 'thread-child',
    title: 'Child',
    parentThreadID: 'thread-parent',
  });
});

test('rejects legacy thread frontmatter', () => {
  const content = [
    '---',
    'thread: "thread:thread-123"',
    'title: "Hello"',
    '---',
  ].join('\n');

  assert.equal(parseCanonicalChatFrontmatter(content), null);
  assert.equal(isCanonicalChatMarkdownContent(content), false);
});

test('rejects invalid canonical chat frontmatter', () => {
  assert.equal(normalizeCanonicalChatFrontmatter({ thread: 'thread:abc', title: 'Hello' }), null);
  assert.equal(normalizeCanonicalChatFrontmatter({ thread: 'thread-123', title: '' }), null);
  assert.equal(normalizeCanonicalChatFrontmatter({ thread: '', title: 'Hello' }), null);
});
