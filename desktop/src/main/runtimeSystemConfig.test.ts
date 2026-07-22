import assert from 'node:assert/strict';
import { once } from 'node:events';
import test from 'node:test';
import { WebSocketServer } from 'ws';

import {
  normalizeRuntimeSystemConfig,
  requestRuntimeSystemConfig,
  waitForRuntimeSystemConfig,
} from './runtimeSystemConfig';

test('normalizeRuntimeSystemConfig requires the runtime-owned workspace', () => {
  assert.deepEqual(
    normalizeRuntimeSystemConfig({
      baseDir: ' /runtime/base ',
      defaultWorkspace: ' /runtime/base/workspace ',
      hostID: ' host-one ',
    }),
    {
      baseDir: '/runtime/base',
      defaultWorkspace: '/runtime/base/workspace',
      hostID: 'host-one',
    },
  );
  assert.throws(
    () => normalizeRuntimeSystemConfig({ baseDir: '/runtime/base' }),
    /defaultWorkspace/,
  );
});

test('requestRuntimeSystemConfig reads config/system/get over WebSocket', async (t) => {
  const server = new WebSocketServer({ host: '127.0.0.1', port: 0 });
  t.after(() => {
    server.close();
  });
  await once(server, 'listening');
  server.on('connection', (socket) => {
    socket.on('message', (data) => {
      const request = JSON.parse(data.toString()) as { id: number; method: string };
      assert.equal(request.method, 'config/system/get');
      socket.send(JSON.stringify({
        jsonrpc: '2.0',
        id: request.id,
        result: {
          baseDir: '/runtime/base',
          defaultWorkspace: '/runtime/base/workspace',
          hostID: 'host-one',
        },
      }));
    });
  });
  const address = server.address();
  if (!address || typeof address === 'string') {
    throw new Error('WebSocket test server did not expose a TCP address');
  }

  const result = await requestRuntimeSystemConfig(`ws://127.0.0.1:${address.port}`);
  assert.equal(result.defaultWorkspace, '/runtime/base/workspace');
});

test('waitForRuntimeSystemConfig retries while the Runtime session starts', async (t) => {
  const server = new WebSocketServer({ host: '127.0.0.1', port: 0 });
  t.after(() => {
    server.close();
  });
  await once(server, 'listening');
  let requestCount = 0;
  server.on('connection', (socket) => {
    socket.on('message', (data) => {
      requestCount += 1;
      const request = JSON.parse(data.toString()) as { id: number };
      if (requestCount === 1) {
        socket.send(JSON.stringify({
          jsonrpc: '2.0',
          id: request.id,
          error: { message: 'OpAgent session not initialized' },
        }));
        return;
      }
      socket.send(JSON.stringify({
        jsonrpc: '2.0',
        id: request.id,
        result: {
          baseDir: '/runtime/base',
          defaultWorkspace: '/runtime/base/workspace',
        },
      }));
    });
  });
  const address = server.address();
  if (!address || typeof address === 'string') {
    throw new Error('WebSocket test server did not expose a TCP address');
  }

  const result = await waitForRuntimeSystemConfig(`ws://127.0.0.1:${address.port}`, {
    attempts: 2,
    intervalMs: 1,
    requestTimeoutMs: 500,
  });
  assert.equal(result.defaultWorkspace, '/runtime/base/workspace');
  assert.equal(requestCount, 2);
});
