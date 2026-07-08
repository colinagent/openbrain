import assert from 'node:assert/strict';
import test from 'node:test';

import { getChatWorkspaceStore, removeChatWorkspaceStore } from './chatWorkspaceStore';
import { useModelsStore } from './modelsStore';
import type { ThreadSnapshot } from '../services/threadService';
import type { ModelsConfig } from '../types/electron';

const EMPTY_MODELS_CONFIG: ModelsConfig = {
  version: 5,
  defaultModelKey: null,
  providers: {},
  models: [],
  updatedAt: 0,
};

function resetModelsConfig(config: ModelsConfig = EMPTY_MODELS_CONFIG) {
  useModelsStore.setState({ config });
}

function createStore(tabId: string) {
  resetModelsConfig();
  removeChatWorkspaceStore(tabId);
  const store = getChatWorkspaceStore(tabId);
  return store;
}

function threadTarget(chatPath: string, threadID = 'thread-test') {
  return { kind: 'thread' as const, threadID, chatPath };
}

function commandTarget(path: string) {
  return { kind: 'command' as const, path };
}

function threadEntry(id: string): NonNullable<ThreadSnapshot['entries']>[number] {
  return {
    type: 'canonical_message',
    id,
    timestamp: '2026-06-24T00:00:00Z',
    message: {
      role: 'user',
      content: [{ type: 'text', text: id }],
    },
  };
}

function rangeEntries(start: number, end: number): ThreadSnapshot['entries'] {
  return Array.from({ length: Math.max(0, end - start) }, (_, index) => threadEntry(`entry-${start + index}`));
}

function threadSnapshot(
  threadID: string,
  chatPath: string,
  entries: ThreadSnapshot['entries'] = [],
  overrides: Partial<ThreadSnapshot> = {},
): ThreadSnapshot {
  const normalizedEntries = entries || [];
  return {
    meta: {
      threadID,
      fileID: `file-${threadID}`,
      agentID: 'agent-id',
      cwd: '/tmp/workspace',
      chatPath,
      path: chatPath,
      title: threadID,
    },
    entries: normalizedEntries,
    entryWindow: {
      mode: 'tail',
      limit: 400,
      start: 0,
      end: normalizedEntries.length,
      total: normalizedEntries.length,
      hasBefore: false,
      hasAfter: false,
    },
    revision: `${threadID}:1`,
    runStatus: 'idle',
    tailStatus: 'complete',
    continuationReason: '',
    ...overrides,
  };
}

function messageRecord(
  id: string,
  overrides: Partial<NonNullable<ThreadSnapshot['messageRecords']>[number]> = {},
): NonNullable<ThreadSnapshot['messageRecords']>[number] {
  const threadID = overrides.threadID || 'thread-message-records';
  return {
    id,
    channelID: overrides.channelID || `channel-${threadID}`,
    threadID,
    agentID: overrides.agentID || 'agent-id',
    sender: overrides.sender || 'agent',
    kind: overrides.kind || 'message',
    status: overrides.status || 'open',
    title: overrides.title,
    body: overrides.body || id,
    actions: overrides.actions,
    questions: overrides.questions,
    replyToMessageID: overrides.replyToMessageID,
    actionID: overrides.actionID,
    answers: overrides.answers,
    createdAt: overrides.createdAt || '2026-06-24T00:00:00Z',
    updatedAt: overrides.updatedAt || '2026-06-24T00:00:00Z',
    meta: overrides.meta,
  };
}

test('createPendingConversation falls back to default chain when no remembered model exists', (t) => {
  const tabId = 'workspace-chat-model-memory-empty';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().createPendingConversation();
  const state = store.getState();

  assert.equal(state.rememberedModelKey, null);
  assert.deepEqual(state.selectedConversationTarget, {
    kind: 'pending',
    id: state.pendingConversations[0]?.id,
  });
  assert.equal(state.getSelectedModelKey(), null);
  assert.equal(state.getModelKeyForTarget(state.selectedConversationTarget), null);
});

test('createPendingConversation selects enabled Default Chat Model when no remembered model exists', (t) => {
  const tabId = 'workspace-chat-model-default-chat';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);
  resetModelsConfig({
    ...EMPTY_MODELS_CONFIG,
    models: [{
      key: 'cloud:gpt-5.5',
      id: 'gpt-5.5',
      label: 'GPT-5.5',
      enabled: true,
      provider: 'cloud',
      api: 'openai-responses',
      reasoning: true,
    }],
    strategies: {
      auto: {
        defaultChatModelID: 'cloud:gpt-5.5',
      },
    },
  });

  store.getState().createPendingConversation();
  const state = store.getState();

  assert.equal(state.rememberedModelKey, null);
  assert.equal(state.getSelectedModelKey(), 'cloud:gpt-5.5');
  assert.equal(state.getModelKeyForTarget(state.selectedConversationTarget), 'cloud:gpt-5.5');
});

test('createPendingConversation does not select disabled Default Chat Model', (t) => {
  const tabId = 'workspace-chat-model-default-disabled';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);
  resetModelsConfig({
    ...EMPTY_MODELS_CONFIG,
    models: [{
      key: 'cloud:gpt-5.5',
      id: 'gpt-5.5',
      label: 'GPT-5.5',
      enabled: false,
      provider: 'cloud',
      api: 'openai-responses',
      reasoning: true,
    }],
    strategies: {
      auto: {
        defaultChatModelID: 'cloud:gpt-5.5',
      },
    },
  });

  store.getState().createPendingConversation();
  const state = store.getState();

  assert.equal(state.getSelectedModelKey(), null);
  assert.equal(state.getModelKeyForTarget(state.selectedConversationTarget), null);
});

test('chat workspace stores do not carry pinned file UI state', (t) => {
  const tabId = 'workspace-chat-no-pin-state';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  assert.equal('conversationFilePinned' in store.getState(), false);
  assert.equal('setConversationFilePinned' in store.getState(), false);
});

test('clearAllAwaitingUsers removes pending question state', (t) => {
  const tabId = 'workspace-chat-clear-all-awaiting';
  const chatPath = '/tmp/workspace/.agent/chat/awaiting.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().setAwaitingUser(chatPath, {
    requestID: 'req-awaiting',
    questions: [{
      header: 'Q1',
      question: 'Continue?',
      options: [{ label: 'Yes' }],
      custom: false,
    }],
    currentIndex: 0,
    answers: [[]],
    customModeByIndex: [false],
    requestedAt: 1,
  });

  assert.equal(store.getState().getAwaitingUser(chatPath)?.requestID, 'req-awaiting');

  store.getState().clearAllAwaitingUsers();

  assert.equal(store.getState().getAwaitingUser(chatPath), null);
});

test('createPendingConversation reuses the remembered explicit model in the same workspace', (t) => {
  const tabId = 'workspace-chat-model-memory-explicit';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().setRememberedModelKey('cloud:claude-opus-4-6');
  store.getState().createPendingConversation();
  const state = store.getState();

  assert.equal(state.rememberedModelKey, 'cloud:claude-opus-4-6');
  assert.equal(state.getSelectedModelKey(), 'cloud:claude-opus-4-6');
  assert.equal(state.getModelKeyForTarget(state.selectedConversationTarget), 'cloud:claude-opus-4-6');
});

test('setSelectedModelKey creates a pending conversation when no target is selected', (t) => {
  const tabId = 'workspace-chat-model-manual-select-without-target';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().setRememberedModelKey('cloud:gpt-5.4');
  store.getState().setSelectedModelKey('cloud:gpt-5.4');
  const state = store.getState();

  assert.deepEqual(state.selectedConversationTarget, {
    kind: 'pending',
    id: state.pendingConversations[0]?.id,
  });
  assert.equal(state.getSelectedModelKey(), 'cloud:gpt-5.4');
  assert.equal(state.getModelKeyForTarget(state.selectedConversationTarget), 'cloud:gpt-5.4');
});

test('implicit pending creation from draft input inherits the remembered model', (t) => {
  const tabId = 'workspace-chat-model-memory-draft';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().setRememberedModelKey('cloud:gpt-5.4');
  store.getState().setDraftForSelectedTarget('hello');
  const state = store.getState();

  assert.deepEqual(state.selectedConversationTarget, {
    kind: 'pending',
    id: state.pendingConversations[0]?.id,
  });
  assert.equal(state.getSelectedModelKey(), 'cloud:gpt-5.4');
  assert.equal(state.getModelKeyForTarget(state.selectedConversationTarget), 'cloud:gpt-5.4');
});

test('selecting an existing chat thread does not overwrite the remembered workspace model', (t) => {
  const tabId = 'workspace-chat-model-memory-existing-thread';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);
  const chatPath = '/tmp/workspace/.agent/chat/existing.md';

  store.getState().setRememberedModelKey('local-claude:claude-opus-4-6');
  store.getState().syncChatSettings(chatPath, { modelKey: 'cloud:gpt-5.4' });
  store.getState().selectChatConversation(chatPath);
  const state = store.getState();

  assert.equal(state.rememberedModelKey, 'local-claude:claude-opus-4-6');
  assert.equal(state.getSelectedModelKey(), 'cloud:gpt-5.4');
});

test('remembered model is isolated per workspace store', (t) => {
  const tabIdA = 'workspace-chat-model-memory-a';
  const tabIdB = 'workspace-chat-model-memory-b';
  t.after(() => removeChatWorkspaceStore(tabIdA));
  t.after(() => removeChatWorkspaceStore(tabIdB));
  const storeA = createStore(tabIdA);
  const storeB = createStore(tabIdB);

  storeA.getState().setRememberedModelKey('local-claude:claude-opus-4-6');
  storeB.getState().setRememberedModelKey('cloud:gpt-5.4');
  storeA.getState().createPendingConversation();
  storeB.getState().createPendingConversation();

  assert.equal(storeA.getState().getSelectedModelKey(), 'local-claude:claude-opus-4-6');
  assert.equal(storeB.getState().getSelectedModelKey(), 'cloud:gpt-5.4');
});

test('consumePendingConversation transfers running state from pending to chat path', (t) => {
  const tabId = 'workspace-chat-inprogress-transfer';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  const pendingId = store.getState().createPendingConversation();
  const pendingTarget = store.getState().selectedConversationTarget;
  store.getState().setTargetInProgress(pendingTarget, true);
  const chatPath = '/tmp/workspace/.agent/chat/new.md';
  store.getState().consumePendingConversation(pendingId, chatPath, 'thread-new');

  assert.equal(store.getState().isTargetInProgress({ kind: 'pending', id: pendingId }), false);
  assert.equal(store.getState().isTargetInProgress(threadTarget(chatPath, 'thread-new')), true);
});

test('clearLiveOverlay preserves activity panel expanded state and user override', (t) => {
  const tabId = 'workspace-chat-activity-clear-preserves-panel-state';
  const chatPath = '/tmp/workspace/.agent/chat/activity.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().setActivityExpanded(chatPath, true, { userAction: true });
  store.getState().appendStreamingText(chatPath, 'reply');
  store.getState().pushLiveStep(chatPath, {
    id: 'step-1',
    type: 'toolcall',
    label: 'Running tool',
    status: 'running',
    ts: 1,
  });
  store.getState().clearLiveOverlay(chatPath);

  let bucket = store.getState().getLiveOverlay(chatPath);
  assert.equal(bucket.expanded, true);
  assert.equal(bucket.userOverride, 'expanded');
  assert.equal(bucket.streamingText, '');
  assert.equal(bucket.streamingSegments.length, 0);
  assert.equal(bucket.steps.length, 0);

  store.getState().setActivityExpanded(chatPath, false, { userAction: true });
  store.getState().appendStreamingText(chatPath, 'reply');
  store.getState().clearLiveOverlay(chatPath);

  bucket = store.getState().getLiveOverlay(chatPath);
  assert.equal(bucket.expanded, false);
  assert.equal(bucket.userOverride, 'collapsed');
  assert.equal(bucket.streamingText, '');
  assert.equal(bucket.streamingSegments.length, 0);
});

test('prepareLiveOverlayForNewRun clears live run artifacts and preserves panel state', (t) => {
  const tabId = 'workspace-chat-activity-prepare-new-run';
  const chatPath = '/tmp/workspace/.agent/chat/activity-prepare.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().setActivityExpanded(chatPath, true, { userAction: true });
  store.getState().pushLiveStep(chatPath, {
    id: 'reason-1',
    type: 'reasoning',
    label: 'Thinking',
    status: 'done',
    detail: 'old thinking',
    ts: 1,
  });
  store.getState().appendStreamingText(chatPath, 'old answer');
  store.getState().setLiveTokenUsage(chatPath, {
    inputTokens: 10,
    outputTokens: 20,
    totalTokens: 30,
    contextWindow: 1000,
    contextTokens: 40,
    contextPercent: 4,
  });
  store.getState().setErrorForChatPath(chatPath, 'old error', 'old_code');

  store.getState().prepareLiveOverlayForNewRun(chatPath);

  let bucket = store.getState().getLiveOverlay(chatPath);
  assert.equal(bucket.expanded, true);
  assert.equal(bucket.userOverride, 'expanded');
  assert.equal(bucket.streamingText, '');
  assert.deepEqual(bucket.streamingSegments.map((segment) => segment.text), []);
  assert.equal(bucket.activeStreamingSegmentID, null);
  assert.equal(bucket.steps.length, 0);
  assert.equal(bucket.errorMessage, null);
  assert.equal(bucket.errorCode, null);
  assert.deepEqual(bucket.loopUsage, { inputTokens: 0, outputTokens: 0, totalTokens: 0 });

  store.getState().appendStreamingText(chatPath, 'new answer');
  bucket = store.getState().getLiveOverlay(chatPath);
  assert.equal(bucket.streamingText, 'new answer');
  assert.deepEqual(bucket.streamingSegments.map((segment) => segment.text), ['new answer']);
});

test('syncThreadSnapshot stores cloned raw entries by thread id', (t) => {
  const tabId = 'workspace-chat-thread-snapshot-entries';
  const chatPath = '/tmp/workspace/.agent/chat/thread-snapshot.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);
  const entries = [{
    type: 'canonical_message',
    id: 'entry-1',
    role: 'user',
    content: [{ type: 'input_text', text: 'hello' }],
    timestamp: '2026-06-24T00:00:00Z',
  }];
  const snapshot = threadSnapshot('thread-snapshot', chatPath, entries);

  store.getState().syncThreadSnapshot(snapshot);
  entries[0].content = [];

  const stored = store.getState().getThreadSnapshot(chatPath);
  assert.equal(stored?.meta.threadID, 'thread-snapshot');
  assert.equal(stored?.revision, 'thread-snapshot:1');
  assert.deepEqual(stored?.entries?.[0]?.content, [{ type: 'input_text', text: 'hello' }]);
  assert.equal(store.getState().getThreadSnapshotForTarget(threadTarget(chatPath, 'thread-snapshot'))?.meta.threadID, 'thread-snapshot');
});

test('upsertThreadMessageRecords patches existing thread snapshot message records', (t) => {
  const tabId = 'workspace-chat-thread-message-record-upsert';
  const chatPath = '/tmp/workspace/.agent/chat/thread-message-records.md';
  const threadID = 'thread-message-records';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().syncThreadSnapshot(threadSnapshot(threadID, chatPath, [], {
    messageRecords: [
      messageRecord('request-1', {
        threadID,
        kind: 'request',
        status: 'open',
        body: 'Choose.',
        questions: [{
          id: 'decision',
          question: 'Pick one.',
          options: [{ id: 'a', label: 'A' }],
        }],
      }),
    ],
  }));

  store.getState().upsertThreadMessageRecords([
    messageRecord('request-1', {
      threadID,
      kind: 'request',
      status: 'resolved',
      body: 'Choose.',
      questions: [{
        id: 'decision',
        question: 'Pick one.',
        options: [{ id: 'a', label: 'A' }],
      }],
    }),
    messageRecord('reply-1', {
      threadID,
      sender: 'user',
      replyToMessageID: 'request-1',
      body: 'A',
      answers: [{ questionID: 'decision', optionID: 'a', label: 'A' }],
    }),
  ]);

  let records = store.getState().getThreadSnapshot(chatPath)?.messageRecords || [];
  assert.deepEqual(records.map((record) => `${record.id}:${record.status}:${record.sender}`), [
    'request-1:resolved:agent',
    'reply-1:open:user',
  ]);
  assert.deepEqual(records[1]?.answers, [{ questionID: 'decision', optionID: 'a', label: 'A' }]);

  store.getState().upsertThreadMessageRecords([
    messageRecord('reply-1', {
      threadID,
      status: 'archived',
      sender: 'user',
    }),
  ]);
  records = store.getState().getThreadSnapshot(chatPath)?.messageRecords || [];
  assert.deepEqual(records.map((record) => record.id), ['request-1']);
});

test('upsertThreadMessageRecords does not create snapshots for unknown threads', (t) => {
  const tabId = 'workspace-chat-thread-message-record-noop';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().upsertThreadMessageRecords([
    messageRecord('reply-unknown', {
      threadID: 'thread-unknown',
      sender: 'user',
    }),
  ]);

  const state = store.getState() as any;
  assert.equal(state.threadSnapshotByID['thread-unknown'], undefined);
});

test('syncThreadSnapshot caps tail entries to the frontend window limit', (t) => {
  const tabId = 'workspace-chat-thread-snapshot-tail-cap';
  const chatPath = '/tmp/workspace/.agent/chat/thread-tail-cap.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().syncThreadSnapshot(threadSnapshot('thread-tail-cap', chatPath, rangeEntries(0, 700), {
    entryWindow: {
      mode: 'tail',
      limit: 700,
      start: 0,
      end: 700,
      total: 700,
      hasBefore: false,
      hasAfter: false,
    },
  }));

  const stored = store.getState().getThreadSnapshot(chatPath);
  assert.equal(stored?.entries?.length, 600);
  assert.equal(stored?.entries?.[0]?.id, 'entry-100');
  assert.equal(stored?.entries?.[599]?.id, 'entry-699');
  assert.equal(stored?.entryWindow?.start, 100);
  assert.equal(stored?.entryWindow?.end, 700);
});

test('mergeThreadSnapshotWindow prepends older entries and evicts the newer tail', (t) => {
  const tabId = 'workspace-chat-thread-snapshot-prepend';
  const chatPath = '/tmp/workspace/.agent/chat/thread-prepend.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().syncThreadSnapshot(threadSnapshot('thread-prepend', chatPath, rangeEntries(400, 800), {
    entryWindow: {
      mode: 'tail',
      limit: 400,
      start: 400,
      end: 800,
      total: 1000,
      hasBefore: true,
      hasAfter: true,
    },
    revision: 'thread-prepend:1000',
  }));
  store.getState().mergeThreadSnapshotWindow(threadSnapshot('thread-prepend', chatPath, rangeEntries(0, 400), {
    entryWindow: {
      mode: 'before',
      anchorId: 'entry-400',
      limit: 400,
      start: 0,
      end: 400,
      total: 1000,
      hasBefore: false,
      hasAfter: true,
    },
    revision: 'thread-prepend:1000',
  }), 'prepend');

  const stored = store.getState().getThreadSnapshot(chatPath);
  assert.equal(stored?.entries?.length, 600);
  assert.equal(stored?.entries?.[0]?.id, 'entry-0');
  assert.equal(stored?.entries?.[599]?.id, 'entry-599');
  assert.equal(stored?.entryWindow?.start, 0);
  assert.equal(stored?.entryWindow?.end, 600);
  assert.equal(stored?.entryWindow?.hasBefore, false);
  assert.equal(stored?.entryWindow?.hasAfter, true);
});

test('mergeThreadSnapshotWindow appends newer entries and evicts the older head', (t) => {
  const tabId = 'workspace-chat-thread-snapshot-append';
  const chatPath = '/tmp/workspace/.agent/chat/thread-append.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().syncThreadSnapshot(threadSnapshot('thread-append', chatPath, rangeEntries(200, 800), {
    entryWindow: {
      mode: 'before',
      anchorId: 'entry-800',
      limit: 600,
      start: 200,
      end: 800,
      total: 1000,
      hasBefore: true,
      hasAfter: true,
    },
    revision: 'thread-append:1000',
  }));
  store.getState().mergeThreadSnapshotWindow(threadSnapshot('thread-append', chatPath, rangeEntries(800, 1000), {
    entryWindow: {
      mode: 'after',
      anchorId: 'entry-799',
      limit: 200,
      start: 800,
      end: 1000,
      total: 1000,
      hasBefore: true,
      hasAfter: false,
    },
    revision: 'thread-append:1000',
  }), 'append');

  const stored = store.getState().getThreadSnapshot(chatPath);
  assert.equal(stored?.entries?.length, 600);
  assert.equal(stored?.entries?.[0]?.id, 'entry-400');
  assert.equal(stored?.entries?.[599]?.id, 'entry-999');
  assert.equal(stored?.entryWindow?.start, 400);
  assert.equal(stored?.entryWindow?.end, 1000);
  assert.equal(stored?.entryWindow?.hasBefore, true);
  assert.equal(stored?.entryWindow?.hasAfter, false);
});

test('composerVisible does not affect selected thread snapshot lookup', (t) => {
  const tabId = 'workspace-chat-composer-visible-snapshot';
  const chatPath = '/tmp/workspace/.agent/chat/composer-visible-snapshot.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().openThreadConversation('thread-composer-visible', { chatPath });
  store.getState().syncThreadSnapshot(threadSnapshot('thread-composer-visible', chatPath));

  const selectedTarget = store.getState().selectedConversationTarget;
  assert.equal(store.getState().composerVisible, true);
  assert.equal(store.getState().getThreadSnapshotForTarget(selectedTarget)?.meta.threadID, 'thread-composer-visible');

  store.getState().hideComposer();
  assert.equal(store.getState().composerVisible, false);
  assert.equal(store.getState().getThreadSnapshotForTarget(selectedTarget)?.meta.threadID, 'thread-composer-visible');

  store.getState().showComposer();
  assert.equal(store.getState().composerVisible, true);
  assert.equal(store.getState().getThreadSnapshotForTarget(selectedTarget)?.meta.threadID, 'thread-composer-visible');
});

test('openThreadConversation selects a thread target, opens composer, and expands activity', (t) => {
  const tabId = 'workspace-chat-messenger-thread-snapshot';
  const chatPath = '/tmp/workspace/.agent/chat/messenger-thread-snapshot.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().openThreadConversation('messenger-thread', {
    title: 'Messenger thread',
    agentID: 'agent-coder',
  });
  store.getState().syncThreadSnapshot(threadSnapshot('messenger-thread', chatPath));

  const state = store.getState() as any;
  assert.deepEqual(state.selectedConversationTarget, { kind: 'thread', threadID: 'messenger-thread', chatPath });
  assert.equal(state.composerVisible, true);
  assert.equal(state.openComposerTargets[0]?.threadID, 'messenger-thread');
  assert.equal(state.openComposerTargets[0]?.title, 'Messenger thread');
  assert.equal(state.threadSnapshotByID['messenger-thread']?.meta.threadID, 'messenger-thread');
  assert.equal(state.threadSnapshotByID[chatPath], undefined);
  assert.equal(state.liveOverlayByThreadID['messenger-thread']?.expanded, true);
  assert.equal(state.liveOverlayByThreadID['messenger-thread']?.userOverride, 'expanded');
  assert.deepEqual(store.getState().getAgentForTarget(threadTarget(chatPath, 'messenger-thread')), {
    agentID: 'agent-id',
    agentName: null,
    agentCwd: '/tmp/workspace',
  });
  assert.equal(store.getState().getThreadMeta(chatPath)?.threadID, 'messenger-thread');
});

test('syncThreadSnapshot hydrates thread meta and preserves explicit thread agent selection', (t) => {
  const tabId = 'workspace-chat-snapshot-meta-agent';
  const chatPath = '/tmp/workspace/.agent/chat/snapshot-meta-agent.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().syncThreadSnapshot(threadSnapshot('thread-snapshot-agent', chatPath));

  assert.deepEqual(store.getState().getAgentForTarget(threadTarget(chatPath, 'thread-snapshot-agent')), {
    agentID: 'agent-id',
    agentName: null,
    agentCwd: '/tmp/workspace',
  });
  assert.equal(store.getState().getThreadMeta(chatPath)?.threadID, 'thread-snapshot-agent');

  store.getState().setAgentForTarget(threadTarget(chatPath, 'thread-snapshot-agent'), {
    agentID: 'agent-coder',
    agentName: 'coder',
    agentCwd: '/tmp/workspace',
  });
  store.getState().syncThreadSnapshot(threadSnapshot('thread-snapshot-agent', chatPath, [], {
    meta: {
      threadID: 'thread-snapshot-agent',
      fileID: 'file-thread-snapshot-agent',
      agentID: 'agent-id',
      cwd: '/tmp/workspace',
      chatPath,
      path: chatPath,
      title: 'Snapshot Agent',
    },
  }));

  assert.deepEqual(store.getState().getAgentForTarget(threadTarget(chatPath, 'thread-snapshot-agent')), {
    agentID: 'agent-coder',
    agentName: 'coder',
    agentCwd: '/tmp/workspace',
  });
});

test('thread state selectors keep stable references for the current snapshot', (t) => {
  const tabId = 'workspace-chat-thread-state-stable';
  const chatPath = '/tmp/workspace/.agent/chat/thread-state-stable.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().syncThreadSnapshot(threadSnapshot('thread-state-stable', chatPath));

  const firstByPath = store.getState().getThreadState(chatPath);
  const secondByPath = store.getState().getThreadState(chatPath);
  const firstByTarget = store.getState().getThreadStateForTarget(
    threadTarget(chatPath, 'thread-state-stable')
  );
  const secondByTarget = store.getState().getThreadStateForTarget(
    threadTarget(chatPath, 'thread-state-stable')
  );

  assert.strictEqual(firstByPath, secondByPath);
  assert.strictEqual(firstByTarget, secondByTarget);
  assert.deepEqual(firstByPath, {
    runStatus: 'idle',
    tailStatus: 'complete',
    continuationReason: '',
  });
});

test('syncThreadSnapshot clears settled live overlay but preserves expanded state', (t) => {
  const tabId = 'workspace-chat-thread-snapshot-clears-live';
  const chatPath = '/tmp/workspace/.agent/chat/thread-snapshot-clears.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().setActivityExpanded(chatPath, true, { userAction: true });
  store.getState().pushLiveStep(chatPath, {
    id: 'tool-1',
    type: 'toolcall',
    label: 'Read file',
    status: 'done',
    ts: Date.now(),
  });
  store.getState().appendStreamingText(chatPath, 'answer');
  store.getState().syncThreadSnapshot(threadSnapshot('thread-clears', chatPath, [{
    type: 'canonical_message',
    id: 'entry-1',
    role: 'assistant',
    content: [{ type: 'output_text', text: 'answer' }],
  }]));

  const bucket = store.getState().getLiveOverlay(chatPath);
  assert.equal(bucket.expanded, true);
  assert.equal(bucket.userOverride, 'expanded');
  assert.equal(bucket.streamingText, '');
  assert.equal(bucket.streamingSegments.length, 0);
  assert.equal(bucket.steps.length, 0);
  assert.equal(store.getState().getThreadSnapshot(chatPath)?.entries?.length, 1);
});

test('activity streaming text starts a new timeline segment after a step', (t) => {
  const tabId = 'workspace-chat-activity-stream-segments';
  const chatPath = '/tmp/workspace/.agent/chat/activity-segments.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().appendStreamingText(chatPath, 'first');
  store.getState().pushLiveStep(chatPath, {
    id: 'tool-1',
    type: 'toolcall',
    label: 'Read file',
    status: 'done',
    ts: Date.now(),
  });
  store.getState().appendStreamingText(chatPath, 'second');

  const bucket = store.getState().getLiveOverlay(chatPath);
  assert.equal(bucket.streamingText, 'firstsecond');
  assert.deepEqual(bucket.streamingSegments.map((segment) => segment.text), ['first', 'second']);
  assert.ok(bucket.streamingSegments[0].order < (bucket.steps[0].order || 0));
  assert.ok((bucket.steps[0].order || 0) < bucket.streamingSegments[1].order);
});

test('activity text_start starts a new streaming segment without requiring a step', (t) => {
  const tabId = 'workspace-chat-activity-stream-text-start';
  const chatPath = '/tmp/workspace/.agent/chat/activity-text-start.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().appendStreamingText(chatPath, 'first');
  store.getState().startStreamingTextBlock(chatPath);
  store.getState().appendStreamingText(chatPath, 'second');

  const bucket = store.getState().getLiveOverlay(chatPath);
  assert.equal(bucket.streamingText, 'firstsecond');
  assert.deepEqual(bucket.streamingSegments.map((segment) => segment.text), ['first', 'second']);
  assert.ok(bucket.streamingSegments[0].order < bucket.streamingSegments[1].order);
});

test('consumePendingConversation preserves draft text when pending chat becomes a real chat path', (t) => {
  const tabId = 'workspace-chat-draft-transfer';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  const pendingId = store.getState().createPendingConversation();
  store.getState().setDraftForSelectedTarget('draft body');
  const chatPath = '/tmp/workspace/.agent/chat/draft.md';
  store.getState().consumePendingConversation(pendingId, chatPath, 'thread-draft');

  assert.equal(
    store.getState().getDraftForTarget(threadTarget(chatPath, 'thread-draft')),
    'draft body',
  );
});

test('consumePendingConversation preserves the plan block state when pending chat becomes a real chat path', (t) => {
  const tabId = 'workspace-chat-plan-transfer';
  const chatPath = '/tmp/workspace/.agent/chat/draft.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  const pendingId = store.getState().createPendingConversation();
  const pendingTarget = store.getState().selectedConversationTarget;
  store.getState().setComposerPlanStateForTarget(pendingTarget, {
    anchor: 4,
    beforeSpacer: { from: 3, to: 4, text: '\n' },
    afterSpacer: { from: 4, to: 5, text: '\n' },
  });
  store.getState().consumePendingConversation(pendingId, chatPath, 'thread-plan');

  assert.deepEqual(
    store.getState().getComposerPlanStateForTarget(threadTarget(chatPath, 'thread-plan')),
    {
      anchor: 4,
      beforeSpacer: { from: 3, to: 4, text: '\n' },
      afterSpacer: { from: 4, to: 5, text: '\n' },
    },
  );
});

test('consumePendingConversation preserves pending agent selection on the created thread', (t) => {
  const tabId = 'workspace-chat-agent-transfer';
  const chatPath = '/tmp/workspace/.agent/chat/agent-transfer.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  const pendingId = store.getState().createPendingConversation();
  store.getState().setAgentForSelectedTarget({
    agentID: 'agent-gbrain',
    agentName: 'gbrain',
    agentCwd: '/tmp/workspace',
  });
  store.getState().consumePendingConversation(pendingId, chatPath, 'thread-agent');

  assert.deepEqual(
    store.getState().getAgentForTarget(threadTarget(chatPath, 'thread-agent')),
    {
      agentID: 'agent-gbrain',
      agentName: 'gbrain',
      agentCwd: '/tmp/workspace',
    },
  );
  assert.equal(store.getState().getAgentForTarget({ kind: 'pending', id: pendingId }), null);
});

test('setAgentInfo updates the workspace default without mutating the selected thread agent', (t) => {
  const tabId = 'workspace-chat-agent-default-does-not-retarget-thread';
  const chatPath = '/tmp/workspace/.agent/chat/agent-default.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  const target = threadTarget(chatPath, 'thread-agent-default');
  store.getState().selectThreadConversation('thread-agent-default', chatPath);
  store.getState().setAgentForSelectedTarget({
    agentID: 'agent-opagent',
    agentName: 'opagent',
    agentCwd: '/tmp/workspace',
  });

  store.getState().setAgentInfo('agent-gbrain', 'gbrain', '/tmp/workspace');

  assert.deepEqual(
    store.getState().getAgentForTarget(target),
    {
      agentID: 'agent-opagent',
      agentName: 'opagent',
      agentCwd: '/tmp/workspace',
    },
  );
  assert.deepEqual(
    {
      agentID: store.getState().agentID,
      agentName: store.getState().agentName,
      agentCwd: store.getState().agentCwd,
    },
    {
      agentID: 'agent-gbrain',
      agentName: 'gbrain',
      agentCwd: '/tmp/workspace',
    },
  );
});

test('new pending conversations inherit the workspace default agent', (t) => {
  const tabId = 'workspace-chat-agent-default-pending';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().setAgentInfo('agent-gbrain', 'gbrain', '/tmp/workspace');
  const pendingId = store.getState().createPendingConversation();

  assert.deepEqual(
    store.getState().getAgentForTarget({ kind: 'pending', id: pendingId }),
    {
      agentID: 'agent-gbrain',
      agentName: 'gbrain',
      agentCwd: '/tmp/workspace',
    },
  );
});

test('upsertThreadMeta migrates command agent selection to the thread target', (t) => {
  const tabId = 'workspace-chat-agent-command-migration';
  const chatPath = '/tmp/workspace/.agent/chat/agent-command.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().selectChatConversation(chatPath);
  store.getState().setAgentForSelectedTarget({
    agentID: 'agent-gbrain',
    agentName: 'gbrain',
    agentCwd: '/tmp/workspace',
  });
  store.getState().upsertThreadMeta({
    threadID: 'thread-agent-command',
    fileID: 'file-agent-command',
    agentID: 'agent-opagent',
    cwd: '/tmp/workspace',
    chatPath,
    title: 'Agent Command',
  });

  assert.deepEqual(
    store.getState().getAgentForTarget(threadTarget(chatPath, 'thread-agent-command')),
    {
      agentID: 'agent-gbrain',
      agentName: 'gbrain',
      agentCwd: '/tmp/workspace',
    },
  );
  assert.equal(store.getState().getAgentForTarget(commandTarget(chatPath)), null);
});

test('upsertThreadMeta uses legacy meta agent only when the thread has no selected agent', (t) => {
  const tabId = 'workspace-chat-agent-meta-default';
  const chatPath = '/tmp/workspace/.agent/chat/agent-meta.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().upsertThreadMeta({
    threadID: 'thread-agent-meta',
    fileID: 'file-agent-meta',
    agentID: 'agent-opagent',
    cwd: '/tmp/workspace',
    chatPath,
    title: 'Agent Meta',
  });

  assert.deepEqual(
    store.getState().getAgentForTarget(threadTarget(chatPath, 'thread-agent-meta')),
    {
      agentID: 'agent-opagent',
      agentName: null,
      agentCwd: '/tmp/workspace',
    },
  );

  store.getState().setAgentForTarget(threadTarget(chatPath, 'thread-agent-meta'), {
    agentID: 'agent-gbrain',
    agentName: 'gbrain',
    agentCwd: '/tmp/workspace',
  });
  store.getState().upsertThreadMeta({
    threadID: 'thread-agent-meta',
    fileID: 'file-agent-meta',
    agentID: 'agent-opagent',
    cwd: '/tmp/workspace',
    chatPath,
    title: 'Agent Meta',
  });

  assert.deepEqual(
    store.getState().getAgentForTarget(threadTarget(chatPath, 'thread-agent-meta')),
    {
      agentID: 'agent-gbrain',
      agentName: 'gbrain',
      agentCwd: '/tmp/workspace',
    },
  );
});

test('queued messages are scoped to their chat path', (t) => {
  const tabId = 'workspace-chat-queue-scope';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().syncQueuedMessages('/tmp/workspace/.agent/chat/a.md', {
    steering: [],
    followUp: [{
      chatPath: '/tmp/workspace/.agent/chat/a.md',
      id: 'queue-1',
      kind: 'follow_up',
      text: 'queued',
      selectedSkill: null,
      selectedSkillIDs: [],
      selectedSkillContext: null,
      planTurn: false,
    }],
  });

  assert.equal(
    store.getState().getQueuedMessages('/tmp/workspace/.agent/chat/a.md').followUp[0]?.text,
    'queued',
  );
  assert.deepEqual(
    store.getState().getQueuedMessages('/tmp/workspace/.agent/chat/b.md'),
    { steering: [], followUp: [] },
  );
  assert.deepEqual(
    store.getState().getQueuedMessagesForTarget({ kind: 'pending', id: 'pending-1' }),
    { steering: [], followUp: [] },
  );
});

test('appendQueuedMessage adds a pending steering item before server ack', (t) => {
  const tabId = 'workspace-chat-queue-append';
  const chatPath = '/tmp/workspace/.agent/chat/queued.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().appendQueuedMessage(chatPath, {
    chatPath,
    id: 'optimistic-steering-1',
    kind: 'steering',
    text: 'queued now',
    selectedSkill: null,
    selectedSkillIDs: [],
    selectedSkillContext: null,
    planTurn: false,
    pending: true,
  });

  assert.deepEqual(store.getState().getQueuedMessages(chatPath), {
    steering: [{
      chatPath,
      id: 'optimistic-steering-1',
      kind: 'steering',
      text: 'queued now',
      selectedSkill: null,
      selectedSkillIDs: [],
      selectedSkillContext: null,
      planTurn: false,
      pending: true,
    }],
    followUp: [],
  });
});

test('queued messages preserve the turn agent snapshot', (t) => {
  const tabId = 'workspace-chat-queue-agent-snapshot';
  const chatPath = '/tmp/workspace/.agent/chat/queued-agent.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().appendQueuedMessage(chatPath, {
    chatPath,
    id: 'optimistic-follow-up-1',
    kind: 'follow_up',
    text: 'queued with agent',
    agentID: 'agent-gbrain',
    agentName: 'gbrain',
    agentCwd: '/tmp/workspace',
    selectedSkill: null,
    selectedSkillIDs: [],
    selectedSkillContext: null,
    planTurn: false,
    pending: true,
  });

  assert.deepEqual(store.getState().getQueuedMessages(chatPath).followUp[0], {
    chatPath,
    id: 'optimistic-follow-up-1',
    kind: 'follow_up',
    text: 'queued with agent',
    agentID: 'agent-gbrain',
    agentName: 'gbrain',
    agentCwd: '/tmp/workspace',
    selectedSkill: null,
    selectedSkillIDs: [],
    selectedSkillContext: null,
    planTurn: false,
    pending: true,
  });
});

test('chat scroll-to-bottom requests are one-shot per chat path', (t) => {
  const tabId = 'workspace-chat-scroll-bottom-request';
  const chatPath = '/tmp/workspace/.agent/chat/request.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  assert.equal(store.getState().shouldScrollChatToBottom(chatPath), false);

  store.getState().requestChatScrollToBottom(chatPath);
  assert.equal(store.getState().shouldScrollChatToBottom(chatPath), true);

  store.getState().consumeChatScrollToBottom(chatPath);
  assert.equal(store.getState().shouldScrollChatToBottom(chatPath), false);
});

test('chat scroll-to-bottom requests follow chat path retargeting', (t) => {
  const tabId = 'workspace-chat-scroll-bottom-retarget';
  const oldPath = '/tmp/workspace/.agent/chat/old.md';
  const newPath = '/tmp/workspace/.agent/chat/new.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().requestChatScrollToBottom(oldPath);
  store.getState().retargetChatPath(oldPath, newPath);

  assert.equal(store.getState().shouldScrollChatToBottom(oldPath), false);
  assert.equal(store.getState().shouldScrollChatToBottom(newPath), true);
});

test('retargetChatPath moves the plan block state to the new chat path', (t) => {
  const tabId = 'workspace-chat-plan-retarget';
  const oldPath = '/tmp/workspace/.agent/chat/old.md';
  const newPath = '/tmp/workspace/.agent/chat/new.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  store.getState().setComposerPlanStateForTarget(commandTarget(oldPath), {
    anchor: 7,
    beforeSpacer: { from: 6, to: 7, text: '\n' },
    afterSpacer: { from: 7, to: 8, text: '\n' },
  });
  store.getState().retargetChatPath(oldPath, newPath);

  assert.equal(store.getState().getComposerPlanStateForTarget(commandTarget(oldPath)), null);
  assert.deepEqual(
    store.getState().getComposerPlanStateForTarget(commandTarget(newPath)),
    {
      anchor: 7,
      beforeSpacer: { from: 6, to: 7, text: '\n' },
      afterSpacer: { from: 7, to: 8, text: '\n' },
    },
  );
});

test('clearing the selected skill clears the current target plan block state', (t) => {
  const tabId = 'workspace-chat-plan-clear-selected-skill';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  const pendingId = store.getState().createPendingConversation();
  const pendingTarget = store.getState().selectedConversationTarget;
  store.getState().setSelectedSkill({ id: 'skill-plan', slug: 'plan', name: 'Plan' });
  store.getState().setComposerPlanStateForTarget(pendingTarget, {
    anchor: 1,
    beforeSpacer: null,
    afterSpacer: { from: 1, to: 2, text: '\n' },
  });
  store.getState().clearSelectedSkill();

  assert.deepEqual(store.getState().selectedConversationTarget, { kind: 'pending', id: pendingId });
  assert.equal(store.getState().selectedSkill, null);
  assert.equal(store.getState().getComposerPlanStateForTarget(pendingTarget), null);
});

test('switching away from the plan skill clears plan spacer drafts for every target', (t) => {
  const tabId = 'workspace-chat-plan-clear-all-targets';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  const firstPendingId = store.getState().createPendingConversation();
  const firstTarget = store.getState().selectedConversationTarget;
  store.getState().setDraftForSelectedTarget('hello\n\n');
  store.getState().setComposerPlanStateForTarget(firstTarget, {
    anchor: 6,
    beforeSpacer: { from: 5, to: 6, text: '\n' },
    afterSpacer: { from: 6, to: 7, text: '\n' },
  });

  const secondPendingId = store.getState().createPendingConversation();
  const secondTarget = store.getState().selectedConversationTarget;
  store.getState().setDraftForSelectedTarget('abc\n\n\ndef');
  store.getState().setComposerPlanStateForTarget(secondTarget, {
    anchor: 4,
    beforeSpacer: { from: 3, to: 4, text: '\n' },
    afterSpacer: { from: 4, to: 6, text: '\n\n' },
  });

  store.getState().setSelectedSkill({ id: 'skill-run', slug: 'run', name: 'Run' });

  assert.deepEqual(store.getState().selectedConversationTarget, { kind: 'pending', id: secondPendingId });
  assert.equal(store.getState().selectedSkill?.slug, 'run');
  assert.equal(store.getState().getComposerPlanStateForTarget({ kind: 'pending', id: firstPendingId }), null);
  assert.equal(store.getState().getComposerPlanStateForTarget({ kind: 'pending', id: secondPendingId }), null);
  assert.equal(store.getState().getDraftForTarget(firstTarget), 'hello');
  assert.equal(store.getState().getDraftForTarget(secondTarget), 'abcdef');
});

test('requestComposerFocus increments the one-shot focus sequence without changing selection', (t) => {
  const tabId = 'workspace-chat-composer-focus-request';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  const pendingId = store.getState().createPendingConversation();
  assert.equal(store.getState().composerFocusRequestSeq, 0);

  store.getState().requestComposerFocus();
  store.getState().requestComposerFocus();

  assert.equal(store.getState().composerFocusRequestSeq, 2);
  assert.deepEqual(store.getState().selectedConversationTarget, {
    kind: 'pending',
    id: pendingId,
  });
});

test('requestComposerBlockInsert queues markdown until the composer consumes it', (t) => {
  const tabId = 'workspace-chat-composer-block-insert-request';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);

  const firstID = store.getState().requestComposerBlockInsert('[file: one](/tmp/one.ts)');
  const secondID = store.getState().requestComposerBlockInsert('[file: two](/tmp/two.ts)');

  assert.ok(firstID);
  assert.ok(secondID);
  assert.deepEqual(
    store.getState().pendingComposerInsertQueue.map((item) => item.markdown),
    ['[file: one](/tmp/one.ts)', '[file: two](/tmp/two.ts)'],
  );

  store.getState().consumeComposerBlockInsert(firstID || '');

  assert.deepEqual(
    store.getState().pendingComposerInsertQueue.map((item) => item.markdown),
    ['[file: two](/tmp/two.ts)'],
  );
});

test('getConversationRunStatus prefers awaiting user over running progress', (t) => {
  const tabId = 'workspace-chat-run-status-running';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);
  const chatPath = '/tmp/workspace/.agent/chat/run.md';
  const target = threadTarget(chatPath, 'thread-run');
  store.getState().upsertThreadMeta({
    threadID: 'thread-run',
    fileID: 'file-run',
    agentID: 'agent-id',
    cwd: '/tmp/workspace',
    chatPath,
    title: 'Run',
  });

  store.getState().setAwaitingUser(chatPath, {
    requestID: 'req-running',
    questions: [{
      header: 'Q1',
      question: 'Need your input',
      options: [{ label: 'Accept' }],
      custom: false,
    }],
    currentIndex: 0,
    answers: [[]],
    customModeByIndex: [false],
    requestedAt: 1,
  });
  store.getState().setTargetInProgress(target, true);

  assert.equal(store.getState().getConversationRunStatus(target), 'awaiting_user');
});

test('getConversationRunStatus returns awaiting_user after user-request-like state is recorded', (t) => {
  const tabId = 'workspace-chat-run-status-awaiting';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);
  const chatPath = '/tmp/workspace/.agent/chat/awaiting.md';
  store.getState().upsertThreadMeta({
    threadID: 'thread-awaiting',
    fileID: 'file-awaiting',
    agentID: 'agent-id',
    cwd: '/tmp/workspace',
    chatPath,
    title: 'Awaiting',
  });

  store.getState().setAwaitingUser(chatPath, {
    requestID: 'req-awaiting',
    questions: [{
      header: 'Q1',
      question: 'Please answer a question',
      options: [],
      custom: true,
    }],
    currentIndex: 0,
    answers: [[]],
    customModeByIndex: [false],
    requestedAt: 2,
  });

  assert.equal(
    store.getState().getConversationRunStatus(threadTarget(chatPath, 'thread-awaiting')),
    'awaiting_user',
  );
});

test('getConversationRunStatus returns complete for completed idle thread', (t) => {
  const tabId = 'workspace-chat-run-status-complete';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);
  const chatPath = '/tmp/workspace/.agent/chat/complete.md';
  const target = threadTarget(chatPath, 'thread-complete');
  store.getState().upsertThreadMeta({
    threadID: 'thread-complete',
    fileID: 'file-complete',
    agentID: 'agent-id',
    cwd: '/tmp/workspace',
    chatPath,
    title: 'Complete',
  });

  store.getState().patchThreadSnapshotState(chatPath, {
    runStatus: 'idle',
    tailStatus: 'complete',
  });

  assert.equal(store.getState().getConversationRunStatus(target), 'complete');
});

test('getConversationRunStatus prefers running over stale complete state', (t) => {
  const tabId = 'workspace-chat-run-status-complete-running';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);
  const chatPath = '/tmp/workspace/.agent/chat/complete-running.md';
  const target = threadTarget(chatPath, 'thread-complete-running');
  store.getState().upsertThreadMeta({
    threadID: 'thread-complete-running',
    fileID: 'file-complete-running',
    agentID: 'agent-id',
    cwd: '/tmp/workspace',
    chatPath,
    title: 'Complete Running',
  });

  store.getState().patchThreadSnapshotState(chatPath, {
    runStatus: 'idle',
    tailStatus: 'complete',
  });
  store.getState().setTargetInProgress(target, true);

  assert.equal(store.getState().getConversationRunStatus(target), 'running');
});

test('retargetChatPath preserves awaiting user state on the new chat path', (t) => {
  const tabId = 'workspace-chat-awaiting-retarget';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);
  const oldPath = '/tmp/workspace/.agent/chat/old.md';
  const newPath = '/tmp/workspace/.agent/chat/new.md';
  store.getState().upsertThreadMeta({
    threadID: 'thread-retarget',
    fileID: 'file-retarget',
    agentID: 'agent-id',
    cwd: '/tmp/workspace',
    chatPath: oldPath,
    title: 'Old',
  });

  store.getState().setAwaitingUser(oldPath, {
    requestID: 'req-retarget',
    questions: [{
      header: 'Q1',
      question: 'Need confirmation',
      options: [{ label: 'Accept' }],
      custom: false,
    }],
    currentIndex: 0,
    answers: [[]],
    customModeByIndex: [false],
    requestedAt: 3,
  });
  store.getState().retargetChatPath(oldPath, newPath);

  assert.equal(
    store.getState().getConversationRunStatus(commandTarget(oldPath)),
    'idle',
  );
  assert.equal(
    store.getState().getConversationRunStatus(threadTarget(newPath, 'thread-retarget')),
    'awaiting_user',
  );
});

test('upsertThreadMeta migrates path-keyed chat state to thread-keyed state', (t) => {
  const tabId = 'workspace-chat-thread-key-migration';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createStore(tabId);
  const chatPath = '/tmp/workspace/.agent/chat/threaded.md';

  store.getState().syncQueuedMessages(chatPath, {
    steering: [{
      id: 'queue-1',
      kind: 'steering',
      text: 'queued',
      selectedSkill: null,
      selectedSkillIDs: [],
      selectedSkillContext: null,
      planTurn: false,
      chatPath,
    }],
    followUp: [],
  });
  store.getState().setAwaitingUser(chatPath, {
    requestID: 'req-1',
    questions: [{
      header: 'Q1',
      question: 'Continue?',
      options: [{ label: 'Yes', description: 'Continue' }],
      custom: false,
    }],
    currentIndex: 0,
    answers: [[]],
    customModeByIndex: [false],
    requestedAt: 1,
  });
  store.getState().appendStreamingText(chatPath, 'hello');
  store.getState().syncThreadSnapshot(threadSnapshot('thread-demo', chatPath, [{
    type: 'canonical_message',
    id: 'entry-1',
    role: 'assistant',
    content: [{ type: 'output_text', text: 'hello' }],
  }]));
  store.getState().setReviews(chatPath, [{
    threadID: 'thread-demo',
    turnID: 'turn-1',
    chatPath,
    status: 'pending',
    createdAt: '2026-04-16T00:00:00Z',
    canReview: true,
    canRollback: false,
    unresolved: 1,
    approvedCount: 0,
    rejectedCount: 0,
    rolledBackCount: 0,
    files: [],
  }]);
  store.getState().bumpPlanRevision(chatPath);
  store.getState().requestChatScrollToBottom(chatPath);

  store.getState().upsertThreadMeta({
    threadID: 'thread-demo',
    fileID: 'file-demo',
    agentID: 'agent-id',
    cwd: '/tmp/workspace',
    chatPath,
    title: 'Demo',
  });

  const state = store.getState() as any;
  assert.ok(state.queuedMessagesByThreadID['thread-demo']);
  assert.equal(state.queuedMessagesByThreadID[chatPath], undefined);
  assert.ok(state.awaitingUserByThreadID['thread-demo']);
  assert.equal(state.awaitingUserByThreadID[chatPath], undefined);
  assert.ok(state.threadSnapshotByID['thread-demo']);
  assert.equal(state.threadSnapshotByID[chatPath], undefined);
  assert.ok(state.liveOverlayByThreadID['thread-demo']);
  assert.equal(state.liveOverlayByThreadID[chatPath], undefined);
  assert.ok(state.reviewByThreadID['thread-demo']);
  assert.equal(state.reviewByThreadID[chatPath], undefined);
  assert.equal(state.planRevisionByThreadID['thread-demo'], 1);
  assert.equal(state.planRevisionByThreadID[chatPath], undefined);
  assert.equal(state.pendingScrollToBottomByThreadID['thread-demo'], true);
  assert.equal(state.pendingScrollToBottomByThreadID[chatPath], undefined);
});
