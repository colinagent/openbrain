import assert from 'node:assert/strict';
import test from 'node:test';

import {
  ArchiveCleanupScheduler,
  collectArchiveCleanupInvocations,
  isArchiveManagedFilePath,
} from './archiveCleanup';

test('isArchiveManagedFilePath matches chat and plan documents only', () => {
  assert.equal(isArchiveManagedFilePath('/workspace/.agent/chat/demo.md'), true);
  assert.equal(isArchiveManagedFilePath('/workspace/.agent/context/release.plan.md'), true);
  assert.equal(isArchiveManagedFilePath('/workspace/.agent/plan/release.plan.md'), false);
  assert.equal(isArchiveManagedFilePath('/workspace/README.md'), false);
});

test('collectArchiveCleanupInvocations groups local and remote workspaces by endpoint', () => {
  const invocations = collectArchiveCleanupInvocations({
    windows: [
      { id: 1, sessionId: 'window-1', mode: 'local', workspacePath: '/workspace/a' },
      { id: 2, sessionId: 'window-2', mode: 'remote' },
    ],
    sessionsById: {
      'window-1': {
        version: 1,
        activeTabId: 'tab-a',
        tabs: [{
          id: 'tab-a',
          kind: 'local',
          workspacePath: '/workspace/a',
          currentDir: '/workspace/a',
          openEditorFilePaths: [
            '/workspace/a/.agent/chat/demo.md',
            '/workspace/a/.agent/context/release.plan.md',
            '/workspace/a/README.md',
          ],
          chatSession: {
            openChats: [{ threadID: 'thread-demo', path: '/workspace/a/.agent/chat/demo.md', title: 'Demo' }],
          },
        }],
      },
      'window-2': {
        version: 1,
        activeTabId: 'tab-b',
        tabs: [{
          id: 'tab-b',
          kind: 'remote',
          currentDir: '/remote/workspace',
          openEditorFilePaths: ['/remote/workspace/.agent/chat/remote.md'],
          chatSession: {
            openChats: [{ threadID: 'thread-remote', path: '/remote/workspace/.agent/chat/remote.md', title: 'Remote' }],
          },
        }],
      },
    },
    getRemoteSession: (windowId, tabId) => {
      if (windowId === 2 && tabId === 'tab-b') {
        return {
          wsUrl: 'ws://127.0.0.1:20001/ws',
          workspaceDir: '/remote/workspace',
        };
      }
      return null;
    },
    localWsUrl: 'ws://127.0.0.1:19530/ws',
  });

  assert.deepEqual(invocations, [
    {
      endpointUrl: 'ws://127.0.0.1:19530/ws',
      workspaceRoots: ['/workspace/a'],
      openFilePaths: [
        '/workspace/a/.agent/chat/demo.md',
        '/workspace/a/.agent/context/release.plan.md',
      ],
    },
    {
      endpointUrl: 'ws://127.0.0.1:20001/ws',
      workspaceRoots: ['/remote/workspace'],
      openFilePaths: ['/remote/workspace/.agent/chat/remote.md'],
    },
  ]);
});

test('ArchiveCleanupScheduler coalesces delayed runs while one sweep is in flight', async () => {
  let releaseFirstRun: () => void = () => {};
  let calls = 0;

  const scheduler = new ArchiveCleanupScheduler({
    collectInvocations: () => [{
      endpointUrl: 'ws://127.0.0.1:19530/ws',
      workspaceRoots: ['/workspace/a'],
      openFilePaths: ['/workspace/a/.agent/chat/demo.md'],
    }],
    runInvocation: async () => {
      calls += 1;
      if (calls === 1) {
        await new Promise<void>((resolve) => {
          releaseFirstRun = resolve;
        });
      }
    },
    intervalMs: 60_000,
    triggerDelayMs: 1,
  });

  scheduler.scheduleSoon(0);
  await new Promise((resolve) => setTimeout(resolve, 20));
  assert.equal(calls, 1);

  scheduler.scheduleSoon(0);
  await new Promise((resolve) => setTimeout(resolve, 20));
  assert.equal(calls, 1);

  releaseFirstRun();
  await new Promise((resolve) => setTimeout(resolve, 20));
  assert.equal(calls, 2);

  scheduler.stop();
});
