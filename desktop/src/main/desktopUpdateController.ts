export type DesktopUpdatePhase =
  | 'unsupported'
  | 'idle'
  | 'checking'
  | 'downloading'
  | 'ready'
  | 'installing'
  | 'error';

export type DesktopUpdateState = {
  phase: DesktopUpdatePhase;
  currentVersion: string | null;
  targetVersion: string | null;
  error?: string;
};

type DesktopUpdateListener = (state: DesktopUpdateState) => void;

export type DesktopAutoUpdater = {
  autoDownload: boolean;
  autoInstallOnAppQuit: boolean;
  allowPrerelease: boolean;
  on(event: 'error', listener: (error: Error) => void): unknown;
  on(event: 'update-available', listener: (info: { version?: string | null }) => void): unknown;
  on(event: 'update-not-available', listener: () => void): unknown;
  on(event: 'update-downloaded', listener: (info: { version?: string | null }) => void): unknown;
  setFeedURL?(options: { provider: 'generic'; url: string }): void;
  checkForUpdates(): Promise<unknown>;
  quitAndInstall(isSilent?: boolean, isForceRunAfter?: boolean): void;
};

type DesktopUpdateControllerOptions = {
  appIsPackaged: boolean;
  currentVersion: string | null;
  updater: DesktopAutoUpdater;
  feedURL?: string;
  pollIntervalMs?: number;
  logger?: {
    log: (message?: unknown, ...optionalParams: unknown[]) => void;
    warn: (message?: unknown, ...optionalParams: unknown[]) => void;
  };
};

const DEFAULT_DESKTOP_UPDATE_POLL_INTERVAL_MS = 10 * 60 * 1000;

export class DesktopUpdateController {
  private readonly appIsPackaged: boolean;
  private readonly updater: DesktopAutoUpdater;
  private readonly logger: DesktopUpdateControllerOptions['logger'];
  private readonly pollIntervalMs: number;
  private readonly feedURL: string | null;
  private readonly listeners = new Set<DesktopUpdateListener>();

  private started = false;
  private pollTimer: NodeJS.Timeout | null = null;

  private state: DesktopUpdateState;

  constructor(options: DesktopUpdateControllerOptions) {
    this.appIsPackaged = options.appIsPackaged;
    this.updater = options.updater;
    this.logger = options.logger ?? console;
    this.pollIntervalMs = normalizePollIntervalMs(options.pollIntervalMs);
    this.feedURL = normalizeFeedURL(options.feedURL);
    this.state = {
      phase: options.appIsPackaged ? 'idle' : 'unsupported',
      currentVersion: normalizeVersion(options.currentVersion),
      targetVersion: null,
      error: undefined,
    };
  }

  getSnapshot(): DesktopUpdateState {
    return { ...this.state };
  }

  async waitForStartupDecision(timeoutMs: number): Promise<DesktopUpdateState> {
    if (isStartupDecisionState(this.state.phase)) {
      return this.getSnapshot();
    }

    return new Promise<DesktopUpdateState>((resolve) => {
      let settled = false;
      let timer: NodeJS.Timeout | null = null;

      const finish = (snapshot?: DesktopUpdateState) => {
        if (settled) {
          return;
        }
        settled = true;
        if (timer) {
          clearTimeout(timer);
          timer = null;
        }
        unsubscribe();
        resolve(snapshot ? { ...snapshot } : this.getSnapshot());
      };

      const unsubscribe = this.subscribe((snapshot) => {
        if (isStartupDecisionState(snapshot.phase)) {
          finish(snapshot);
        }
      });

      if (timeoutMs > 0) {
        timer = setTimeout(() => finish(), timeoutMs);
      }
    });
  }

  subscribe(listener: DesktopUpdateListener): () => void {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }

  start() {
    if (this.started) {
      return;
    }
    this.started = true;

    if (!this.appIsPackaged) {
      this.setState({
        phase: 'unsupported',
        targetVersion: null,
        error: undefined,
      });
      return;
    }

    this.updater.autoDownload = true;
    this.updater.autoInstallOnAppQuit = false;
    this.updater.allowPrerelease = false;
    if (this.feedURL && this.updater.setFeedURL) {
      this.updater.setFeedURL({ provider: 'generic', url: this.feedURL });
    }

    this.updater.on('error', (error) => {
      this.logger?.warn('[desktopUpdater] auto-update failed:', error);
      this.setState({
        phase: 'error',
        error: normalizeErrorMessage(error),
      });
    });

    this.updater.on('update-available', (info) => {
      const targetVersion = normalizeVersion(info?.version);
      this.logger?.log('[desktopUpdater] update available:', targetVersion);
      this.setState({
        phase: 'downloading',
        targetVersion,
        error: undefined,
      });
    });

    this.updater.on('update-not-available', () => {
      this.logger?.log('[desktopUpdater] no update available');
      this.setState({
        phase: 'idle',
        targetVersion: null,
        error: undefined,
      });
    });

    this.updater.on('update-downloaded', (info) => {
      this.setState({
        phase: 'ready',
        targetVersion: normalizeVersion(info?.version),
        error: undefined,
      });
    });

    this.schedulePolling();
    this.triggerCheck('startup');
  }

  private schedulePolling() {
    if (this.pollIntervalMs <= 0 || this.pollTimer) {
      return;
    }
    this.pollTimer = setInterval(() => {
      this.triggerCheck('poll');
    }, this.pollIntervalMs);
    this.pollTimer.unref?.();
  }

  private triggerCheck(reason: 'startup' | 'poll') {
    if (!this.appIsPackaged) {
      return;
    }
    if (
      this.state.phase === 'checking'
      || this.state.phase === 'downloading'
      || this.state.phase === 'installing'
      || this.state.phase === 'ready'
    ) {
      return;
    }

    this.logger?.log(`[desktopUpdater] checking for updates (${reason})`);
    this.setState({
      phase: 'checking',
      targetVersion: null,
      error: undefined,
    });

    void this.updater.checkForUpdates().catch((error) => {
      this.logger?.warn('[desktopUpdater] failed to check for updates:', error);
      this.setState({
        phase: 'error',
        error: normalizeErrorMessage(error),
      });
    });
  }

  beginInstall(): { success: boolean; error?: string } {
    if (!this.appIsPackaged) {
      return { success: false, error: 'Desktop update is unavailable in development mode.' };
    }
    if (this.state.phase === 'installing') {
      return { success: true };
    }
    if (this.state.phase !== 'ready') {
      return { success: false, error: 'Desktop update is not ready to install.' };
    }

    this.setState({
      phase: 'installing',
      error: undefined,
    });
    return { success: true };
  }

  finalizeInstall(): { success: boolean; error?: string } {
    if (this.state.phase !== 'installing') {
      return { success: false, error: 'Desktop update install has not started.' };
    }

    try {
      this.updater.quitAndInstall();
      return { success: true };
    } catch (error) {
      const message = normalizeErrorMessage(error);
      this.setState({
        phase: 'error',
        error: message,
      });
      return { success: false, error: message };
    }
  }

  private setState(patch: Partial<DesktopUpdateState>) {
    const nextState: DesktopUpdateState = {
      ...this.state,
      ...patch,
    };
    if (statesEqual(this.state, nextState)) {
      return;
    }
    this.state = nextState;
    for (const listener of this.listeners) {
      listener({ ...nextState });
    }
  }
}

function normalizePollIntervalMs(value: unknown): number {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) {
    return DEFAULT_DESKTOP_UPDATE_POLL_INTERVAL_MS;
  }
  return Math.max(1, Math.floor(value));
}

function normalizeFeedURL(value: unknown): string | null {
  if (typeof value !== 'string') {
    return null;
  }
  const text = value.trim().replace(/\/+$/, '');
  return text ? text : null;
}

function normalizeVersion(value: unknown): string | null {
  if (typeof value !== 'string') {
    return null;
  }
  const text = value.trim();
  return text ? text : null;
}

function normalizeErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message || 'Unknown desktop update error';
  }
  if (typeof error === 'string' && error.trim()) {
    return error.trim();
  }
  return 'Unknown desktop update error';
}

function statesEqual(a: DesktopUpdateState, b: DesktopUpdateState) {
  return a.phase === b.phase
    && a.currentVersion === b.currentVersion
    && a.targetVersion === b.targetVersion
    && a.error === b.error;
}

function isStartupDecisionState(phase: DesktopUpdatePhase) {
  return phase === 'idle'
    || phase === 'ready'
    || phase === 'error'
    || phase === 'unsupported';
}
