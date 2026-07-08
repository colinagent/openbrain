import assert from 'node:assert/strict';
import test from 'node:test';

import { WSConnection, type ConnectionState } from './wsConnection';

type StubbedGlobalName = 'window' | 'document' | 'navigator' | 'WebSocket';
type EventListener = () => void;

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

function createEventTargetStub() {
  const listeners = new Map<string, Set<EventListener>>();
  return {
    addEventListener(type: string, listener: EventListener) {
      const bucket = listeners.get(type) || new Set<EventListener>();
      bucket.add(listener);
      listeners.set(type, bucket);
    },
    removeEventListener(type: string, listener: EventListener) {
      listeners.get(type)?.delete(listener);
    },
    dispatch(type: string) {
      for (const listener of listeners.get(type) || []) {
        listener();
      }
    },
  };
}

function stubDomGlobals() {
  const windowTarget = createEventTargetStub();
  const documentTarget = createEventTargetStub();
  const windowStub = {
    ...windowTarget,
    electronAPI: {},
  };
  const documentStub = {
    ...documentTarget,
    visibilityState: 'hidden' as 'hidden' | 'visible',
  };

  const restores = [
    stubGlobal('window', windowStub),
    stubGlobal('document', documentStub),
    stubGlobal('navigator', { onLine: true }),
  ];

  return {
    windowStub,
    documentStub,
    restore() {
      for (const restore of restores.reverse()) {
        restore();
      }
    },
  };
}

class MockWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  static instances: MockWebSocket[] = [];

  readonly url: string;
  readyState = MockWebSocket.CONNECTING;
  closeCalls = 0;
  onopen: ((event: Event) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
  }

  close() {
    this.closeCalls += 1;
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.({ code: 1000, reason: 'closed' } as CloseEvent);
  }

  open() {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.({} as Event);
  }
}

function stubWebSocketGlobal() {
  MockWebSocket.instances = [];
  return stubGlobal('WebSocket', MockWebSocket as unknown as typeof WebSocket);
}

function configureFastRetries(connection: WSConnection, connectTimeoutMs = 10, reconnectDelayMs = 1) {
  Object.defineProperty(connection, 'connectTimeoutMs', {
    configurable: true,
    value: connectTimeoutMs,
  });
  Object.defineProperty(connection, 'minReconnectDelay', {
    configurable: true,
    value: reconnectDelayMs,
  });
  Object.defineProperty(connection, 'maxReconnectDelay', {
    configurable: true,
    value: reconnectDelayMs,
  });
  Object.defineProperty(connection, 'jitterFactor', {
    configurable: true,
    value: 0,
  });
  (connection as unknown as { reconnectDelay: number }).reconnectDelay = reconnectDelayMs;
}

function wait(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

test('forceReconnect discards a stalled CONNECTING socket immediately', async (t) => {
  const dom = stubDomGlobals();
  const restoreWebSocket = stubWebSocketGlobal();

  const connection = new WSConnection();
  t.after(() => {
    connection.dispose();
    restoreWebSocket();
    dom.restore();
  });

  const states: ConnectionState[] = [];
  let disconnectCount = 0;
  connection.connect('ws://example.test/ws', {
    onStateChange: (state) => states.push(state),
    onDisconnect: () => {
      disconnectCount += 1;
    },
  });

  assert.equal(MockWebSocket.instances.length, 1);
  const firstSocket = MockWebSocket.instances[0];
  assert.equal(firstSocket.readyState, MockWebSocket.CONNECTING);

  connection.forceReconnect('test');

  assert.equal(firstSocket.closeCalls, 1);
  assert.equal(MockWebSocket.instances.length, 2);
  assert.equal(disconnectCount, 1);
  assert.equal(states.at(-1), 'reconnecting');

  MockWebSocket.instances[1].open();
  assert.equal(states.at(-1), 'connected');
});

test('connect timeout closes a stalled socket and schedules a fresh attempt', async (t) => {
  const dom = stubDomGlobals();
  const restoreWebSocket = stubWebSocketGlobal();

  const connection = new WSConnection();
  configureFastRetries(connection, 10, 1);
  t.after(() => {
    connection.dispose();
    restoreWebSocket();
    dom.restore();
  });

  const states: ConnectionState[] = [];
  connection.connect('ws://example.test/ws', {
    onStateChange: (state) => states.push(state),
  });

  const firstSocket = MockWebSocket.instances[0];
  await wait(30);

  assert.equal(firstSocket.closeCalls, 1);
  assert.ok(MockWebSocket.instances.length >= 2);
  assert.ok(states.includes('reconnecting'));
});

test('visibilitychange forces reconnect instead of reusing a stale CONNECTING socket', async (t) => {
  const dom = stubDomGlobals();
  const restoreWebSocket = stubWebSocketGlobal();

  const connection = new WSConnection();
  t.after(() => {
    connection.dispose();
    restoreWebSocket();
    dom.restore();
  });

  connection.connect('ws://example.test/ws', {});
  const firstSocket = MockWebSocket.instances[0];
  dom.documentStub.visibilityState = 'visible';
  dom.documentStub.dispatch('visibilitychange');

  assert.equal(firstSocket.closeCalls, 1);
  assert.equal(MockWebSocket.instances.length, 2);
});

test('successful open clears the connect timeout and avoids a forced retry', async (t) => {
  const dom = stubDomGlobals();
  const restoreWebSocket = stubWebSocketGlobal();

  const connection = new WSConnection();
  configureFastRetries(connection, 10, 1);
  t.after(() => {
    connection.dispose();
    restoreWebSocket();
    dom.restore();
  });

  const states: ConnectionState[] = [];
  let disconnectCount = 0;
  connection.connect('ws://example.test/ws', {
    onStateChange: (state) => states.push(state),
    onDisconnect: () => {
      disconnectCount += 1;
    },
  });

  assert.equal(MockWebSocket.instances.length, 1);
  MockWebSocket.instances[0].open();
  await wait(25);

  assert.equal(MockWebSocket.instances.length, 1);
  assert.equal(disconnectCount, 0);
  assert.equal(states.at(-1), 'connected');
});
