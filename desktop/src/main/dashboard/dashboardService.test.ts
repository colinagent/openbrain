import assert from 'node:assert/strict';
import test from 'node:test';

import { fetchDashboardHosts } from './dashboardService';

const baseAuth = {
  version: 1,
  baseUrl: 'https://api.op-agent.com',
  gateway: 'https://api.op-agent.com',
  aiGateway: 'https://api.op-agent.com',
  token: 'token-123',
  uid: 'user-1',
  key: 'user-key',
  updatedAt: Date.now(),
};

test('fetchDashboardHosts paginates and normalizes host snapshots', async () => {
  const calls: string[] = [];
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async (input: URL | RequestInfo) => {
    const url = typeof input === 'string'
      ? input
      : input instanceof URL
        ? input.toString()
        : input.url;
    calls.push(url);

    const cursor = new URL(url).searchParams.get('cursor');
    if (cursor === '0') {
      return new Response(JSON.stringify({
        items: [
          {
            id: 'host-a1b2',
            hostname: 'desktop-1',
            env: 'local',
            baseDir: '/root/.openbrain',
            lastSeenAt: '2026-03-18T10:00:00Z',
            receivedAt: '2026-03-18T10:00:01Z',
            online: true,
            snapshot: {
              updater: {
                currentVersion: '1.0.5-beta',
                targetVersion: '1.0.6-beta',
                stagedVersion: '1.0.6-beta',
                phase: 'staged',
                downloaded: true,
                lastCheckedAt: '2026-03-18T10:00:00Z',
              },
              connections: {
                runtime: [
                  {
                    nodeID: 'local:host-a1b2:agent:file:///root/.openbrain/agents/coder/.agent/AGENT.md',
                    name: 'coder',
                    transport: 'stdio',
                    daemon: true,
                    pid: 4242,
                    startedAt: '2026-03-18T09:58:00Z',
                    uptimeSec: 121,
                  },
                ],
              },
            },
          },
        ],
        nextCursor: '1',
      }), { status: 200 });
    }

    return new Response(JSON.stringify({
      items: [
        {
          id: 'host-b2c3',
          hostname: 'delta',
          env: 'local',
          baseDir: '/srv/.openbrain',
          lastSeenAt: '2026-03-18T09:00:00Z',
          online: false,
          snapshot: {
            connections: {
              runtime: [
                {
                  nodeID: 'remote-http',
                  name: 'remote-http',
                  transport: 'http_streamable',
                  daemon: true,
                  lastActiveAt: '2026-03-18T08:59:30Z',
                  url: 'https://example.com/mcp',
                },
              ],
            },
          },
        },
      ],
      nextCursor: '0',
    }), { status: 200 });
  }) as typeof fetch;

  try {
    const hosts = await fetchDashboardHosts(baseAuth);
    assert.equal(calls.length, 2);
    assert.equal(hosts.length, 2);

    const first = hosts.find((host) => host.id === 'host-a1b2');
    assert.ok(first);
    assert.equal(first.hostname, 'desktop-1');
    assert.equal(first.online, true);
    assert.equal(first.receivedAt, '2026-03-18T10:00:01Z');
    assert.deepEqual(first.runtimeUpdater, {
      currentVersion: '1.0.5-beta',
      targetVersion: '1.0.6-beta',
      stagedVersion: '1.0.6-beta',
      phase: 'staged',
      downloaded: true,
      applying: undefined,
      lastCheckedAt: '2026-03-18T10:00:00Z',
      lastError: undefined,
    });
    assert.deepEqual(first.runtimeConnections, [
      {
        nodeID: 'local:host-a1b2:agent:file:///root/.openbrain/agents/coder/.agent/AGENT.md',
        name: 'coder',
        transport: 'stdio',
        daemon: true,
        pid: 4242,
        startedAt: '2026-03-18T09:58:00Z',
        uptimeSec: 121,
        lastActiveAt: undefined,
        url: undefined,
      },
    ]);

    const second = hosts.find((host) => host.id === 'host-b2c3');
    assert.ok(second);
    assert.equal(second.online, false);
    assert.equal(second.runtimeUpdater, undefined);
    assert.deepEqual(second.runtimeConnections, [
      {
        nodeID: 'remote-http',
        name: 'remote-http',
        transport: 'http_streamable',
        daemon: true,
        pid: undefined,
        startedAt: undefined,
        uptimeSec: undefined,
        lastActiveAt: '2026-03-18T08:59:30Z',
        url: 'https://example.com/mcp',
      },
    ]);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test('fetchDashboardHosts returns empty when gateway auth is unavailable', async () => {
  const hosts = await fetchDashboardHosts({
    ...baseAuth,
    gateway: '',
    token: '',
  });
  assert.deepEqual(hosts, []);
});
