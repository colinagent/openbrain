import { spawn } from 'node:child_process';
import * as readline from 'node:readline';
import { mainT } from '../i18n/main';
import { fetchReleaseManifest, getLocalPlatform, pickPlatformAssets } from './releaseManifest';

export const DEFAULT_LOCAL_RUNTIME_MANIFEST_URL =
  'https://github.com/colinagent/openbrain/releases/latest/download/runtime-manifest.json';

type BootstrapCommand = 'status' | 'ensure';

type ResolvedBootstrapBundle = {
  latestVersion: string;
  bundleUrl?: string;
  bundleSha256?: string;
  bundleFilePath?: string;
};

type BootstrapScriptStatus = {
  ok: boolean;
  phase?: string;
  message?: string;
  error?: string;
  detail?: string;
  installedVersion?: string;
  runningVersion?: string;
  latestVersion?: string;
  needsInstall?: boolean;
  needsUpdate?: boolean;
  needsStart?: boolean;
  healthy?: boolean;
  offline?: boolean;
  isFirstInstall?: boolean;
};

type BootstrapScriptEvent = BootstrapScriptStatus & {
  type?: 'event' | 'result';
};

export type LocalRuntimeBootstrapPhase =
  | 'idle'
  | 'checking'
  | 'installing'
  | 'updating'
  | 'starting'
  | 'ready'
  | 'error';

export type LocalRuntimeBootstrapState = {
  phase: LocalRuntimeBootstrapPhase;
  visible: boolean;
  busy: boolean;
  ready: boolean;
  message: string;
  detail?: string;
  error?: string;
  canRetry: boolean;
  canQuit: boolean;
  installedVersion?: string;
  runningVersion?: string;
  latestVersion?: string;
  needsInstall: boolean;
  needsUpdate: boolean;
  needsStart: boolean;
  healthy: boolean;
  offline: boolean;
  isFirstInstall: boolean;
  lastUpdatedAt: number;
};

type LocalRuntimeBootstrapControllerOptions = {
  appIsPackaged: boolean;
  bootstrapperPath: string | null;
  bundledRuntimeBundlePath?: string | null;
  currentVersion?: string | null;
  baseDir: string;
  manifestUrl: string;
  port: number;
};

function now() {
  return Date.now();
}

function createInitialState(): LocalRuntimeBootstrapState {
  return {
    phase: 'idle',
    visible: false,
    busy: false,
    ready: false,
    message: '',
    canRetry: false,
    canQuit: false,
    needsInstall: false,
    needsUpdate: false,
    needsStart: false,
    healthy: false,
    offline: false,
    isFirstInstall: false,
    lastUpdatedAt: now(),
  };
}

function normalizeOptionalString(value: unknown): string | undefined {
  if (typeof value !== 'string') {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed || undefined;
}

function hasWork(payload: BootstrapScriptStatus): boolean {
  return payload.needsInstall === true || payload.needsUpdate === true || payload.needsStart === true;
}

function hasBlockingWork(payload: Pick<BootstrapScriptStatus, 'needsInstall' | 'needsUpdate'>): boolean {
  return payload.needsInstall === true || payload.needsUpdate === true;
}

export function resolveBootstrapEnsureMode(
  payload: Pick<BootstrapScriptStatus, 'needsInstall' | 'needsUpdate' | 'needsStart'>,
): 'none' | 'background-start' | 'blocking-ensure' {
  if (hasBlockingWork(payload)) {
    return 'blocking-ensure';
  }
  if (payload.needsStart === true) {
    return 'background-start';
  }
  return 'none';
}

function mapPhase(payload: Pick<BootstrapScriptStatus, 'phase' | 'needsInstall' | 'needsUpdate' | 'needsStart'>): LocalRuntimeBootstrapPhase {
  const raw = (payload.phase || '').trim().toLowerCase();
  if (raw === 'ready') {
    return 'ready';
  }
  if (raw === 'error') {
    return 'error';
  }
  if (raw === 'checking') {
    return 'checking';
  }
  if (
    raw === 'update'
    || raw === 'downloading'
    || raw === 'verifying'
    || raw === 'extracting'
    || raw === 'installing'
  ) {
    return payload.needsUpdate ? 'updating' : 'installing';
  }
  if (raw === 'install') {
    return payload.needsUpdate ? 'updating' : 'installing';
  }
  if (
    raw === 'start'
    || raw === 'starting'
    || raw === 'wait-health'
    || raw === 'waiting-health'
  ) {
    return 'starting';
  }
  if (payload.needsUpdate) {
    return 'updating';
  }
  if (payload.needsInstall) {
    return 'installing';
  }
  if (payload.needsStart) {
    return 'starting';
  }
  return 'checking';
}

function stateFromScript(
  payload: BootstrapScriptStatus,
  options?: { forceVisible?: boolean; forceBusy?: boolean; forceReady?: boolean; forceError?: boolean },
): LocalRuntimeBootstrapState {
  const phase = options?.forceError
    ? 'error'
    : options?.forceReady
      ? 'ready'
      : mapPhase(payload);
  const ready = options?.forceReady === true || (phase === 'ready' && payload.healthy === true);
  const busy = options?.forceBusy === true || (phase !== 'ready' && phase !== 'error');
  const visible = options?.forceVisible === true || phase === 'error' || (busy && hasWork(payload));
  return {
    phase,
    visible,
    busy,
    ready,
    message: normalizeOptionalString(payload.message) || '',
    detail: normalizeOptionalString(payload.detail),
    error: normalizeOptionalString(payload.error),
    canRetry: phase === 'error',
    canQuit: visible,
    installedVersion: normalizeOptionalString(payload.installedVersion),
    runningVersion: normalizeOptionalString(payload.runningVersion),
    latestVersion: normalizeOptionalString(payload.latestVersion),
    needsInstall: payload.needsInstall === true,
    needsUpdate: payload.needsUpdate === true,
    needsStart: payload.needsStart === true,
    healthy: payload.healthy === true,
    offline: payload.offline === true,
    isFirstInstall: payload.isFirstInstall === true,
    lastUpdatedAt: now(),
  };
}

async function runBootstrapper(
  bootstrapperPath: string,
  command: BootstrapCommand,
  options: { baseDir: string; port: number; resolvedBundle?: ResolvedBootstrapBundle | null },
  onEvent?: (payload: BootstrapScriptEvent) => void,
): Promise<BootstrapScriptStatus> {
  const args = [
    command,
    '--base-dir',
    options.baseDir,
    '--port',
    String(options.port),
    '--json-events',
  ];
  if (options.resolvedBundle) {
    args.push(
      '--version',
      options.resolvedBundle.latestVersion,
    );
    if (options.resolvedBundle.bundleFilePath) {
      args.push('--bundle-file', options.resolvedBundle.bundleFilePath);
    } else if (options.resolvedBundle.bundleUrl && options.resolvedBundle.bundleSha256) {
      args.push(
        '--bundle-url',
        options.resolvedBundle.bundleUrl,
        '--bundle-sha256',
        options.resolvedBundle.bundleSha256,
      );
    }
  }
  const child = spawn(bootstrapperPath, args, {
    stdio: ['ignore', 'pipe', 'pipe'],
    windowsHide: true,
  });

  let stdoutBuffer = '';
  let stderrBuffer = '';
  let lastPayload: BootstrapScriptStatus | null = null;

  const rl = readline.createInterface({
    input: child.stdout,
    crlfDelay: Infinity,
  });

  rl.on('line', (line) => {
    const trimmed = line.trim();
    if (!trimmed) {
      return;
    }
    stdoutBuffer += `${trimmed}\n`;
    try {
      const payload = JSON.parse(trimmed) as BootstrapScriptEvent;
      lastPayload = payload;
      onEvent?.(payload);
    } catch {
      // Ignore non-JSON lines from the helper script.
    }
  });

  child.stderr.on('data', (chunk) => {
    stderrBuffer += String(chunk ?? '');
  });

  const exitCode = await new Promise<number | null>((resolve, reject) => {
    child.once('error', reject);
    child.once('exit', (code) => resolve(code));
  });

  rl.close();

  if (lastPayload) {
    if (exitCode === 0) {
      return lastPayload;
    }
    const payload = lastPayload as BootstrapScriptStatus;
    return {
      phase: payload.phase,
      message: payload.message,
      detail: payload.detail,
      installedVersion: payload.installedVersion,
      runningVersion: payload.runningVersion,
      latestVersion: payload.latestVersion,
      needsInstall: payload.needsInstall,
      needsUpdate: payload.needsUpdate ?? payload.needsInstall,
      needsStart: payload.needsStart,
      healthy: payload.healthy,
      offline: payload.offline,
      isFirstInstall: payload.isFirstInstall,
      ok: false,
      error: normalizeOptionalString(payload.error)
        || normalizeOptionalString(stderrBuffer)
        || `runtime bootstrapper exited with code ${exitCode ?? -1}`,
    };
  }

  if (exitCode !== 0) {
    return {
      ok: false,
      phase: 'error',
      error: normalizeOptionalString(stderrBuffer)
        || normalizeOptionalString(stdoutBuffer)
        || `runtime bootstrapper exited with code ${exitCode ?? -1}`,
      message: mainT('error:main.runtimeStartupFailed'),
    };
  }

  throw new Error('runtime bootstrapper returned no result payload');
}

async function resolveLatestRuntimeBundle(manifestUrl: string): Promise<ResolvedBootstrapBundle> {
  const manifest = await fetchReleaseManifest(manifestUrl);
  const picked = pickPlatformAssets(manifest, getLocalPlatform());
  return {
    latestVersion: picked.version,
    bundleUrl: picked.assets.bundle.url,
    bundleSha256: picked.assets.bundle.sha256,
  };
}

export class LocalRuntimeBootstrapController {
  private readonly options: LocalRuntimeBootstrapControllerOptions;

  private snapshot: LocalRuntimeBootstrapState = createInitialState();

  private readonly listeners = new Set<(snapshot: LocalRuntimeBootstrapState) => void>();

  private inFlight: Promise<void> | null = null;

  constructor(options: LocalRuntimeBootstrapControllerOptions) {
    this.options = options;
  }

  getSnapshot(): LocalRuntimeBootstrapState {
    return { ...this.snapshot };
  }

  subscribe(listener: (snapshot: LocalRuntimeBootstrapState) => void): () => void {
    this.listeners.add(listener);
    listener(this.getSnapshot());
    return () => {
      this.listeners.delete(listener);
    };
  }

  async start(): Promise<void> {
    await this.ensureRuntimeReady();
  }

  async retry(): Promise<void> {
    await this.ensureRuntimeReady();
  }

  private setSnapshot(next: LocalRuntimeBootstrapState) {
    this.snapshot = next;
    for (const listener of this.listeners) {
      listener(this.getSnapshot());
    }
  }

  private async ensureRuntimeReady(): Promise<void> {
    if (this.inFlight) {
      return this.inFlight;
    }
    this.inFlight = this.runEnsureRuntimeReady()
      .finally(() => {
        this.inFlight = null;
      });
    return this.inFlight;
  }

  private async runEnsureRuntimeReady(): Promise<void> {
    if (!this.options.appIsPackaged) {
      this.setSnapshot({
        ...createInitialState(),
        phase: 'idle',
        ready: false,
        message: mainT('error:main.runtimeDevManaged'),
        lastUpdatedAt: now(),
      });
      return;
    }

    if (!this.options.bootstrapperPath) {
      this.setSnapshot({
        ...createInitialState(),
        phase: 'error',
        visible: true,
        busy: false,
        ready: false,
        message: mainT('error:main.runtimeStartupFailed'),
        error: 'Bundled runtime bootstrapper is missing.',
        canRetry: true,
        canQuit: true,
        lastUpdatedAt: now(),
      });
      return;
    }

    const hasBundledRuntime = Boolean(this.options.bundledRuntimeBundlePath && this.options.currentVersion);

    this.setSnapshot({
      ...this.snapshot,
      phase: 'checking',
      visible: false,
      busy: true,
      ready: false,
      message: hasBundledRuntime ? 'Checking the built-in runtime' : 'Checking local runtime',
      error: undefined,
      canRetry: false,
      canQuit: false,
      lastUpdatedAt: now(),
    });

    const bundledResolved = hasBundledRuntime
      ? {
          latestVersion: this.options.currentVersion!,
          bundleFilePath: this.options.bundledRuntimeBundlePath!,
        }
      : null;

    const status = await runBootstrapper(this.options.bootstrapperPath, 'status', {
      baseDir: this.options.baseDir,
      port: this.options.port,
      resolvedBundle: bundledResolved,
    });
    if (status.ok && !hasWork(status)) {
      const statusState = stateFromScript(status);
      this.setSnapshot({
        ...statusState,
        phase: 'ready',
        visible: false,
        busy: false,
        ready: true,
        canRetry: false,
        canQuit: false,
        lastUpdatedAt: now(),
      });
      return;
    }

    const ensureMode = status.ok
      ? resolveBootstrapEnsureMode(status)
      : 'blocking-ensure';

    let resolvedBundle: ResolvedBootstrapBundle | null = bundledResolved;
    if (!resolvedBundle) {
      try {
        resolvedBundle = await resolveLatestRuntimeBundle(this.options.manifestUrl);
      } catch (error) {
        this.setSnapshot({
          ...createInitialState(),
          phase: 'error',
          visible: true,
          busy: false,
          ready: false,
          message: mainT('error:main.runtimeStartupFailed'),
          error: normalizeOptionalString(status.error)
            || (error instanceof Error ? error.message : 'Unable to fetch the runtime bundle manifest.'),
          canRetry: true,
          canQuit: true,
          installedVersion: normalizeOptionalString(status.installedVersion),
          runningVersion: normalizeOptionalString(status.runningVersion),
          lastUpdatedAt: now(),
        });
        return;
      }
    }

    const showBlockingOverlay = ensureMode === 'blocking-ensure';
    this.setSnapshot({
      ...createInitialState(),
      visible: showBlockingOverlay,
      busy: true,
      ready: false,
      phase: showBlockingOverlay ? 'checking' : 'starting',
      message: showBlockingOverlay
        ? (bundledResolved ? 'Preparing the built-in runtime' : 'Repairing local runtime')
        : (bundledResolved ? 'Starting the built-in runtime' : 'Starting local runtime'),
      detail: undefined,
      canRetry: false,
      canQuit: showBlockingOverlay,
      installedVersion: normalizeOptionalString(status.installedVersion),
      runningVersion: normalizeOptionalString(status.runningVersion),
      latestVersion: normalizeOptionalString(resolvedBundle.latestVersion) || normalizeOptionalString(status.latestVersion),
      needsInstall: status.needsInstall === true,
      needsUpdate: status.needsUpdate === true,
      needsStart: status.needsStart === true,
      healthy: status.healthy === true,
      offline: status.offline === true,
      isFirstInstall: status.isFirstInstall === true,
      lastUpdatedAt: now(),
    });

    const result = await runBootstrapper(
      this.options.bootstrapperPath,
      'ensure',
      {
        baseDir: this.options.baseDir,
        port: this.options.port,
        resolvedBundle,
      },
      (event) => {
        const eventState = stateFromScript(event, {
          forceVisible: showBlockingOverlay && event.type === 'event',
        });
        this.setSnapshot({
          ...this.snapshot,
          ...eventState,
          visible: showBlockingOverlay && event.type === 'event' ? true : false,
          busy: event.type === 'event' ? true : eventState.busy,
          ready: event.type === 'event' ? false : eventState.ready,
          canRetry: false,
          canQuit: showBlockingOverlay,
          error: undefined,
          lastUpdatedAt: now(),
        });
      },
    );

    if (!result.ok) {
      this.setSnapshot(stateFromScript(result, {
        forceVisible: true,
        forceBusy: false,
        forceError: true,
      }));
      return;
    }

    this.setSnapshot({
      ...stateFromScript(result, {
        forceReady: true,
      }),
      visible: false,
      busy: false,
      ready: true,
      canRetry: false,
      canQuit: false,
      lastUpdatedAt: now(),
    });
  }
}
