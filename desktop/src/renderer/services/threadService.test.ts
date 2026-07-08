import assert from 'node:assert/strict';
import test from 'node:test';

import {
  getThreadSnapshot,
  getThreadMeta,
  updateThreadMeta,
} from './threadService';
import { getChatWorkspaceStore, removeChatWorkspaceStore } from '../store/chatWorkspaceStore';
import { getWorkspaceStore, removeWorkspaceStore } from '../store/appStore';
import { useTabManagerStore } from '../store/tabManagerStore';

test('updateThreadMeta resolves threadID and fileID from chatPath before POST', async (t) => {
  const originalFetch = globalThis.fetch;
  const originalTabState = useTabManagerStore.getState();
  const tabId = 'chat-session-update-meta-resolve-ids';
  const chatPath = '/tmp/workspace/.agent/chat/demo.md';

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
    tabs: [{ id: tabId, title: 'Workspace', editorId: 'markdown' } as any],
    activeTabId: tabId,
  });

  const workspaceStore = getWorkspaceStore(tabId);
  workspaceStore.setState({
    remoteSession: null,
  } as any);

  const chatStore = getChatWorkspaceStore(tabId);
  chatStore.getState().upsertThreadMeta({
    threadID: 'thread-demo',
    fileID: 'file-demo',
    agentID: 'agent-id',
    cwd: '/tmp/workspace',
    chatPath,
    title: 'Demo',
  });

  const requests: Array<{ url: string; body: any }> = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const body = init?.body ? JSON.parse(String(init.body)) : null;
    requests.push({ url, body });
    return new Response(JSON.stringify({
      threadID: 'thread-demo',
      fileID: 'file-demo',
      agentID: 'agent-id',
      cwd: '/tmp/workspace',
      path: chatPath,
      chatPath,
      title: 'Demo',
    }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    });
  }) as unknown as typeof fetch;

  const meta = await updateThreadMeta({
    chatPath,
    title: 'Renamed Demo',
  }, tabId);

  assert.equal(requests.length, 1);
  assert.equal(requests[0]?.body?.threadID, 'thread-demo');
  assert.equal(requests[0]?.body?.fileID, 'file-demo');
  assert.equal(meta.threadID, 'thread-demo');
  assert.equal(meta.fileID, 'file-demo');
});

test('getThreadMeta normalizes missing runtime session errors', async (t) => {
  const originalFetch = globalThis.fetch;
  const originalTabState = useTabManagerStore.getState();
  const tabId = 'chat-session-missing-meta';

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
    tabs: [{ id: tabId, title: 'Workspace', editorId: 'markdown' } as any],
    activeTabId: tabId,
  });
  getWorkspaceStore(tabId).setState({
    remoteSession: null,
  } as any);

  globalThis.fetch = (async () => new Response(JSON.stringify({
    error: 'calling "node/operation": file does not exist',
  }), {
    status: 404,
    headers: { 'Content-Type': 'application/json' },
  })) as unknown as typeof fetch;

  await assert.rejects(
    () => getThreadMeta({ threadID: 'thread-missing' }, tabId),
    (error: unknown) => {
      assert.ok(error instanceof Error);
      assert.equal(error.message, 'Get thread meta failed: thread not found');
      assert.equal((error as { status?: number }).status, 404);
      return true;
    },
  );
});

test('getThreadSnapshot serializes entry window query through remote runtime base URL', async (t) => {
  const originalFetch = globalThis.fetch;
  const originalTabState = useTabManagerStore.getState();
  const tabId = 'chat-session-snapshot-window';
  const chatPath = '/tmp/workspace/.agent/chat/window.md';
  const requests: string[] = [];

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
    tabs: [{ id: tabId, title: 'Remote Workspace', editorId: 'markdown' } as any],
    activeTabId: tabId,
  });
  getWorkspaceStore(tabId).setState({
    remoteSession: { localPort: 25001 },
  } as any);

  globalThis.fetch = (async (input: RequestInfo | URL) => {
    requests.push(String(input));
    return new Response(JSON.stringify({
      meta: {
        threadID: 'thread-window',
        agentID: 'agent-id',
        cwd: '/tmp/workspace',
        path: chatPath,
        chatPath,
        title: 'Window',
      },
      entries: [],
      entryWindow: {
        mode: 'before',
        anchorId: 'entry-10',
        limit: 200,
        start: 0,
        end: 0,
        total: 0,
        hasBefore: false,
        hasAfter: false,
      },
      revision: 'thread-window',
    }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    });
  }) as unknown as typeof fetch;

  await getThreadSnapshot({
    chatPath,
    threadID: 'thread-window',
    entryWindow: {
      mode: 'before',
      anchorId: 'entry-10',
      limit: 200,
    },
  }, tabId);

  assert.equal(requests.length, 1);
  const url = new URL(requests[0] || '');
  assert.equal(url.origin, 'http://127.0.0.1:25001');
  assert.equal(url.pathname, '/v1/thread/snapshot');
  assert.equal(url.searchParams.get('threadID'), 'thread-window');
  assert.equal(url.searchParams.get('chatPath'), chatPath);
  assert.equal(url.searchParams.get('entryWindow'), 'before');
  assert.equal(url.searchParams.get('entryAnchorId'), 'entry-10');
  assert.equal(url.searchParams.get('entryLimit'), '200');
});
