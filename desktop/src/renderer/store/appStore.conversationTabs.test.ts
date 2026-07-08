import assert from 'node:assert/strict';
import test from 'node:test';

import { getChatWorkspaceStore, removeChatWorkspaceStore } from './chatWorkspaceStore';
import { getWorkspaceStore, removeWorkspaceStore } from './appStore';
import { WSConnection } from '../services/wsConnection';

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

function buildChatContent(threadID = 'thread-a', title = 'Chat', body = '# Chat') {
  return [
    '---',
    `thread: ${threadID}`,
    `title: ${title}`,
    '---',
    '',
    body,
  ].join('\n');
}

function findDocumentByPath(store: ReturnType<typeof getWorkspaceStore>, filePath: string) {
  return store.getState().documents.find((doc) => doc.filePath === filePath) || null;
}

function stubWorkspaceRequests(t: any, chatPath: string, content = buildChatContent()) {
  const originalRequest = WSConnection.prototype.request;
  WSConnection.prototype.request = (async function request(method: string, params?: unknown) {
    if (method === 'fs/readFile') {
      return {
        path: (params as { path?: string } | undefined)?.path || chatPath,
        content,
      };
    }
    if (method === 'fs/watch') {
      return { watchId: 'watch-chat' };
    }
    throw new Error(`Unexpected method: ${method}`);
  }) as WSConnection['request'];
  t.after(() => {
    WSConnection.prototype.request = originalRequest;
  });
}

test('openThreadTab reads the chat backing document without activating the primary editor', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-conversation-open-unpinned';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const store = getWorkspaceStore(tabId);
  const chatPath = '/tmp/workspace/.agent/chat/a.md';
  stubWorkspaceRequests(t, chatPath, buildChatContent('thread-a', 'a.md', '# Existing chat'));

  store.getState().openUntitledTab();
  const docTabId = store.getState().activeTabId;
  await store.getState().openThreadTab(chatPath, 'a.md');

  assert.equal(store.getState().activeTabId, docTabId);
  assert.equal(store.getState().currentFilePath, null);
  assert.equal(findDocumentByPath(store, chatPath)?.documentRole, 'conversation');
  assert.match(findDocumentByPath(store, chatPath)?.content || '', /# Existing chat/);
});

test('openThreadTab can open a conversation from provided content without reading the backing file', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-conversation-open-inline-content';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const store = getWorkspaceStore(tabId);
  const chatPath = '/tmp/workspace/.agent/chat/inline.md';

  const originalRequest = WSConnection.prototype.request;
  WSConnection.prototype.request = (async function request(method: string) {
    if (method === 'fs/watch') {
      return { watchId: 'watch-inline-chat' };
    }
    throw new Error(`Unexpected method: ${method}`);
  }) as WSConnection['request'];
  t.after(() => {
    WSConnection.prototype.request = originalRequest;
  });

  store.getState().openUntitledTab();
  const docTabId = store.getState().activeTabId;
  await store.getState().openThreadTab(chatPath, 'inline.md', buildChatContent('thread-inline', 'inline.md', '# Inline chat'));

  assert.equal(store.getState().activeTabId, docTabId);
  assert.equal(store.getState().currentFilePath, null);
  assert.match(findDocumentByPath(store, chatPath)?.content || '', /# Inline chat/);
});

test('retargetActiveBlankTab paints created chat initial content into the active editor', (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-conversation-retarget-created-content';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const store = getWorkspaceStore(tabId);
  const chatPath = '/tmp/workspace/.agent/chat/created.md';
  const initialContent = buildChatContent('thread-created', 'created.md', '# Created chat');

  store.getState().openUntitledTab();
  const retargeted = store.getState().retargetActiveBlankTab(chatPath, 'created.md', initialContent);
  const state = store.getState();
  const doc = findDocumentByPath(store, chatPath);

  assert.equal(retargeted, true);
  assert.equal(state.currentFilePath, chatPath);
  assert.equal(state.fileContent, initialContent);
  assert.equal(doc?.documentRole, 'conversation');
  assert.equal(doc?.content, initialContent);
});

test('openThreadTab reads backing content and keeps the primary editor active while a conversation is pinned', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-conversation-open-pinned';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const store = getWorkspaceStore(tabId);
  const chatPath = '/tmp/workspace/.agent/chat/a.md';
  const pinnedChatPath = '/tmp/workspace/.agent/chat/pinned.md';
  stubWorkspaceRequests(t, chatPath, buildChatContent('thread-a', 'a.md', '# Pinned chat'));

  store.getState().openUntitledTab();
  const docTabId = store.getState().activeTabId;
  store.getState().ensureThreadTab(pinnedChatPath, 'pinned.md', buildChatContent('thread-pinned', 'pinned.md', '# Existing pinned chat'));
  const pinnedTabId = findDocumentByPath(store, pinnedChatPath)?.id || null;
  assert.ok(pinnedTabId);
  store.getState().setPinnedTab(pinnedTabId);
  await store.getState().openThreadTab(chatPath, 'a.md');

  assert.equal(store.getState().activeTabId, docTabId);
  assert.equal(store.getState().currentFilePath, null);
  assert.equal(store.getState().pinnedTabId, findDocumentByPath(store, chatPath)?.id);
  assert.match(findDocumentByPath(store, chatPath)?.content || '', /# Pinned chat/);
});

test('restoreChatTabsSession restores backing tabs without stealing primary editor focus', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-conversation-restore-no-focus';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const store = getWorkspaceStore(tabId);
  const chatPath = '/tmp/workspace/.agent/chat/a.md';
  stubWorkspaceRequests(t, chatPath, buildChatContent('thread-a', 'a.md'));

  store.setState({ connectionState: 'connected' });
  store.setState({ currentDir: '/tmp/workspace' });
  store.getState().openUntitledTab();
  const docTabId = store.getState().activeTabId;
  const result = await store.getState().restoreChatTabsSession([
    { threadID: 'thread-a', path: chatPath, title: 'a.md' },
  ], chatPath);

  assert.deepEqual(result, {
    restoredPaths: [chatPath],
    selectedPath: chatPath,
  });
  assert.equal(store.getState().activeTabId, docTabId);
  assert.equal(store.getState().currentFilePath, null);
  assert.equal(findDocumentByPath(store, chatPath)?.documentRole, 'conversation');
});

test('pinning a regular document never falls back to a conversation backing tab in the primary editor', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-pin-regular-doc-no-chat-fallback';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const store = getWorkspaceStore(tabId);
  store.setState({ currentDir: '/tmp/workspace' });
  store.getState().openUntitledTab();
  const articleTabId = store.getState().activeTabId;
  store.getState().ensureThreadTab('/tmp/workspace/.agent/chat/a.md', 'a.md', buildChatContent('thread-a', 'a.md', '# chat'));
  const articlePath = '/tmp/workspace/article.md';
  await store.getState().openFile(articlePath);
  const articleOpenTabId = store.getState().activeTabId;

  assert.ok(articleTabId);
  assert.ok(articleOpenTabId);
  store.getState().togglePinnedTab(articleOpenTabId!);

  assert.equal(store.getState().pinnedTabId, articleOpenTabId);
  assert.equal(store.getState().activeTabId, articleTabId);
  assert.equal(store.getState().currentFilePath, null);
});
