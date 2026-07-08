import { setTimeout as delay } from 'node:timers/promises';
import net from 'node:net';
import { DEFAULT_RUNTIME_MANIFEST_URL } from '../openbrain/releaseManifest';
import { resolveHostLabel, sanitizeSshHost } from '../ssh/sshHostUtils';
import type { SshHost, SshHostWithSecrets } from '../ssh/sshTypes';
import { fetchLatestRuntimeRelease, resolveRemoteRuntimeTarget, type LatestRelease } from './remoteRuntimeRelease';
import {
  buildInstallScript,
  buildStartExistingScript,
  resolveRemoteWorkspaceDir,
} from './remoteRuntimeScripts';
import { getRemoteDefaultWorkspace } from './remoteRuntimeState';
import { runSsh, startPortForward } from './ssh2Transport';

export type { SshHost } from '../ssh/sshTypes';

export type RemoteSessionInfo = {
  hostLabel: string;
  localPort: number;
  remotePort: number;
  wsUrl: string;
  httpUrl: string;
  remoteHome: string;
  workspaceDir: string;
  installDir: string;
};

type ConnectOptions = {
  remotePort?: number;
  // Advanced override (mostly for debugging)
  manifestUrl?: string;
};

type ForwardEntry = {
  localPort: number;
  remotePort: number;
  host: SshHost;
  close: () => void;
  refCount: number;
};

const DEFAULT_REMOTE_PORT = 19530;
const DEFAULT_MANIFEST_URL = DEFAULT_RUNTIME_MANIFEST_URL;

const activeSessions = new Map<string, { session: RemoteSessionInfo; forwardKey: string }>();
const forwardRegistry = new Map<string, ForwardEntry>();

function formatUnknownError(error: unknown) {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}

function buildCombinedError(summary: string, details: Array<{ label: string; error: unknown }>) {
  const lines = [summary];
  for (const detail of details) {
    if (detail.error != null) {
      lines.push(`${detail.label}: ${formatUnknownError(detail.error)}`);
    }
  }
  return new Error(lines.join('\n'));
}

function getSessionKey(windowId: number, tabId: string) {
  return `${windowId}:${tabId}`;
}

function getForwardKey(host: SshHost, remotePort: number) {
  return [
    host.id || '',
    resolveHostLabel(host),
    host.port || '',
    host.authMethod || '',
    host.identityFile || '',
    host.credentialID || '',
    remotePort,
  ].join('|');
}

function joinRemoteSessionPath(remoteHome: string, ...parts: string[]) {
  const windows = /^[A-Za-z]:[\\/]/.test(remoteHome) || remoteHome.includes('\\');
  const sep = windows ? '\\' : '/';
  return [remoteHome.replace(/[\\/]+$/, ''), ...parts.map((part) => part.replace(/^[\\/]+|[\\/]+$/g, ''))].join(sep);
}

async function getAvailablePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.unref();
    server.on('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      if (!address || typeof address === 'string') {
        server.close();
        reject(new Error('Failed to allocate port'));
        return;
      }
      server.close(() => resolve(address.port));
    });
  });
}

async function waitForHealth(url: string, attempts = 20, intervalMs = 500): Promise<void> {
  for (let i = 0; i < attempts; i += 1) {
    try {
      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), 1500);
      const res = await fetch(url, { signal: controller.signal });
      clearTimeout(timeout);
      if (res.ok) {
        return;
      }
    } catch {
      // retry
    }
    await delay(intervalMs);
  }
  throw new Error(`Server health check failed at ${url}`);
}

async function acquirePortForward(host: SshHostWithSecrets, remotePort: number) {
  const key = getForwardKey(host, remotePort);
  const existing = forwardRegistry.get(key);
  if (existing) {
    existing.refCount += 1;
    return { key, entry: existing };
  }

  const localPort = await getAvailablePort();
  let closeForward: (() => void) | null = null;
  const forward = await startPortForward(host, localPort, remotePort, () => {
    const current = forwardRegistry.get(key);
    if (current?.close === closeForward) {
      forwardRegistry.delete(key);
    }
    for (const [sessionKey, sessionEntry] of activeSessions.entries()) {
      if (sessionEntry.forwardKey === key) {
        activeSessions.delete(sessionKey);
      }
    }
  });

  closeForward = forward.close;
  const entry: ForwardEntry = {
    localPort,
    remotePort,
    host: sanitizeSshHost(host),
    close: forward.close,
    refCount: 1,
  };
  forwardRegistry.set(key, entry);
  return { key, entry };
}

function releasePortForward(key: string) {
  const entry = forwardRegistry.get(key);
  if (!entry) {
    return;
  }
  entry.refCount -= 1;
  if (entry.refCount <= 0) {
    entry.close();
    forwardRegistry.delete(key);
  }
}

async function tryConnectExisting(
  host: SshHostWithSecrets,
  remotePort: number,
  remoteHome: string,
  workspaceDir: string,
) {
  const { key: forwardKey, entry } = await acquirePortForward(host, remotePort);
  const httpUrl = `http://127.0.0.1:${entry.localPort}/health`;
  try {
    await waitForHealth(httpUrl);
  } catch (error) {
    releasePortForward(forwardKey);
    throw error;
  }
  const session: RemoteSessionInfo = {
    hostLabel: resolveHostLabel(host),
    localPort: entry.localPort,
    remotePort,
    wsUrl: `ws://127.0.0.1:${entry.localPort}/ws`,
    httpUrl,
    remoteHome,
    workspaceDir,
    installDir: joinRemoteSessionPath(remoteHome, '.openbrain', 'agents', 'opagent-server'),
  };
  return { session, forwardKey };
}

export async function connectSsh(
  windowId: number,
  tabId: string,
  host: SshHostWithSecrets,
  options: ConnectOptions = {},
): Promise<RemoteSessionInfo> {
  await disconnectRemote(windowId, tabId);

  const remotePort = options.remotePort ?? DEFAULT_REMOTE_PORT;
  const manifestUrl = options.manifestUrl ?? DEFAULT_MANIFEST_URL;
  const target = await resolveRemoteRuntimeTarget(host);
  const remoteHome = target.home;
  const remoteDefaultWorkspace = await getRemoteDefaultWorkspace(host, target);
  const workspaceDir = resolveRemoteWorkspaceDir(remoteDefaultWorkspace ?? undefined, remoteHome);

  let existingRuntimeError: unknown = null;
  try {
    const { session, forwardKey } = await tryConnectExisting(host, remotePort, remoteHome, workspaceDir);
    activeSessions.set(getSessionKey(windowId, tabId), { session, forwardKey });
    return session;
  } catch (error) {
    existingRuntimeError = error;
  }

  let startExistingError: unknown = null;
  try {
    await runSsh(host, buildStartExistingScript({ remotePort, target }), 60_000);
    const { session, forwardKey } = await tryConnectExisting(host, remotePort, remoteHome, workspaceDir);
    activeSessions.set(getSessionKey(windowId, tabId), { session, forwardKey });
    return session;
  } catch (error) {
    startExistingError = error;
  }

  let latest: LatestRelease;
  try {
    latest = await fetchLatestRuntimeRelease(target.platform, manifestUrl);
  } catch (error) {
    throw buildCombinedError(
      'Remote runtime is unavailable. Automatic repair could not proceed because the runtime manifest could not be fetched.',
      [
        { label: 'Existing runtime health check failed', error: existingRuntimeError },
        { label: 'Existing runtime start failed', error: startExistingError },
        { label: 'Manifest fetch failed', error },
      ],
    );
  }

  try {
    await runSsh(host, buildInstallScript({ remotePort, target, ...latest }), 180_000);
  } catch (error) {
    throw buildCombinedError(
      'Remote runtime is unavailable. Automatic repair failed during installation.',
      [
        { label: 'Existing runtime health check failed', error: existingRuntimeError },
        { label: 'Existing runtime start failed', error: startExistingError },
        { label: 'Runtime repair failed', error },
      ],
    );
  }

  const { session, forwardKey } = await tryConnectExisting(host, remotePort, remoteHome, workspaceDir);
  activeSessions.set(getSessionKey(windowId, tabId), { session, forwardKey });
  return session;
}

export async function disconnectRemote(windowId: number, tabId?: string): Promise<void> {
  if (tabId) {
    const key = getSessionKey(windowId, tabId);
    const entry = activeSessions.get(key);
    if (entry) {
      releasePortForward(entry.forwardKey);
      activeSessions.delete(key);
    }
    return;
  }
  for (const [key, entry] of activeSessions.entries()) {
    if (key.startsWith(`${windowId}:`)) {
      releasePortForward(entry.forwardKey);
      activeSessions.delete(key);
    }
  }
}

export function getRemoteStatus(windowId: number, tabId: string): RemoteSessionInfo | null {
  return activeSessions.get(getSessionKey(windowId, tabId))?.session || null;
}

export function getActiveRemoteSessions(): Array<{ wsUrl: string; remoteHome: string }> {
  const seen = new Set<string>();
  const result: Array<{ wsUrl: string; remoteHome: string }> = [];
  for (const { session } of activeSessions.values()) {
    if (!seen.has(session.wsUrl)) {
      seen.add(session.wsUrl);
      result.push({ wsUrl: session.wsUrl, remoteHome: session.remoteHome });
    }
  }
  return result;
}
