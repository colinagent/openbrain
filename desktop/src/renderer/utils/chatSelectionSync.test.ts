import assert from 'node:assert/strict';
import test from 'node:test';

import { shouldSyncConversationSelectionWithActiveChat } from './chatSelectionSync';

test('does not let the active editor override a pending new chat selection', () => {
  assert.equal(
    shouldSyncConversationSelectionWithActiveChat(
      '/tmp/workspace/.agent/chat/current.md',
      { kind: 'pending', id: 'pending-1' },
    ),
    false,
  );
});

test('syncs when no conversation target is selected yet', () => {
  assert.equal(
    shouldSyncConversationSelectionWithActiveChat('/tmp/workspace/.agent/chat/current.md', null),
    true,
  );
});

test('syncs when a different chat thread is selected', () => {
  assert.equal(
    shouldSyncConversationSelectionWithActiveChat(
      '/tmp/workspace/.agent/chat/current.md',
      { kind: 'thread', threadID: 'thread-other', chatPath: '/tmp/workspace/.agent/chat/other.md' },
    ),
    true,
  );
});

test('skips sync when the active chat is already selected', () => {
  assert.equal(
    shouldSyncConversationSelectionWithActiveChat(
      '/tmp/workspace/.agent/chat/current.md',
      { kind: 'thread', threadID: 'thread-current', chatPath: '/tmp/workspace/.agent/chat/current.md' },
    ),
    false,
  );
});
