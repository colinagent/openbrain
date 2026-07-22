import { app, BrowserWindow, ipcMain, shell, dialog, Menu, nativeImage, powerMonitor, powerSaveBlocker } from 'electron';
import type { WebContents } from 'electron';
import { spawn } from 'child_process';
import * as path from 'path';
import { readFileSync, existsSync, watch as fsWatch, type FSWatcher } from 'fs';
import * as fs from 'fs/promises';
import { parse as parseJsonc, type ParseError } from 'jsonc-parser';
import { release } from 'os';
import { createHash } from 'crypto';
import {
  getDesktopUpdateController,
  startDesktopAutoUpdate,
  type DesktopUpdateState,
} from './desktopUpdater';
import { DesktopUpdateInstallCoordinator } from './desktopUpdateInstallCoordinator';
import {
  DEFAULT_LOCAL_RUNTIME_MANIFEST_URL,
  LocalRuntimeBootstrapController,
  type LocalRuntimeBootstrapState,
} from './openbrain/localRuntimeBootstrap';
import { getOpenBrainBaseDir } from './openbrain/runtime';
import { listSshHosts } from './ssh/sshConfig';
import { connectSsh, disconnectRemote, getRemoteStatus, SshHost } from './remote/remoteSsh';
import {
  deleteManualSshHost,
  listManualSshHosts,
  resolveSshHostForConnect,
  saveManualSshHost,
} from './ssh/manualSshHosts';
import {
  ArchiveCleanupScheduler,
  collectArchiveCleanupInvocations,
  type ArchiveWorkspaceTabsSessionState,
} from './archiveCleanup';
import { startConfigSync, syncAllToTarget, type ConfigSyncPushPayload } from './configSync';
import {
  loadAllSettings,
  normalizeMarkdownContentWidth,
  normalizeMarkdownTextOffset,
  saveSettings,
  SettingsState,
  getSettingsRoot,
  ensureSettingsInitialized,
  migrateUiThemeIdsOnBuiltInUpgrade,
  DEPRECATED_THEME_IDS,
  getLegacyVersionSettingsPath,
  getSettingsVersionTemplateFileName,
  getVersionSettingsPath,
  isSettingsConfigFileName,
  settingsBasenameFromFileName,
  settingsConfigFileNameVariants,
  getIdleSleepPolicy,
} from './settings/settingsStore';
import { AgentSleepInhibitorController } from './agentSleepInhibitor';
import { waitForRuntimeSystemConfig } from './runtimeSystemConfig';
import {
  loadAuthConfig,
  saveAuthConfig,
  clearAuthConfig,
  parseAuthCallbackUrl,
  createAuthConfig,
  normalizeActiveOrgID,
  type AuthConfig,
} from './auth/authStore';
import { isAuthInvalidError } from './auth/authErrors';
import { authFetch, readableNetworkError } from './auth/netFetch';
import {
  discoverGatewayInfo,
  normalizeManualGateway,
  type GatewayInfo,
  type LoginOptions,
} from './auth/gatewayDiscovery';
import {
  deviceVerificationLoginUri,
  pollDeviceToken,
  requestDeviceCode,
  type DeviceCodeSession,
  type DeviceTokenResponse,
} from './auth/deviceCodeAuth';
import {
  loadProfile,
  saveProfile,
  clearProfile,
  createFallbackProfile,
  fetchProfile,
  type UserProfile,
} from './auth/profileStore';
import {
  fetchBillingSubscription,
  type BillingSubscription,
} from './auth/billingStore';
import {
  createLocalEmptyWorkspace,
  createOpenbrainWorkspace,
  createLocalIndexWorkspace,
  listGBrainSourceWorkspaces,
  listWorkspaceTemplates,
  materializeWorkspace,
  registerGBrainSourceForWorkspace,
} from './workspace/openbrainWorkspace';
import {
  applyOpenBrainSourceAction,
  archiveOpenBrainSource,
  createOpenBrainSource,
  getOpenBrainProviderStatus,
  listOpenBrainSources,
  queryOpenBrain as queryOpenBrainProvider,
  removeOpenBrainSourceFromDevice,
} from './openbrain/brainProvider';
import {
  cacheNodeAvatar,
  loadNodesJson,
  upsertNodes,
} from './avatarService';
import {
  loadModelsConfig,
  saveModelsConfig,
  mergeOpenBrainOrgCatalogs,
  type ModelsConfig,
  type OpenBrainCatalog,
  type OpenBrainModelEntry,
} from './models/modelsStore';
import { normalizeModelReasoningControl } from './shared/modelReasoning';
import { fetchDashboardHosts } from './dashboard/dashboardService';
import { buildMarkdownPdfDefaultPath } from './pdfExportPaths';
import {
  initMainI18n,
  mainT,
  setMainI18nLocale,
} from './i18n/main';
import { normalizeDisplayLocale } from './i18n/locales';

const APP_DISPLAY_NAME = 'OpenBrain';
const DESKTOP_UPDATE_STARTUP_BUDGET_MS = 3_000;
const OFFICIAL_OPENBRAIN_HOSTS = new Set([
  'openbrain.chat',
  'app.openbrain.chat',
  'api.op-agent.com',
]);
let pendingLoginGatewayInfo: GatewayInfo | null = null;
let pendingLoginOrgSlug: string | undefined;
let activeDeviceLoginAttempt = 0;

try {
  app.setName(APP_DISPLAY_NAME);
  process.title = APP_DISPLAY_NAME;
} catch (err) {
  console.warn('[app] Failed to apply runtime app identity:', err);
}

type WindowMode = 'local' | 'remote';
type WindowPresentation = 'default' | 'newWindowLanding';

type WindowInfo = {
  id: number;
  sessionId: string;
  label: string;
  mode: WindowMode;
  presentation: WindowPresentation;
  authRequired?: boolean;
  workspaceId: string;
  workspacePath?: string;
  remoteHost?: SshHost;
  active: boolean;
};

type WorkspaceTabSession = {
  id: string;
  label: string;
  kind: WindowMode;
  workspaceId: string;
  workspacePath?: string;
  remoteHost?: SshHost;
  currentDir?: string;
  chatSession?: WorkspaceChatSession;
  openEditorFilePaths?: string[];
};

type WorkspaceChatTabSession = {
  threadID: string;
  path: string;
  title: string;
};

type WorkspaceChatSession = {
  openChats: WorkspaceChatTabSession[];
  selectedThreadID?: string;
};

type WorkspaceTabsSessionState = {
  version: number;
  activeTabId: string;
  tabs: WorkspaceTabSession[];
};

type WorkspaceTabsSessionStore = {
  version: number;
  lastSessionId?: string;
  sessions: Record<string, WorkspaceTabsSessionState>;
};

type PdfExportRemoteSession = {
  hostLabel: string;
  localPort: number;
  remotePort: number;
  wsUrl: string;
  httpUrl: string;
  remoteHome: string;
  workspaceDir: string;
  installDir: string;
};

type MarkdownPdfExportPayload = {
  title: string;
  content: string;
  sourcePath?: string;
  currentDir?: string;
  remoteSession: PdfExportRemoteSession | null;
  baseDir?: string;
  workspaceRootDir?: string;
  agentsRootDir?: string;
  instanceID?: string;
};

type PendingPdfExportSession = {
  payload: MarkdownPdfExportPayload;
  settled: boolean;
  resolveReady: () => void;
  rejectReady: (error: Error) => void;
};

const windowRegistry = new Map<number, { win: BrowserWindow; info: WindowInfo }>();
const pdfExportSessions = new Map<number, PendingPdfExportSession>();
const pendingWindowCloseRequests = new Set<number>();
const approvedWindowCloses = new Set<number>();
const desktopUpdateInstallCoordinator = new DesktopUpdateInstallCoordinator();
let settingsCache: SettingsState | null = null;
let settingsWatcher: FSWatcher | null = null;
let settingsWatchDebounce: NodeJS.Timeout | null = null;
let workspaceTabsSessionCache: WorkspaceTabsSessionStore | null = null;
let workspaceTabsSessionSaveTimer: NodeJS.Timeout | null = null;
let archiveCleanupScheduler: ArchiveCleanupScheduler | null = null;
let localRuntimeBootstrapController: LocalRuntimeBootstrapController | null = null;
let desktopUpdateInstallPending = false;
let appQuitRequested = false;
const agentSleepInhibitor = new AgentSleepInhibitorController({ powerSaveBlocker });

function syncAgentSleepInhibitorFromSettings(settings: SettingsState | null | undefined): void {
  agentSleepInhibitor.setPolicy(
    settings?.system ? getIdleSleepPolicy(settings.system) : 'off',
  );
}

const WORKSPACE_TABS_SESSION_FILENAME = 'workspace-tabs-session.json';

function createId(prefix: string) {
  return `${prefix}-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;
}

function normalizeOptionalString(value: unknown): string | undefined {
  if (typeof value !== 'string') {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed || undefined;
}

function isOfficialOpenBrainHost(hostname: string): boolean {
  const host = hostname.toLowerCase();
  return OFFICIAL_OPENBRAIN_HOSTS.has(host);
}

function isPrivateAuthService(auth: AuthConfig): boolean {
  const candidates = [auth.baseUrl, auth.gateway, auth.aiGateway]
    .map((value) => (value || '').trim())
    .filter(Boolean);
  if (candidates.length === 0) {
    return false;
  }
  return candidates.some((value) => {
    try {
      const hostname = new URL(value).hostname.toLowerCase();
      return !isOfficialOpenBrainHost(hostname);
    } catch {
      return true;
    }
  });
}

function normalizeSshHost(value: unknown): SshHost | undefined {
  if (!value || typeof value !== 'object') {
    return undefined;
  }
  const host = value as Record<string, unknown>;
  const alias = normalizeOptionalString(host.alias);
  if (!alias) {
    return undefined;
  }
  return {
    id: normalizeOptionalString(host.id),
    alias,
    hostname: normalizeOptionalString(host.hostname),
    user: normalizeOptionalString(host.user),
    port: normalizeOptionalString(host.port),
    identityFile: normalizeOptionalString(host.identityFile),
    source: normalizeOptionalString(host.source),
    authMethod:
      host.authMethod === 'agent' || host.authMethod === 'keyFile' || host.authMethod === 'password'
        ? host.authMethod
        : undefined,
    credentialID: normalizeOptionalString(host.credentialID),
    hasPassword: host.hasPassword === true,
    hasPassphrase: host.hasPassphrase === true,
  };
}

function normalizeOptionalNumber(value: unknown): number | undefined {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return undefined;
  }
  return value;
}

function normalizePdfExportRemoteSession(value: unknown): PdfExportRemoteSession | null {
  if (!value || typeof value !== 'object') {
    return null;
  }
  const raw = value as Record<string, unknown>;
  const hostLabel = normalizeOptionalString(raw.hostLabel);
  const localPort = normalizeOptionalNumber(raw.localPort);
  const remotePort = normalizeOptionalNumber(raw.remotePort);
  const wsUrl = normalizeOptionalString(raw.wsUrl);
  const httpUrl = normalizeOptionalString(raw.httpUrl);
  const remoteHome = normalizeOptionalString(raw.remoteHome);
  const workspaceDir = normalizeOptionalString(raw.workspaceDir);
  const installDir = normalizeOptionalString(raw.installDir);
  if (
    !hostLabel ||
    localPort === undefined ||
    remotePort === undefined ||
    !wsUrl ||
    !httpUrl ||
    !remoteHome ||
    !workspaceDir ||
    !installDir
  ) {
    return null;
  }
  return {
    hostLabel,
    localPort,
    remotePort,
    wsUrl,
    httpUrl,
    remoteHome,
    workspaceDir,
    installDir,
  };
}

function normalizeMarkdownPdfExportPayload(value: unknown): MarkdownPdfExportPayload | null {
  if (!value || typeof value !== 'object') {
    return null;
  }
  const raw = value as Record<string, unknown>;
  if (typeof raw.content !== 'string') {
    return null;
  }
  return {
    title: normalizeOptionalString(raw.title) || 'Untitled',
    content: raw.content,
    sourcePath: normalizeOptionalString(raw.sourcePath),
    currentDir: normalizeOptionalString(raw.currentDir),
    remoteSession: normalizePdfExportRemoteSession(raw.remoteSession),
    baseDir: normalizeOptionalString(raw.baseDir),
    workspaceRootDir: normalizeOptionalString(raw.workspaceRootDir),
    agentsRootDir: normalizeOptionalString(raw.agentsRootDir),
    instanceID: normalizeOptionalString(raw.instanceID),
  };
}

function resolveWorkspaceTabLabel(kind: WindowMode, workspacePath?: string, remoteHost?: SshHost): string {
  if (kind === 'remote') {
    return resolveHostLabel(remoteHost);
  }
  if (workspacePath) {
    return path.basename(workspacePath.replace(/[\\/]+$/, '')) || 'Untitled';
  }
  return 'Untitled';
}

function shouldRegenerateWorkspaceLabel(label: string | undefined): boolean {
  return !label || label === 'Untitled';
}

function normalizeWorkspaceChatSession(value: unknown): WorkspaceChatSession | undefined {
  if (!value || typeof value !== 'object') {
    return undefined;
  }

  const raw = value as Record<string, unknown>;
  const openChats = Array.isArray(raw.openChats)
    ? raw.openChats.reduce<WorkspaceChatTabSession[]>((acc, entry) => {
      if (!entry || typeof entry !== 'object') {
        return acc;
      }
      const rawEntry = entry as Record<string, unknown>;
      const threadIDValue = normalizeOptionalString(rawEntry.threadID);
      const pathValue = normalizeOptionalString(rawEntry.path);
      if (!threadIDValue || !pathValue || acc.some((item) => item.threadID === threadIDValue)) {
        return acc;
      }
      const titleValue = normalizeOptionalString(rawEntry.title) || path.basename(pathValue) || pathValue;
      acc.push({
        threadID: threadIDValue,
        path: pathValue,
        title: titleValue,
      });
      return acc;
    }, [])
    : [];

  if (openChats.length === 0) {
    return undefined;
  }

  const selectedThreadID = normalizeOptionalString(raw.selectedThreadID);
  return {
    openChats,
    ...(selectedThreadID && openChats.some((entry) => entry.threadID === selectedThreadID)
      ? { selectedThreadID }
      : {}),
  };
}

function createFallbackWorkspaceTabSession(): WorkspaceTabSession {
  return {
    id: createId('tab'),
    kind: 'local',
    label: 'Untitled',
    workspaceId: hashWorkspaceId(`local:empty:${Date.now()}`),
  };
}

function normalizeWorkspaceTabsSession(session: unknown): WorkspaceTabsSessionState {
  const fallback = createFallbackWorkspaceTabSession();
  if (!session || typeof session !== 'object') {
    return { version: 1, activeTabId: fallback.id, tabs: [fallback] };
  }

  const raw = session as Record<string, unknown>;
  const seen = new Set<string>();
  const tabs = Array.isArray(raw.tabs)
    ? raw.tabs.reduce<WorkspaceTabSession[]>((acc, value) => {
      if (!value || typeof value !== 'object') {
        return acc;
      }
      const entry = value as Record<string, unknown>;
      const id = normalizeOptionalString(entry.id);
      if (!id || seen.has(id)) {
        return acc;
      }
      const kind: WindowMode = entry.kind === 'remote' ? 'remote' : 'local';
      const rawWorkspacePath = normalizeOptionalString(entry.workspacePath);
      const currentDir = normalizeOptionalString(entry.currentDir);
      const workspacePath = rawWorkspacePath || (kind === 'local' ? currentDir : undefined);
      const remoteHost = normalizeSshHost(entry.remoteHost);
      const workspaceId = normalizeOptionalString(entry.workspaceId)
        || resolveWorkspaceId(kind, workspacePath, remoteHost);
      const rawLabel = normalizeOptionalString(entry.label);
      const label = shouldRegenerateWorkspaceLabel(rawLabel)
        ? resolveWorkspaceTabLabel(kind, workspacePath, remoteHost)
        : rawLabel || resolveWorkspaceTabLabel(kind, workspacePath, remoteHost);
      const chatSession = normalizeWorkspaceChatSession(entry.chatSession);
      const openEditorFilePaths = Array.isArray(entry.openEditorFilePaths)
        ? Array.from(new Set(
          entry.openEditorFilePaths
            .map((value) => normalizeOptionalString(value))
            .filter((value): value is string => Boolean(value))
        ))
        : undefined;
      seen.add(id);
      acc.push({
        id,
        label,
        kind,
        workspaceId,
        workspacePath,
        remoteHost,
        currentDir,
        ...(chatSession ? { chatSession } : {}),
        ...(openEditorFilePaths && openEditorFilePaths.length > 0 ? { openEditorFilePaths } : {}),
      });
      return acc;
    }, [])
    : [];

  if (tabs.length === 0) {
    return { version: 1, activeTabId: fallback.id, tabs: [fallback] };
  }

  const activeTabId = normalizeOptionalString(raw.activeTabId);
  return {
    version: 1,
    activeTabId: activeTabId && tabs.some((tab) => tab.id === activeTabId) ? activeTabId : tabs[0].id,
    tabs,
  };
}

function normalizeWorkspaceTabsSessionStore(value: unknown): WorkspaceTabsSessionStore {
  const fallback: WorkspaceTabsSessionStore = { version: 1, sessions: {} };
  if (!value || typeof value !== 'object') {
    return fallback;
  }

  const raw = value as Record<string, unknown>;
  const sessions: Record<string, WorkspaceTabsSessionState> = {};
  const source = raw.sessions;
  if (source && typeof source === 'object' && !Array.isArray(source)) {
    for (const [sessionId, session] of Object.entries(source as Record<string, unknown>)) {
      const key = normalizeOptionalString(sessionId);
      if (!key) {
        continue;
      }
      sessions[key] = normalizeWorkspaceTabsSession(session);
    }
  }

  const lastSessionId = normalizeOptionalString(raw.lastSessionId);
  return {
    version: 1,
    lastSessionId: lastSessionId && sessions[lastSessionId] ? lastSessionId : Object.keys(sessions)[0],
    sessions,
  };
}

function getWorkspaceTabsSessionPath(homeDir: string): string {
  return path.join(getSettingsRoot(homeDir), 'state', WORKSPACE_TABS_SESSION_FILENAME);
}

async function ensureWorkspaceTabsSessionCache(): Promise<WorkspaceTabsSessionStore> {
  if (workspaceTabsSessionCache) {
    return workspaceTabsSessionCache;
  }
  const homeDir = app.getPath('home');
  try {
    const raw = await fs.readFile(getWorkspaceTabsSessionPath(homeDir), 'utf8');
    workspaceTabsSessionCache = normalizeWorkspaceTabsSessionStore(parseJsonc(raw));
  } catch {
    workspaceTabsSessionCache = { version: 1, sessions: {} };
  }
  return workspaceTabsSessionCache;
}

async function persistWorkspaceTabsSessionCache(): Promise<void> {
  const cache = await ensureWorkspaceTabsSessionCache();
  const homeDir = app.getPath('home');
  const filePath = getWorkspaceTabsSessionPath(homeDir);
  await fs.mkdir(path.dirname(filePath), { recursive: true });
  await fs.writeFile(filePath, JSON.stringify(cache, null, 2), 'utf8');
}

function scheduleWorkspaceTabsSessionSave() {
  if (workspaceTabsSessionSaveTimer) {
    clearTimeout(workspaceTabsSessionSaveTimer);
  }
  workspaceTabsSessionSaveTimer = setTimeout(() => {
    workspaceTabsSessionSaveTimer = null;
    persistWorkspaceTabsSessionCache().catch((err) => {
      console.warn('Failed to persist workspace tabs session:', err);
    });
  }, 200);
}

async function updateWorkspaceTabsSession(sessionId: string, session: unknown): Promise<WorkspaceTabsSessionState> {
  const cache = await ensureWorkspaceTabsSessionCache();
  const normalized = normalizeWorkspaceTabsSession(session);
  cache.sessions[sessionId] = normalized;
  const record = Array.from(windowRegistry.values()).find((entry) => entry.info.sessionId === sessionId);
  const activeTab = getActiveWorkspaceTabFromSession(normalized);
  if (record && activeTab) {
    const workspacePath = activeTab.kind === 'local'
      ? activeTab.workspacePath || activeTab.currentDir
      : activeTab.workspacePath;
    record.info.mode = activeTab.kind;
    record.info.workspacePath = workspacePath;
    record.info.remoteHost = activeTab.remoteHost;
    record.info.workspaceId = activeTab.workspaceId;
    record.info.label = shouldRegenerateWorkspaceLabel(activeTab.label)
      ? resolveWorkspaceTabLabel(activeTab.kind, workspacePath, activeTab.remoteHost)
      : activeTab.label;
    broadcastWindowList();
  }
  scheduleWorkspaceTabsSessionSave();
  return normalized;
}

async function getWorkspaceTabsSession(sessionId: string): Promise<WorkspaceTabsSessionState | null> {
  const cache = await ensureWorkspaceTabsSessionCache();
  return cache.sessions[sessionId] || null;
}

async function markLastWorkspaceTabsSession(sessionId: string): Promise<void> {
  const cache = await ensureWorkspaceTabsSessionCache();
  if (cache.lastSessionId === sessionId) {
    return;
  }
  cache.lastSessionId = sessionId;
  scheduleWorkspaceTabsSessionSave();
}

async function resolveLastWorkspaceTabsSession(): Promise<{ sessionId?: string; session?: WorkspaceTabsSessionState }> {
  const cache = await ensureWorkspaceTabsSessionCache();
  const sessionId = cache.lastSessionId;
  if (!sessionId) {
    return {};
  }
  const session = cache.sessions[sessionId];
  return session ? { sessionId, session } : {};
}

function getActiveWorkspaceTabFromSession(session: WorkspaceTabsSessionState | null | undefined): WorkspaceTabSession | null {
  if (!session || session.tabs.length === 0) {
    return null;
  }
  return session.tabs.find((tab) => tab.id === session.activeTabId) || session.tabs[0] || null;
}

function ensureArchiveCleanupScheduler() {
  if (archiveCleanupScheduler) {
    return archiveCleanupScheduler;
  }
  archiveCleanupScheduler = new ArchiveCleanupScheduler({
    collectInvocations: () => {
      const cache = workspaceTabsSessionCache;
      return collectArchiveCleanupInvocations({
        windows: Array.from(windowRegistry.values()).map(({ info }) => ({
          id: info.id,
          sessionId: info.sessionId,
          mode: info.mode,
          workspacePath: info.workspacePath,
        })),
        sessionsById: (cache?.sessions || {}) as Record<string, ArchiveWorkspaceTabsSessionState | null | undefined>,
        getRemoteSession: (windowId, tabId) => getRemoteStatus(windowId, tabId),
        localWsUrl: LOCAL_RUNTIME_WS_URL,
      });
    },
  });
  return archiveCleanupScheduler;
}

function scheduleArchiveCleanupSoon(delayMs?: number) {
  ensureArchiveCleanupScheduler().scheduleSoon(delayMs);
}

function broadcastConfigSyncPush(payload: ConfigSyncPushPayload) {
  for (const win of BrowserWindow.getAllWindows()) {
    if (!win.isDestroyed()) {
      win.webContents.send('configSync:push', payload);
    }
  }
}

function broadcastSettingsChanged(settings: SettingsState) {
  for (const win of BrowserWindow.getAllWindows()) {
    if (!win.isDestroyed()) {
      win.webContents.send('settings:changed', settings);
    }
  }
}

function broadcastRuntimeBootstrapChanged(snapshot: LocalRuntimeBootstrapState) {
  for (const win of BrowserWindow.getAllWindows()) {
    if (!win.isDestroyed()) {
      win.webContents.send('runtimeBootstrap:changed', snapshot);
    }
  }
}

function broadcastDesktopUpdateChanged(snapshot: DesktopUpdateState) {
  for (const win of BrowserWindow.getAllWindows()) {
    if (!win.isDestroyed()) {
      win.webContents.send('desktopUpdate:changed', snapshot);
    }
  }
}

function requestWindowPrepareClose(win: BrowserWindow) {
  if (win.isDestroyed() || approvedWindowCloses.has(win.id) || pendingWindowCloseRequests.has(win.id)) {
    return;
  }
  pendingWindowCloseRequests.add(win.id);
  win.webContents.send('window:prepareClose');
}

function maybeFinalizeDesktopUpdateInstall() {
  if (!desktopUpdateInstallPending || desktopUpdateInstallCoordinator.isActive()) {
    return;
  }
  const result = getDesktopUpdateController().finalizeInstall();
  if (!result.success) {
    desktopUpdateInstallPending = false;
    desktopUpdateInstallCoordinator.reset();
  }
}

function requestDesktopUpdateInstall() {
  if (desktopUpdateInstallPending) {
    return { success: true };
  }

  const controller = getDesktopUpdateController();
  const result = controller.beginInstall();
  if (!result.success) {
    return result;
  }

  desktopUpdateInstallPending = true;
  const plan = desktopUpdateInstallCoordinator.planInstall(
    windowRegistry.keys(),
    pendingWindowCloseRequests,
  );

  if (plan.shouldInstallImmediately) {
    maybeFinalizeDesktopUpdateInstall();
    return { success: true };
  }

  for (const windowId of plan.requestCloseWindowIds) {
    const record = getWindowRecordById(windowId);
    if (!record) {
      continue;
    }
    requestWindowPrepareClose(record.win);
  }

  return { success: true };
}

const SETTINGS_WATCH_DEBOUNCE_MS = 300;
const SETTINGS_WATCH_BASENAMES = ['theme', 'markdown-themes', 'code-themes', 'ui'];
const SETTINGS_WATCH_TARGETS = new Set(SETTINGS_WATCH_BASENAMES.flatMap(settingsConfigFileNameVariants));
const REQUIRED_SETTINGS_WATCH_BASENAMES = new Set(['theme', 'ui']);
const SETTINGS_WATCH_RETRY_LIMIT = 5;

function parseSettingsJsonc<T>(data: string): T | null {
  const errors: ParseError[] = [];
  const parsed = parseJsonc(data, errors, {
    allowTrailingComma: true,
    disallowComments: false,
  });
  if (errors.length > 0 || parsed === undefined || parsed === null) {
    return null;
  }
  return parsed as T;
}

async function areWatchedSettingsFilesStable(homeDir: string): Promise<boolean> {
  const settingsRoot = getSettingsRoot(homeDir);
  for (const basename of SETTINGS_WATCH_BASENAMES) {
    const activePath = await resolveSettingsVariantPath(settingsRoot, basename);
    if (!activePath) {
      if (REQUIRED_SETTINGS_WATCH_BASENAMES.has(basename)) {
        return false;
      }
      continue;
    }
    const parsed = parseSettingsJsonc(await fs.readFile(activePath, 'utf8'));
    if (!parsed) {
      return false;
    }
  }
  return true;
}

async function reloadSettingsAndBroadcast(homeDir: string, retryCount = 0) {
  if (!(await areWatchedSettingsFilesStable(homeDir))) {
    if (retryCount < SETTINGS_WATCH_RETRY_LIMIT) {
      scheduleSettingsReload(homeDir, retryCount + 1);
    }
    return;
  }
  settingsCache = await loadAllSettings(homeDir);
  await persistThemeBackground(settingsCache);
  applySettingsDisplayLocale(settingsCache);
  syncAgentSleepInhibitorFromSettings(settingsCache);
  broadcastSettingsChanged(settingsCache);
}

function applySettingsDisplayLocale(settings: SettingsState): void {
  const locale = normalizeDisplayLocale(
    settings.ui?.displayLocale,
    app.getLocale(),
  );
  setMainI18nLocale(locale);
  setupMacDockMenu();
}

function scheduleSettingsReload(homeDir: string, retryCount = 0) {
  if (settingsWatchDebounce) {
    clearTimeout(settingsWatchDebounce);
  }
  settingsWatchDebounce = setTimeout(() => {
    settingsWatchDebounce = null;
    reloadSettingsAndBroadcast(homeDir, retryCount).catch((err) => {
      console.warn('[settingsWatch] failed to reload settings:', err);
    });
  }, SETTINGS_WATCH_DEBOUNCE_MS);
}

async function startSettingsWatch(homeDir: string) {
  stopSettingsWatch();
  const settingsRoot = getSettingsRoot(homeDir);
  await fs.mkdir(settingsRoot, { recursive: true });
  settingsWatcher = fsWatch(settingsRoot, (eventType, filename) => {
    if (!filename) {
      return;
    }
    const name = filename.toString();
    if (!isSettingsConfigFileName(name) || !SETTINGS_WATCH_TARGETS.has(name)) {
      return;
    }
    if (eventType === 'rename' || eventType === 'change') {
      scheduleSettingsReload(homeDir);
    }
  });
  settingsWatcher.on('error', (err) => {
    console.warn('[settingsWatch] watcher error:', err);
  });
}

function stopSettingsWatch() {
  if (settingsWatcher) {
    try {
      settingsWatcher.close();
    } catch {
      // ignore
    }
    settingsWatcher = null;
  }
  if (settingsWatchDebounce) {
    clearTimeout(settingsWatchDebounce);
    settingsWatchDebounce = null;
  }
}

const isDev = process.env.NODE_ENV === 'development' || !app.isPackaged;
const devServerUrl = (process.env.OPENBRAIN_DEV_SERVER_URL || '').trim() || 'http://localhost:5173';
const shouldOpenDevTools = process.env.OPENBRAIN_DESKTOP_OPEN_DEVTOOLS === '1';
const shouldLogRendererConsole = isDev && process.env.OPENBRAIN_DESKTOP_LOG_RENDERER !== '0';
const PDF_EXPORT_VIEW = 'markdown-pdf-export';
const PDF_EXPORT_READY_TIMEOUT_MS = 30_000;
// Match VS Code's default custom titlebar height.
// (VS Code uses DEFAULT_CUSTOM_TITLEBAR_HEIGHT = 35)
const CUSTOM_TITLEBAR_HEIGHT = 35;
const LOCAL_OPENBRAIN_SERVER_PORT = 19530;
const LOCAL_RUNTIME_WS_URL = `ws://127.0.0.1:${LOCAL_OPENBRAIN_SERVER_PORT}/ws`;

type AppIconFormat = 'png' | 'icns';

const resolvedAppIconPaths: Partial<Record<AppIconFormat, string | null>> = {};
const loggedAppIconPaths = new Set<string>();
let hasLoggedMissingAppIcon = false;

function attachDevWindowDiagnostics(win: BrowserWindow, label: string): void {
  if (!isDev) {
    return;
  }

  if (shouldLogRendererConsole) {
    const levelNames = ['verbose', 'info', 'warning', 'error'];
    win.webContents.on('console-message', (_event, level, message, line, sourceId) => {
      const levelName = levelNames[level] ?? `level-${level}`;
      console.log(`[renderer:${label}:${win.id}:${levelName}] ${message}`, {
        sourceId,
        line,
      });
    });
  }

  win.webContents.on('render-process-gone', (_event, details) => {
    console.error(`[renderer:${label}:${win.id}] render-process-gone`, details);
  });

  win.webContents.on('unresponsive', () => {
    console.warn(`[renderer:${label}:${win.id}] unresponsive`);
  });

  win.webContents.on('responsive', () => {
    console.warn(`[renderer:${label}:${win.id}] responsive`);
  });
}

function resolveAppIconPath(format: AppIconFormat): string | null {
  const fileName = `icon.${format}`;
  const candidates: string[] = [];

  try {
    candidates.push(path.join(app.getAppPath(), 'build', fileName));
  } catch {
    // ignore
  }

  if ((process as any).resourcesPath) {
    const resourcesPath = (process as any).resourcesPath;
    candidates.push(path.join(resourcesPath, 'build', fileName));
    candidates.push(path.join(resourcesPath, fileName));
  }

  candidates.push(path.join(__dirname, '../../build', fileName));

  for (const candidate of candidates) {
    if (existsSync(candidate)) {
      return candidate;
    }
  }

  return null;
}

function getAppIconPath(preferredFormats: AppIconFormat[] = ['png', 'icns']): string | undefined {
  const orderedFormats: AppIconFormat[] = Array.from(new Set<AppIconFormat>([...preferredFormats, 'png', 'icns']));

  for (const format of orderedFormats) {
    if (resolvedAppIconPaths[format] === undefined) {
      resolvedAppIconPaths[format] = resolveAppIconPath(format);
    }

    const candidate = resolvedAppIconPaths[format];
    if (candidate) {
      if (!loggedAppIconPaths.has(candidate)) {
        console.log(`[icon] Using app icon: ${candidate}`);
        loggedAppIconPaths.add(candidate);
      }
      return candidate;
    }
  }

  if (!hasLoggedMissingAppIcon) {
    console.warn('[icon] App icon not found; using Electron default icon.');
    hasLoggedMissingAppIcon = true;
  }
  return undefined;
}

function applyAppIcon() {
  if (process.platform !== 'darwin' || !app.dock) {
    return;
  }

  const dockIconPath = getAppIconPath(['png']);
  if (!dockIconPath) {
    return;
  }

  try {
    const iconImage = nativeImage.createFromPath(dockIconPath);
    if (iconImage.isEmpty()) {
      console.warn(`[icon] Failed to decode app icon: ${dockIconPath}`);
      return;
    }
    app.dock.setIcon(iconImage);
  } catch (err) {
    console.warn('[icon] Failed to apply Dock icon:', err);
  }
}

function isTahoeOrNewerDarwin(darwinRelease: string): boolean {
  // VS Code uses a helper to detect "Tahoe or newer" to adjust traffic light size.
  // Here we approximate using Darwin major version. macOS 15 = Darwin 24.x, macOS 26 = Darwin 25.x.
  const major = Number.parseInt(darwinRelease.split('.')[0] ?? '0', 10);
  return Number.isFinite(major) && major >= 25;
}

function updateMacWindowButtonsForTitlebar(win: BrowserWindow, titlebarHeight: number) {
  // Mirror VS Code behavior:
  // - compute offset so traffic lights are centered vertically
  // - also offset x to keep equal distance from window frame in both directions
  const buttonHeight = isTahoeOrNewerDarwin(release()) ? 14 : 16;
  const offset = Math.floor((titlebarHeight - buttonHeight) / 2);
  if (!offset) {
    win.setWindowButtonPosition(null);
  } else {
    win.setWindowButtonPosition({ x: offset + 1, y: offset });
  }
}

function hashWorkspaceId(value: string): string {
  return createHash('sha1').update(value).digest('hex').slice(0, 12);
}

function resolveHostLabel(host?: SshHost): string {
  if (!host) {
    return 'remote';
  }
  if (host.user && host.hostname) {
    return `${host.user}@${host.hostname}`;
  }
  if (host.hostname) {
    return host.hostname;
  }
  return host.alias;
}

function resolveWorkspaceId(mode: WindowMode, workspacePath?: string, remoteHost?: SshHost): string {
  if (mode === 'remote') {
    const remoteIdentity = remoteHost?.id
      ? `${remoteHost.source || 'remote'}:${remoteHost.id}`
      : resolveHostLabel(remoteHost);
    return hashWorkspaceId(`remote:${remoteIdentity}`);
  }
  if (workspacePath) {
    return hashWorkspaceId(`local:${workspacePath}`);
  }
  return hashWorkspaceId(`local:empty:${Date.now()}`);
}

function resolveWindowLabel(mode: WindowMode, workspacePath?: string, remoteHost?: SshHost): string {
  if (mode === 'remote') {
    return resolveHostLabel(remoteHost);
  }
  if (workspacePath) {
    return path.basename(workspacePath.replace(/[\\/]+$/, '')) || 'Untitled';
  }
  return 'Untitled';
}

type OpenBrainCatalogResponse = {
  data?: Array<{
    id?: string;
    name?: string;
    api?: string;
    reasoning?: boolean;
    reasoning_control?: string;
    reasoning_levels?: string[];
    context_windows?: number[];
    default_context_window?: number;
    service_tiers?: string[];
    max_output_tokens?: number;
    availability?: {
      available?: boolean;
      provider_count?: number;
    };
  }>;
};

type OpenBrainPoliciesResponse = {
  modelSelection?: {
    defaultChatModelID?: string;
    defaultChatThinkingLevel?: string;
    defaultInlineCompletionModelID?: string;
    defaultInlineCompletionThinkingLevel?: string;
  };
};

type AuthOrgEntry = {
  id: string;
  slug?: string;
  name?: string;
};

type OpenBrainOrgEntry = {
  id: string;
  name: string;
};

const FALLBACK_DEFAULT_ORG_ID = 'cloud';

function workspaceCreationOrgTargets(_auth: AuthConfig, orgs: AuthOrgEntry[]): AuthOrgEntry[] {
  return orgs;
}

function parseOpenBrainCatalogApi(
  value: string | undefined
): 'openai-completions' | 'openai-responses' | 'anthropic-messages' | 'gemini-native' | null {
  const normalized = (value || '').trim();
  switch (normalized) {
    case 'openai-completions':
    case 'openai-responses':
    case 'anthropic-messages':
    case 'gemini-native':
      return normalized;
    default:
      return null;
  }
}

async function fetchAuthOrgs(gateway: string, token: string): Promise<AuthOrgEntry[]> {
  const base = (gateway || '').trim();
  const authToken = (token || '').trim();
  if (!base || !authToken) {
    return [];
  }
  const url = `${base.replace(/\/$/, '')}/v1/user/orgs`;
  const res = await authFetch(url, {
    method: 'GET',
    headers: {
      Authorization: `Bearer ${authToken}`,
      Accept: 'application/json',
    },
  });
  if (!res.ok) {
    return [];
  }
  const payload = (await res.json().catch(() => null)) as { orgs?: Array<{ id?: string; slug?: string; name?: string }> } | null;
  if (!Array.isArray(payload?.orgs)) {
    return [];
  }
  const out: AuthOrgEntry[] = [];
  for (const org of payload.orgs) {
    const id = normalizeActiveOrgID(org.id);
    if (!id) {
      continue;
    }
    out.push({
      id,
      slug: (org.slug || '').trim() || id,
      name: (org.name || '').trim() || undefined,
    });
  }
  return out;
}

function normalizeOrganizationCode(raw: unknown): string | undefined {
  const value = typeof raw === 'string' ? raw.trim().toLowerCase() : '';
  if (!value) {
    return undefined;
  }
  if (value.startsWith('org-')) {
    throw new Error('Organization code should not include an org- prefix.');
  }
  if (!/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(value)) {
    throw new Error('Organization code must be 1-63 lowercase letters, digits, or hyphens without a trailing hyphen.');
  }
  return value;
}

async function resolveDefaultActiveOrg(config: AuthConfig): Promise<AuthConfig> {
  let orgs: AuthOrgEntry[] = [];
  try {
    orgs = await fetchAuthOrgs(config.gateway, config.token);
  } catch {
    return config;
  }
  const activeOrgID = normalizeActiveOrgID(config.activeOrgID);
  if (activeOrgID && orgs.some((org) => org.id === activeOrgID)) {
    return {
      ...config,
      activeOrgID,
      activeOrgName: (config.activeOrgName || '').trim() || orgs.find((org) => org.id === activeOrgID)?.name || activeOrgID,
    };
  }
  if (orgs.length === 0) {
    if (!config.activeOrgID && !config.activeOrgName) {
      return config;
    }
    return {
      ...config,
      activeOrgID: undefined,
      activeOrgName: undefined,
      updatedAt: Date.now(),
    };
  }
  const defaultOrgID = normalizeActiveOrgID(config.defaultOrgID);
  const first = (defaultOrgID ? orgs.find((org) => org.id === defaultOrgID) : undefined) || orgs[0];
  return {
    ...config,
    activeOrgID: first.id,
    activeOrgName: first.name || first.id,
    updatedAt: Date.now(),
  };
}

async function ensureDefaultActiveOrg(homeDir: string, config: AuthConfig): Promise<AuthConfig> {
  const next = await resolveDefaultActiveOrg(config);
  if (JSON.stringify(next) !== JSON.stringify(config)) {
    await saveAuthConfig(homeDir, next);
  }
  return next;
}

async function ensureRequestedActiveOrg(homeDir: string, config: AuthConfig, orgSlug?: string): Promise<AuthConfig> {
  const requestedSlug = normalizeOrganizationCode(orgSlug);
  if (!requestedSlug) {
    return ensureDefaultActiveOrg(homeDir, config);
  }
  let orgs: AuthOrgEntry[] = [];
  try {
    orgs = await fetchAuthOrgs(config.gateway, config.token);
  } catch (err) {
    console.warn('[Auth] Failed to fetch organizations for organization code:', readableNetworkError(err));
    throw new Error('Could not verify organization code right now. Check your network and try again.');
  }
  const matched = orgs.find((org) => org.slug === requestedSlug || org.id === requestedSlug);
  if (!matched) {
    throw new Error(`Your account is not a member of organization "${requestedSlug}".`);
  }
  const next: AuthConfig = {
    ...config,
    activeOrgID: matched.id,
    activeOrgName: matched.name || matched.id,
    updatedAt: Date.now(),
  };
  await saveAuthConfig(homeDir, next);
  return next;
}

function normalizeOpenBrainCatalogStrategies(payload: OpenBrainPoliciesResponse | null | undefined): OpenBrainCatalog['strategies'] {
  const selection = payload?.modelSelection;
  if (!selection) {
    return undefined;
  }
  const defaultChatModelID = (selection.defaultChatModelID || '').trim();
  const defaultChatThinkingLevel = (selection.defaultChatThinkingLevel || '').trim();
  const defaultInlineCompletionModelID = (selection.defaultInlineCompletionModelID || '').trim();
  const defaultInlineCompletionThinkingLevel = (selection.defaultInlineCompletionThinkingLevel || '').trim();
  if (!defaultChatModelID && !defaultInlineCompletionModelID) {
    return undefined;
  }
  return {
    auto: {
      ...(defaultChatModelID ? { defaultChatModelID } : {}),
      ...(defaultChatThinkingLevel ? { defaultChatThinkingLevel } : {}),
      ...(defaultInlineCompletionModelID ? { defaultInlineCompletionModelID } : {}),
      ...(defaultInlineCompletionThinkingLevel ? { defaultInlineCompletionThinkingLevel } : {}),
    },
  };
}

async function fetchOptionalOpenBrainPolicies(url: string, headers: Record<string, string>): Promise<OpenBrainPoliciesResponse | null> {
  const res = await fetch(url, { headers });
  if (res.status === 404) {
    return null;
  }
  if (!res.ok) {
    throw new Error(`OpenBrain policies responded ${res.status}`);
  }
  return res.json() as Promise<OpenBrainPoliciesResponse>;
}

async function fetchOpenBrainModelsPayload(url: string, headers: Record<string, string>): Promise<OpenBrainCatalogResponse> {
  const res = await fetch(url, { headers });
  if (!res.ok) {
    throw new Error(`OpenBrain catalog responded ${res.status}`);
  }
  return res.json() as Promise<OpenBrainCatalogResponse>;
}

function appendGatewayPath(baseUrl: string, suffix: string): string {
  const base = baseUrl.trim().replace(/\/+$/, '');
  const path = suffix.trim().replace(/^\/+/, '');
  return `${base}/${path}`;
}

function normalizeReasoningLevels(raw: unknown): string[] | undefined {
  if (!Array.isArray(raw)) {
    return undefined;
  }
  const levels = raw
    .map((level) => (level || '').trim())
    .filter(Boolean);
  return levels.length > 0 ? levels : undefined;
}

function normalizePositiveInteger(raw: unknown): number | undefined {
  return typeof raw === 'number' && Number.isInteger(raw) && raw > 0 ? raw : undefined;
}

function normalizePositiveIntegerList(raw: unknown): number[] | undefined {
  if (!Array.isArray(raw)) {
    return undefined;
  }
  const seen = new Set<number>();
  const out: number[] = [];
  for (const value of raw) {
    const normalized = normalizePositiveInteger(value);
    if (!normalized || seen.has(normalized)) {
      continue;
    }
    seen.add(normalized);
    out.push(normalized);
  }
  out.sort((a, b) => a - b);
  return out.length > 0 ? out : undefined;
}

function normalizeServiceTierList(raw: unknown): Array<'priority' | 'flex'> | undefined {
  if (!Array.isArray(raw)) {
    return undefined;
  }
  const seen = new Set<'priority' | 'flex'>();
  const out: Array<'priority' | 'flex'> = [];
  for (const value of raw) {
    if (typeof value !== 'string') {
      continue;
    }
    const normalized = value.trim().toLowerCase();
    const tier = normalized === 'priority'
      ? 'priority'
      : normalized === 'flex'
        ? 'flex'
        : null;
    if (!tier || seen.has(tier)) {
      continue;
    }
    seen.add(tier);
    out.push(tier);
  }
  return out.length > 0 ? out : undefined;
}

async function fetchOpenBrainCatalogForOrg(auth: AuthConfig, org: OpenBrainOrgEntry): Promise<OpenBrainCatalog> {
  const aiGateway = (auth.aiGateway || '').trim();
  if (!aiGateway) {
    return { providerKey: org.id, providerLabel: org.name, models: [], strategies: undefined };
  }
  const headers: Record<string, string> = {
    Authorization: `Bearer ${auth.token}`,
  };
  const defaultOrgID = normalizeActiveOrgID(auth.defaultOrgID) || FALLBACK_DEFAULT_ORG_ID;
  if (org.id !== defaultOrgID) {
    headers['X-Org-ID'] = org.id;
  }
  const modelsUrl = appendGatewayPath(aiGateway, '/v1/models');
  const policiesUrl = appendGatewayPath(aiGateway, '/v1/policies');
  const [modelsPayload, policiesPayload] = await Promise.all([
    fetchOpenBrainModelsPayload(modelsUrl, headers),
    fetchOptionalOpenBrainPolicies(policiesUrl, headers),
  ]);
  if (!Array.isArray(modelsPayload.data)) {
    return {
      providerKey: org.id,
      providerLabel: org.name,
      models: [],
      strategies: normalizeOpenBrainCatalogStrategies(policiesPayload),
    };
  }
  const models: OpenBrainModelEntry[] = [];
  for (const item of modelsPayload.data) {
    const id = (item?.id || '').trim();
    if (!id) {
      continue;
    }
    if (item?.availability?.available === false) {
      continue;
    }
    const reasoningLevels = normalizeReasoningLevels(item?.reasoning_levels);
    const api = parseOpenBrainCatalogApi((item?.api || '').trim());
    if (!api) {
      continue;
    }
    models.push({
      id,
      label: (item?.name || '').trim() || undefined,
      api,
      reasoning: item?.reasoning === true || Boolean(reasoningLevels?.length),
      reasoningControl: normalizeModelReasoningControl(item?.reasoning_control),
      reasoningLevels,
      contextWindows: normalizePositiveIntegerList(item?.context_windows),
      defaultContextWindow: normalizePositiveInteger(item?.default_context_window),
      serviceTiers: normalizeServiceTierList(item?.service_tiers),
      maxOutputTokens: normalizePositiveInteger(item?.max_output_tokens),
    });
  }
  return {
    providerKey: org.id,
    providerLabel: org.name,
    models,
    strategies: normalizeOpenBrainCatalogStrategies(policiesPayload),
  };
}

async function fetchOpenBrainOrgCatalogs(auth: AuthConfig): Promise<OpenBrainCatalog[]> {
  const orgs = await fetchAuthOrgs(auth.gateway, auth.token);
  const privateService = isPrivateAuthService(auth);
  const activeOrgID = normalizeActiveOrgID(auth.activeOrgID);
  const defaultOrgID = normalizeActiveOrgID(auth.defaultOrgID) || FALLBACK_DEFAULT_ORG_ID;
  const defaultOrgName = (auth.defaultOrgName || '').trim() || 'Cloud';
  const activeOrg = activeOrgID ? orgs.find((org) => org.id === activeOrgID) : undefined;
  const privateOrg = activeOrg || orgs.find((org) => org.id === defaultOrgID) || orgs[0] || null;
  const orgEntries: OpenBrainOrgEntry[] = privateService
    ? [{
        id: privateOrg?.id || defaultOrgID,
        name: privateOrg?.name || privateOrg?.id || defaultOrgName,
      }]
    : (orgs.length > 0
        ? orgs.map((org) => ({ id: org.id, name: org.name || org.id }))
        : [{ id: defaultOrgID, name: defaultOrgName }]);
  const catalogs = await Promise.all(
    orgEntries.map((org) =>
      fetchOpenBrainCatalogForOrg(auth, org).catch((error) => {
        console.warn(`[models] org catalog fetch failed for ${org.id}:`, (error as Error).message);
        return { providerKey: org.id, providerLabel: org.name, models: [], strategies: undefined };
      }),
    ),
  );
  return catalogs.filter((catalog) => catalog.models.length > 0);
}

function listWindows(): WindowInfo[] {
  return Array.from(windowRegistry.values()).map(({ info }) => ({ ...info }));
}

function getWindowRecordById(windowId: number) {
  return windowRegistry.get(windowId) || null;
}

function getWindowRecordByWebContents(contents: WebContents) {
  const win = BrowserWindow.fromWebContents(contents);
  if (!win) {
    return null;
  }
  return windowRegistry.get(win.id) || null;
}

function settlePdfExportSession(contentsId: number, error?: Error) {
  const session = pdfExportSessions.get(contentsId);
  if (!session || session.settled) {
    return;
  }
  session.settled = true;
  if (error) {
    session.rejectReady(error);
    return;
  }
  session.resolveReady();
}

async function createMarkdownPdfExportWindow(payload: MarkdownPdfExportPayload): Promise<{
  win: BrowserWindow;
  readyPromise: Promise<void>;
}> {
  const win = new BrowserWindow({
    show: false,
    width: 900,
    height: 1200,
    backgroundColor: '#ffffff',
    paintWhenInitiallyHidden: true,
    webPreferences: {
      nodeIntegration: false,
      contextIsolation: true,
      webSecurity: !isDev,
      preload: path.join(__dirname, 'preload.js'),
      backgroundThrottling: false,
    },
  });

  const contentsId = win.webContents.id;
  const readyPromise = new Promise<void>((resolve, reject) => {
    pdfExportSessions.set(contentsId, {
      payload,
      settled: false,
      resolveReady: resolve,
      rejectReady: reject,
    });
  });

  win.on('closed', () => {
    settlePdfExportSession(contentsId, new Error('PDF export window closed before rendering completed'));
    pdfExportSessions.delete(contentsId);
  });

  win.webContents.on('did-fail-load', (_event, errorCode, errorDescription, validatedURL, isMainFrame) => {
    if (!isMainFrame) {
      return;
    }
    settlePdfExportSession(
      contentsId,
      new Error(`PDF export renderer failed to load (${errorCode}): ${errorDescription} (${validatedURL || 'unknown'})`)
    );
  });

  win.webContents.on('render-process-gone', (_event, details) => {
    settlePdfExportSession(
      contentsId,
      new Error(`PDF export renderer exited unexpectedly: ${details.reason}`)
    );
  });

  await loadRendererView(win, { view: PDF_EXPORT_VIEW });

  return { win, readyPromise };
}

function resolvePdfSaveDialogDefaultPath(defaultPath: string): string {
  const trimmed = defaultPath.trim();
  if (!trimmed) {
    return getLocalPdfFallbackPath('Untitled.pdf');
  }
  const parentDir = path.dirname(trimmed);
  if (path.isAbsolute(trimmed) && existsSync(parentDir)) {
    return trimmed;
  }
  return getLocalPdfFallbackPath(trimmed);
}

function ensurePdfExtension(filePath: string): string {
  return path.extname(filePath).toLowerCase() === '.pdf' ? filePath : `${filePath}.pdf`;
}

function getLocalPdfFallbackPath(fileName: string): string {
  let fallbackDir = '';
  try {
    fallbackDir = app.getPath('documents');
  } catch {
    fallbackDir = app.getPath('home');
  }
  return path.join(fallbackDir, path.basename(fileName) || 'Untitled.pdf');
}

function broadcastWindowList() {
  const payload = listWindows();
  for (const { win } of windowRegistry.values()) {
    win.webContents.send('window:listChanged', payload);
  }
}

function setWindowActive(windowId: number, active: boolean) {
  const record = windowRegistry.get(windowId);
  if (!record || record.info.active === active) {
    return;
  }
  record.info.active = active;
  if (active) {
    void markLastWorkspaceTabsSession(record.info.sessionId);
  }
  record.win.webContents.send('window:activeChanged', { active });
  broadcastWindowList();
}

async function cleanupWindowResources(windowId: number) {
  await disconnectRemote(windowId);
}

async function exists(p: string): Promise<boolean> {
  try {
    await fs.access(p);
    return true;
  } catch {
    return false;
  }
}

function getRuntimePlatformKey(): string | null {
  if (process.platform === 'darwin') {
    return process.arch === 'arm64' ? 'darwin-arm64' : process.arch === 'x64' ? 'darwin-amd64' : null;
  }
  if (process.platform === 'linux') {
    return process.arch === 'arm64' ? 'linux-arm64' : process.arch === 'x64' ? 'linux-amd64' : null;
  }
  if (process.platform === 'win32') {
    return process.arch === 'x64' ? 'windows-amd64' : null;
  }
  return null;
}

async function getBundledBootstrapperPath(): Promise<string | null> {
  const binaryName = process.platform === 'win32' ? 'openbrain-bootstrap.exe' : 'openbrain-bootstrap';
  const candidates: string[] = [];
  try {
    candidates.push(path.join(app.getAppPath(), '..', 'openbrain', 'bin', binaryName));
  } catch {
    // ignore
  }
  if ((process as any).resourcesPath) {
    candidates.push(path.join((process as any).resourcesPath, 'openbrain', 'bin', binaryName));
  }
  candidates.push(path.join(__dirname, '../../openbrain/bin', binaryName));

  for (const candidate of candidates) {
    if (await exists(candidate)) {
      return candidate;
    }
  }
  return null;
}

async function getBundledRuntimeBundlePath(): Promise<string | null> {
  const platform = getRuntimePlatformKey();
  if (!platform) {
    return null;
  }

  const candidates: string[] = [];
  try {
    candidates.push(path.join(app.getAppPath(), '..', 'openbrain', 'bundles', platform, 'bundle.tar.gz'));
  } catch {
    // ignore
  }
  if ((process as any).resourcesPath) {
    candidates.push(path.join((process as any).resourcesPath, 'openbrain', 'bundles', platform, 'bundle.tar.gz'));
  }
  candidates.push(path.join(__dirname, `../../openbrain/bundles/${platform}/bundle.tar.gz`));

  for (const candidate of candidates) {
    if (await exists(candidate)) {
      return candidate;
    }
  }
  return null;
}

async function getBundledRuntimeVersion(): Promise<string | null> {
  const candidates: string[] = [];
  try {
    candidates.push(path.join(app.getAppPath(), '..', 'openbrain', 'runtime-version.txt'));
  } catch {
    // ignore
  }
  if ((process as any).resourcesPath) {
    candidates.push(path.join((process as any).resourcesPath, 'openbrain', 'runtime-version.txt'));
  }
  candidates.push(path.join(__dirname, '../../openbrain/runtime-version.txt'));

  for (const candidate of candidates) {
    try {
      const value = (await fs.readFile(candidate, 'utf8')).trim();
      if (value) {
        return value;
      }
    } catch {
      // Try the next packaged location.
    }
  }
  return null;
}

async function ensureLocalRuntimeBootstrapController(): Promise<LocalRuntimeBootstrapController> {
  if (localRuntimeBootstrapController) {
    return localRuntimeBootstrapController;
  }

  const bootstrapperPath = await getBundledBootstrapperPath();
  const bundledRuntimeBundlePath = await getBundledRuntimeBundlePath();
  const bundledRuntimeVersion = bundledRuntimeBundlePath ? await getBundledRuntimeVersion() : null;
  const controller = new LocalRuntimeBootstrapController({
    appIsPackaged: app.isPackaged,
    bootstrapperPath,
    bundledRuntimeBundlePath,
    currentVersion: bundledRuntimeVersion || app.getVersion(),
    baseDir: getOpenBrainBaseDir(app.getPath('home')),
    manifestUrl: DEFAULT_LOCAL_RUNTIME_MANIFEST_URL,
    port: LOCAL_OPENBRAIN_SERVER_PORT,
  });
  controller.subscribe((snapshot) => {
    broadcastRuntimeBootstrapChanged(snapshot);
  });
  localRuntimeBootstrapController = controller;
  return controller;
}

async function getSettingsTemplateDir(): Promise<string | null> {
  // Dev: `app.getAppPath()` points to `.../openbrain/client`
  // Template is at `.../openbrain/settings`
  const candidates: string[] = [];
  try {
    candidates.push(path.join(app.getAppPath(), '..', 'settings'));
  } catch {
    // ignore
  }

  // Packaged: allow shipping template under resources/settings
  // (packager should copy `openbrain/settings/*` there)
  if ((process as any).resourcesPath) {
    candidates.push(path.join((process as any).resourcesPath, 'settings'));
  }

  // Allow shipping alongside main bundle
  candidates.push(path.join(__dirname, 'settings'));

  for (const p of candidates) {
    if (await exists(p)) {
      return p;
    }
  }
  return null;
}

async function ensureLocalWorkspaceDir(workspacePath: string): Promise<string> {
  await fs.mkdir(workspacePath, { recursive: true });
  return workspacePath;
}

async function resolveDefaultWorkspacePath(): Promise<string> {
  const systemConfig = await waitForRuntimeSystemConfig(LOCAL_RUNTIME_WS_URL, {
    attempts: 12,
    intervalMs: 250,
    requestTimeoutMs: 750,
  });
  return systemConfig.defaultWorkspace;
}

async function copyMissingFromTemplate(templateDir: string, targetDir: string): Promise<void> {
  await fs.mkdir(targetDir, { recursive: true });
  const entries = await fs.readdir(templateDir, { withFileTypes: true });
  for (const entry of entries) {
    if (entry.name.startsWith('.')) {
      continue;
    }
    const src = path.join(templateDir, entry.name);
    const basename = settingsBasenameFromFileName(entry.name);
    const dst = basename
      ? path.join(targetDir, settingsConfigFileNameVariants(basename)[0])
      : path.join(targetDir, entry.name);
    if (entry.isDirectory()) {
      await copyMissingFromTemplate(src, dst);
      continue;
    }
    if (basename) {
      const existingVariant = await resolveSettingsVariantPath(targetDir, basename);
      if (existingVariant) {
        continue;
      }
    } else if (await exists(dst)) {
      continue;
    }
    await fs.mkdir(path.dirname(dst), { recursive: true });
    await fs.copyFile(src, dst);
  }
}

async function syncReleaseVersionFromTemplate(templateDir: string, settingsRoot: string): Promise<void> {
  const src = path.join(templateDir, getSettingsVersionTemplateFileName());
  if (!(await exists(src))) {
    return;
  }

  const dst = getVersionSettingsPath(settingsRoot);
  const legacyDst = getLegacyVersionSettingsPath(settingsRoot);
  const data = await fs.readFile(src, 'utf8');
  try {
    if ((await fs.readFile(dst, 'utf8')) === data) {
      return;
    }
  } catch {
    // Missing or unreadable target: rewrite from bundled release metadata.
  }

  await fs.mkdir(path.dirname(dst), { recursive: true });
  await fs.writeFile(dst, data, 'utf8');
  try {
    await fs.rm(legacyDst, { force: true });
  } catch {
    // Ignore legacy cleanup failures.
  }
}

async function readSettingsVariantFile(dir: string, basename: string): Promise<string | null> {
  for (const fileName of settingsConfigFileNameVariants(basename)) {
    const filePath = path.join(dir, fileName);
    if (await exists(filePath)) {
      return fs.readFile(filePath, 'utf8');
    }
  }
  return null;
}

async function resolveSettingsVariantPath(dir: string, basename: string): Promise<string | null> {
  for (const fileName of settingsConfigFileNameVariants(basename)) {
    const filePath = path.join(dir, fileName);
    if (await exists(filePath)) {
      return filePath;
    }
  }
  return null;
}

type ThemeConfigFile = {
  version?: number;
  builtInVersion?: number;
  themes?: Array<{
    id: string;
    name?: string;
    markdownTheme?: string;
    codeTheme?: string;
    core?: Record<string, string>;
  }>;
};

type MarkdownThemeConfigFile = {
  version?: number;
  builtInVersion?: number;
  themes?: Array<{
    id: string;
    name?: string;
    core?: Record<string, string>;
    editor?: Record<string, string>;
    syntax?: Record<string, string>;
    preview?: Record<string, string>;
  }>;
};

type CodeThemeConfigFile = {
  version?: number;
  builtInVersion?: number;
  themes?: Array<{
    id: string;
    name?: string;
    tokens?: Record<string, string>;
    core?: Record<string, string>;
  }>;
};

async function mergeBuiltInThemesFromTemplate(templateDir: string, settingsRoot: string) {
  const builtInIds = new Set(['default-light', 'default-dark', 'openbrain-light', 'openbrain-dark']);
  const templateThemeData = await readSettingsVariantFile(templateDir, 'theme');
  const userThemePath = await resolveSettingsVariantPath(settingsRoot, 'theme');

  if (!templateThemeData) {
    return;
  }

  // If user theme config doesn't exist, let copyMissingFromTemplate handle it.
  if (!userThemePath) {
    return;
  }

  let templateCfg: ThemeConfigFile | null = null;
  let userCfg: ThemeConfigFile | null = null;
  templateCfg = parseSettingsJsonc<ThemeConfigFile>(templateThemeData);
  userCfg = parseSettingsJsonc<ThemeConfigFile>(await fs.readFile(userThemePath, 'utf8'));
  if (!templateCfg || !userCfg) {
    return;
  }

  const templateBuiltInVersion = Number(templateCfg?.builtInVersion || 0);
  const userBuiltInVersion = Number(userCfg?.builtInVersion || 0);
  const shouldOverrideBuiltIns = templateBuiltInVersion > userBuiltInVersion;

  if (shouldOverrideBuiltIns) {
    await migrateUiThemeIdsOnBuiltInUpgrade(settingsRoot, userBuiltInVersion, templateBuiltInVersion);
  }

  const templateThemes = Array.isArray(templateCfg?.themes) ? templateCfg!.themes! : [];
  const userThemes = Array.isArray(userCfg?.themes) ? userCfg!.themes! : [];

  const templateById = new Map(templateThemes.map((t) => [t.id, t]));
  const userById = new Map(userThemes.map((t) => [t.id, t]));

  const merged: Array<{ id: string; name?: string; core?: Record<string, string> }> = [];

  // Always ensure built-ins exist; override them only when builtInVersion increases.
  for (const id of builtInIds) {
    const tpl = templateById.get(id);
    const usr = userById.get(id);
    if (tpl && (shouldOverrideBuiltIns || !usr)) {
      merged.push(tpl);
    } else if (usr) {
      merged.push(usr);
    }
  }

  // Keep everything else from user config untouched (user-added themes stay).
  for (const t of userThemes) {
    if (builtInIds.has(t.id) || DEPRECATED_THEME_IDS.has(t.id)) {
      continue;
    }
    merged.push(t);
  }

  const nextCfg: ThemeConfigFile = {
    ...(userCfg || {}),
    version: 1,
    builtInVersion: shouldOverrideBuiltIns ? templateBuiltInVersion : userBuiltInVersion,
    themes: merged,
  };

  await fs.writeFile(userThemePath, JSON.stringify(nextCfg, null, 2), 'utf8');
}

async function mergeBuiltInMarkdownThemesFromTemplate(templateDir: string, settingsRoot: string) {
  const builtInIds = new Set(['default-light', 'default-dark', 'openbrain-light', 'openbrain-dark']);
  const templateMarkdownThemeData = await readSettingsVariantFile(templateDir, 'markdown-themes');
  const userMarkdownThemePath = await resolveSettingsVariantPath(settingsRoot, 'markdown-themes');

  if (!templateMarkdownThemeData) {
    return;
  }

  // If user markdown theme config doesn't exist, let copyMissingFromTemplate handle it.
  if (!userMarkdownThemePath) {
    return;
  }

  const templateCfg = parseSettingsJsonc<MarkdownThemeConfigFile>(templateMarkdownThemeData);
  const userCfg = parseSettingsJsonc<MarkdownThemeConfigFile>(
    await fs.readFile(userMarkdownThemePath, 'utf8')
  );
  if (!templateCfg || !userCfg) {
    return;
  }

  const templateBuiltInVersion = Number(templateCfg.builtInVersion || 0);
  const userBuiltInVersion = Number(userCfg.builtInVersion || 0);
  const shouldOverrideBuiltIns = templateBuiltInVersion > userBuiltInVersion;

  const templateThemes = Array.isArray(templateCfg.themes) ? templateCfg.themes : [];
  const userThemes = Array.isArray(userCfg.themes) ? userCfg.themes : [];
  const templateById = new Map(templateThemes.map((theme) => [theme.id, theme]));
  const userById = new Map(userThemes.map((theme) => [theme.id, theme]));

  const merged: Array<{
    id: string;
    name?: string;
    editor?: Record<string, string>;
    syntax?: Record<string, string>;
    preview?: Record<string, string>;
  }> = [];

  for (const id of builtInIds) {
    const tpl = templateById.get(id);
    const usr = userById.get(id);
    if (tpl && (shouldOverrideBuiltIns || !usr)) {
      merged.push(tpl);
    } else if (usr) {
      merged.push(usr);
    }
  }

  for (const theme of userThemes) {
    if (builtInIds.has(theme.id) || DEPRECATED_THEME_IDS.has(theme.id)) {
      continue;
    }
    merged.push(theme);
  }

  const nextCfg: MarkdownThemeConfigFile = {
    ...(userCfg || {}),
    version: 1,
    builtInVersion: shouldOverrideBuiltIns ? templateBuiltInVersion : userBuiltInVersion,
    themes: merged,
  };

  await fs.writeFile(userMarkdownThemePath, JSON.stringify(nextCfg, null, 2), 'utf8');
}

async function mergeBuiltInCodeThemesFromTemplate(templateDir: string, settingsRoot: string) {
  const builtInIds = new Set([
    'openbrain-code-light',
    'openbrain-code-dark',
    'opagent-code-light',
    'opagent-code-dark',
  ]);
  const templateCodeThemeData = await readSettingsVariantFile(templateDir, 'code-themes');
  const userCodeThemePath = await resolveSettingsVariantPath(settingsRoot, 'code-themes');

  if (!templateCodeThemeData) {
    return;
  }
  if (!userCodeThemePath) {
    return;
  }

  const templateCfg = parseSettingsJsonc<CodeThemeConfigFile>(templateCodeThemeData);
  const userCfg = parseSettingsJsonc<CodeThemeConfigFile>(await fs.readFile(userCodeThemePath, 'utf8'));
  if (!templateCfg || !userCfg) {
    return;
  }

  const templateBuiltInVersion = Number(templateCfg.builtInVersion || 0);
  const userBuiltInVersion = Number(userCfg.builtInVersion || 0);
  const shouldOverrideBuiltIns = templateBuiltInVersion > userBuiltInVersion;

  const templateThemes = Array.isArray(templateCfg.themes) ? templateCfg.themes : [];
  const userThemes = Array.isArray(userCfg.themes) ? userCfg.themes : [];
  const templateById = new Map(templateThemes.map((theme) => [theme.id, theme]));
  const userById = new Map(userThemes.map((theme) => [theme.id, theme]));

  const merged: Array<{ id: string; name?: string; tokens?: Record<string, string> }> = [];

  for (const id of builtInIds) {
    const tpl = templateById.get(id);
    const usr = userById.get(id);
    if (tpl && (shouldOverrideBuiltIns || !usr)) {
      merged.push(tpl);
    } else if (usr) {
      merged.push(usr);
    }
  }

  for (const theme of userThemes) {
    if (builtInIds.has(theme.id)) {
      continue;
    }
    merged.push(theme);
  }

  const nextCfg: CodeThemeConfigFile = {
    ...(userCfg || {}),
    version: 1,
    builtInVersion: shouldOverrideBuiltIns ? templateBuiltInVersion : userBuiltInVersion,
    themes: merged,
  };

  await fs.writeFile(userCodeThemePath, JSON.stringify(nextCfg, null, 2), 'utf8');
}

const THEME_BG_CACHE_FILENAME = 'theme-bg.json';
const DEFAULT_WINDOW_BG = '#f4f9f7';

/** Read persisted theme background for BrowserWindow (sync, used at window create). */
function getPersistedThemeBackground(): string {
  try {
    const filePath = path.join(app.getPath('userData'), THEME_BG_CACHE_FILENAME);
    if (!existsSync(filePath)) return DEFAULT_WINDOW_BG;
    const raw = readFileSync(filePath, 'utf8');
    const data = JSON.parse(raw) as { backgroundColor?: string };
    const bg = typeof data?.backgroundColor === 'string' ? data.backgroundColor.trim() : '';
    if (/^#[0-9A-Fa-f]{6}$/.test(bg)) return bg;
  } catch {
    // ignore
  }
  return DEFAULT_WINDOW_BG;
}

/** Persist active theme background so next window create uses it (e.g. after sleep). */
async function persistThemeBackground(settings: SettingsState): Promise<void> {
  try {
    const themeId = settings?.ui?.themeId;
    const themes = settings?.theme?.themes;
    if (!themeId || !Array.isArray(themes)) return;
    const theme = themes.find((t) => t.id === themeId);
    const bg = theme?.core?.background;
    if (typeof bg !== 'string' || !/^#[0-9A-Fa-f]{6}$/.test(bg.trim())) return;
    const filePath = path.join(app.getPath('userData'), THEME_BG_CACHE_FILENAME);
    await fs.writeFile(filePath, JSON.stringify({ backgroundColor: bg }), 'utf8');
  } catch {
    // ignore
  }
}

async function loadRendererView(
  win: BrowserWindow,
  options?: { view?: string; openDevTools?: boolean }
): Promise<void> {
  const view = normalizeOptionalString(options?.view);
  const openDevTools = options?.openDevTools === true;
  if (isDev) {
    const target = new URL(devServerUrl);
    if (view) {
      target.searchParams.set('view', view);
    }
    console.log(`[dev] Loading renderer from ${target.toString()}`);
    await win.loadURL(target.toString());
    if (openDevTools) {
      win.webContents.openDevTools({ mode: 'undocked' });
    }
    return;
  }

  const rendererHtml = path.join(__dirname, '../renderer/index.html');
  await win.loadFile(
    rendererHtml,
    view ? { query: { view } } : undefined
  );
}

type WindowInit = {
  sessionId?: string;
  mode?: WindowMode;
  presentation?: WindowPresentation;
  authRequired?: boolean;
  workspacePath?: string;
  remoteHost?: SshHost;
  workspaceId?: string;
};

function createWindow(init: WindowInit = {}) {
  const isMac = process.platform === 'darwin';
  const sessionId = init.sessionId ?? createId('window');
  const mode = init.mode ?? 'local';
  const presentation = init.presentation ?? 'default';
  const authRequired = init.authRequired === true;
  const macTitleBarStyle: 'default' | 'hidden' =
    presentation === 'newWindowLanding' ? 'default' : 'hidden';
  const workspacePath = init.workspacePath;
  const remoteHost = init.remoteHost;
  const workspaceId = init.workspaceId ?? resolveWorkspaceId(mode, workspacePath, remoteHost);
  const label = resolveWindowLabel(mode, workspacePath, remoteHost);
  const appIconPath = getAppIconPath(['png', 'icns']);

  const win = new BrowserWindow({
    width: 1400,
    height: 900,
    minWidth: 800,
    minHeight: 600,
    ...(isMac
      ? {
          titleBarStyle: macTitleBarStyle,
        }
      : {
          // Keep native frame on other platforms for now.
          // (We can switch to hidden + titleBarOverlay later if needed.)
          titleBarStyle: 'default' as const,
        }),
    backgroundColor: getPersistedThemeBackground(),
    ...(appIconPath ? { icon: appIconPath } : {}),
    webPreferences: {
      nodeIntegration: false,
      contextIsolation: true,
      webSecurity: !isDev,
      preload: path.join(__dirname, 'preload.js'),
    },
  });

  const info: WindowInfo = {
    id: win.id,
    sessionId,
    label,
    mode,
    presentation,
    authRequired,
    workspaceId,
    workspacePath,
    remoteHost,
    active: win.isFocused(),
  };
  windowRegistry.set(win.id, { win, info });
  attachDevWindowDiagnostics(win, 'main');

  // VS Code-style traffic lights alignment for custom titlebar on macOS.
  if (isMac && presentation !== 'newWindowLanding') {
    updateMacWindowButtonsForTitlebar(win, CUSTOM_TITLEBAR_HEIGHT);
  }

  if (isDev) {
    void loadRendererView(win, { openDevTools: shouldOpenDevTools });
  } else {
    void loadRendererView(win);
  }

  // Open external links in browser
  win.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url);
    return { action: 'deny' };
  });

  win.webContents.on(
    'did-fail-load',
    (_event, errorCode, errorDescription, validatedURL, isMainFrame) => {
      console.error('[window] did-fail-load', {
        windowId: win.id,
        errorCode,
        errorDescription,
        validatedURL,
        isMainFrame,
      });
    },
  );

  win.on('focus', () => setWindowActive(win.id, true));
  win.on('blur', () => setWindowActive(win.id, false));
  win.on('show', () => setWindowActive(win.id, win.isFocused()));
  win.on('hide', () => setWindowActive(win.id, false));
  win.on('close', (event) => {
    if (approvedWindowCloses.has(win.id)) {
      approvedWindowCloses.delete(win.id);
      pendingWindowCloseRequests.delete(win.id);
      return;
    }

    event.preventDefault();
    if (pendingWindowCloseRequests.has(win.id)) {
      return;
    }

    pendingWindowCloseRequests.add(win.id);
    win.webContents.send('window:prepareClose');
  });

  const webContentsId = win.webContents.id;
  win.on('closed', () => {
    pendingWindowCloseRequests.delete(win.id);
    approvedWindowCloses.delete(win.id);
    windowRegistry.delete(win.id);
    agentSleepInhibitor.clearWindow(webContentsId);
    if (desktopUpdateInstallCoordinator.markWindowClosed(win.id)) {
      maybeFinalizeDesktopUpdateInstall();
    }
    cleanupWindowResources(win.id).catch(() => {});
    broadcastWindowList();
  });

  win.webContents.on('did-finish-load', () => {
    win.webContents.send('window:bootstrap', {
      windowId: win.id,
      info,
      initialWorkspace: {
        mode,
        workspacePath,
        remoteHost,
      },
    });
    broadcastWindowList();
  });

  broadcastWindowList();
  return win;
}

function createNewWindowLanding() {
  return createWindow({ mode: 'local', presentation: 'newWindowLanding' });
}

function setupMacDockMenu() {
  if (process.platform !== 'darwin' || !app.dock) {
    return;
  }
  const dockMenu = Menu.buildFromTemplate([
    {
      label: mainT('menu:newWindow'),
      click: () => {
        const created = createNewWindowLanding();
        focusWindow(created);
      },
    },
  ]);
  app.dock.setMenu(dockMenu);
}

function focusWindow(win: BrowserWindow) {
  if (win.isDestroyed()) {
    return;
  }
  if (win.isMinimized()) {
    win.restore();
  }
  if (!win.isVisible()) {
    win.show();
  }
  win.focus();
}

async function createPrimaryWindow(homeDir: string): Promise<BrowserWindow> {
  const auth = await loadAuthConfig(homeDir);
  if (!auth) {
    return createWindow({ mode: 'local', authRequired: true });
  }

  const restored = await resolveLastWorkspaceTabsSession();
  const activeTab = getActiveWorkspaceTabFromSession(restored.session);
  if (activeTab) {
    const workspacePath = activeTab.kind === 'local' && activeTab.workspacePath
      ? await ensureLocalWorkspaceDir(activeTab.workspacePath)
      : activeTab.workspacePath;
    return createWindow({
      sessionId: restored.sessionId,
      mode: activeTab.kind,
      workspacePath,
      remoteHost: activeTab.remoteHost,
      workspaceId: activeTab.workspaceId,
    });
  }

  try {
    const workspacePath = await resolveDefaultWorkspacePath();
    return createWindow({ mode: 'local', workspacePath });
  } catch {
    // The renderer will resolve the Runtime-owned workspace after connecting.
    return createWindow({ mode: 'local' });
  }
}

async function focusOrCreatePrimaryWindow() {
  const firstWindow = BrowserWindow.getAllWindows()[0];
  if (firstWindow) {
    focusWindow(firstWindow);
    return;
  }

  if (!app.isReady()) {
    return;
  }

  const homeDir = app.getPath('home');
  try {
    const created = await createPrimaryWindow(homeDir);
    focusWindow(created);
  } catch {
    const created = createWindow({ mode: 'local' });
    focusWindow(created);
  }
}

async function ensureSettings() {
  if (!settingsCache) {
    const homeDir = app.getPath('home');
    const settingsRoot = getSettingsRoot(homeDir);

    const templateDir = await getSettingsTemplateDir();
    if (templateDir) {
      await syncReleaseVersionFromTemplate(templateDir, settingsRoot);
      await mergeBuiltInThemesFromTemplate(templateDir, settingsRoot);
      await mergeBuiltInMarkdownThemesFromTemplate(templateDir, settingsRoot);
      await mergeBuiltInCodeThemesFromTemplate(templateDir, settingsRoot);
      await copyMissingFromTemplate(templateDir, settingsRoot);
    }

    await ensureSettingsInitialized(homeDir);
    settingsCache = await loadAllSettings(homeDir);
    await persistThemeBackground(settingsCache);
    applySettingsDisplayLocale(settingsCache);
  }
  return settingsCache;
}

// Auth protocol handling
const PROTOCOL_NAMES = ['openbrain'];
let pendingAuthCallbackUrl: string | null = null;

function extractAuthCallbackUrl(argv: string[]): string | null {
  const url = argv.find((arg) => isAuthCallbackUrl(arg));
  return url ?? null;
}

function isAuthCallbackUrl(url: string): boolean {
  return PROTOCOL_NAMES.some((scheme) => url.startsWith(`${scheme}://auth/callback`));
}

function scheduleAuthCallback(url: string) {
  if (!isAuthCallbackUrl(url)) {
    return;
  }
  if (app.isReady()) {
    void handleAuthCallback(url);
    return;
  }
  pendingAuthCallbackUrl = url;
}

type AuthLogoutReason = 'logout' | 'session_expired';

function broadcastAuthLoggedOut(reason: AuthLogoutReason) {
  for (const { win } of windowRegistry.values()) {
    win.webContents.send('auth:changed', {
      loggedIn: false,
      profile: undefined,
      reason,
    });
  }
}

function authChangedPayload(config: AuthConfig, profile?: UserProfile | null) {
  const matchedProfile = profileMatchesAuth(config, profile) ? profile : null;
  return {
    loggedIn: true,
    uid: config.uid,
    email: config.email,
    baseUrl: config.baseUrl,
    aiGateway: config.aiGateway,
    activeOrgID: config.activeOrgID,
    activeOrgName: config.activeOrgName,
    profile: matchedProfile || undefined,
  };
}

function broadcastAuthChanged(config: AuthConfig, profile?: UserProfile | null) {
  const payload = authChangedPayload(config, profile);
  for (const { win } of windowRegistry.values()) {
    win.webContents.send('auth:changed', payload);
  }
}

function profileMatchesAuth(config: AuthConfig, profile?: UserProfile | null): profile is UserProfile {
  if (!profile) {
    return false;
  }
  if (profile.uid !== config.uid) {
    return false;
  }
  const profileEmail = (profile.email || '').trim().toLowerCase();
  const authEmail = (config.email || '').trim().toLowerCase();
  return !profileEmail || !authEmail || profileEmail === authEmail;
}

async function loadProfileForAuth(homeDir: string, config: AuthConfig): Promise<UserProfile | null> {
  const profile = await loadProfile(homeDir);
  if (!profile) {
    return null;
  }
  if (profileMatchesAuth(config, profile)) {
    return profile;
  }
  await clearProfile(homeDir);
  return null;
}

async function invalidateAuthSession(reason: AuthLogoutReason = 'session_expired') {
  const homeDir = app.getPath('home');
  console.warn('[Auth] Invalidating desktop auth session:', {
    reason,
    authPath: path.join(homeDir, '.openbrain', 'configs', 'user', 'auth.json'),
  });
  await clearAuthConfig(homeDir);
  await clearProfile(homeDir);
  broadcastAuthLoggedOut(reason);
}

// Fetch and save profile after successful login.
// Profile endpoints live on gateway; do not use website baseUrl.
async function fetchAndSaveProfile(gateway: string, token: string): Promise<UserProfile | null> {
  const homeDir = app.getPath('home');
  try {
    const profile = await fetchProfile(gateway, token);
    if (profile) {
      await saveProfile(homeDir, profile);
      console.log('[Auth] Profile saved successfully');
      return profile;
    }
  } catch (err) {
    if (isAuthInvalidError(err)) {
      throw err;
    }
    console.error('[Auth] Failed to fetch/save profile:', err);
  }
  return null;
}

async function saveFallbackProfile(
  homeDir: string,
  uid: string,
  email?: string,
): Promise<UserProfile> {
  const profile = createFallbackProfile(uid, email);
  try {
    await saveProfile(homeDir, profile);
  } catch (err) {
    console.warn('[Auth] Failed to save fallback profile:', readableNetworkError(err));
  }
  return profile;
}

// Handle auth callback URL from deep link
async function handleAuthCallback(url: string) {
  const parsed = parseAuthCallbackUrl(url);
  if (!parsed) {
    console.error('[Auth] Failed to parse auth callback URL:', url);
    return;
  }

  const homeDir = app.getPath('home');

  try {
    const fallbackInfo = pendingLoginGatewayInfo;
    const authBaseUrl = parsed.baseUrl || fallbackInfo?.baseUrl || parsed.gateway || fallbackInfo?.gateway;
    const authGateway = parsed.gateway || fallbackInfo?.gateway || authBaseUrl;
    const config = await ensureRequestedActiveOrg(
      homeDir,
      createAuthConfig(
        parsed.token,
        parsed.uid,
        parsed.email,
        authBaseUrl,
        authGateway,
        parsed.aiGateway || fallbackInfo?.aiGateway,
        parsed.defaultOrgID || fallbackInfo?.defaultOrg?.id,
        parsed.defaultOrgName || fallbackInfo?.defaultOrg?.name
      ),
      pendingLoginOrgSlug
    );
    pendingLoginGatewayInfo = null;
    pendingLoginOrgSlug = undefined;
    await saveAuthConfig(homeDir, config);
    const profile =
      (await fetchAndSaveProfile(config.gateway, parsed.token)) ||
      (await saveFallbackProfile(homeDir, parsed.uid, parsed.email));
    console.log('[Auth] Auth config saved successfully');

    broadcastAuthChanged(config, profile);

    await focusOrCreatePrimaryWindow();
  } catch (err) {
    if (isAuthInvalidError(err)) {
      await invalidateAuthSession('session_expired');
    }
    console.error('[Auth] Failed to save auth config:', err);
  }
}

// Register custom protocol (openbrain://)
// In development, Electron requires passing the executable + app entry as args.
// Without this, the browser deep link won't be delivered to openbrain.
try {
  for (const protocolName of PROTOCOL_NAMES) {
    if (app.isPackaged) {
      app.setAsDefaultProtocolClient(protocolName);
    } else {
      const appEntry = process.argv[1] ? path.resolve(process.argv[1]) : '';
      if (appEntry) {
        app.setAsDefaultProtocolClient(protocolName, process.execPath, [appEntry]);
      } else {
        app.setAsDefaultProtocolClient(protocolName);
      }
    }
  }
} catch (err) {
  console.error('[Auth] Failed to register protocol client:', err);
}

// macOS: Handle protocol URL when app is already running
app.on('open-url', (event, url) => {
  event.preventDefault();
  scheduleAuthCallback(url);
});

// Windows/Linux: Single instance lock and handle second instance with protocol URL
const gotTheLock = app.requestSingleInstanceLock();
if (!gotTheLock) {
  app.quit();
} else {
  // Handle protocol URL passed on initial launch (mainly Windows/Linux).
  const startupUrl = extractAuthCallbackUrl(process.argv);
  if (startupUrl) {
    scheduleAuthCallback(startupUrl);
  }

  app.on('second-instance', (_, argv) => {
    // Find the protocol URL in argv
    const url = extractAuthCallbackUrl(argv);
    if (url) {
      scheduleAuthCallback(url);
    }

    if (app.isReady()) {
      void focusOrCreatePrimaryWindow();
      return;
    }

    void app.whenReady().then(async () => {
      applyAppIcon();
      await focusOrCreatePrimaryWindow();
    });
  });
}

app.on('before-quit', () => {
  appQuitRequested = true;
});

app.whenReady().then(async () => {
  applyAppIcon();
  const desktopUpdateController = getDesktopUpdateController();
  desktopUpdateController.subscribe((snapshot) => {
    broadcastDesktopUpdateChanged(snapshot);
  });
  startDesktopAutoUpdate();
  const startupDesktopUpdate = await desktopUpdateController.waitForStartupDecision(DESKTOP_UPDATE_STARTUP_BUDGET_MS);
  if (startupDesktopUpdate.phase === 'ready') {
    requestDesktopUpdateInstall();
    return;
  }

  const localRuntimeController = await ensureLocalRuntimeBootstrapController();
  void localRuntimeController.start()
    .catch((err) => {
      console.error('Failed to prepare local runtime bootstrap controller:', err);
    });

  if (pendingAuthCallbackUrl) {
    const url = pendingAuthCallbackUrl;
    pendingAuthCallbackUrl = null;
    await handleAuthCallback(url);
  }
  const homeDir = app.getPath('home');
  initMainI18n(undefined, app.getLocale());
  await ensureSettings();
  agentSleepInhibitor.setAppSessionActive(true);
  syncAgentSleepInhibitorFromSettings(settingsCache);
  setupMacDockMenu();
  try {
    await startConfigSync(homeDir, broadcastConfigSyncPush);
  } catch (err) {
    console.warn('Failed to start config sync:', err);
  }
  try {
    await startSettingsWatch(homeDir);
  } catch (err) {
    console.warn('Failed to start settings watch:', err);
  }
  await ensureWorkspaceTabsSessionCache();
  ensureArchiveCleanupScheduler().start();
  await createPrimaryWindow(homeDir);
  scheduleArchiveCleanupSoon();

  app.on('browser-window-blur', () => {
    scheduleWorkspaceTabsSessionSave();
  });

  powerMonitor.on('lock-screen', () => {
    scheduleWorkspaceTabsSessionSave();
  });

  powerMonitor.on('suspend', () => {
    scheduleWorkspaceTabsSessionSave();
  });

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createPrimaryWindow(homeDir)
        .then((win) => focusWindow(win))
        .catch(() => createWindow({ mode: 'local', authRequired: true }));
    }
  });
});

app.on('will-quit', () => {
  appQuitRequested = true;
  agentSleepInhibitor.setAppSessionActive(false);
  stopSettingsWatch();
  archiveCleanupScheduler?.stop();
  archiveCleanupScheduler = null;
  if (workspaceTabsSessionSaveTimer) {
    clearTimeout(workspaceTabsSessionSaveTimer);
    workspaceTabsSessionSaveTimer = null;
  }
  void persistWorkspaceTabsSessionCache().catch(() => {});
});

app.on('window-all-closed', () => {
  if (desktopUpdateInstallPending) {
    return;
  }
  if (appQuitRequested) {
    app.quit();
    return;
  }
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

// IPC handlers

ipcMain.handle('window:list', async () => {
  return listWindows();
});

ipcMain.handle('window:getBootstrap', async (event) => {
  const record = getWindowRecordByWebContents(event.sender);
  if (!record) {
    return null;
  }
  const { info } = record;
  const workspaceTabsSession = await getWorkspaceTabsSession(info.sessionId);
  return {
    windowId: info.id,
    info,
    workspaceTabsSession,
    initialWorkspace: {
      mode: info.mode,
      workspacePath: info.workspacePath,
      remoteHost: info.remoteHost,
    },
    runtimeBootstrap: localRuntimeBootstrapController?.getSnapshot() || null,
  };
});

ipcMain.handle('window:updateWorkspaceTabsSession', async (event, session: unknown) => {
  const record = getWindowRecordByWebContents(event.sender);
  if (!record) {
    return null;
  }
  const normalized = await updateWorkspaceTabsSession(record.info.sessionId, session);
  if (record.info.active) {
    await markLastWorkspaceTabsSession(record.info.sessionId);
  }
  scheduleArchiveCleanupSoon();
  return normalized;
});

ipcMain.handle('runtimeBootstrap:retry', async () => {
  if (!localRuntimeBootstrapController) {
    return { success: false };
  }
  void localRuntimeBootstrapController.retry().catch((err) => {
    console.error('Failed to retry runtime bootstrap:', err);
  });
  return { success: true };
});

ipcMain.handle('runtimeBootstrap:quit', async () => {
  app.quit();
  return { success: true };
});

ipcMain.handle('desktopUpdate:getState', async () => {
  return getDesktopUpdateController().getSnapshot();
});

ipcMain.handle('desktopUpdate:install', async () => {
  return requestDesktopUpdateInstall();
});

ipcMain.handle('window:focus', async (_, windowId: number) => {
  const record = getWindowRecordById(windowId);
  record?.win.focus();
  return { success: Boolean(record) };
});

ipcMain.handle('window:close', async (_, windowId: number) => {
  const record = getWindowRecordById(windowId);
  record?.win.close();
  return { success: Boolean(record) };
});

ipcMain.handle('window:readyToClose', async (event) => {
  const record = getWindowRecordByWebContents(event.sender);
  if (!record) {
    return { success: false };
  }
  const windowId = record.info.id;
  if (!pendingWindowCloseRequests.has(windowId)) {
    return { success: false };
  }

  pendingWindowCloseRequests.delete(windowId);
  approvedWindowCloses.add(windowId);
  record.win.close();
  return { success: true };
});

ipcMain.handle('window:createNew', async () => {
  createNewWindowLanding();
  return { success: true };
});

ipcMain.handle('window:createLocal', async (event, options?: { path?: string }) => {
  let selectedPath = options?.path;
  if (!selectedPath) {
    const record = getWindowRecordByWebContents(event.sender);
    const parentWindow = record?.win;
    const result = parentWindow
      ? await dialog.showOpenDialog(parentWindow, {
          properties: ['openDirectory'],
        })
      : await dialog.showOpenDialog({
          properties: ['openDirectory'],
        });
    if (result.canceled || result.filePaths.length === 0) {
      return { canceled: true };
    }
    selectedPath = result.filePaths[0];
  }
  createWindow({ mode: 'local', workspacePath: selectedPath });
  return { canceled: false };
});

ipcMain.handle('window:createRemote', async (_, host: SshHost) => {
  createWindow({ mode: 'remote', remoteHost: host });
  return { success: true };
});

ipcMain.handle('workspace:listTemplates', async (_event, input?: { orgID?: string | null; targetOrgID?: string | null }) => {
  const homeDir = app.getPath('home');
  const auth = await loadAuthConfig(homeDir);
  if (!auth) {
    return { success: false, error: 'Sign in required.' };
  }
  try {
    const orgID = normalizeActiveOrgID(input?.targetOrgID);
    const result = await listWorkspaceTemplates(auth, orgID);
    return { success: true, ...result };
  } catch (err) {
    return { success: false, error: (err as Error).message || 'Failed to list workspace templates.' };
  }
});

ipcMain.handle('workspace:listOpenBrains', async () => {
  const homeDir = app.getPath('home');
  try {
    const sources = await listGBrainSourceWorkspaces(homeDir);
    return { success: true, sources };
  } catch (err) {
    return { success: false, error: (err as Error).message || 'Failed to list GBrain sources.' };
  }
});

async function currentOpenBrainProviderContext() {
  const homeDir = app.getPath('home');
  const currentSettings = await ensureSettings();
  const loadedAuth = await loadAuthConfig(homeDir);
  const auth = loadedAuth ? await ensureDefaultActiveOrg(homeDir, loadedAuth) : null;
  return {
    homeDir,
    settings: currentSettings.user.openBrain,
    auth,
  };
}

ipcMain.handle('openBrain:getProvider', async () => {
  const { settings, auth } = await currentOpenBrainProviderContext();
  return await getOpenBrainProviderStatus(settings, auth);
});

ipcMain.handle('openBrain:setProvider', async (_event, input?: { provider?: string; local?: unknown }) => {
  const homeDir = app.getPath('home');
  const currentSettings = await ensureSettings();
  const current = currentSettings.user.openBrain || {};
  const provider = input?.provider === 'local' ? 'local' : 'cloud';
  const merged = await saveSettings(homeDir, {
    user: {
      ...currentSettings.user,
      openBrain: {
        provider,
        local: input?.local && typeof input.local === 'object'
          ? input.local as any
          : current.local || {},
      },
    },
  });
  settingsCache = merged;
  broadcastSettingsChanged(settingsCache);
  const loadedAuth = await loadAuthConfig(homeDir);
  const auth = loadedAuth ? await ensureDefaultActiveOrg(homeDir, loadedAuth) : null;
  return await getOpenBrainProviderStatus(merged.user.openBrain, auth);
});

ipcMain.handle('openBrain:listSources', async () => {
  try {
    const context = await currentOpenBrainProviderContext();
    return await listOpenBrainSources(context);
  } catch (err) {
    return {
      success: false,
      code: 'openbrain_error',
      error: (err as Error).message || 'Failed to list OpenBrain sources.',
      sources: [],
    };
  }
});

ipcMain.handle('openBrain:query', async (_event, input?: { brainID?: string; scope?: 'brain' | 'workspace'; workspaceID?: string; orgID?: string; publicOwnerUID?: string; query?: string; limit?: number }) => {
  try {
    const context = await currentOpenBrainProviderContext();
    return await queryOpenBrainProvider({ ...context, input });
  } catch (err) {
    return {
      success: false,
      code: 'openbrain_error',
      error: (err as Error).message || 'Failed to query OpenBrain.',
      results: [],
    };
  }
});

function normalizeWorkspaceStorageBackend(value?: string): string {
  return (value || '').trim().toLowerCase();
}

function workspaceStorageBackendSettingsURL(baseURL: string, backend: string): string {
  const normalizedBaseURL = (baseURL || 'https://openbrain.chat').replace(/\/+$/, '');
  return `${normalizedBaseURL}/settings/storage-providers?backend=${encodeURIComponent(backend)}`;
}

ipcMain.handle('workspace:openStorageBackendSettings', async (_event, input?: { storageBackend?: string; provider?: string }) => {
  const backend = normalizeWorkspaceStorageBackend(input?.storageBackend || input?.provider || 'github');
  if (!['github', 'gitlab', 'gitee', 'google-drive', 'lark-drive', 'feishu'].includes(backend)) {
    return { success: false, error: 'Unsupported storage backend.' };
  }
  const homeDir = app.getPath('home');
  const auth = await loadAuthConfig(homeDir);
  const baseURL = auth?.baseUrl || 'https://openbrain.chat';
  try {
    await shell.openExternal(workspaceStorageBackendSettingsURL(baseURL, backend));
    return { success: true };
  } catch (err) {
    return { success: false, error: (err as Error).message || 'Failed to open storage settings.' };
  }
});

ipcMain.handle('workspace:createOpenBrain', async (_event, input?: { name?: string; localPath?: string }) => {
  const homeDir = app.getPath('home');
  const localPath = (input?.localPath || '').trim();
  if (!localPath) {
    return { success: false, error: 'Select a local workspace directory first.' };
  }
  const name = (input?.name || path.basename(localPath) || '').trim();
  if (!name) {
    return { success: false, error: 'GBrain source name is required.' };
  }
  try {
    const auth = await loadAuthConfig(homeDir);
    const materialized = await createLocalIndexWorkspace(homeDir, { name, localPath }, auth);
    const registered = await registerGBrainSourceForWorkspace(homeDir, materialized);
    return { success: true, workspace: registered };
  } catch (err) {
    return { success: false, error: (err as Error).message || 'Failed to create GBrain source.' };
  }
});

ipcMain.handle('openBrain:createSource', async (_event, input?: { name?: string; localPath?: string; remotePath?: string; tabId?: string; remoteHost?: SshHost }) => {
  try {
    const context = await currentOpenBrainProviderContext();
    if (context.settings?.provider !== 'local') {
      return {
        success: false,
        code: 'openbrain_source_flow_required',
        error: 'Create or bind OpenBrain Cloud sources through the active runtime server.',
      };
    }
    return await createOpenBrainSource({
      ...context,
      name: input?.name,
      localPath: input?.localPath,
    });
  } catch (err) {
    return {
      success: false,
      code: 'openbrain_error',
      error: (err as Error).message || 'Failed to create OpenBrain source.',
    };
  }
});

ipcMain.handle('openBrain:removeSourceFromDevice', async (_event, input?: { workspaceID?: string; sourceID?: string; orgID?: string; path?: string }) => {
  try {
    const context = await currentOpenBrainProviderContext();
    return await removeOpenBrainSourceFromDevice({
      ...context,
      workspaceID: input?.workspaceID || input?.sourceID,
      orgID: input?.orgID,
      path: input?.path,
    });
  } catch (err) {
    return {
      success: false,
      code: 'openbrain_error',
      error: (err as Error).message || 'Failed to remove OpenBrain source from this device.',
    };
  }
});

ipcMain.handle('openBrain:archiveSource', async (_event, input?: { workspaceID?: string; sourceID?: string; orgID?: string; path?: string }) => {
  try {
    const context = await currentOpenBrainProviderContext();
    return await archiveOpenBrainSource({
      ...context,
      workspaceID: input?.workspaceID || input?.sourceID,
      orgID: input?.orgID,
      path: input?.path,
    });
  } catch (err) {
    return {
      success: false,
      code: 'openbrain_error',
      error: (err as Error).message || 'Failed to stop OpenBrain Cloud queries for this source.',
    };
  }
});

ipcMain.handle('openBrain:sourceAction', async (_event, input?: {
  workspaceID?: string;
  sourceID?: string;
  orgID?: string;
  path?: string;
  disableQueries?: boolean;
  enableQueries?: boolean;
  disableSync?: boolean;
  hardDelete?: boolean;
  confirmWorkspaceID?: string;
  confirmName?: string;
}) => {
  try {
    const context = await currentOpenBrainProviderContext();
    return await applyOpenBrainSourceAction({
      ...context,
      workspaceID: input?.workspaceID || input?.sourceID,
      orgID: input?.orgID,
      path: input?.path,
      disableQueries: input?.disableQueries,
      enableQueries: input?.enableQueries,
      disableSync: input?.disableSync,
      hardDelete: input?.hardDelete,
      confirmWorkspaceID: input?.confirmWorkspaceID,
      confirmName: input?.confirmName,
    });
  } catch (err) {
    return {
      success: false,
      code: 'openbrain_error',
      error: (err as Error).message || 'Failed to update OpenBrain Cloud source.',
    };
  }
});

ipcMain.handle('workspace:createFromTemplate', async (_event, input?: { templateID?: string; storageBackend?: string; provider?: string; repositoryOwner?: string; repositoryName?: string; name?: string; orgID?: string | null; targetOrgID?: string | null; localPath?: string }) => {
  const homeDir = app.getPath('home');
  const localPath = (input?.localPath || '').trim();
  if (!localPath) {
    return { success: false, error: 'Select a local workspace directory first.' };
  }
  let storageBackend = normalizeWorkspaceStorageBackend(input?.storageBackend || input?.provider);
  if (storageBackend === 'feishu') {
    storageBackend = 'lark-drive';
  }
  if (input?.templateID === 'empty-workspace' && storageBackend === 'none') {
    try {
      const auth = await loadAuthConfig(homeDir);
      const materialized = await createLocalEmptyWorkspace(homeDir, { name: input?.name, localPath }, auth);
      return { success: true, workspace: materialized };
    } catch (err) {
      return { success: false, error: (err as Error).message || 'Failed to create local workspace.' };
    }
  }
  if (input?.templateID === 'local-index-workspace' && (!storageBackend || storageBackend === 'none')) {
    try {
      const auth = await loadAuthConfig(homeDir);
      const materialized = await createLocalIndexWorkspace(homeDir, { name: input?.name, localPath }, auth);
      return { success: true, workspace: materialized };
    } catch (err) {
      return { success: false, error: (err as Error).message || 'Failed to create local workspace.' };
    }
  }
  if (input?.templateID === 'openbrain-cloud') {
    return {
      success: false,
      code: 'openbrain_source_flow_required',
      error: 'Create or bind OpenBrain Cloud sources from the OpenBrain source flow.',
    };
  }
  const auth = await loadAuthConfig(homeDir);
  if (!auth) {
    return { success: false, error: 'Sign in required.' };
  }
  try {
    const orgID = normalizeActiveOrgID(input?.targetOrgID);
    const created = await createOpenbrainWorkspace(auth, {
      templateID: input?.templateID,
      provider: storageBackend === 'github' || storageBackend === 'gitlab' || storageBackend === 'gitee' ? storageBackend : undefined,
      storageProvider: storageBackend || input?.provider,
      repositoryOwner: input?.repositoryOwner,
      repositoryName: input?.repositoryName,
      name: input?.name,
      orgID,
    });
    const materialized = await materializeWorkspace(homeDir, created, auth, {
      localPath,
      writeManifest: input?.templateID !== 'openbrain-cloud',
    });
    return { success: true, workspace: materialized };
  } catch (err) {
    return { success: false, error: (err as Error).message || 'Failed to create workspace.' };
  }
});

ipcMain.handle('auth:listOrgs', async () => {
  const homeDir = app.getPath('home');
  const loadedConfig = await loadAuthConfig(homeDir);
  const auth = loadedConfig ? await ensureDefaultActiveOrg(homeDir, loadedConfig) : null;
  if (!auth) {
    return { success: false, error: 'Not logged in', orgs: [] };
  }
  try {
    const orgs = await fetchAuthOrgs(auth.gateway, auth.token);
    return {
      success: true,
      defaultOrgID: normalizeActiveOrgID(auth.defaultOrgID) || FALLBACK_DEFAULT_ORG_ID,
      orgs,
      workspaceTargets: workspaceCreationOrgTargets(auth, orgs),
    };
  } catch (err) {
    return { success: false, error: (err as Error).message || 'Failed to list organizations.', orgs: [] };
  }
});

// Get app paths
ipcMain.handle('app:getPath', async (_, name: string) => {
  return app.getPath(name as any);
});

// Get user home directory
ipcMain.handle('app:getHomeDir', async () => {
  return app.getPath('home');
});

// Get default openbrain directory
ipcMain.handle('app:getDefaultDir', async () => {
  return resolveDefaultWorkspacePath();
});

function normalizeLocalPickerPath(targetPath: unknown): string | null {
  const normalized = typeof targetPath === 'string' ? targetPath.trim() : '';
  if (!normalized) {
    return null;
  }
  return path.normalize(normalized);
}

async function safeGetAppDirectory(name: Parameters<typeof app.getPath>[0]): Promise<string | null> {
  try {
    const target = app.getPath(name as any);
    return typeof target === 'string' && target.trim() ? target : null;
  } catch {
    return null;
  }
}

ipcMain.handle('app:getLocalSpecialDirectories', async () => {
  const homeDir = app.getPath('home');
  const defaultDir = await resolveDefaultWorkspacePath();
  const desktopDir = await safeGetAppDirectory('desktop');
  const downloadsDir = await safeGetAppDirectory('downloads');
  const rootDir = path.parse(homeDir).root || path.parse(defaultDir).root || '/';

  const entries = [
    { key: 'workspace', label: mainT('error:main.specialDirWorkspace'), path: defaultDir },
    { key: 'home', label: mainT('error:main.specialDirHome'), path: homeDir },
    desktopDir ? { key: 'desktop', label: mainT('error:main.specialDirDesktop'), path: desktopDir } : null,
    downloadsDir ? { key: 'downloads', label: mainT('error:main.specialDirDownloads'), path: downloadsDir } : null,
    rootDir ? { key: 'root', label: mainT('error:main.specialDirRoot'), path: rootDir } : null,
  ].filter((item): item is { key: string; label: string; path: string } => Boolean(item));

  const seen = new Set<string>();
  return entries.filter((entry) => {
    const normalized = path.normalize(entry.path);
    if (!normalized || seen.has(normalized)) {
      return false;
    }
    seen.add(normalized);
    return true;
  });
});

ipcMain.handle('app:listLocalDirectory', async (_, payload?: { path?: string }) => {
  const targetPath = normalizeLocalPickerPath(payload?.path);
  if (!targetPath) {
    return { error: mainT('error:dirPicker.pathRequired') };
  }
  if (!path.isAbsolute(targetPath)) {
    return { error: mainT('error:dirPicker.pathAbsolute') };
  }

  try {
    const dirEntries = await fs.readdir(targetPath, { withFileTypes: true });
    const entries = (await Promise.all(dirEntries.map(async (entry) => {
      const entryPath = path.join(targetPath, entry.name);
      try {
        const stat = await fs.stat(entryPath);
        return {
          name: entry.name,
          isDir: stat.isDirectory(),
          size: stat.size,
          modTime: stat.mtimeMs,
        };
      } catch {
        return null;
      }
    }))).filter((entry): entry is { name: string; isDir: boolean; size: number; modTime: number } => Boolean(entry));

    return {
      path: targetPath,
      entries,
    };
  } catch (error) {
    return {
      error: error instanceof Error ? error.message : 'Failed to list local directory',
    };
  }
});

ipcMain.handle('app:statLocalPath', async (_, payload?: { path?: string }) => {
  const targetPath = normalizeLocalPickerPath(payload?.path);
  if (!targetPath) {
    return { error: mainT('error:dirPicker.pathRequired') };
  }
  if (!path.isAbsolute(targetPath)) {
    return { error: mainT('error:dirPicker.pathAbsolute') };
  }

  try {
    const stat = await fs.stat(targetPath);
    return {
      path: targetPath,
      name: path.basename(targetPath),
      size: stat.size,
      isDir: stat.isDirectory(),
      modTime: stat.mtimeMs,
    };
  } catch (error) {
    return {
      error: error instanceof Error ? error.message : 'Failed to stat local path',
    };
  }
});

ipcMain.handle('app:localMkdir', async (_, payload?: { path?: string }) => {
  const targetPath = normalizeLocalPickerPath(payload?.path);
  if (!targetPath) return { error: mainT('error:dirPicker.pathRequired') };
  if (!path.isAbsolute(targetPath)) return { error: mainT('error:dirPicker.pathAbsolute') };
  try {
    await fs.mkdir(targetPath, { recursive: true });
    return { success: true };
  } catch (error) {
    return { error: error instanceof Error ? error.message : 'Failed to create directory' };
  }
});

ipcMain.handle('app:localWriteFile', async (_, payload?: { path?: string; content?: string }) => {
  const targetPath = normalizeLocalPickerPath(payload?.path);
  if (!targetPath) return { error: mainT('error:dirPicker.pathRequired') };
  if (!path.isAbsolute(targetPath)) return { error: mainT('error:dirPicker.pathAbsolute') };
  try {
    await fs.mkdir(path.dirname(targetPath), { recursive: true });
    await fs.writeFile(targetPath, payload?.content || '', 'utf8');
    return {};
  } catch (error) {
    return { error: error instanceof Error ? error.message : 'Failed to write file' };
  }
});

ipcMain.handle('app:revealInFileManager', async (_, payload?: { path?: string }) => {
  const targetPath = typeof payload?.path === 'string' ? payload.path.trim() : '';
  if (!targetPath) {
    return { success: false, error: mainT('error:dirPicker.pathRequired') };
  }
  if (!path.isAbsolute(targetPath)) {
    return { success: false, error: mainT('error:dirPicker.pathAbsolute') };
  }
  try {
    shell.showItemInFolder(targetPath);
    return { success: true };
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : 'Failed to reveal item in file manager',
    };
  }
});

ipcMain.handle('pdfExport:getPayload', async (event) => {
  return pdfExportSessions.get(event.sender.id)?.payload || null;
});

ipcMain.on('pdfExport:reportReady', (event) => {
  settlePdfExportSession(event.sender.id);
});

ipcMain.on('pdfExport:reportError', (event, payload?: { message?: string }) => {
  const message = normalizeOptionalString(payload?.message) || 'Markdown PDF export renderer failed';
  settlePdfExportSession(event.sender.id, new Error(message));
});

ipcMain.handle('app:exportMarkdownPdfToPath', async (_, payload: unknown) => {
  const normalized = normalizeMarkdownPdfExportPayload(payload);
  if (!normalized) {
    return { canceled: false, error: 'Invalid markdown PDF export payload' };
  }
  const outputPathRaw = typeof (payload as { outputPath?: unknown })?.outputPath === 'string'
    ? ((payload as { outputPath: string }).outputPath).trim()
    : '';
  if (!outputPathRaw || !path.isAbsolute(outputPathRaw)) {
    return { canceled: false, error: 'Output path is required and must be absolute' };
  }
  const outputPath = ensurePdfExtension(outputPathRaw);

  let exportWin: BrowserWindow | null = null;
  try {
    const created = await createMarkdownPdfExportWindow(normalized);
    exportWin = created.win;
    await Promise.race([
      created.readyPromise,
      new Promise<never>((_, reject) => {
        setTimeout(() => reject(new Error('Timed out waiting for PDF renderer')), PDF_EXPORT_READY_TIMEOUT_MS);
      }),
    ]);
    const pdfData = await exportWin.webContents.printToPDF({
      printBackground: true,
      pageSize: 'A4',
      preferCSSPageSize: true,
      margins: { top: 0.5, bottom: 0.5, left: 0.5, right: 0.5 },
    });
    await fs.writeFile(outputPath, pdfData);
    return { canceled: false, filePath: outputPath };
  } catch (error) {
    return { canceled: false, error: error instanceof Error ? error.message : String(error) };
  } finally {
    if (exportWin && !exportWin.isDestroyed()) {
      pdfExportSessions.delete(exportWin.webContents.id);
      exportWin.close();
    }
  }
});

ipcMain.handle('app:getPdfExportDefaultPath', async (_, payload?: { sourcePath?: string; currentDir?: string; isRemote?: boolean }) => {
  const initialDefaultPath = buildMarkdownPdfDefaultPath({
    sourcePath: payload?.sourcePath || '',
    currentDir: payload?.currentDir || '',
  });
  const defaultPath = payload?.isRemote
    ? getLocalPdfFallbackPath(path.basename(initialDefaultPath))
    : resolvePdfSaveDialogDefaultPath(initialDefaultPath);
  return {
    defaultDir: path.dirname(defaultPath),
    defaultFileName: path.basename(defaultPath),
  };
});

// Backup IPC handlers for Untitled tabs
const getBackupsDir = async () => {
  const homeDir = app.getPath('home');
  const backupsDir = path.join(homeDir, '.openbrain', 'backups');
  await fs.mkdir(backupsDir, { recursive: true });
  return backupsDir;
};

ipcMain.handle('backup:save', async (_, data: {
  id: string;
  title: string;
  content: string;
  editorId: string;
}) => {
  const backupsDir = await getBackupsDir();
  const filePath = path.join(backupsDir, `${data.id}.json`);
  await fs.writeFile(filePath, JSON.stringify({
    ...data,
    timestamp: Date.now(),
  }), 'utf-8');
  return { success: true };
});

ipcMain.handle('backup:load', async () => {
  const backupsDir = await getBackupsDir();
  try {
    const files = await fs.readdir(backupsDir);
    const backups: Array<{
      id: string;
      title: string;
      content: string;
      editorId: string;
      timestamp: number;
    }> = [];
    for (const file of files) {
      if (!file.endsWith('.json')) continue;
      try {
        const filePath = path.join(backupsDir, file);
        const content = await fs.readFile(filePath, 'utf-8');
        backups.push(JSON.parse(content));
      } catch {
        // Skip invalid backup files
      }
    }
    return backups;
  } catch {
    return [];
  }
});

ipcMain.handle('backup:delete', async (_, tabId: string) => {
  const backupsDir = await getBackupsDir();
  const filePath = path.join(backupsDir, `${tabId}.json`);
  try {
    await fs.unlink(filePath);
  } catch {
    // Ignore if file doesn't exist
  }
  return { success: true };
});

ipcMain.handle('ssh:listHosts', async () => {
  const homeDir = app.getPath('home');
  const [configHosts, manualHosts] = await Promise.all([
    listSshHosts(),
    listManualSshHosts(homeDir),
  ]);
  return [...manualHosts, ...configHosts];
});

ipcMain.handle('ssh:saveHost', async (_, input) => {
  return saveManualSshHost(app.getPath('home'), input);
});

ipcMain.handle('ssh:deleteHost', async (_, params: { id?: string }) => {
  return deleteManualSshHost(app.getPath('home'), params?.id || '');
});

ipcMain.handle('ssh:pickIdentityFile', async (event) => {
  const record = getWindowRecordByWebContents(event.sender);
  const parentWindow = record?.win;
  const result = parentWindow
    ? await dialog.showOpenDialog(parentWindow, {
        properties: ['openFile'],
        title: mainT('error:main.selectSshKey'),
      })
    : await dialog.showOpenDialog({
        properties: ['openFile'],
        title: mainT('error:main.selectSshKey'),
      });
  if (result.canceled || result.filePaths.length === 0) {
    return { canceled: true };
  }
  return { canceled: false, path: result.filePaths[0] };
});

ipcMain.handle('remote:connectSsh', async (event, params: { host: SshHost; tabId: string }) => {
  const record = getWindowRecordByWebContents(event.sender);
  if (!record) {
    throw new Error('Window not found');
  }
  const host = await resolveSshHostForConnect(app.getPath('home'), params.host);
  // The connected Runtime returns its own defaultWorkspace through config/system/get.
  const session = await connectSsh(record.info.id, params.tabId, host);
  // Do not block the UI on config sync. Best-effort push in background.
  syncAllToTarget({ force: true }).catch((err) => {
    console.warn('Failed to sync config to remote:', err);
  });
  scheduleArchiveCleanupSoon();
  return session;
});

ipcMain.handle('remote:disconnect', async (event, params?: { tabId?: string }) => {
  const record = getWindowRecordByWebContents(event.sender);
  if (!record) {
    return { success: false };
  }
  await disconnectRemote(record.info.id, params?.tabId);
  return { success: true };
});

ipcMain.handle('remote:status', async (event, params: { tabId: string }) => {
  const record = getWindowRecordByWebContents(event.sender);
  if (!record) {
    return null;
  }
  return getRemoteStatus(record.info.id, params.tabId);
});

ipcMain.handle('settings:get', async () => {
  return ensureSettings();
});

ipcMain.handle('settings:getRoot', async () => {
  const homeDir = app.getPath('home');
  const settingsRoot = getSettingsRoot(homeDir);
  await ensureSettings();
  await fs.mkdir(settingsRoot, { recursive: true });
  return settingsRoot;
});

ipcMain.handle('settings:set', async (_, patch: Partial<SettingsState>) => {
  await ensureSettings();
  const homeDir = app.getPath('home');
  const merged = await saveSettings(homeDir, patch);
  settingsCache = merged;
  await persistThemeBackground(settingsCache);
  syncAgentSleepInhibitorFromSettings(settingsCache);
  broadcastSettingsChanged(settingsCache);
  return merged;
});

ipcMain.handle('power:setAgentRunning', async (event, payload?: { running?: boolean }) => {
  agentSleepInhibitor.setWindowRunning(event.sender.id, payload?.running === true);
});

ipcMain.on('settings:previewMarkdownTextOffset', async (_, payload: { value?: unknown }) => {
  const current = await ensureSettings();
  const nextValue = normalizeMarkdownTextOffset(payload?.value);
  settingsCache = {
    ...current,
    ui: {
      ...current.ui,
      markdownTextOffset: nextValue,
    },
  };
  broadcastSettingsChanged(settingsCache);
});

ipcMain.on('settings:previewMarkdownContentWidth', async (_, payload: { value?: unknown }) => {
  const current = await ensureSettings();
  const nextValue = normalizeMarkdownContentWidth(payload?.value);
  settingsCache = {
    ...current,
    ui: {
      ...current.ui,
      markdownContentWidth: nextValue,
    },
  };
  broadcastSettingsChanged(settingsCache);
});

// Auth IPC handlers
ipcMain.handle('auth:get', async () => {
  const homeDir = app.getPath('home');
  const loadedConfig = await loadAuthConfig(homeDir);
  const config = loadedConfig ? await ensureDefaultActiveOrg(homeDir, loadedConfig) : null;
  if (!config) {
    return null;
  }
  const profile = await loadProfileForAuth(homeDir, config);
  return {
    loggedIn: true,
    uid: config.uid,
    email: config.email,
    baseUrl: config.baseUrl,
    aiGateway: config.aiGateway,
    activeOrgID: config.activeOrgID,
    activeOrgName: config.activeOrgName,
    profile: profile || undefined,
  };
});

async function completeDeviceCodeLoginAttempt(
  attemptID: number,
  homeDir: string,
  gateway: string,
  gatewayInfo: GatewayInfo | null,
  orgSlug: string | undefined,
  result: DeviceTokenResponse,
) {
  if (attemptID !== activeDeviceLoginAttempt) {
    console.log('[Auth] Ignoring stale device code login result');
    return;
  }
  try {
    const authBaseUrl = result.baseUrl || gatewayInfo?.baseUrl || gateway;
    const authGateway = result.gateway || gatewayInfo?.gateway || gateway;
    const cfg = await ensureRequestedActiveOrg(
      homeDir,
      createAuthConfig(
        result.token,
        result.uid,
        result.email,
        authBaseUrl,
        authGateway,
        result.aiGateway || gatewayInfo?.aiGateway,
        result.defaultOrg?.id || gatewayInfo?.defaultOrg?.id,
        result.defaultOrg?.name || gatewayInfo?.defaultOrg?.name
      ),
      orgSlug
    );
    await saveAuthConfig(homeDir, cfg);
    console.log('[Auth] Device code auth saved:', {
      authPath: path.join(homeDir, '.openbrain', 'configs', 'user', 'auth.json'),
      uid: cfg.uid,
      email: cfg.email,
    });
    const profile =
      (await fetchAndSaveProfile(cfg.gateway, result.token)) ||
      (await saveFallbackProfile(homeDir, result.uid, result.email));
    console.log('[Auth] Device code login successful');
    activeDeviceLoginAttempt = 0;

    for (const { win } of windowRegistry.values()) {
      win.webContents.send('auth:deviceCodeComplete', { success: true });
    }
    broadcastAuthChanged(cfg, profile);
    const firstWindow = BrowserWindow.getAllWindows()[0];
    if (firstWindow) {
      if (firstWindow.isMinimized()) firstWindow.restore();
      firstWindow.focus();
    }
  } catch (err) {
    if (attemptID !== activeDeviceLoginAttempt) {
      console.log('[Auth] Ignoring stale device code login failure');
      return;
    }
    activeDeviceLoginAttempt = 0;
    if (isAuthInvalidError(err)) {
      await invalidateAuthSession('session_expired');
    }
    console.error('[Auth] Device code login failed:', err);
    for (const { win } of windowRegistry.values()) {
      win.webContents.send('auth:deviceCodeComplete', { success: false, error: err instanceof Error ? err.message : String(err) });
    }
  }
}

async function failDeviceCodeLoginAttempt(attemptID: number, err: unknown) {
  if (attemptID !== activeDeviceLoginAttempt) {
    console.log('[Auth] Ignoring stale device code login failure');
    return;
  }
  activeDeviceLoginAttempt = 0;
  if (isAuthInvalidError(err)) {
    await invalidateAuthSession('session_expired');
  }
  console.error('[Auth] Device code login failed:', err);
  for (const { win } of windowRegistry.values()) {
    win.webContents.send('auth:deviceCodeComplete', { success: false, error: err instanceof Error ? err.message : String(err) });
  }
}

async function startDeviceCodeLoginFlow(homeDir: string, gateway: string, gatewayInfo: GatewayInfo | null, orgSlug?: string) {
  const attemptID = activeDeviceLoginAttempt + 1;
  activeDeviceLoginAttempt = attemptID;
  let session: DeviceCodeSession;
  try {
    session = await requestDeviceCode(gateway);
  } catch (err) {
    if (attemptID === activeDeviceLoginAttempt) {
      activeDeviceLoginAttempt = 0;
    }
    throw err;
  }
  if (attemptID !== activeDeviceLoginAttempt) {
    throw new Error('A newer sign-in attempt is already running.');
  }

  console.log('[Auth] Device code:', session.userCode);
  const verificationLoginUri = deviceVerificationLoginUri(session.verificationUri);
  console.log('[Auth] Verification URL:', session.verificationUri);
  console.log('[Auth] Verification login URL:', verificationLoginUri);
  for (const { win } of windowRegistry.values()) {
    win.webContents.send('auth:deviceCode', {
      userCode: session.userCode,
      verificationUri: verificationLoginUri,
      expiresAt: session.expiresAt,
    });
  }

  void shell.openExternal(verificationLoginUri).catch((err) => {
    console.warn('[Auth] Failed to open device verification URL:', readableNetworkError(err));
  });

  void pollDeviceToken(session, gateway)
    .then((result) => completeDeviceCodeLoginAttempt(attemptID, homeDir, gateway, gatewayInfo, orgSlug, result))
    .catch((err) => {
      void failDeviceCodeLoginAttempt(attemptID, err);
    });
}

ipcMain.handle('auth:startLogin', async (_event, options?: LoginOptions) => {
  const homeDir = app.getPath('home');
  const loadedConfig = await loadAuthConfig(homeDir);
  const config = loadedConfig ? await ensureDefaultActiveOrg(homeDir, loadedConfig) : null;
  const requestedGateway = (options?.gateway || '').trim();
  const manualGateway = normalizeManualGateway(options?.gateway);
  const requestedOrgSlug = normalizeOrganizationCode(options?.orgSlug);

  if (requestedGateway && !manualGateway) {
    throw new Error('Custom gateway must be a valid http(s) URL.');
  }
  if (manualGateway) {
    await startDeviceCodeLoginFlow(homeDir, manualGateway, null, requestedOrgSlug);
    return { success: true, mode: 'device_code' };
  }

  const gatewayInfo = await discoverGatewayInfo();
  pendingLoginGatewayInfo = gatewayInfo;
  pendingLoginOrgSlug = requestedOrgSlug;

  // Device Code Flow is the desktop sign-in boundary: the desktop process only
  // becomes signed in after it polls a token and writes auth.json locally.
  console.log(app.isPackaged ? '[Auth] Starting Device Code Flow...' : '[Auth][Dev] Starting Device Code Flow...');
  await startDeviceCodeLoginFlow(homeDir, gatewayInfo.gateway, gatewayInfo, requestedOrgSlug);
  return { success: true, mode: 'device_code' };
});

ipcMain.handle('auth:logout', async () => {
  await invalidateAuthSession('logout');
  return { success: true };
});

ipcMain.handle('auth:setActiveOrg', async (_event, params: { orgID?: string | null; orgName?: string | null }) => {
  const homeDir = app.getPath('home');
  const loadedConfig = await loadAuthConfig(homeDir);
  const config = loadedConfig ? await ensureDefaultActiveOrg(homeDir, loadedConfig) : null;
  if (!config) {
    return { success: false, error: 'Not logged in' };
  }
  const orgID = normalizeActiveOrgID(params?.orgID);
  const next: AuthConfig = {
    ...config,
    activeOrgID: orgID,
    activeOrgName: orgID ? (params?.orgName || '').trim() || orgID : undefined,
    updatedAt: Date.now(),
  };
  await saveAuthConfig(homeDir, next);
  const profile = await loadProfileForAuth(homeDir, next);
  broadcastAuthChanged(next, profile);
  try {
    const localConfig = await loadModelsConfig(homeDir);
    const openbrainCatalogs = await fetchOpenBrainOrgCatalogs(next);
    const merged = mergeOpenBrainOrgCatalogs(localConfig, openbrainCatalogs, Date.now(), {
      activeOrgID: next.activeOrgID,
      privateOnly: isPrivateAuthService(next),
    });
    if (JSON.stringify(merged) !== JSON.stringify(localConfig)) {
      await saveModelsConfig(homeDir, merged);
    }
  } catch (err) {
    console.warn('[auth:setActiveOrg] openbrain model refresh failed:', (err as Error).message);
  }
  return {
    success: true,
    activeOrgID: next.activeOrgID,
    activeOrgName: next.activeOrgName,
  };
});

ipcMain.handle('billing:getSubscription', async () => {
  const homeDir = app.getPath('home');
  const config = await loadAuthConfig(homeDir);
  if (!config) {
    return { success: false, error: 'Not logged in' };
  }
  let subscription: BillingSubscription | null = null;
  try {
    subscription = await fetchBillingSubscription(config.gateway, config.token);
  } catch (err) {
    if (isAuthInvalidError(err)) {
      await invalidateAuthSession('session_expired');
      return { success: false, error: err.message || 'Session expired', authInvalid: true };
    }
    throw err;
  }
  if (!subscription) {
    return { success: false, error: 'Failed to fetch billing subscription' };
  }
  return {
    success: true,
    subscription: subscription as BillingSubscription,
  };
});

// Models IPC handlers
ipcMain.handle('models:get', async () => {
  const homeDir = app.getPath('home');
  const localConfig = await loadModelsConfig(homeDir);
  const loadedAuth = await loadAuthConfig(homeDir);
  const auth = loadedAuth ? await ensureDefaultActiveOrg(homeDir, loadedAuth) : null;
  if (!auth) {
    return localConfig;
  }
  try {
    const openbrainCatalogs = await fetchOpenBrainOrgCatalogs(auth);
    const merged = mergeOpenBrainOrgCatalogs(localConfig, openbrainCatalogs, Date.now(), {
      activeOrgID: auth.activeOrgID,
      privateOnly: isPrivateAuthService(auth),
    });
    if (JSON.stringify(merged) !== JSON.stringify(localConfig)) {
      return await saveModelsConfig(homeDir, merged);
    }
    return merged;
  } catch (err) {
    console.warn('[models:get] openbrain fetch failed:', (err as Error).message);
    return localConfig;
  }
});

ipcMain.handle('models:set', async (_event, config: ModelsConfig) => {
  const homeDir = app.getPath('home');
  return saveModelsConfig(homeDir, config);
});

ipcMain.handle('models:refreshFromOpenBrain', async () => {
  const homeDir = app.getPath('home');
  const localConfig = await loadModelsConfig(homeDir);
  const loadedAuth = await loadAuthConfig(homeDir);
  const auth = loadedAuth ? await ensureDefaultActiveOrg(homeDir, loadedAuth) : null;
  if (!auth) {
    return { success: false, error: 'Not logged in', config: localConfig };
  }
  try {
    const openbrainCatalogs = await fetchOpenBrainOrgCatalogs(auth);
    const merged = mergeOpenBrainOrgCatalogs(localConfig, openbrainCatalogs, Date.now(), {
      activeOrgID: auth.activeOrgID,
      privateOnly: isPrivateAuthService(auth),
    });
    const saved = JSON.stringify(merged) !== JSON.stringify(localConfig) ? await saveModelsConfig(homeDir, merged) : merged;
    return { success: true, config: saved };
  } catch (err) {
    return { success: false, error: (err as Error).message, config: localConfig };
  }
});

ipcMain.handle('dashboard:getHosts', async () => {
  const homeDir = app.getPath('home');
  const loadedAuth = await loadAuthConfig(homeDir);
  const auth = loadedAuth ? await ensureDefaultActiveOrg(homeDir, loadedAuth) : null;
  if (!auth) {
    return [];
  }
  return fetchDashboardHosts(auth);
});

// Profile IPC handlers
ipcMain.handle('profile:get', async () => {
  const homeDir = app.getPath('home');
  const loadedConfig = await loadAuthConfig(homeDir);
  const config = loadedConfig ? await ensureDefaultActiveOrg(homeDir, loadedConfig) : null;
  return config ? loadProfileForAuth(homeDir, config) : null;
});

ipcMain.handle('profile:refresh', async () => {
  const homeDir = app.getPath('home');
  const loadedConfig = await loadAuthConfig(homeDir);
  const config = loadedConfig ? await ensureDefaultActiveOrg(homeDir, loadedConfig) : null;
  if (!config) {
    return { success: false, error: 'Not logged in' };
  }
  let profile: UserProfile | null = null;
  try {
    profile = await fetchAndSaveProfile(config.gateway, config.token);
  } catch (err) {
    if (isAuthInvalidError(err)) {
      await invalidateAuthSession('session_expired');
      return { success: false, error: err.message || 'Session expired', authInvalid: true };
    }
    throw err;
  }
  if (!profile) {
    return { success: false, error: 'Failed to fetch profile' };
  }

  // Notify all windows about profile update
  for (const { win } of windowRegistry.values()) {
    win.webContents.send('auth:changed', authChangedPayload(config, profile));
  }

  return {
    success: true,
    activeOrgID: config.activeOrgID,
    activeOrgName: config.activeOrgName,
    profile,
  };
});

ipcMain.handle('nodes:get', async () => {
  const homeDir = app.getPath('home');
  return loadNodesJson(homeDir);
});

ipcMain.handle('nodes:upsert', async (_, params: { hostId: string; nodes: Array<Record<string, unknown>> }) => {
  try {
    const homeDir = app.getPath('home');
    const hostId = (params?.hostId || '').trim();
    const nodes = Array.isArray(params?.nodes) ? params.nodes : [];
    await upsertNodes(homeDir, hostId, nodes as any[]);
    return { success: true };
  } catch (err) {
    return { success: false, error: (err as Error)?.message || 'Failed to upsert nodes' };
  }
});

ipcMain.handle('avatar:cacheNode', async (_, params: { hostId: string; node: Record<string, unknown> }) => {
  try {
    const homeDir = app.getPath('home');
    const hostId = (params?.hostId || '').trim();
    const node = (params?.node || {}) as Record<string, unknown>;
    const updated = await cacheNodeAvatar(homeDir, hostId, node as any);
    return { success: true, node: updated };
  } catch (err) {
    return { success: false, error: (err as Error)?.message || 'Failed to cache node avatar' };
  }
});
