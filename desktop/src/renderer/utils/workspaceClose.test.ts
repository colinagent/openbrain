import assert from 'node:assert/strict';
import test from 'node:test';

import { getWorkspaceStore, removeWorkspaceStore } from '../store/appStore';
import { removeChatWorkspaceStore } from '../store/chatWorkspaceStore';
import { useTabManagerStore, type WorkspaceTab } from '../store/tabManagerStore';
import { WSConnection } from '../services/wsConnection';
import {
  closeWorkspaceTabWithDefaultFallback,
  resolveDefaultLocalWorkspacePath,
} from './workspaceClose';

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

function setWorkspaceTabs(tabs: WorkspaceTab[], activeTabId: string) {
  useTabManagerStore.setState((state) => ({
    ...state,
    tabs,
    activeTabId,
  }));
}

function snapshotTabManagerState() {
  const { tabs, activeTabId } = useTabManagerStore.getState();
  return {
    tabs: tabs.map((tab) => ({ ...tab })),
    activeTabId,
  };
}

function restoreTabManagerState(snapshot: ReturnType<typeof snapshotTabManagerState>) {
  useTabManagerStore.setState((state) => ({
    ...state,
    tabs: snapshot.tabs,
    activeTabId: snapshot.activeTabId,
  }));
}

test('resolveDefaultLocalWorkspacePath prefers the configured default dir', async () => {
  const result = await resolveDefaultLocalWorkspacePath({
    getDefaultDir: async () => '/Users/example/custom-workspace',
    getHomeDir: async () => '/Users/example',
  });

  assert.equal(result, '/Users/example/custom-workspace');
});

test('resolveDefaultLocalWorkspacePath falls back to the home workspace dir', async () => {
  const result = await resolveDefaultLocalWorkspacePath({
    getDefaultDir: async () => '',
    getHomeDir: async () => '/Users/example',
  });

  assert.equal(result, '/Users/example/.openbrain/workspace');
});

test('closeWorkspaceTabWithDefaultFallback replaces the last local workspace tab', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabManagerSnapshot = snapshotTabManagerState();
  t.after(() => restoreTabManagerState(tabManagerSnapshot));

  const originalConnect = WSConnection.prototype.connect;
  WSConnection.prototype.connect = function connect() {};
  t.after(() => {
    WSConnection.prototype.connect = originalConnect;
  });

  const closingTab: WorkspaceTab = {
    id: 'closing-local-tab',
    kind: 'local',
    label: 'project-a',
    workspaceId: 'project-a-id',
    workspacePath: '/tmp/project-a',
  };
  setWorkspaceTabs([closingTab], closingTab.id);
  t.after(() => {
    removeWorkspaceStore(closingTab.id);
    removeChatWorkspaceStore(closingTab.id);
  });

  await closeWorkspaceTabWithDefaultFallback(closingTab.id, {
    workspaceTabs: useTabManagerStore.getState().tabs,
    createWorkspaceTab: (init) => useTabManagerStore.getState().createTab(init),
    getWorkspaceStore,
    disconnectRemote: async () => {},
    setWorkspaceActive: () => {},
    disposeChatWorkspaceRuntime: () => {},
    removeWorkspaceStore,
    removeChatWorkspaceStore,
    closeWorkspaceTab: (tabId) => {
      useTabManagerStore.getState().closeTab(tabId);
    },
    resolveDefaultLocalWorkspacePath: async () => '/Users/example/.openbrain/workspace',
  });

  const { tabs, activeTabId } = useTabManagerStore.getState();
  assert.equal(tabs.length, 1);
  assert.equal(activeTabId, tabs[0]?.id);
  assert.equal(tabs[0]?.kind, 'local');
  assert.equal(tabs[0]?.workspacePath, '/Users/example/.openbrain/workspace');
  assert.equal(tabs[0]?.label, 'workspace');

  const replacementStore = getWorkspaceStore(activeTabId).getState();
  assert.equal(replacementStore.currentDir, '/Users/example/.openbrain/workspace');

  t.after(() => {
    removeWorkspaceStore(activeTabId);
    removeChatWorkspaceStore(activeTabId);
  });
});

test('closeWorkspaceTabWithDefaultFallback disconnects a remote tab before closing and restores the default local workspace', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabManagerSnapshot = snapshotTabManagerState();
  t.after(() => restoreTabManagerState(tabManagerSnapshot));

  const originalConnect = WSConnection.prototype.connect;
  WSConnection.prototype.connect = function connect() {};
  t.after(() => {
    WSConnection.prototype.connect = originalConnect;
  });

  const closingTab: WorkspaceTab = {
    id: 'closing-remote-tab',
    kind: 'remote',
    label: 'user@example.com',
    workspaceId: 'remote-id',
    remoteHost: {
      alias: 'prod',
      hostname: 'example.com',
      user: 'rune',
    },
  };
  setWorkspaceTabs([closingTab], closingTab.id);
  t.after(() => {
    removeWorkspaceStore(closingTab.id);
    removeChatWorkspaceStore(closingTab.id);
  });

  const events: string[] = [];

  await closeWorkspaceTabWithDefaultFallback(closingTab.id, {
    workspaceTabs: useTabManagerStore.getState().tabs,
    createWorkspaceTab: (init) => useTabManagerStore.getState().createTab(init),
    getWorkspaceStore,
    disconnectRemote: async (tabId) => {
      events.push(`disconnect:${tabId}`);
    },
    setWorkspaceActive: () => {},
    disposeChatWorkspaceRuntime: () => {},
    removeWorkspaceStore: (tabId) => {
      events.push(`remove-store:${tabId}`);
      removeWorkspaceStore(tabId);
    },
    removeChatWorkspaceStore: (tabId) => {
      events.push(`remove-chat-store:${tabId}`);
      removeChatWorkspaceStore(tabId);
    },
    closeWorkspaceTab: (tabId) => {
      events.push(`close:${tabId}`);
      useTabManagerStore.getState().closeTab(tabId);
    },
    resolveDefaultLocalWorkspacePath: async () => '/Users/example/.openbrain/workspace',
  });

  const { tabs, activeTabId } = useTabManagerStore.getState();
  assert.equal(tabs.length, 1);
  assert.equal(tabs[0]?.kind, 'local');
  assert.equal(tabs[0]?.workspacePath, '/Users/example/.openbrain/workspace');
  assert.equal(tabs[0]?.label, 'workspace');
  assert.equal(getWorkspaceStore(activeTabId).getState().currentDir, '/Users/example/.openbrain/workspace');

  assert.notEqual(events.indexOf('disconnect:closing-remote-tab'), -1);
  assert.notEqual(events.indexOf('close:closing-remote-tab'), -1);
  assert.ok(events.indexOf('disconnect:closing-remote-tab') < events.indexOf('close:closing-remote-tab'));

  t.after(() => {
    removeWorkspaceStore(activeTabId);
    removeChatWorkspaceStore(activeTabId);
  });
});
