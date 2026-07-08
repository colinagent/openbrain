import assert from 'node:assert/strict';
import test from 'node:test';

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

test('setActive(true) force-reconnects a disconnected workspace store', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-connection-recovery-disconnected';
  t.after(() => removeWorkspaceStore(tabId));

  const originalConnect = WSConnection.prototype.connect;
  const originalForceReconnect = WSConnection.prototype.forceReconnect;

  const connectCalls: string[] = [];
  const forceReconnectReasons: string[] = [];

  WSConnection.prototype.connect = function connect(url, callbacks) {
    connectCalls.push(url);
    callbacks.onStateChange?.('connecting');
  };
  WSConnection.prototype.forceReconnect = function forceReconnect(reason) {
    forceReconnectReasons.push(reason || '');
  };

  t.after(() => {
    WSConnection.prototype.connect = originalConnect;
    WSConnection.prototype.forceReconnect = originalForceReconnect;
  });

  const store = getWorkspaceStore(tabId);
  store.getState().setActive(true);

  assert.equal(connectCalls.length, 1);
  assert.deepEqual(forceReconnectReasons, ['workspace-activated']);
});

test('setActive(true) does not force-reconnect an already connected workspace store', async (t) => {
  const restoreGlobals = stubDomGlobals();
  t.after(restoreGlobals);

  const tabId = 'workspace-connection-recovery-connected';
  t.after(() => removeWorkspaceStore(tabId));

  const originalConnect = WSConnection.prototype.connect;
  const originalForceReconnect = WSConnection.prototype.forceReconnect;

  let connectCalls = 0;
  let forceReconnectCalls = 0;

  WSConnection.prototype.connect = function connect() {
    connectCalls += 1;
  };
  WSConnection.prototype.forceReconnect = function forceReconnect() {
    forceReconnectCalls += 1;
  };

  t.after(() => {
    WSConnection.prototype.connect = originalConnect;
    WSConnection.prototype.forceReconnect = originalForceReconnect;
  });

  const store = getWorkspaceStore(tabId);
  store.setState({ connectionState: 'connected' });
  store.getState().setActive(true);

  assert.equal(connectCalls, 0);
  assert.equal(forceReconnectCalls, 0);
});
