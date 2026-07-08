import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildClearContextChatFileName,
  clearContextExecutePlan,
  clearAwaitingUserForNewRun,
  disposeChatWorkspaceRuntime,
  handleSSEEvent,
  isLoginRequiredError,
  refreshThreadState,
  resolveAutoRetitleTitle,
  resolveSelectedSkillContext,
  submitChatTurn,
} from './chatService';
import { useAuthStore } from '../store/authStore';
import { getChatWorkspaceStore, removeChatWorkspaceStore } from '../store/chatWorkspaceStore';
import { getWorkspaceStore, removeWorkspaceStore } from '../store/appStore';
import { useTabManagerStore } from '../store/tabManagerStore';
import { useBillingReminderStore } from '../store/billingReminderStore';
import { useModelsStore } from '../store/modelsStore';

function createChatStore(tabId: string) {
  removeChatWorkspaceStore(tabId);
  return getChatWorkspaceStore(tabId);
}

function createWorkspaceStub() {
  return {
    documents: [] as Array<{ filePath: string; documentRole?: 'editor' | 'conversation' }>,
    openThreadTab: async () => null,
    moveChatTabToLast: () => {},
  };
}

type StubbedGlobalName = 'window' | 'document' | 'navigator';

function stubGlobal(name: StubbedGlobalName, value: unknown): () => void {
  const descriptor = Object.getOwnPropertyDescriptor(globalThis, name);
  Object.defineProperty(globalThis, name, {
    configurable: true,
    writable: true,
    value,
  });
  return () => {
    if (descriptor) {
      Object.defineProperty(globalThis, name, descriptor);
      return;
    }
    Reflect.deleteProperty(globalThis, name);
  };
}

function stubDomGlobals() {
  const restores = [
    stubGlobal('window', {
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => true,
      electronAPI: {
        onConfigSyncPush: () => () => {},
      },
    }),
    stubGlobal('document', {
      addEventListener: () => {},
      removeEventListener: () => {},
      visibilityState: 'visible',
      querySelector: () => null,
    }),
    stubGlobal('navigator', {
      onLine: true,
    }),
  ];

  return () => {
    for (const restore of restores.reverse()) {
      restore();
    }
  };
}

const PLAN_SKILL = {
  id: 'skill-plan',
  slug: 'plan',
  name: 'Plan',
} as const;
const CHAT_MODEL_KEY = 'cloud:gpt-5.4';

function setEnabledChatModelConfig() {
  useModelsStore.setState({
    config: {
      version: 5,
      defaultModelKey: null,
      providers: {},
      models: [{
        key: CHAT_MODEL_KEY,
        provider: 'cloud',
        providerLabel: 'Cloud',
        id: 'gpt-5.4',
        label: 'GPT-5.4',
        enabled: true,
        api: 'openai',
        updatedAt: 0,
      }],
      strategies: {
        auto: {
          defaultChatModelID: CHAT_MODEL_KEY,
        },
      },
      updatedAt: 0,
    } as any,
    loading: false,
    error: null,
  });
}

function restoreModelsState(state: ReturnType<typeof useModelsStore.getState>) {
  useModelsStore.setState({
    config: state.config,
    loading: state.loading,
    error: state.error,
  });
}

function threadTarget(chatPath: string, threadID = 'thread-test') {
  return { kind: 'thread' as const, threadID, chatPath };
}

test('isLoginRequiredError recognizes frontend and runtime auth failures', () => {
  assert.equal(isLoginRequiredError(new Error('Please sign in first.')), true);
  assert.equal(isLoginRequiredError(new Error('unauthorized: please sign in first')), true);
  assert.equal(isLoginRequiredError(new Error('Not connected to server')), false);
});

test('handleSSEEvent leaves a normal end in idle state', (t) => {
  const tabId = 'chat-service-end-idle';
  const chatPath = '/tmp/workspace/.agent/chat/idle.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createChatStore(tabId);

  handleSSEEvent(
    tabId,
    {
      meta: { type: 'end', chatPath },
      content: { text: '' } as any,
    },
    chatPath,
    {
      chatState: store.getState(),
      ws: createWorkspaceStub(),
    },
  );

  assert.equal(
    store.getState().getConversationRunStatus({ kind: 'command', path: chatPath }),
    'idle',
  );
});

test('handleSSEEvent reuses the running tool row when result id differs from streamed item id', (t) => {
  const tabId = 'chat-service-tool-result-reuses-running-step';
  const chatPath = '/tmp/workspace/.agent/chat/tool-result.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createChatStore(tabId);
  const ws = createWorkspaceStub();

  handleSSEEvent(
    tabId,
    {
      meta: { type: 'toolcall_start', chatPath, id: 'fc_1', name: 'read' },
      content: { text: '' } as any,
    },
    chatPath,
    { chatState: store.getState(), ws },
  );
  handleSSEEvent(
    tabId,
    {
      meta: { type: 'text_start', chatPath },
      content: { text: '' } as any,
    },
    chatPath,
    { chatState: store.getState(), ws },
  );
  handleSSEEvent(
    tabId,
    {
      meta: { type: 'text_delta', chatPath },
      content: { text: 'after tool' } as any,
    },
    chatPath,
    { chatState: store.getState(), ws },
  );
  handleSSEEvent(
    tabId,
    {
      meta: { type: 'tool_result_step', chatPath },
      content: {
        payload: {
          tool_call_id: 'call_1',
          name: 'read',
          content: 'ok',
        },
      } as any,
    },
    chatPath,
    { chatState: store.getState(), ws },
  );

  const activity = store.getState().getLiveOverlay(chatPath);
  assert.equal(activity.steps.length, 1);
  assert.equal(activity.steps[0].id, 'fc_1');
  assert.equal(activity.steps[0].status, 'done');
  assert.equal(activity.steps[0].toolOutput, 'ok');
  assert.deepEqual(activity.streamingSegments.map((segment) => segment.text), ['after tool']);
  assert.ok((activity.steps[0].order || 0) < activity.streamingSegments[0].order);
});

test('handleSSEEvent appends repeated thinking blocks instead of reusing the first index row', (t) => {
  const tabId = 'chat-service-thinking-repeated-index';
  const chatPath = '/tmp/workspace/.agent/chat/thinking-repeat.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createChatStore(tabId);
  const ws = createWorkspaceStub();

  const send = (type: string, text = '') => {
    handleSSEEvent(
      tabId,
      {
        meta: { type, chatPath, contentIndex: 0 },
        content: { text } as any,
      },
      chatPath,
      { chatState: store.getState(), ws },
    );
  };

  send('thinking_start');
  send('thinking_delta', 'first');
  send('thinking_end');
  send('text_start');
  send('text_delta', 'answer');
  send('thinking_start');
  send('thinking_delta', 'second');

  const activity = store.getState().getLiveOverlay(chatPath);
  const reasoningSteps = activity.steps.filter((step) => step.type === 'reasoning');
  assert.equal(reasoningSteps.length, 2);
  assert.equal(reasoningSteps[0].detail, 'first');
  assert.equal(reasoningSteps[0].status, 'done');
  assert.equal(reasoningSteps[1].detail, 'second');
  assert.equal(reasoningSteps[1].status, 'running');
  assert.ok((reasoningSteps[0].order || 0) < activity.streamingSegments[0].order);
  assert.ok(activity.streamingSegments[0].order < (reasoningSteps[1].order || 0));
});

test('handleSSEEvent clears active thinking when output moves to text', (t) => {
  const tabId = 'chat-service-thinking-clears-before-text';
  const chatPath = '/tmp/workspace/.agent/chat/thinking-clear.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createChatStore(tabId);
  const ws = createWorkspaceStub();

  const send = (type: string, text = '') => {
    handleSSEEvent(
      tabId,
      {
        meta: { type, chatPath, contentIndex: 0 },
        content: { text } as any,
      },
      chatPath,
      { chatState: store.getState(), ws },
    );
  };

  send('thinking_start');
  send('thinking_delta', 'first');
  send('text_start');
  send('text_delta', 'answer');
  send('thinking_delta', 'late-new-thinking');

  const activity = store.getState().getLiveOverlay(chatPath);
  const reasoningSteps = activity.steps.filter((step) => step.type === 'reasoning');
  assert.equal(reasoningSteps.length, 2);
  assert.equal(reasoningSteps[0].detail, 'first');
  assert.equal(reasoningSteps[0].status, 'done');
  assert.equal(reasoningSteps[1].detail, 'late-new-thinking');
  assert.equal(reasoningSteps[1].status, 'running');
  assert.ok((reasoningSteps[0].order || 0) < activity.streamingSegments[0].order);
  assert.ok(activity.streamingSegments[0].order < (reasoningSteps[1].order || 0));
});

test('handleSSEEvent keeps a thinking block active across interleaved tool lifecycle events', (t) => {
  const tabId = 'chat-service-thinking-spans-tool';
  const chatPath = '/tmp/workspace/.agent/chat/thinking-tool.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createChatStore(tabId);
  const ws = createWorkspaceStub();

  const send = (type: string, text = '', meta: Record<string, any> = {}) => {
    handleSSEEvent(
      tabId,
      {
        meta: { type, chatPath, contentIndex: 0, ...meta },
        content: { text } as any,
      },
      chatPath,
      { chatState: store.getState(), ws },
    );
  };

  send('thinking_start');
  send('thinking_delta', 'thinking draft');
  send('toolcall_start', '', { contentIndex: 1, id: 'fc_1', name: 'read' });
  send('toolcall_delta', '{"path":"/tmp/a"}', { contentIndex: 1, id: 'fc_1', name: 'read' });
  send('toolcall_end', '', { contentIndex: 1, id: 'fc_1', name: 'read' });
  send('thinking_end', 'thinking final');

  const activity = store.getState().getLiveOverlay(chatPath);
  const reasoningSteps = activity.steps.filter((step) => step.type === 'reasoning');
  const toolSteps = activity.steps.filter((step) => step.type === 'toolcall');
  assert.equal(reasoningSteps.length, 1);
  assert.equal(toolSteps.length, 1);
  assert.equal(reasoningSteps[0].detail, 'thinking final');
  assert.equal(reasoningSteps[0].status, 'done');
  assert.equal(toolSteps[0].id, 'toolcall-index-1');
  assert.equal(toolSteps[0].toolCall?.id, 'fc_1');
  assert.equal(toolSteps[0].detail, '{"path":"/tmp/a"}');
  assert.ok((reasoningSteps[0].order || 0) < (toolSteps[0].order || 0));
});

test('handleSSEEvent keeps one tool row when final tool id and arguments arrive at end', (t) => {
  const tabId = 'chat-service-tool-end-snapshot';
  const chatPath = '/tmp/workspace/.agent/chat/tool-end-snapshot.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createChatStore(tabId);
  const ws = createWorkspaceStub();

  const send = (type: string, payload: unknown, meta: Record<string, any> = {}) => {
    handleSSEEvent(
      tabId,
      {
        meta: { type, chatPath, contentIndex: 1, ...meta },
        content: { payload } as any,
      },
      chatPath,
      { chatState: store.getState(), ws },
    );
  };

  send('toolcall_start', { toolCall: { name: 'read' } });
  send('toolcall_end', {
    toolCall: {
      id: 'call_1',
      name: 'read',
      rawArguments: '{"path":"/tmp/docs/thread.md"}',
      arguments: { path: '/tmp/docs/thread.md' },
      complete: true,
    },
  });
  send('tool_result_step', {
    tool_call_id: 'call_1',
    name: 'read',
    content: 'Loaded /tmp/docs/thread.md',
  });

  const toolSteps = store.getState().getLiveOverlay(chatPath).steps.filter((step) => step.type === 'toolcall');
  assert.equal(toolSteps.length, 1);
  assert.equal(toolSteps[0].id, 'toolcall-index-1');
  assert.equal(toolSteps[0].label, 'read');
  assert.equal(toolSteps[0].status, 'done');
  assert.equal(toolSteps[0].detail, '{"path":"/tmp/docs/thread.md"}');
  assert.deepEqual(toolSteps[0].toolCall, {
    id: 'call_1',
    name: 'read',
    rawArguments: '{"path":"/tmp/docs/thread.md"}',
    arguments: { path: '/tmp/docs/thread.md' },
    complete: true,
  });
  assert.equal(toolSteps[0].toolOutput, 'Loaded /tmp/docs/thread.md');
});

test('handleSSEEvent finishes active thinking on context cancellation', (t) => {
  const tabId = 'chat-service-thinking-context-cancel';
  const chatPath = '/tmp/workspace/.agent/chat/thinking-cancel.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createChatStore(tabId);
  const ws = createWorkspaceStub();

  const send = (type: string, text = '') => {
    handleSSEEvent(
      tabId,
      {
        meta: { type, chatPath, contentIndex: 0 },
        content: { text } as any,
      },
      chatPath,
      { chatState: store.getState(), ws },
    );
  };

  send('thinking_start');
  send('thinking_delta', 'partial');
  send('error', 'context canceled');

  const activity = store.getState().getLiveOverlay(chatPath);
  const reasoningSteps = activity.steps.filter((step) => step.type === 'reasoning');
  assert.equal(reasoningSteps.length, 1);
  assert.equal(reasoningSteps[0].detail, 'partial');
  assert.equal(reasoningSteps[0].status, 'done');
  assert.equal(activity.errorMessage, 'Aborted');
});

test('handleSSEEvent end keeps persisted queued messages until snapshot refresh replaces them', (t) => {
  const tabId = 'chat-service-end-keeps-queue';
  const chatPath = '/tmp/workspace/.agent/chat/end-queue.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createChatStore(tabId);

  store.getState().syncQueuedMessages(chatPath, {
    steering: [{
      id: 'queue-1',
      kind: 'steering',
      text: 'queued steering',
      selectedSkill: null,
      selectedSkillIDs: [],
      selectedSkillContext: null,
      planTurn: false,
      chatPath,
    }],
    followUp: [],
  });

  handleSSEEvent(
    tabId,
    {
      meta: { type: 'end', chatPath },
      content: { text: '' } as any,
    },
    chatPath,
    {
      chatState: store.getState(),
      ws: createWorkspaceStub(),
    },
  );

  assert.deepEqual(store.getState().getQueuedMessages(chatPath), {
    steering: [{
      id: 'queue-1',
      kind: 'steering',
      text: 'queued steering',
      selectedSkill: null,
      selectedSkillIDs: [],
      selectedSkillContext: null,
      planTurn: false,
      chatPath,
    }],
    followUp: [],
  });
});

test('refreshThreadState syncs persisted queue, continuation state, and run status from snapshot', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const originalFetch = globalThis.fetch;
  const originalTabState = useTabManagerStore.getState();
  const tabId = 'chat-service-refresh-thread-state';
  const chatPath = '/tmp/workspace/.agent/chat/thread-state.md';

  t.after(() => {
    globalThis.fetch = originalFetch;
    useTabManagerStore.setState({
      tabs: originalTabState.tabs,
      activeTabId: originalTabState.activeTabId,
    });
    removeWorkspaceStore(tabId);
    removeChatWorkspaceStore(tabId);
  });

  useTabManagerStore.setState({
    tabs: [{
      id: tabId,
      label: 'Test',
      kind: 'local',
      workspaceId: 'workspace-test',
    }],
    activeTabId: tabId,
  });

  getWorkspaceStore(tabId);
  const chatStore = createChatStore(tabId);

  globalThis.fetch = (async (input) => {
    const url = String(input);
    if (url.includes('/v1/thread/snapshot')) {
      return new Response(JSON.stringify({
        meta: {
          threadID: 'thread-test',
          agentID: 'agent-id',
          cwd: '/tmp/workspace',
          chatPath,
          title: 'Test Chat',
        },
        entries: [],
        entryWindow: {
          mode: 'tail',
          limit: 400,
          start: 0,
          end: 0,
          total: 0,
          hasBefore: false,
          hasAfter: false,
        },
        revision: 'thread-test',
        runStatus: 'running',
        tailStatus: 'needs_continuation',
        continuationReason: 'assistant_tool_use',
        queuedMessages: {
          steering: [{
            id: 'queue-1',
            message: {
              role: 'user',
              content: 'queued steering',
            },
          }],
          followUp: [{
            id: 'queue-2',
            message: {
              role: 'user',
              content: 'queued follow up',
            },
          }],
        },
      }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    throw new Error(`unexpected fetch URL: ${url}`);
  }) as typeof globalThis.fetch;

  const snapshot = await refreshThreadState(chatPath);
  assert.equal(snapshot?.runStatus, 'running');
  assert.equal(chatStore.getState().isTargetInProgress(threadTarget(chatPath, 'thread-test')), true);

  const activity = chatStore.getState().getLiveOverlayForTarget(threadTarget(chatPath, 'thread-test'));
  assert.equal(activity.errorMessage, null);
  assert.deepEqual(chatStore.getState().getThreadState(chatPath), {
    runStatus: 'running',
    tailStatus: 'needs_continuation',
    continuationReason: 'assistant_tool_use',
  });

  assert.deepEqual(chatStore.getState().getQueuedMessages(chatPath), {
    steering: [{
      id: 'queue-1',
      kind: 'steering',
      text: 'queued steering',
      selectedSkill: null,
      selectedSkillIDs: [],
      selectedSkillContext: null,
      planTurn: false,
      chatPath,
    }],
    followUp: [{
      id: 'queue-2',
      kind: 'follow_up',
      text: 'queued follow up',
      selectedSkill: null,
      selectedSkillIDs: [],
      selectedSkillContext: null,
      planTurn: false,
      chatPath,
    }],
  });
});

test('queueSteering shows an optimistic queued message before control ack', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const originalFetch = globalThis.fetch;
  const originalAuthState = useAuthStore.getState();
  const originalTabState = useTabManagerStore.getState();
  const originalModelsState = useModelsStore.getState();
  const tabId = 'chat-service-optimistic-steering';
  const chatPath = '/tmp/workspace/.agent/chat/running.md';
  let releaseControlAck: (() => void) | null = null;
  let controlPayload: Record<string, unknown> | null = null;

  t.after(() => {
    globalThis.fetch = originalFetch;
    useAuthStore.setState(originalAuthState);
    useTabManagerStore.setState({
      tabs: originalTabState.tabs,
      activeTabId: originalTabState.activeTabId,
    });
    restoreModelsState(originalModelsState);
    removeWorkspaceStore(tabId);
    removeChatWorkspaceStore(tabId);
  });

  setEnabledChatModelConfig();
  useTabManagerStore.setState({
    tabs: [{
      id: tabId,
      label: 'Test',
      kind: 'local',
      workspaceId: 'workspace-test',
    }],
    activeTabId: tabId,
  });
  useAuthStore.setState({ ...originalAuthState, loggedIn: true });
  getWorkspaceStore(tabId);
  const store = createChatStore(tabId);
  store.getState().upsertThreadMeta({
    threadID: 'thread-running',
    fileID: 'file-running',
    agentID: 'agent-id',
    cwd: '/tmp/workspace',
    chatPath,
    title: 'Running',
  });
  store.getState().selectChatConversation(chatPath);
  store.getState().setSelectedModelKey(CHAT_MODEL_KEY);
  store.getState().setTargetInProgress(threadTarget(chatPath, 'thread-running'), true);
  store.getState().setDraftForSelectedTarget('interrupt now');

  globalThis.fetch = (async (input, init) => {
    const url = String(input);
    if (url.includes('/v1/thread/meta')) {
      return new Response(JSON.stringify({
        threadID: 'thread-running',
        fileID: 'file-running',
        agentID: 'agent-id',
        cwd: '/tmp/workspace',
        chatPath,
        path: chatPath,
        title: 'Running',
      }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    if (url.includes('/v1/chat/control')) {
      controlPayload = JSON.parse(String(init?.body || '{}')) as Record<string, unknown>;
      await new Promise<void>((resolve) => {
        releaseControlAck = resolve;
      });
      return new Response(JSON.stringify({
        ok: true,
        threadID: 'thread-running',
        queuedMessages: {
          steering: [{
            id: 'queue-real-1',
            message: {
              role: 'user',
              content: 'interrupt now',
            },
          }],
          followUp: [],
        },
      }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    throw new Error(`unexpected fetch URL: ${url}`);
  }) as typeof globalThis.fetch;

  const submit = submitChatTurn();
  await new Promise<void>((resolve) => setTimeout(resolve, 0));

  const optimistic = store.getState().getQueuedMessages(chatPath).steering[0];
  assert.equal(store.getState().getDraftForTarget(threadTarget(chatPath, 'thread-running')), '');
  assert.match(optimistic.id, /^optimistic-steering-/);
  assert.deepEqual(store.getState().getQueuedMessages(chatPath), {
    steering: [{
      chatPath,
      id: optimistic.id,
      kind: 'steering',
      text: 'interrupt now',
      agentID: 'agent-id',
      agentCwd: '/tmp/workspace',
      selectedSkill: null,
      selectedSkillIDs: [],
      selectedSkillContext: null,
      planTurn: false,
      pending: true,
    }],
    followUp: [],
  });

  const release = releaseControlAck as (() => void) | null;
  assert.ok(release);
  release();
  await submit;

  assert.ok(controlPayload);
  assert.deepEqual(store.getState().getQueuedMessages(chatPath), {
    steering: [{
      chatPath,
      id: 'queue-real-1',
      kind: 'steering',
      text: 'interrupt now',
      selectedSkill: null,
      selectedSkillIDs: [],
      selectedSkillContext: null,
      planTurn: false,
    }],
    followUp: [],
  });
});

test('clearAwaitingUserForNewRun clears waiting status before a new send cycle', (t) => {
  const tabId = 'chat-service-clear-awaiting';
  const chatPath = '/tmp/workspace/.agent/chat/clear.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createChatStore(tabId);

  store.getState().setAwaitingUser(chatPath, {
    requestID: 'req-clear',
    questions: [{
      header: 'Q1',
      question: 'Waiting for a reply',
      options: [{ label: 'Accept' }],
      custom: false,
    }],
    currentIndex: 0,
    answers: [[]],
    customModeByIndex: [false],
    requestedAt: 1,
  });
  clearAwaitingUserForNewRun(store.getState(), chatPath);

  assert.equal(
    store.getState().getConversationRunStatus({ kind: 'command', path: chatPath }),
    'idle',
  );
});

test('handleSSEEvent clears waiting state on context cancellation', (t) => {
  const tabId = 'chat-service-context-cancel-clears-awaiting';
  const chatPath = '/tmp/workspace/.agent/chat/cancel.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createChatStore(tabId);

  store.getState().setAwaitingUser(chatPath, {
    requestID: 'req-cancel',
    questions: [{
      header: 'Q1',
      question: 'Waiting for a reply',
      options: [{ label: 'Accept' }],
      custom: false,
    }],
    currentIndex: 0,
    answers: [[]],
    customModeByIndex: [false],
    requestedAt: 1,
  });
  store.getState().pushLiveStep(chatPath, {
    id: 'tool-shell',
    type: 'toolcall',
    label: 'shell',
    status: 'running',
    ts: 1,
  });

  handleSSEEvent(
    tabId,
    {
      meta: { type: 'error', chatPath },
      content: { text: 'context canceled' } as any,
    },
    chatPath,
    {
      chatState: store.getState(),
      ws: createWorkspaceStub(),
    },
  );

  assert.equal(store.getState().getAwaitingUser(chatPath), null);
  assert.equal(
    store.getState().getConversationRunStatus({ kind: 'command', path: chatPath }),
    'idle',
  );
  assert.equal(store.getState().getLiveOverlay(chatPath).steps[0]?.status, 'done');
  assert.equal(store.getState().getLiveOverlay(chatPath).errorMessage, 'Aborted');
});

test('refreshThreadState clears stale waiting state when snapshot is idle', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const originalFetch = globalThis.fetch;
  const tabId = 'chat-service-idle-snapshot-clears-awaiting';
  const chatPath = '/tmp/workspace/.agent/chat/idle-snapshot.md';

  t.after(() => {
    globalThis.fetch = originalFetch;
    removeWorkspaceStore(tabId);
    removeChatWorkspaceStore(tabId);
  });

  getWorkspaceStore(tabId);
  const store = createChatStore(tabId);
  store.getState().setAwaitingUser(chatPath, {
    requestID: 'req-stale',
    questions: [{
      header: 'Q1',
      question: 'Waiting for a reply',
      options: [{ label: 'Accept' }],
      custom: false,
    }],
    currentIndex: 0,
    answers: [[]],
    customModeByIndex: [false],
    requestedAt: 1,
  });

  globalThis.fetch = (async (input) => {
    const url = String(input);
    if (url.includes('/v1/thread/snapshot')) {
      return new Response(JSON.stringify({
        meta: {
          threadID: 'thread-idle',
          agentID: 'agent-id',
          cwd: '/tmp/workspace',
          chatPath,
          title: 'Idle Chat',
        },
        entries: [],
        entryWindow: {
          mode: 'tail',
          limit: 400,
          start: 0,
          end: 0,
          total: 0,
          hasBefore: false,
          hasAfter: false,
        },
        revision: 'thread-idle',
        runStatus: 'idle',
        tailStatus: 'empty',
        queuedMessages: {
          steering: [],
          followUp: [],
        },
      }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    throw new Error(`unexpected fetch URL: ${url}`);
  }) as typeof globalThis.fetch;

  await refreshThreadState(chatPath, tabId);

  assert.equal(store.getState().getAwaitingUser(chatPath), null);
  assert.equal(
    store.getState().getConversationRunStatus(threadTarget(chatPath, 'thread-idle')),
    'idle',
  );
});

test('disposeChatWorkspaceRuntime clears waiting and running state', (t) => {
  const tabId = 'chat-service-dispose-clears-awaiting';
  const chatPath = '/tmp/workspace/.agent/chat/dispose.md';
  t.after(() => removeChatWorkspaceStore(tabId));
  const store = createChatStore(tabId);

  store.getState().setChatPathInProgress(chatPath, true);
  store.getState().setAwaitingUser(chatPath, {
    requestID: 'req-dispose',
    questions: [{
      header: 'Q1',
      question: 'Waiting for a reply',
      options: [{ label: 'Accept' }],
      custom: false,
    }],
    currentIndex: 0,
    answers: [[]],
    customModeByIndex: [false],
    requestedAt: 1,
  });

  disposeChatWorkspaceRuntime(tabId);

  assert.equal(store.getState().getAwaitingUser(chatPath), null);
  assert.equal(
    store.getState().getConversationRunStatus({ kind: 'command', path: chatPath }),
    'idle',
  );
});

test('resolveSelectedSkillContext keeps new plan turns in planDir mode with title context', () => {
  assert.deepEqual(
    resolveSelectedSkillContext(PLAN_SKILL, {
      planDir: '/tmp/workspace/.agent/context',
      title: '现在是怎么把当前工作目录提交给模型的',
    }),
    {
      planDir: '/tmp/workspace/.agent/context',
      title: '现在是怎么把当前工作目录提交给模型的',
    },
  );
});

test('resolveAutoRetitleTitle uses new text for default-title auto paths', () => {
  assert.equal(
    resolveAutoRetitleTitle({
      chatPath: '/tmp/workspace/.agent/chat/untitled-chat.md',
      path: '/tmp/workspace/.agent/chat/untitled-chat.md',
      title: 'Untitled Chat',
    }, '帮我生成一个16:9的X文章封面图'),
    '帮我生成一个16:9的X文章封面图',
  );
});

test('resolveAutoRetitleTitle uses existing meaningful title for auto paths', () => {
  assert.equal(
    resolveAutoRetitleTitle({
      chatPath: '/tmp/workspace/.agent/chat/untitled-chat.md',
      path: '/tmp/workspace/.agent/chat/untitled-chat.md',
      title: '帮我生成一个16:9的X文章封面图',
    }, '不要用渐变色'),
    '帮我生成一个16:9的X文章封面图',
  );
});

test('resolveAutoRetitleTitle skips explicit chat paths', () => {
  assert.equal(
    resolveAutoRetitleTitle({
      chatPath: '/tmp/workspace/.agent/chat/custom-thread.md',
      path: '/tmp/workspace/.agent/chat/custom-thread.md',
      title: 'Untitled Chat',
    }, 'Refined Title'),
    '',
  );
});

test('buildClearContextChatFileName prefixes plan files once', () => {
  assert.equal(
    buildClearContextChatFileName('/tmp/workspace/.agent/context/website-hero-font-refresh.md'),
    'build-website-hero-font-refresh.md',
  );
  assert.equal(
    buildClearContextChatFileName('/tmp/workspace/.agent/context/build-user-mgbj6m0g.plan.md'),
    'build-user-mgbj6m0g.plan.md',
  );
});

test('clearContextExecutePlan sends workspace chatBaseDir and build-prefixed chatFileName', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const originalFetch = globalThis.fetch;
  const originalAuthState = useAuthStore.getState();
  const originalTabState = useTabManagerStore.getState();
  const originalModelsState = useModelsStore.getState();
  const tabId = 'chat-service-clear-context-execute-plan';
  const workspaceRoot = '/tmp/workspace';
  const sourceChatPath = `${workspaceRoot}/.agent/chat/source-thread.md`;
  const planPath = `${workspaceRoot}/.agent/context/website-hero-font-refresh.md`;
  const sourceContent = [
    '---',
    'thread: thread-parent',
    'title: "Source Thread"',
    '---',
    '',
    'body',
  ].join('\n');
  let forkPayload: Record<string, unknown> | null = null;

  t.after(() => {
    globalThis.fetch = originalFetch;
    useAuthStore.setState(originalAuthState);
    useTabManagerStore.setState({
      tabs: originalTabState.tabs,
      activeTabId: originalTabState.activeTabId,
    });
    restoreModelsState(originalModelsState);
    removeWorkspaceStore(tabId);
    removeChatWorkspaceStore(tabId);
  });

  setEnabledChatModelConfig();
  useTabManagerStore.setState({
    tabs: [{
      id: tabId,
      label: 'Test',
      kind: 'local',
      workspaceId: 'workspace-test',
    }],
    activeTabId: tabId,
  });
  useAuthStore.setState({ ...originalAuthState, loggedIn: false });

  const workspaceStore = getWorkspaceStore(tabId);
  createChatStore(tabId);
  workspaceStore.setState({
    currentDir: workspaceRoot,
    connectionState: 'connected',
    skillNodes: [{
      id: 'skill-execute-plan',
      kind: 'skill',
      cwd: '/tmp/skills/execute-plan',
      meta: {
        slug: 'execute-plan',
        name: 'Execute Plan',
      },
    }],
    tabs: [{
      id: 'source-tab',
      title: 'Source Thread',
      filePath: sourceChatPath,
      content: sourceContent,
      editorId: 'markdown',
      isDirty: false,
      pendingScrollHeading: null,
    }] as any,
    readTextFile: async (path: string) => (path === sourceChatPath ? sourceContent : null),
    reloadOpenTabsByPaths: async () => {},
    openThreadTab: () => {},
    resolveAgentByID: (agentID: string) => (
      agentID === 'external-agent'
        ? {
          name: 'External Agent',
          avatar: '',
          defaultModel: null,
        }
        : null
    ),
  } as any);

  globalThis.fetch = (async (input, init) => {
    const url = String(input);
    if (url.includes('/v1/thread/meta')) {
      return new Response(JSON.stringify({
        threadID: 'thread-parent',
        agentID: 'workspace-agent',
        cwd: workspaceRoot,
        chatPath: sourceChatPath,
        title: 'Source Thread',
      }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    if (url.includes('/v1/thread/fork')) {
      forkPayload = JSON.parse(String(init?.body || '{}')) as Record<string, unknown>;
      return new Response(JSON.stringify({
        threadID: 'thread-child',
        agentID: 'external-agent',
        cwd: '/tmp/agents/external',
        chatPath: `${workspaceRoot}/.agent/chat/build-website-hero-font-refresh.md`,
        title: 'Website Hero Font Refresh Build',
        executionPlanPath: planPath,
      }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    throw new Error(`unexpected fetch URL: ${url}`);
  }) as typeof globalThis.fetch;

  await clearContextExecutePlan({
    chatPath: sourceChatPath,
    planPath,
    planTitle: 'Website Hero Font Refresh',
    agentID: 'external-agent',
    agentName: 'External Agent',
    agentCwd: '/tmp/agents/external',
    modelKey: CHAT_MODEL_KEY,
    thinkingLevel: 'off',
  });

  assert.ok(forkPayload);
  const capturedForkPayload = forkPayload as Record<string, unknown>;
  assert.deepEqual(
    {
      chatBaseDir: capturedForkPayload.chatBaseDir,
      chatFileName: capturedForkPayload.chatFileName,
      cwd: capturedForkPayload.cwd,
    },
    {
      chatBaseDir: workspaceRoot,
      chatFileName: 'build-website-hero-font-refresh.md',
      cwd: '/tmp/agents/external',
    },
  );
});
