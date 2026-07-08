import assert from 'node:assert/strict';
import test from 'node:test';

import {
  getChatWorkspaceStore,
  removeChatWorkspaceStore,
} from './chatWorkspaceStore';
import { getWorkspaceStore, removeWorkspaceStore } from './appStore';
import type { OpNode } from '../services/agentService';
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
        settings: {
          get: async () => ({
            editor: {
              openableExtensions: [],
              workbenchEditorAssociations: {},
            },
          }),
        },
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

function createGlobalAgentNode(cwd: string, id = 'agent-global') {
  return {
    id,
    uid: 'user-1',
    kind: 'agent',
    uri: 'file:///root/.openbrain/agents/global/.agent/AGENT.md',
    cwd,
    opCodes: ['thread/submit'],
    meta: { name: 'Global Agent' },
  };
}

function createWorkspaceAgentReaddirResult(path: string, workspaceDir: string) {
  if (path === workspaceDir) {
    return {
      path,
      entries: [{ name: '.agent', isDir: true, size: 0, modTime: 0 }],
    };
  }
  if (path === `${workspaceDir}/.agent`) {
    return {
      path,
      entries: [
        { name: 'AGENT.md', isDir: false, size: 24, modTime: 0 },
        { name: 'chat', isDir: true, size: 0, modTime: 0 },
      ],
    };
  }
  return { path, entries: [] };
}

function maybeFileWatchResponse(method: string, params?: unknown) {
  const payload = (params || {}) as Record<string, unknown>;
  if (method === 'fs/watch') {
    return { watchId: `watch:${String(payload.path || '')}` };
  }
  if (method === 'fs/unwatch') {
    return { success: true };
  }
  return null;
}

async function waitFor(predicate: () => boolean, timeoutMs = 1000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (predicate()) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 5));
  }
  assert.equal(predicate(), true);
}

test('loadDirectory keeps cached children on transient refresh timeout', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);
  const originalConsoleError = console.error;
  console.error = () => {};
  t.after(() => {
    console.error = originalConsoleError;
  });

  const tabId = 'workspace-file-tree-transient-timeout';
  t.after(() => removeWorkspaceStore(tabId));

  const workspaceDir = '/root/workspace';
  const cachedEntries = [
    { name: 'note.md', isDir: false, size: 24, modTime: 1 },
  ];

  const originalRequest = WSConnection.prototype.request;
  WSConnection.prototype.request = async function request(
    method: string,
  ) {
    if (method === 'fs/readdir') {
      throw new Error('Request timeout');
    }
    throw new Error(`Unexpected method: ${method}`);
  } as WSConnection['request'];
  t.after(() => {
    WSConnection.prototype.request = originalRequest;
  });

  const store = getWorkspaceStore(tabId);
  store.setState({
    connectionState: 'connected',
    currentDir: workspaceDir,
    entries: cachedEntries,
    dirEntries: new Map([[workspaceDir, cachedEntries]]),
    dirErrors: new Map([[workspaceDir, 'previous error']]),
  });

  await store.getState().loadDirectory(workspaceDir);

  assert.deepEqual(store.getState().dirEntries.get(workspaceDir), cachedEntries);
  assert.deepEqual(store.getState().entries, cachedEntries);
  assert.equal(store.getState().dirLoading.has(workspaceDir), false);
  assert.equal(store.getState().dirErrors.has(workspaceDir), false);
});

test('loadDirectory surfaces transient timeout when no directory snapshot exists', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);
  const originalConsoleError = console.error;
  console.error = () => {};
  t.after(() => {
    console.error = originalConsoleError;
  });

  const tabId = 'workspace-file-tree-initial-timeout';
  t.after(() => removeWorkspaceStore(tabId));

  const workspaceDir = '/root/workspace';

  const originalRequest = WSConnection.prototype.request;
  WSConnection.prototype.request = async function request(
    method: string,
  ) {
    if (method === 'fs/readdir') {
      throw new Error('Request timeout');
    }
    throw new Error(`Unexpected method: ${method}`);
  } as WSConnection['request'];
  t.after(() => {
    WSConnection.prototype.request = originalRequest;
  });

  const store = getWorkspaceStore(tabId);
  store.setState({
    connectionState: 'connected',
    currentDir: workspaceDir,
  });

  await store.getState().loadDirectory(workspaceDir);

  assert.deepEqual(store.getState().dirEntries.get(workspaceDir), []);
  assert.deepEqual(store.getState().entries, []);
  assert.equal(store.getState().dirLoading.has(workspaceDir), false);
  assert.equal(store.getState().dirErrors.get(workspaceDir), 'Request timeout');
});

test('setCurrentDir proactively refreshes root agent binding after node list refresh', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-agent-open-root-scan';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const workspaceDir = '/outside/project';
  let agentScanCount = 0;
  let resolveNodeList: (() => void) | null = null;
  const nodeListCanResolve = new Promise<void>((resolve) => {
    resolveNodeList = resolve;
  });

  const originalRequest = WSConnection.prototype.request;
  WSConnection.prototype.request = async function request(
    method: string,
    params?: unknown,
  ) {
    const payload = (params || {}) as Record<string, unknown>;
    const watchResponse = maybeFileWatchResponse(method, payload);
    if (watchResponse) {
      return watchResponse;
    }
    if (method === 'fs/readdir') {
      return createWorkspaceAgentReaddirResult(
        String(payload.path || ''),
        workspaceDir,
      );
    }
    if (method === 'node/list') {
      await nodeListCanResolve;
      return { nodes: [] };
    }
    if (method === 'agent/scan') {
      const dir = String(payload.dir || '');
      if (dir === workspaceDir) {
        agentScanCount += 1;
        resolveNodeList?.();
        return { nodes: [createGlobalAgentNode(workspaceDir)] };
      }
      return { nodes: [] };
    }
    if (method === 'git/branches') {
      return { isRepo: false };
    }
    throw new Error(`Unexpected method: ${method}`);
  } as WSConnection['request'];
  t.after(() => {
    WSConnection.prototype.request = originalRequest;
  });

  const store = getWorkspaceStore(tabId);
  store.setState({ connectionState: 'connected' });

  store.getState().setCurrentDir(workspaceDir);

  await waitFor(() => agentScanCount > 0);
  await waitFor(() => store.getState().agentNodesLoading === false);
  await waitFor(() =>
    Boolean(store.getState().getChatAgentForCwd(workspaceDir)),
  );
  assert.deepEqual(store.getState().getChatAgentForCwd(workspaceDir), {
    agentID: 'agent-global',
    agentName: 'Global Agent',
    agentCwd: workspaceDir,
  });
});

test('unmountAgentSubagent removes a direct subagent reference from the effective agent file', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-agent-unmount-direct-subagent';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const agentDir = '/root/.openbrain/agents/coder';
  const brainDir = '/root/.openbrain/agents/gbrain';
  const helperDir = `${agentDir}/.agent/subagents/helper`;
  let manifest = [
    '---',
    'id: agent-coder',
    'name: coder',
    'subagents:',
    '  - ./subagents/helper',
    '  - "@agent-gbrain"',
    '---',
    '',
    'Prompt',
    '',
  ].join('\n');
  const writeCalls: Array<{ path: string; content: string }> = [];

  const parentNode = () => ({
    id: 'agent-coder',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${agentDir}/.agent/AGENT.md`,
    cwd: agentDir,
    opCodes: ['thread/submit'],
    meta: {
      name: 'coder',
      subAgents: manifest.includes('@agent-gbrain')
        ? ['agent-helper', 'agent-gbrain']
        : ['agent-helper'],
    },
  });
  const helperNode = {
    id: 'agent-helper',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${helperDir}/.agent/AGENT.md`,
    cwd: helperDir,
    opCodes: ['thread/submit'],
    meta: { name: 'helper' },
  };
  const brainNode = {
    id: 'agent-gbrain',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${brainDir}/.agent/AGENT.md`,
    cwd: brainDir,
    opCodes: ['thread/submit'],
    meta: { name: 'GBrain' },
  };

  const originalRequest = WSConnection.prototype.request;
  WSConnection.prototype.request = async function request(
    method: string,
    params?: unknown,
  ) {
    const payload = (params || {}) as Record<string, unknown>;
    const watchResponse = maybeFileWatchResponse(method, payload);
    if (watchResponse) {
      return watchResponse;
    }
    if (method === 'node/list') {
      return { nodes: [parentNode(), helperNode, brainNode] };
    }
    if (method === 'fs/readFile') {
      assert.equal(String(payload.path || ''), `${agentDir}/.agent/AGENT.md`);
      return { content: manifest };
    }
    if (method === 'fs/writeFile') {
      writeCalls.push({
        path: String(payload.path || ''),
        content: String(payload.content || ''),
      });
      manifest = String(payload.content || '');
      return { path: payload.path, size: manifest.length };
    }
    if (method === 'fs/readdir') {
      return { path: payload.path, entries: [] };
    }
    if (method === 'agent/scan') {
      return { nodes: [parentNode()] };
    }
    throw new Error(`Unexpected method: ${method}`);
  } as WSConnection['request'];
  t.after(() => {
    WSConnection.prototype.request = originalRequest;
  });

  const store = getWorkspaceStore(tabId);
  store.setState({
    connectionState: 'connected',
    baseDir: '/root/.openbrain',
    nodesByID: new Map<string, OpNode>([
      ['agent-coder', parentNode()],
      ['agent-helper', helperNode],
      ['agent-gbrain', brainNode],
    ]),
    agentNodes: [parentNode(), helperNode, brainNode],
  });

  assert.deepEqual(
    store
      .getState()
      .getAgentSubagents('agent-coder')
      .map((item) => item.id),
    ['agent-helper', 'agent-gbrain'],
  );

  const removed = await store
    .getState()
    .unmountAgentSubagent('agent-coder', 'agent-gbrain');

  assert.equal(removed, true);
  assert.deepEqual(writeCalls, [
    {
      path: `${agentDir}/.agent/AGENT.md`,
      content: [
        '---',
        'id: agent-coder',
        'name: coder',
        'subagents:',
        '  - ./subagents/helper',
        '---',
        '',
        'Prompt',
        '',
      ].join('\n'),
    },
  ]);
});

test('unmountAgentSubagent edits the bind target agent instead of the workspace bind wrapper', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-agent-unmount-bind-target';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const workspaceDir = '/root/.openbrain/workspace/demo';
  const coderDir = '/root/.openbrain/agents/coder';
  const brainDir = '/root/.openbrain/agents/gbrain';
  let targetManifest = [
    '---',
    'id: agent-coder',
    'name: coder',
    'subagents:',
    '  - "@agent-gbrain"',
    '---',
    '',
  ].join('\n');
  const writeCalls: Array<{ path: string; content: string }> = [];

  const bindWrapperNode = {
    id: 'workspace-ref',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${workspaceDir}/.agent/AGENT.md`,
    cwd: workspaceDir,
    opCodes: ['thread/submit'],
    meta: { bind: 'agent-coder' },
  };
  const targetNode = () => ({
    id: 'agent-coder',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${coderDir}/.agent/AGENT.md`,
    cwd: coderDir,
    opCodes: ['thread/submit'],
    meta: {
      name: 'coder',
      subAgents: targetManifest.includes('@agent-gbrain')
        ? ['agent-gbrain']
        : [],
    },
  });
  const brainNode = {
    id: 'agent-gbrain',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${brainDir}/.agent/AGENT.md`,
    cwd: brainDir,
    opCodes: ['thread/submit'],
    meta: { name: 'GBrain' },
  };

  const originalRequest = WSConnection.prototype.request;
  WSConnection.prototype.request = async function request(
    method: string,
    params?: unknown,
  ) {
    const payload = (params || {}) as Record<string, unknown>;
    const watchResponse = maybeFileWatchResponse(method, payload);
    if (watchResponse) {
      return watchResponse;
    }
    if (method === 'node/list') {
      return { nodes: [bindWrapperNode, targetNode(), brainNode] };
    }
    if (method === 'fs/readFile') {
      assert.equal(String(payload.path || ''), `${coderDir}/.agent/AGENT.md`);
      return { content: targetManifest };
    }
    if (method === 'fs/writeFile') {
      writeCalls.push({
        path: String(payload.path || ''),
        content: String(payload.content || ''),
      });
      targetManifest = String(payload.content || '');
      return { path: payload.path, size: targetManifest.length };
    }
    if (method === 'fs/readdir') {
      return { path: payload.path, entries: [] };
    }
    if (method === 'agent/scan') {
      return { nodes: [targetNode()] };
    }
    throw new Error(`Unexpected method: ${method}`);
  } as WSConnection['request'];
  t.after(() => {
    WSConnection.prototype.request = originalRequest;
  });

  const store = getWorkspaceStore(tabId);
  store.setState({
    connectionState: 'connected',
    baseDir: '/root/.openbrain',
    currentDir: workspaceDir,
    nodesByID: new Map<string, OpNode>([
      ['workspace-ref', bindWrapperNode],
      ['agent-coder', targetNode()],
      ['agent-gbrain', brainNode],
    ]),
    agentNodes: [bindWrapperNode, targetNode(), brainNode],
    agentBindingByCwd: new Map([
      [
        workspaceDir,
        {
          cwd: workspaceDir,
          localNodeID: 'workspace-ref',
          effectiveAgentID: 'agent-coder',
          source: 'bind',
        },
      ],
    ]),
  });

  assert.deepEqual(
    store
      .getState()
      .getAgentSubagents('workspace-ref')
      .map((item) => item.id),
    ['agent-gbrain'],
  );

  const removed = await store
    .getState()
    .unmountAgentSubagent('workspace-ref', 'agent-gbrain');

  assert.equal(removed, true);
  assert.deepEqual(writeCalls, [
    {
      path: `${coderDir}/.agent/AGENT.md`,
      content: ['---', 'id: agent-coder', 'name: coder', '---', ''].join(
        '\n',
      ),
    },
  ]);
});

test('getMountableAgentSubagents lists GBrain and mountAgentSubagent appends it to the target agent', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-agent-mount-brain-subagent';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const coderDir = '/root/.openbrain/agents/coder';
  const brainDir = '/root/.openbrain/agents/gbrain';
  const helperDir = `${coderDir}/.agent/subagents/helper`;
  let manifest = [
    '---',
    'id: agent-coder',
    'name: coder',
    'subagents:',
    '  - ./subagents/helper',
    '---',
    '',
  ].join('\n');
  const writeCalls: Array<{ path: string; content: string }> = [];

  const parentNode = () => ({
    id: 'agent-coder',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${coderDir}/.agent/AGENT.md`,
    cwd: coderDir,
    opCodes: ['thread/submit'],
    meta: {
      name: 'coder',
      subAgents: manifest.includes('@agent-gbrain')
        ? ['agent-helper', 'agent-gbrain']
        : ['agent-helper'],
    },
  });
  const helperNode = {
    id: 'agent-helper',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${helperDir}/.agent/AGENT.md`,
    cwd: helperDir,
    opCodes: ['thread/submit'],
    meta: { name: 'helper' },
  };
  const brainNode = {
    id: 'agent-gbrain',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${brainDir}/.agent/AGENT.md`,
    cwd: brainDir,
    opCodes: ['thread/submit'],
    meta: { name: 'GBrain' },
  };

  const originalRequest = WSConnection.prototype.request;
  WSConnection.prototype.request = async function request(
    method: string,
    params?: unknown,
  ) {
    const payload = (params || {}) as Record<string, unknown>;
    const watchResponse = maybeFileWatchResponse(method, payload);
    if (watchResponse) {
      return watchResponse;
    }
    if (method === 'node/list') {
      return { nodes: [parentNode(), helperNode, brainNode] };
    }
    if (method === 'fs/readFile') {
      assert.equal(String(payload.path || ''), `${coderDir}/.agent/AGENT.md`);
      return { content: manifest };
    }
    if (method === 'fs/writeFile') {
      writeCalls.push({
        path: String(payload.path || ''),
        content: String(payload.content || ''),
      });
      manifest = String(payload.content || '');
      return { path: payload.path, size: manifest.length };
    }
    if (method === 'fs/readdir') {
      return { path: payload.path, entries: [] };
    }
    if (method === 'agent/scan') {
      return { nodes: [parentNode()] };
    }
    throw new Error(`Unexpected method: ${method}`);
  } as WSConnection['request'];
  t.after(() => {
    WSConnection.prototype.request = originalRequest;
  });

  const store = getWorkspaceStore(tabId);
  store.setState({
    connectionState: 'connected',
    baseDir: '/root/.openbrain',
    agentsRootDir: '/root/.openbrain/agents',
    nodesByID: new Map<string, OpNode>([
      ['agent-coder', parentNode()],
      ['agent-helper', helperNode],
      ['agent-gbrain', brainNode],
    ]),
    agentNodes: [parentNode(), helperNode, brainNode],
  });

  assert.deepEqual(
    store
      .getState()
      .getMountableAgentSubagents('agent-coder')
      .map(({ id, name }) => ({ id, name })),
    [{ id: 'agent-gbrain', name: 'GBrain' }],
  );

  const mounted = await store
    .getState()
    .mountAgentSubagent('agent-coder', 'agent-gbrain');

  assert.equal(mounted, true);
  assert.deepEqual(writeCalls, [
    {
      path: `${coderDir}/.agent/AGENT.md`,
      content: [
        '---',
        'id: agent-coder',
        'name: coder',
        'subagents:',
        '  - ./subagents/helper',
        '  - "@agent-gbrain"',
        '---',
        '',
      ].join('\n'),
    },
  ]);
});

test('getMountableAgentSubagents includes removed local subagents and remounts them by relative path', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-agent-remount-local-subagent';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const coderDir = '/root/.openbrain/agents/coder';
  const brainDir = '/root/.openbrain/agents/gbrain';
  const helperDir = `${coderDir}/.agent/subagents/helper`;
  let manifest = [
    '---',
    'id: agent-coder',
    'name: coder',
    'subagents:',
    '  - "@agent-gbrain"',
    '---',
    '',
  ].join('\n');
  const writeCalls: Array<{ path: string; content: string }> = [];

  const parentNode = () => ({
    id: 'agent-coder',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${coderDir}/.agent/AGENT.md`,
    cwd: coderDir,
    opCodes: ['thread/submit'],
    meta: {
      name: 'coder',
      subAgents: manifest.includes('./subagents/helper')
        ? ['agent-gbrain', 'agent-helper']
        : ['agent-gbrain'],
    },
  });
  const helperNode = {
    id: 'agent-helper',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${helperDir}/.agent/AGENT.md`,
    cwd: helperDir,
    opCodes: ['thread/submit'],
    meta: { name: 'helper' },
  };
  const brainNode = {
    id: 'agent-gbrain',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${brainDir}/.agent/AGENT.md`,
    cwd: brainDir,
    opCodes: ['thread/submit'],
    meta: { name: 'GBrain' },
  };

  const originalRequest = WSConnection.prototype.request;
  WSConnection.prototype.request = async function request(
    method: string,
    params?: unknown,
  ) {
    const payload = (params || {}) as Record<string, unknown>;
    const watchResponse = maybeFileWatchResponse(method, payload);
    if (watchResponse) {
      return watchResponse;
    }
    if (method === 'node/list') {
      return { nodes: [parentNode(), helperNode, brainNode] };
    }
    if (method === 'fs/readFile') {
      assert.equal(String(payload.path || ''), `${coderDir}/.agent/AGENT.md`);
      return { content: manifest };
    }
    if (method === 'fs/writeFile') {
      writeCalls.push({
        path: String(payload.path || ''),
        content: String(payload.content || ''),
      });
      manifest = String(payload.content || '');
      return { path: payload.path, size: manifest.length };
    }
    if (method === 'fs/readdir') {
      return { path: payload.path, entries: [] };
    }
    if (method === 'agent/scan') {
      return { nodes: [parentNode()] };
    }
    throw new Error(`Unexpected method: ${method}`);
  } as WSConnection['request'];
  t.after(() => {
    WSConnection.prototype.request = originalRequest;
  });

  const store = getWorkspaceStore(tabId);
  store.setState({
    connectionState: 'connected',
    baseDir: '/root/.openbrain',
    agentsRootDir: '/root/.openbrain/agents',
    nodesByID: new Map([
      ['agent-coder', parentNode()],
      ['agent-helper', helperNode],
      ['agent-gbrain', brainNode],
    ]),
    agentNodes: [parentNode(), helperNode, brainNode],
  });

  assert.deepEqual(
    store
      .getState()
      .getMountableAgentSubagents('agent-coder')
      .map(({ id, name }) => ({ id, name })),
    [{ id: 'agent-helper', name: 'helper' }],
  );

  const mounted = await store
    .getState()
    .mountAgentSubagent('agent-coder', 'agent-helper');

  assert.equal(mounted, true);
  assert.deepEqual(writeCalls, [
    {
      path: `${coderDir}/.agent/AGENT.md`,
      content: [
        '---',
        'id: agent-coder',
        'name: coder',
        'subagents:',
        '  - "@agent-gbrain"',
        '  - ./subagents/helper',
        '---',
        '',
      ].join('\n'),
    },
  ]);
});

test('getMountableAgentSubagents infers the agents root from product agent paths', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-agent-mountable-infer-root';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const coderDir = '/root/.openbrain/agents/coder';
  const brainDir = '/root/.openbrain/agents/gbrain';
  const helperDir = `${coderDir}/.agent/subagents/helper`;
  const parentNode = {
    id: 'agent-coder',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${coderDir}/.agent/AGENT.md`,
    cwd: coderDir,
    opCodes: ['thread/submit'],
    meta: {
      name: 'coder',
      subAgents: ['agent-helper'],
    },
  };
  const helperNode = {
    id: 'agent-helper',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${helperDir}/.agent/AGENT.md`,
    cwd: helperDir,
    opCodes: ['thread/submit'],
    meta: { name: 'helper' },
  };
  const brainNode = {
    id: 'agent-gbrain',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${brainDir}/.agent/AGENT.md`,
    cwd: brainDir,
    opCodes: ['thread/submit'],
    meta: { name: 'GBrain' },
  };

  const store = getWorkspaceStore(tabId);
  store.setState({
    connectionState: 'connected',
    baseDir: null,
    agentsRootDir: null,
    nodesByID: new Map([
      ['agent-coder', parentNode],
      ['agent-helper', helperNode],
      ['agent-gbrain', brainNode],
    ]),
    agentNodes: [parentNode, helperNode, brainNode],
  });

  assert.deepEqual(
    store
      .getState()
      .getMountableAgentSubagents('agent-coder')
      .map((item) => item.id),
    ['agent-gbrain'],
  );
});

test('mountAgentSubagent edits the bind target agent instead of the workspace bind wrapper', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-agent-mount-bind-target';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const workspaceDir = '/root/.openbrain/workspace/demo';
  const coderDir = '/root/.openbrain/agents/coder';
  const brainDir = '/root/.openbrain/agents/gbrain';
  let targetManifest = [
    '---',
    'id: agent-coder',
    'name: coder',
    '---',
    '',
  ].join('\n');
  const writeCalls: Array<{ path: string; content: string }> = [];

  const bindWrapperNode = {
    id: 'workspace-ref',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${workspaceDir}/.agent/AGENT.md`,
    cwd: workspaceDir,
    opCodes: ['thread/submit'],
    meta: { bind: 'agent-coder' },
  };
  const targetNode = () => ({
    id: 'agent-coder',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${coderDir}/.agent/AGENT.md`,
    cwd: coderDir,
    opCodes: ['thread/submit'],
    meta: {
      name: 'coder',
      subAgents: targetManifest.includes('@agent-gbrain')
        ? ['agent-gbrain']
        : [],
    },
  });
  const brainNode = {
    id: 'agent-gbrain',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${brainDir}/.agent/AGENT.md`,
    cwd: brainDir,
    opCodes: ['thread/submit'],
    meta: { name: 'GBrain' },
  };

  const originalRequest = WSConnection.prototype.request;
  WSConnection.prototype.request = async function request(
    method: string,
    params?: unknown,
  ) {
    const payload = (params || {}) as Record<string, unknown>;
    const watchResponse = maybeFileWatchResponse(method, payload);
    if (watchResponse) {
      return watchResponse;
    }
    if (method === 'node/list') {
      return { nodes: [bindWrapperNode, targetNode(), brainNode] };
    }
    if (method === 'fs/readFile') {
      assert.equal(String(payload.path || ''), `${coderDir}/.agent/AGENT.md`);
      return { content: targetManifest };
    }
    if (method === 'fs/writeFile') {
      writeCalls.push({
        path: String(payload.path || ''),
        content: String(payload.content || ''),
      });
      targetManifest = String(payload.content || '');
      return { path: payload.path, size: targetManifest.length };
    }
    if (method === 'fs/readdir') {
      return { path: payload.path, entries: [] };
    }
    if (method === 'agent/scan') {
      return { nodes: [targetNode()] };
    }
    throw new Error(`Unexpected method: ${method}`);
  } as WSConnection['request'];
  t.after(() => {
    WSConnection.prototype.request = originalRequest;
  });

  const store = getWorkspaceStore(tabId);
  store.setState({
    connectionState: 'connected',
    baseDir: '/root/.openbrain',
    agentsRootDir: '/root/.openbrain/agents',
    currentDir: workspaceDir,
    nodesByID: new Map<string, OpNode>([
      ['workspace-ref', bindWrapperNode],
      ['agent-coder', targetNode()],
      ['agent-gbrain', brainNode],
    ]),
    agentNodes: [bindWrapperNode, targetNode(), brainNode],
    agentBindingByCwd: new Map([
      [
        workspaceDir,
        {
          cwd: workspaceDir,
          localNodeID: 'workspace-ref',
          effectiveAgentID: 'agent-coder',
          source: 'bind',
        },
      ],
    ]),
  });

  assert.deepEqual(
    store
      .getState()
      .getMountableAgentSubagents('workspace-ref')
      .map((item) => item.id),
    ['agent-gbrain'],
  );

  assert.deepEqual(
    store
      .getState()
      .getMountableAgentSubagents('agent-coder')
      .map((item) => item.id),
    ['agent-gbrain'],
  );

  const mounted = await store
    .getState()
    .mountAgentSubagent('workspace-ref', 'agent-gbrain');

  assert.equal(mounted, true);
  assert.deepEqual(writeCalls, [
    {
      path: `${coderDir}/.agent/AGENT.md`,
      content: [
        '---',
        'id: agent-coder',
        'name: coder',
        'subagents:',
        '  - "@agent-gbrain"',
        '---',
        '',
      ].join('\n'),
    },
  ]);
});

test('unmountAgentSubagent leaves inline subagent arrays untouched', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-agent-unmount-inline-subagent';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const agentDir = '/root/.openbrain/agents/coder';
  const brainDir = '/root/.openbrain/agents/gbrain';
  const manifest = [
    '---',
    'id: agent-coder',
    'name: coder',
    'subagents: ["@agent-gbrain"]',
    '---',
    '',
  ].join('\n');
  const writeCalls: Array<{ path: string; content: string }> = [];

  const parentNode = {
    id: 'agent-coder',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${agentDir}/.agent/AGENT.md`,
    cwd: agentDir,
    opCodes: ['thread/submit'],
    meta: { name: 'coder', subAgents: ['agent-gbrain'] },
  };
  const brainNode = {
    id: 'agent-gbrain',
    uid: 'user-1',
    kind: 'agent',
    uri: `file://${brainDir}/.agent/AGENT.md`,
    cwd: brainDir,
    opCodes: ['thread/submit'],
    meta: { name: 'GBrain' },
  };

  const originalRequest = WSConnection.prototype.request;
  WSConnection.prototype.request = async function request(
    method: string,
    params?: unknown,
  ) {
    const payload = (params || {}) as Record<string, unknown>;
    const watchResponse = maybeFileWatchResponse(method, payload);
    if (watchResponse) {
      return watchResponse;
    }
    if (method === 'node/list') {
      return { nodes: [parentNode, brainNode] };
    }
    if (method === 'fs/readFile') {
      assert.equal(String(payload.path || ''), `${agentDir}/.agent/AGENT.md`);
      return { content: manifest };
    }
    if (method === 'fs/writeFile') {
      writeCalls.push({
        path: String(payload.path || ''),
        content: String(payload.content || ''),
      });
      return { path: payload.path, size: String(payload.content || '').length };
    }
    throw new Error(`Unexpected method: ${method}`);
  } as WSConnection['request'];
  t.after(() => {
    WSConnection.prototype.request = originalRequest;
  });

  const store = getWorkspaceStore(tabId);
  store.setState({
    connectionState: 'connected',
    baseDir: '/root/.openbrain',
    nodesByID: new Map([
      ['agent-coder', parentNode],
      ['agent-gbrain', brainNode],
    ]),
    agentNodes: [parentNode, brainNode],
  });

  const removed = await store
    .getState()
    .unmountAgentSubagent('agent-coder', 'agent-gbrain');

  assert.equal(removed, false);
  assert.deepEqual(writeCalls, []);
});

test('switchAgentReference hard-cuts a custom agent into a bind reference without opening files or chat', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-agent-switch-hard-cut';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const workspaceDir = '/root/.openbrain/workspace/demo';
  const writeCalls: Array<{ path: string; content: string }> = [];

  const originalRequest = WSConnection.prototype.request;
  WSConnection.prototype.request = async function request(
    method: string,
    params?: unknown,
  ) {
    const payload = (params || {}) as Record<string, unknown>;
    const watchResponse = maybeFileWatchResponse(method, payload);
    if (watchResponse) {
      return watchResponse;
    }
    if (method === 'fs/mkdir') {
      return { success: true };
    }
    if (method === 'fs/writeFile') {
      writeCalls.push({
        path: String(payload.path || ''),
        content: String(payload.content || ''),
      });
      return { path: payload.path, size: String(payload.content || '').length };
    }
    if (method === 'fs/readdir') {
      const path = String(payload.path || '');
      return createWorkspaceAgentReaddirResult(path, workspaceDir);
    }
    if (method === 'node/list') {
      return {
        nodes: [createGlobalAgentNode('/root/.openbrain/agents/global')],
      };
    }
    if (method === 'agent/scan') {
      return {
        nodes: [
          {
            id: 'workspace-ref',
            uid: 'user-1',
            kind: 'agent',
            uri: `file://${workspaceDir}/.agent/AGENT.md`,
            cwd: workspaceDir,
            meta: { bind: 'agent-global' },
          },
        ],
      };
    }
    throw new Error(`Unexpected method: ${method}`);
  } as WSConnection['request'];
  t.after(() => {
    WSConnection.prototype.request = originalRequest;
  });

  const store = getWorkspaceStore(tabId);
  const chatStore = getChatWorkspaceStore(tabId);
  store.setState({
    connectionState: 'connected',
    currentDir: workspaceDir,
    workspaceRootDir: '/root/.openbrain/workspace',
    agentsRootDir: '/root/.openbrain/agents',
    agentBindingByCwd: new Map([
      [
        workspaceDir,
        {
          cwd: workspaceDir,
          localNodeID: 'custom-agent',
          effectiveAgentID: 'custom-agent',
          source: 'local',
        },
      ],
    ]),
  });

  const switched = await store
    .getState()
    .switchAgentReference(workspaceDir, 'agent-global');

  assert.equal(switched, true);
  assert.deepEqual(writeCalls, [
    {
      path: `${workspaceDir}/.agent/AGENT.md`,
      content: '---\nbind: @agent-global\n---\n',
    },
  ]);
  assert.deepEqual(store.getState().agentBindingByCwd.get(workspaceDir), {
    cwd: workspaceDir,
    localNodeID: 'workspace-ref',
    effectiveAgentID: 'agent-global',
    source: 'bind',
  });
  assert.deepEqual(store.getState().getChatAgentForCwd(workspaceDir), {
    agentID: 'agent-global',
    agentName: 'Global Agent',
    agentCwd: workspaceDir,
  });
  assert.equal(store.getState().currentFilePath, null);
  assert.equal(chatStore.getState().agentID, null);
  assert.equal(chatStore.getState().agentCwd, null);
});

test('addAgentReference refreshes stale empty agent scan cache before resolving the new binding', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-agent-add-refreshes-cache';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const workspaceDir = '/root/.openbrain/workspace/add-demo';
  const writeCalls: Array<{ path: string; content: string }> = [];
  const agentScanCalls: string[] = [];
  let scanPhase: 'empty' | 'fresh' = 'empty';

  const originalRequest = WSConnection.prototype.request;
  WSConnection.prototype.request = async function request(
    method: string,
    params?: unknown,
  ) {
    const payload = (params || {}) as Record<string, unknown>;
    const watchResponse = maybeFileWatchResponse(method, payload);
    if (watchResponse) {
      return watchResponse;
    }
    if (method === 'fs/mkdir') {
      return { success: true };
    }
    if (method === 'fs/writeFile') {
      writeCalls.push({
        path: String(payload.path || ''),
        content: String(payload.content || ''),
      });
      scanPhase = 'fresh';
      return { path: payload.path, size: String(payload.content || '').length };
    }
    if (method === 'fs/readdir') {
      const path = String(payload.path || '');
      if (path === workspaceDir) {
        return {
          path,
          entries: [{ name: '.agent', isDir: true, size: 0, modTime: 0 }],
        };
      }
      if (path === `${workspaceDir}/.agent`) {
        return {
          path,
          entries: [
            { name: 'AGENT.md', isDir: false, size: 24, modTime: 0 },
            { name: 'chat', isDir: true, size: 0, modTime: 0 },
          ],
        };
      }
      return { path, entries: [] };
    }
    if (method === 'node/list') {
      return {
        nodes: [
          {
            id: 'agent-global',
            uid: 'user-1',
            kind: 'agent',
            uri: 'file:///root/.openbrain/agents/global/.agent/AGENT.md',
            cwd: '/root/.openbrain/agents/global',
            opCodes: ['thread/submit'],
            meta: { name: 'Global Agent' },
          },
        ],
      };
    }
    if (method === 'agent/scan') {
      const dir = String(payload.dir || '');
      agentScanCalls.push(dir);
      if (dir === workspaceDir && scanPhase === 'fresh') {
        return {
          nodes: [createGlobalAgentNode(workspaceDir)],
        };
      }
      return { nodes: [] };
    }
    if (method === 'fs/readFile') {
      return { content: '---\nbind: @agent-global\n---\n' };
    }
    throw new Error(`Unexpected method: ${method}`);
  } as WSConnection['request'];
  t.after(() => {
    WSConnection.prototype.request = originalRequest;
  });

  const store = getWorkspaceStore(tabId);
  store.setState({
    connectionState: 'connected',
    currentDir: workspaceDir,
    workspaceRootDir: '/root/.openbrain/workspace',
    agentsRootDir: '/root/.openbrain/agents',
    agentBindingByCwd: new Map(),
  });

  await store.getState().fetchDirAgentsInfo(workspaceDir);
  assert.equal(store.getState().getChatAgentForCwd(workspaceDir), null);

  await store.getState().addAgentReference(workspaceDir, 'agent-global');

  assert.deepEqual(writeCalls, [
    {
      path: `${workspaceDir}/.agent/AGENT.md`,
      content: '---\nbind: @agent-global\n---\n',
    },
  ]);
  assert.ok(agentScanCalls.filter((dir) => dir === workspaceDir).length >= 2);
  assert.deepEqual(store.getState().getChatAgentForCwd(workspaceDir), {
    agentID: 'agent-global',
    agentName: 'Global Agent',
    agentCwd: workspaceDir,
  });
});

test('fetchDirAgentsInfo bumps nodeGraphRevision when workspace scan updates nodesByID', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-agent-scan-revision';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const workspaceDir = '/root/.openbrain/workspace/revision-demo';
  const originalRequest = WSConnection.prototype.request;
  WSConnection.prototype.request = async function request(
    method: string,
    params?: unknown,
  ) {
    const payload = (params || {}) as Record<string, unknown>;
    const watchResponse = maybeFileWatchResponse(method, payload);
    if (watchResponse) {
      return watchResponse;
    }
    if (method === 'fs/readdir') {
      return createWorkspaceAgentReaddirResult(
        String(payload.path || ''),
        workspaceDir,
      );
    }
    if (method === 'agent/scan') {
      const dir = String(payload.dir || '');
      if (dir === workspaceDir) {
        return {
          nodes: [
            {
              id: 'workspace-agent',
              uid: 'user-1',
              kind: 'agent',
              uri: `file://${workspaceDir}/.agent/AGENT.md`,
              cwd: workspaceDir,
              opCodes: ['thread/submit'],
              meta: { name: 'Workspace Agent' },
            },
          ],
        };
      }
      return { nodes: [] };
    }
    throw new Error(`Unexpected method: ${method}`);
  } as WSConnection['request'];
  t.after(() => {
    WSConnection.prototype.request = originalRequest;
  });

  const store = getWorkspaceStore(tabId);
  store.setState({
    connectionState: 'connected',
    currentDir: workspaceDir,
    workspaceRootDir: '/root/.openbrain/workspace',
    dirEntries: new Map([[workspaceDir, []]]),
    nodesByID: new Map(),
    agentBindingByCwd: new Map(),
    nodeGraphRevision: 0,
  });

  await store.getState().fetchDirAgentsInfo(workspaceDir);

  assert.equal(store.getState().nodeGraphRevision, 1);
  assert.equal(store.getState().nodesByID.has('workspace-agent'), true);
});

test('default openbrain fallback can chat from an unbound workspace cwd', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-agent-default-openbrain-fallback';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const workspaceDir = '/root/projects/personal-notes';
  const openbrainDir = '/root/.openbrain/agents/openbrain';
  const store = getWorkspaceStore(tabId);
  store.setState({
    connectionState: 'connected',
    currentDir: workspaceDir,
    workspaceRootDir: '/root/.openbrain/workspace',
    agentsRootDir: '/root/.openbrain/agents',
    nodesByID: new Map([
      [
        'agent-openbrain',
        createGlobalAgentNode(openbrainDir, 'agent-openbrain'),
      ],
    ]),
    agentNodes: [createGlobalAgentNode(openbrainDir, 'agent-openbrain')],
    agentBindingByCwd: new Map(),
  });

  assert.equal(store.getState().getChatAgentForCwd(workspaceDir), null);
  assert.deepEqual(store.getState().getDefaultOpenBrainForCwd(workspaceDir), {
    agentID: 'agent-openbrain',
    agentName: 'Global Agent',
    agentCwd: workspaceDir,
  });
  assert.equal(
    store.getState().getAgentOpCode('agent-openbrain'),
    'thread/submit',
  );
});

test('addAgentReference rewrites stale selected IDs to the current agent node ID', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-agent-add-stale-id';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const workspaceDir = '/root/.openbrain/workspace/stale-id-demo';
  const agentDir = '/root/.openbrain/agents/global';
  const writeCalls: Array<{ path: string; content: string }> = [];

  const originalRequest = WSConnection.prototype.request;
  WSConnection.prototype.request = async function request(
    method: string,
    params?: unknown,
  ) {
    const payload = (params || {}) as Record<string, unknown>;
    const watchResponse = maybeFileWatchResponse(method, payload);
    if (watchResponse) {
      return watchResponse;
    }
    if (method === 'fs/mkdir') {
      return { success: true };
    }
    if (method === 'fs/writeFile') {
      writeCalls.push({
        path: String(payload.path || ''),
        content: String(payload.content || ''),
      });
      return { path: payload.path, size: String(payload.content || '').length };
    }
    if (method === 'fs/readdir') {
      return createWorkspaceAgentReaddirResult(
        String(payload.path || ''),
        workspaceDir,
      );
    }
    if (method === 'node/list') {
      return { nodes: [createGlobalAgentNode(agentDir)] };
    }
    if (method === 'agent/scan') {
      const dir = String(payload.dir || '');
      if (dir === workspaceDir) {
        return { nodes: [createGlobalAgentNode(workspaceDir)] };
      }
      return { nodes: [] };
    }
    if (method === 'fs/readFile') {
      return { content: '---\nbind: @agent-global\n---\n' };
    }
    throw new Error(`Unexpected method: ${method}`);
  } as WSConnection['request'];
  t.after(() => {
    WSConnection.prototype.request = originalRequest;
  });

  const store = getWorkspaceStore(tabId);
  store.setState({
    connectionState: 'connected',
    currentDir: workspaceDir,
    workspaceRootDir: '/root/.openbrain/workspace',
    agentsRootDir: '/root/.openbrain/agents',
    nodesByID: new Map([
      ['agent-stale', createGlobalAgentNode(agentDir, 'agent-stale')],
    ]),
    agentNodes: [createGlobalAgentNode(agentDir, 'agent-stale')],
    agentBindingByCwd: new Map(),
  });

  await store.getState().addAgentReference(workspaceDir, 'agent-stale');

  assert.deepEqual(writeCalls, [
    {
      path: `${workspaceDir}/.agent/AGENT.md`,
      content: '---\nbind: @agent-global\n---\n',
    },
  ]);
  assert.deepEqual(store.getState().getChatAgentForCwd(workspaceDir), {
    agentID: 'agent-global',
    agentName: 'Global Agent',
    agentCwd: workspaceDir,
  });
});

test('stale in-flight agent scan does not overwrite a refreshed binding', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-agent-stale-inflight';
  t.after(() => removeWorkspaceStore(tabId));
  t.after(() => removeChatWorkspaceStore(tabId));

  const workspaceDir = '/root/.openbrain/workspace/inflight-demo';
  let scanPhase: 'empty' | 'fresh' = 'empty';
  let workspaceScanCount = 0;
  let resolveFirstScanStarted: (() => void) | null = null;
  const staleScan = { resolve: null as (() => void) | null };
  const firstScanStarted = new Promise<void>((resolve) => {
    resolveFirstScanStarted = resolve;
  });

  const originalRequest = WSConnection.prototype.request;
  WSConnection.prototype.request = async function request(
    method: string,
    params?: unknown,
  ) {
    const payload = (params || {}) as Record<string, unknown>;
    const watchResponse = maybeFileWatchResponse(method, payload);
    if (watchResponse) {
      return watchResponse;
    }
    if (method === 'fs/mkdir') {
      return { success: true };
    }
    if (method === 'fs/writeFile') {
      scanPhase = 'fresh';
      return { path: payload.path, size: String(payload.content || '').length };
    }
    if (method === 'fs/readdir') {
      const path = String(payload.path || '');
      return createWorkspaceAgentReaddirResult(path, workspaceDir);
    }
    if (method === 'node/list') {
      return {
        nodes: [createGlobalAgentNode('/root/.openbrain/agents/global')],
      };
    }
    if (method === 'agent/scan') {
      const dir = String(payload.dir || '');
      if (dir === workspaceDir) {
        workspaceScanCount += 1;
        if (workspaceScanCount === 1) {
          resolveFirstScanStarted?.();
          return new Promise<Record<string, unknown>>((resolve) => {
            staleScan.resolve = () => resolve({ nodes: [] });
          });
        }
        if (scanPhase === 'fresh') {
          return {
            nodes: [createGlobalAgentNode(workspaceDir)],
          };
        }
      }
      return { nodes: [] };
    }
    if (method === 'fs/readFile') {
      return { content: '---\nbind: @agent-global\n---\n' };
    }
    throw new Error(`Unexpected method: ${method}`);
  } as WSConnection['request'];
  t.after(() => {
    WSConnection.prototype.request = originalRequest;
  });

  const store = getWorkspaceStore(tabId);
  store.setState({
    connectionState: 'connected',
    currentDir: workspaceDir,
    workspaceRootDir: '/root/.openbrain/workspace',
    agentsRootDir: '/root/.openbrain/agents',
    agentBindingByCwd: new Map(),
  });

  const staleFetch = store.getState().fetchDirAgentsInfo(workspaceDir);
  await firstScanStarted;
  await store.getState().addAgentReference(workspaceDir, 'agent-global');

  assert.deepEqual(store.getState().getChatAgentForCwd(workspaceDir), {
    agentID: 'agent-global',
    agentName: 'Global Agent',
    agentCwd: workspaceDir,
  });

  staleScan.resolve?.();
  await staleFetch;
  await new Promise((resolve) => setTimeout(resolve, 0));

  assert.deepEqual(store.getState().getChatAgentForCwd(workspaceDir), {
    agentID: 'agent-global',
    agentName: 'Global Agent',
    agentCwd: workspaceDir,
  });
});

test('refreshVisibleWorkspaceTree reloads visible dirs and refreshes cached agent info', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-visible-refresh';
  t.after(() => removeWorkspaceStore(tabId));

  const workspaceDir = '/root/.openbrain/workspace/refresh-demo';
  const expandedDir = `${workspaceDir}/src`;
  const readdirCalls: string[] = [];
  const agentScanCalls: string[] = [];
  const nodeListPayloads: Array<Record<string, unknown>> = [];
  let scanPhase: 'stale' | 'fresh' = 'stale';

  const originalRequest = WSConnection.prototype.request;
  WSConnection.prototype.request = async function request(
    method: string,
    params?: unknown,
  ) {
    const payload = (params || {}) as Record<string, unknown>;
    const watchResponse = maybeFileWatchResponse(method, payload);
    if (watchResponse) {
      return watchResponse;
    }
    if (method === 'fs/readdir') {
      const path = String(payload.path || '');
      readdirCalls.push(path);
      if (path === workspaceDir) {
        return {
          path,
          entries: [
            { name: '.agent', isDir: true, size: 0, modTime: 0 },
            { name: 'src', isDir: true, size: 0, modTime: 0 },
          ],
        };
      }
      if (path === expandedDir) {
        return {
          path,
          entries: [],
        };
      }
      return { path, entries: [] };
    }
    if (method === 'node/list') {
      nodeListPayloads.push(payload);
      return { nodes: [] };
    }
    if (method === 'agent/scan') {
      const dir = String(payload.dir || '');
      agentScanCalls.push(dir);
      if (dir === workspaceDir) {
        const stale = scanPhase === 'stale';
        return {
          nodes: [
            {
              id: stale ? 'workspace-agent-stale' : 'workspace-agent-fresh',
              uid: 'user-1',
              kind: 'agent',
              uri: `file://${workspaceDir}/.agent/AGENT.md`,
              cwd: workspaceDir,
              opCodes: ['thread/submit'],
              meta: {
                name: stale ? 'Workspace Agent Stale' : 'Workspace Agent Fresh',
              },
            },
          ],
        };
      }
      if (dir === expandedDir) {
        return {
          nodes: [
            {
              id: 'nested-agent',
              uid: 'user-1',
              kind: 'agent',
              uri: `file://${expandedDir}/.agent/AGENT.md`,
              cwd: expandedDir,
              opCodes: ['thread/submit'],
              meta: { name: 'Nested Agent' },
            },
          ],
        };
      }
      return { nodes: [] };
    }
    throw new Error(`Unexpected method: ${method}`);
  } as WSConnection['request'];
  t.after(() => {
    WSConnection.prototype.request = originalRequest;
  });

  const store = getWorkspaceStore(tabId);
  store.setState({
    connectionState: 'connected',
    currentDir: workspaceDir,
    workspaceRootDir: '/root/.openbrain/workspace',
    expandedDirs: new Set([expandedDir]),
    nodesByID: new Map(),
    agentBindingByCwd: new Map(),
  });

  await store.getState().fetchDirAgentsInfo(workspaceDir);
  assert.deepEqual(store.getState().getChatAgentForCwd(workspaceDir), {
    agentID: 'workspace-agent-stale',
    agentName: 'Workspace Agent Stale',
    agentCwd: workspaceDir,
  });

  readdirCalls.length = 0;
  agentScanCalls.length = 0;
  nodeListPayloads.length = 0;
  scanPhase = 'fresh';

  await store.getState().refreshVisibleWorkspaceTree();

  assert.deepEqual(readdirCalls.sort(), [expandedDir, workspaceDir].sort());
  assert.deepEqual(agentScanCalls.sort(), [expandedDir, workspaceDir].sort());
  assert.deepEqual(nodeListPayloads, [{ refresh: true }]);
  assert.deepEqual(store.getState().getChatAgentForCwd(workspaceDir), {
    agentID: 'workspace-agent-fresh',
    agentName: 'Workspace Agent Fresh',
    agentCwd: workspaceDir,
  });
  assert.deepEqual(store.getState().getChatAgentForCwd(expandedDir), {
    agentID: 'nested-agent',
    agentName: 'Nested Agent',
    agentCwd: expandedDir,
  });
});
