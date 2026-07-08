import WebSocket from 'ws';

export type ArchiveWindowMode = 'local' | 'remote';

export type ArchiveChatTabSession = {
  threadID: string;
  path: string;
  title: string;
};

export type ArchiveChatSession = {
  openChats: ArchiveChatTabSession[];
  selectedThreadID?: string;
};

export type ArchiveWorkspaceTabSession = {
  id: string;
  kind: ArchiveWindowMode;
  workspacePath?: string;
  currentDir?: string;
  chatSession?: ArchiveChatSession;
  openEditorFilePaths?: string[];
};

export type ArchiveWorkspaceTabsSessionState = {
  version: number;
  activeTabId: string;
  tabs: ArchiveWorkspaceTabSession[];
};

export type ArchiveWindowInfo = {
  id: number;
  sessionId: string;
  mode: ArchiveWindowMode;
  workspacePath?: string;
};

export type ArchiveRemoteSessionInfo = {
  wsUrl: string;
  workspaceDir: string;
};

export type ArchiveCleanupInvocation = {
  endpointUrl: string;
  workspaceRoots: string[];
  openFilePaths: string[];
};

export type ArchiveCleanupRpcParams = {
  workspaceRoots: string[];
  openFilePaths: string[];
};

type SchedulerOptions = {
  collectInvocations: () => ArchiveCleanupInvocation[];
  runInvocation?: (invocation: ArchiveCleanupInvocation) => Promise<void>;
  intervalMs?: number;
  triggerDelayMs?: number;
};

const DEFAULT_INTERVAL_MS = 10 * 60 * 1000;
const DEFAULT_TRIGGER_DELAY_MS = 15 * 1000;
const DEFAULT_REQUEST_TIMEOUT_MS = 30 * 1000;

const CHAT_SEGMENT = '/.agent/chat/';
const PLAN_SEGMENT = '/.agent/context/';

function normalizePath(value: string | null | undefined): string {
  const trimmed = (value || '').trim();
  return trimmed ? trimmed.replace(/\\/g, '/').replace(/\/+$/, '') : '';
}

export function isArchiveManagedFilePath(value: string | null | undefined): boolean {
  const normalized = normalizePath(value);
  if (!normalized) {
    return false;
  }
  return normalized.includes(CHAT_SEGMENT) || normalized.includes(PLAN_SEGMENT);
}

function dedupeSorted(values: Iterable<string>): string[] {
  const out = Array.from(new Set(Array.from(values).map((value) => normalizePath(value)).filter(Boolean)));
  out.sort();
  return out;
}

export function collectArchiveCleanupInvocations(params: {
  windows: ArchiveWindowInfo[];
  sessionsById: Record<string, ArchiveWorkspaceTabsSessionState | null | undefined>;
  getRemoteSession: (windowId: number, tabId: string) => ArchiveRemoteSessionInfo | null;
  localWsUrl: string;
}): ArchiveCleanupInvocation[] {
  const grouped = new Map<string, { workspaceRoots: Set<string>; openFilePaths: Set<string> }>();

  for (const win of params.windows) {
    const session = params.sessionsById[win.sessionId];
    if (!session || !Array.isArray(session.tabs)) {
      continue;
    }

    for (const tab of session.tabs) {
      if (!tab || typeof tab !== 'object') {
        continue;
      }

      const endpointUrl = tab.kind === 'remote'
        ? normalizePath(params.getRemoteSession(win.id, tab.id)?.wsUrl)
        : normalizePath(params.localWsUrl);
      if (!endpointUrl) {
        continue;
      }

      const workspaceRoot = tab.kind === 'remote'
        ? normalizePath(params.getRemoteSession(win.id, tab.id)?.workspaceDir || tab.workspacePath || tab.currentDir)
        : normalizePath(tab.workspacePath || tab.currentDir || win.workspacePath);
      if (!workspaceRoot) {
        continue;
      }

      let bucket = grouped.get(endpointUrl);
      if (!bucket) {
        bucket = { workspaceRoots: new Set<string>(), openFilePaths: new Set<string>() };
        grouped.set(endpointUrl, bucket);
      }
      bucket.workspaceRoots.add(workspaceRoot);

      for (const path of tab.openEditorFilePaths || []) {
        if (isArchiveManagedFilePath(path)) {
          bucket.openFilePaths.add(normalizePath(path));
        }
      }
      for (const entry of tab.chatSession?.openChats || []) {
        if (isArchiveManagedFilePath(entry.path)) {
          bucket.openFilePaths.add(normalizePath(entry.path));
        }
      }
    }
  }

  return Array.from(grouped.entries())
    .map(([endpointUrl, bucket]) => ({
      endpointUrl,
      workspaceRoots: dedupeSorted(bucket.workspaceRoots),
      openFilePaths: dedupeSorted(bucket.openFilePaths),
    }))
    .filter((entry) => entry.workspaceRoots.length > 0)
    .sort((a, b) => a.endpointUrl.localeCompare(b.endpointUrl));
}

export async function runArchiveCleanupRpc(
  endpointUrl: string,
  params: ArchiveCleanupRpcParams,
  timeoutMs = DEFAULT_REQUEST_TIMEOUT_MS,
): Promise<void> {
  const normalizedUrl = normalizePath(endpointUrl);
  if (!normalizedUrl) {
    throw new Error('archive cleanup endpoint is required');
  }

  await new Promise<void>((resolve, reject) => {
    const ws = new WebSocket(normalizedUrl);
    let settled = false;
    const requestID = 1;

    const finish = (error?: Error) => {
      if (settled) {
        return;
      }
      settled = true;
      clearTimeout(timeout);
      ws.removeAllListeners();
      try {
        ws.close();
      } catch {
        // ignore close failures
      }
      if (error) {
        reject(error);
      } else {
        resolve();
      }
    };

    const timeout = setTimeout(() => {
      finish(new Error(`archive cleanup timed out after ${timeoutMs}ms`));
    }, timeoutMs);

    ws.on('open', () => {
      ws.send(JSON.stringify({
        jsonrpc: '2.0',
        id: requestID,
        method: 'archive/cleanup/run',
        params,
      }));
    });

    ws.on('message', (raw) => {
      try {
        const text = typeof raw === 'string' ? raw : raw.toString();
        const payload = JSON.parse(text) as {
          id?: number;
          error?: { message?: string };
        };
        if (payload.id !== requestID) {
          return;
        }
        if (payload.error) {
          finish(new Error(payload.error.message || 'archive cleanup failed'));
          return;
        }
        finish();
      } catch (error) {
        finish(error instanceof Error ? error : new Error(String(error)));
      }
    });

    ws.on('error', (error) => {
      finish(error instanceof Error ? error : new Error(String(error)));
    });

    ws.on('close', () => {
      finish(new Error('archive cleanup connection closed before response'));
    });
  });
}

export class ArchiveCleanupScheduler {
  private readonly collectInvocations: SchedulerOptions['collectInvocations'];
  private readonly runInvocation: (invocation: ArchiveCleanupInvocation) => Promise<void>;
  private readonly intervalMs: number;
  private readonly triggerDelayMs: number;

  private intervalTimer: NodeJS.Timeout | null = null;
  private delayedTimer: NodeJS.Timeout | null = null;
  private inFlight = false;
  private pendingDelayedRun = false;

  constructor(options: SchedulerOptions) {
    this.collectInvocations = options.collectInvocations;
    this.runInvocation = options.runInvocation || (async (invocation) => {
      await runArchiveCleanupRpc(invocation.endpointUrl, {
        workspaceRoots: invocation.workspaceRoots,
        openFilePaths: invocation.openFilePaths,
      });
    });
    this.intervalMs = options.intervalMs ?? DEFAULT_INTERVAL_MS;
    this.triggerDelayMs = options.triggerDelayMs ?? DEFAULT_TRIGGER_DELAY_MS;
  }

  start() {
    if (this.intervalTimer) {
      return;
    }
    this.intervalTimer = setInterval(() => {
      void this.runSweep();
    }, this.intervalMs);
  }

  stop() {
    if (this.intervalTimer) {
      clearInterval(this.intervalTimer);
      this.intervalTimer = null;
    }
    if (this.delayedTimer) {
      clearTimeout(this.delayedTimer);
      this.delayedTimer = null;
    }
    this.pendingDelayedRun = false;
  }

  scheduleSoon(delayMs = this.triggerDelayMs) {
    if (this.inFlight) {
      this.pendingDelayedRun = true;
      return;
    }
    if (this.delayedTimer) {
      clearTimeout(this.delayedTimer);
    }
    this.delayedTimer = setTimeout(() => {
      this.delayedTimer = null;
      void this.runSweep();
    }, Math.max(0, delayMs));
  }

  private async runSweep() {
    if (this.inFlight) {
      return;
    }
    this.inFlight = true;
    try {
      const invocations = this.collectInvocations();
      for (const invocation of invocations) {
        await this.runInvocation(invocation);
      }
    } finally {
      this.inFlight = false;
      if (this.pendingDelayedRun) {
        this.pendingDelayedRun = false;
        this.scheduleSoon();
      }
    }
  }
}
