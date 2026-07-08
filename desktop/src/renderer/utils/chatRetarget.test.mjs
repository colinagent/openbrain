import assert from 'node:assert/strict';
import test from 'node:test';

import { retargetChatSnapshot } from './chatRetarget.ts';

test('retargetChatSnapshot migrates chat keyed state to the new path', () => {
  const oldPath = '/tmp/workspace/.agent/chat/old.md';
  const newPath = '/tmp/workspace/.agent/chat/history/2026-03-15/old.md';
  const next = retargetChatSnapshot({
    draftByTargetKey: { [`command:${oldPath}`]: 'draft text' },
    composerPlanStateByTargetKey: { [`command:${oldPath}`]: { anchor: 12 } },
    modelKeyByTargetKey: { [`command:${oldPath}`]: 'cloud:gpt-5' },
    agentByTargetKey: {},
    threadSnapshotByID: { [oldPath]: { entries: [{ id: 'entry-1' }] } },
    liveOverlayByThreadID: { [oldPath]: { streamingText: 'hello' } },
    targetChatPath: oldPath,
    selectedConversationTarget: { kind: 'command', path: oldPath },
  }, oldPath, newPath);

  assert.deepEqual(next.selectedConversationTarget, { kind: 'command', path: newPath });
  assert.equal(next.targetChatPath, newPath);
  assert.equal(next.draftByTargetKey[`command:${newPath}`], 'draft text');
  assert.equal(next.draftByTargetKey[`command:${oldPath}`], undefined);
  assert.deepEqual(next.composerPlanStateByTargetKey[`command:${newPath}`], { anchor: 12 });
  assert.equal(next.modelKeyByTargetKey[`command:${newPath}`], 'cloud:gpt-5');
  assert.deepEqual(next.threadSnapshotByID[newPath], { entries: [{ id: 'entry-1' }] });
  assert.equal(next.threadSnapshotByID[oldPath], undefined);
  assert.deepEqual(next.liveOverlayByThreadID[newPath], { streamingText: 'hello' });
  assert.equal(next.liveOverlayByThreadID[oldPath], undefined);
});
