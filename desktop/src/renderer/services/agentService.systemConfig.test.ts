import assert from 'node:assert/strict';
import test from 'node:test';

import { createAgentService } from './agentService';
import type { WSConnection } from './wsConnection';

test('getSystemConfig retries until the Runtime host session is ready', async () => {
  let requestCount = 0;
  const connection = {
    request: async () => {
      requestCount += 1;
      if (requestCount === 1) {
        throw new Error('OpAgent session not initialized');
      }
      return {
        baseDir: '/runtime/base',
        defaultWorkspace: '/runtime/base/workspace',
        hostID: 'host-one',
      };
    },
  } as unknown as WSConnection;

  const result = await createAgentService(connection).getSystemConfig({
    attempts: 2,
    intervalMs: 0,
  });

  assert.equal(result?.defaultWorkspace, '/runtime/base/workspace');
  assert.equal(requestCount, 2);
});

test('getSystemConfig retries incomplete Runtime responses', async () => {
  let requestCount = 0;
  const connection = {
    request: async () => {
      requestCount += 1;
      return requestCount === 1
        ? { baseDir: '/runtime/base', hostID: 'host-one' }
        : {
            baseDir: '/runtime/base',
            defaultWorkspace: '/runtime/base/workspace',
            hostID: 'host-one',
          };
    },
  } as unknown as WSConnection;

  const result = await createAgentService(connection).getSystemConfig({
    attempts: 2,
    intervalMs: 0,
  });

  assert.equal(result?.defaultWorkspace, '/runtime/base/workspace');
  assert.equal(requestCount, 2);
});
