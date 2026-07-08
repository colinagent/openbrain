import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildActivityThreadMetadataViewModel,
  formatActivityThreadCreatedAt,
  parseThreadCreatedAtFromID,
  resolveActivityChatFileName,
} from './activityThreadMetadata';

test('parseThreadCreatedAtFromID decodes the UTC timestamp encoded in a dated thread id', () => {
  const date = parseThreadCreatedAtFromID('thread-20260704T082103Z-e16a51');
  assert.ok(date instanceof Date);
  assert.equal(date?.getUTCFullYear(), 2026);
  assert.equal(date?.getUTCMonth(), 6);
  assert.equal(date?.getUTCDate(), 4);
  assert.equal(date?.getUTCHours(), 8);
  assert.equal(date?.getUTCMinutes(), 21);
  assert.equal(date?.getUTCSeconds(), 3);
});

test('parseThreadCreatedAtFromID returns null for legacy or malformed thread ids', () => {
  assert.equal(parseThreadCreatedAtFromID('thread-legacy-random'), null);
  assert.equal(parseThreadCreatedAtFromID(''), null);
  assert.equal(parseThreadCreatedAtFromID('  '), null);
  assert.equal(parseThreadCreatedAtFromID(undefined), null);
  assert.equal(parseThreadCreatedAtFromID('thread-20260704T082103Z'), null);
});

test('formatActivityThreadCreatedAt returns empty string for missing or invalid dates', () => {
  assert.equal(formatActivityThreadCreatedAt(null), '');
  assert.equal(formatActivityThreadCreatedAt(undefined), '');
  assert.equal(formatActivityThreadCreatedAt(new Date('not-a-date')), '');
});

test('formatActivityThreadCreatedAt renders a readable label for a parsed date', () => {
  const date = parseThreadCreatedAtFromID('thread-20260704T082103Z-e16a51');
  const label = formatActivityThreadCreatedAt(date);
  assert.ok(label.length > 0);
  assert.match(label, /Jul/);
  assert.match(label, /4/);
});

test('resolveActivityChatFileName extracts the POSIX basename of a chat path', () => {
  assert.equal(resolveActivityChatFileName('/Users/example/.agent/chat/note-coder.md'), 'note-coder.md');
  assert.equal(resolveActivityChatFileName('workspace/.agent/chat/build-hero.md'), 'build-hero.md');
});

test('resolveActivityChatFileName returns empty string for missing or bare paths', () => {
  assert.equal(resolveActivityChatFileName(''), '');
  assert.equal(resolveActivityChatFileName('   '), '');
  assert.equal(resolveActivityChatFileName(null), '');
  assert.equal(resolveActivityChatFileName(undefined), '');
});

test('buildActivityThreadMetadataViewModel aggregates thread, chatfile, and time labels', () => {
  const vm = buildActivityThreadMetadataViewModel({
    threadID: 'thread-20260704T082103Z-e16a51',
    chatPath: '/Users/example/.agent/chat/note-coder.md',
  });
  assert.equal(vm.threadID, 'thread-20260704T082103Z-e16a51');
  assert.equal(vm.chatFileName, 'note-coder.md');
  assert.ok(vm.createdAtLabel.length > 0);
});

test('buildActivityThreadMetadataViewModel omits time for legacy thread ids and chatfile for missing paths', () => {
  const vm = buildActivityThreadMetadataViewModel({
    threadID: 'thread-legacy-random',
    chatPath: '',
  });
  assert.equal(vm.threadID, 'thread-legacy-random');
  assert.equal(vm.chatFileName, '');
  assert.equal(vm.createdAtLabel, '');
});
