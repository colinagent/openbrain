import { create } from 'zustand';
import {
  applyOpenBrainSourceAction,
  archiveOpenBrainSource,
  createOpenBrainSource,
  getOpenBrainProviderStatus,
  getOpenBrainPublicBrainProfile,
  getOpenBrainSourceShare,
  listCachedOpenBrainSources,
  listGBrainSources,
  listOpenBrainRecoveryCandidates,
  listOpenBrainPublicBrains,
  removeOpenBrainSourceFromDevice,
  revokeOpenBrainSourceUserShare,
  revokeOpenBrainSourcePublic,
  setOpenBrainSourcePublic,
  shareOpenBrainSourceWithUser,
  followOpenBrainPublicBrain,
  unfollowOpenBrainPublicBrain,
  updateOpenBrainPublicBrainProfile,
  verifyOpenBrainSource,
  type OpenBrainListSourcesResponse,
  type OpenBrainCreateSourceResponse,
  type OpenBrainProviderStatus,
  type OpenBrainPublicBrainProfile,
  type OpenBrainRecoveryCandidate,
  type OpenBrainSourceActionResponse,
  type OpenBrainSourceShare,
} from '../services/openBrainService';
import type { SshHostPayload } from '../types/electron';
import { useTabManagerStore, type WorkspaceTab } from './tabManagerStore';
import { DEFAULT_OPENBRAIN_PEER_LINKS, type OpenBrainPeerLinkState } from '../components/OpenBrain/openBrainFlow';
import {
  readSourceLinkSettings,
  resolveSourceLinked,
  writeSourceLinkSettings,
  type SourceLinkSettings,
} from '../components/OpenBrain/openBrainLinkSettings';
import { useAppStore } from './appStore';
import { useAuthStore } from './authStore';

export type OpenBrainProviderMode = 'cloud' | 'local';

export type LocalOpenBrainWorkspace = {
  sourceID: string;
  name: string;
  path?: string;
  workspaceID?: string;
  orgID?: string;
  brainID?: string;
  updatedAt?: string;
  pageCount?: number;
  federated?: boolean;
  remoteURL?: string | null;
  openable: boolean;
  localName?: string;
  templateID?: string;
  templateVersion?: number;
  backupEnabled?: boolean;
  defaultLocalName?: string;
  locationKind?: 'local' | 'remote';
  remoteHost?: SshHostPayload;
  instanceID?: string;
  runtimeLabel?: string;
  runtimeReachable?: boolean;
  disabledQueries?: boolean;
  publicAccess?: boolean;
  effectivePermission?: 'read' | 'write' | 'admin';
  canMutateSource?: boolean;
  publicOwnerUID?: string;
  bindingMode?: 'own' | 'granted';
  bindingStatus?: 'connected' | 'needs_binding';
  bindingReason?: 'unbound' | 'moved' | 'mismatch';
  lastVerifiedAt?: string;
  lastVerifyReason?: string;
  /** Local graph link toggle; defaults to true when unset. */
  linked?: boolean;
};

export type PendingOpenBrainSource = {
  pendingID: string;
  name: string;
  path: string;
  locationKind: 'local' | 'remote';
  rebinding: boolean;
  status: 'creating' | 'failed';
  error?: string;
};

export function canManageOpenBrainSource(source: Pick<LocalOpenBrainWorkspace, 'bindingMode' | 'canMutateSource' | 'effectivePermission' | 'publicOwnerUID'> | null | undefined): boolean {
  if (!source) {
    return false;
  }
  if (source.bindingMode === 'granted' || source.publicOwnerUID) {
    return false;
  }
  if (source.effectivePermission === 'read') {
    return false;
  }
  if (typeof source.canMutateSource === 'boolean') {
    return source.canMutateSource;
  }
  return true;
}

export type FollowedPublicBrain = {
  brainID: string;
  ownerUID: string;
  name: string;
  username: string;
  ownerInitial?: string;
  avatar?: string;
  colorKey: string;
  activeSourceCount: number;
  description?: string;
  owned: boolean;
  member: boolean;
  offer?: { offerID: string; unitAmountU: string; currency: 'usd'; interval: 'month'; checkoutAvailable: boolean; includesAIUsage: false };
  membership?: { membershipID: string; status: string; currentPeriodEnd?: string; cancelAtPeriodEnd: boolean; includesAIUsage: false };
  accessMode?: 'public' | 'members_only';
  linked: boolean;
};

export type PublicBrainDirectoryEntry = {
  brainID: string;
  ownerUID: string;
  name: string;
  username: string;
  ownerInitial?: string;
  avatar?: string;
  activeSourceCount: number;
  followed: boolean;
  owned: boolean;
  description?: string;
  member: boolean;
  offer?: { offerID: string; unitAmountU: string; currency: 'usd'; interval: 'month'; checkoutAvailable: boolean; includesAIUsage: false };
  membership?: { membershipID: string; status: string; currentPeriodEnd?: string; cancelAtPeriodEnd: boolean; includesAIUsage: false };
  accessMode?: 'public' | 'members_only';
};

export type OpenBrainSourceActionOptions = {
  disableQueries?: boolean;
  enableQueries?: boolean;
  disableSync?: boolean;
  hardDelete?: boolean;
  confirmWorkspaceID?: string;
  confirmName?: string;
};

export type OpenBrainRuntimeError = Error & {
  code?: string;
  authRequired?: boolean;
  provider?: OpenBrainProviderMode;
  requestID?: string;
  cleanupAttempted?: boolean;
  cleanupSucceeded?: boolean;
  cleanupError?: string;
  pathOwnerUID?: string;
};

export type OpenBrainRuntimeConnection = {
  instanceID: string;
  tabId: string;
  tab: WorkspaceTab;
  reachable: boolean;
};

type OpenBrainStoreState = {
  provider: OpenBrainProviderMode;
  authRequired: boolean;
  githubConnected: boolean;
  cloudReady: boolean;
  providerStatusChecked: boolean;
  githubCheckError: string | null;
  sources: LocalOpenBrainWorkspace[];
  pendingSources: PendingOpenBrainSource[];
  publicBrains: FollowedPublicBrain[];
  publicBrainProfile: OpenBrainPublicBrainProfile | null;
  sourceLinkSettings: SourceLinkSettings;
  peerLinks: OpenBrainPeerLinkState;
  loading: boolean;
  refreshing: boolean;
  error: string | null;
  lastLoadedAt: number | null;
  refreshProviderStatus: () => Promise<OpenBrainProviderStatus>;
  hydrateCachedSources: (workspaceTabId?: string) => Promise<LocalOpenBrainWorkspace[]>;
  refreshInBackground: (workspaceTabId?: string, options?: RefreshInBackgroundOptions) => Promise<LocalOpenBrainWorkspace[]>;
  refresh: (workspaceTabId?: string) => Promise<LocalOpenBrainWorkspace[]>;
  createOpenBrain: (input: { name?: string; localPath?: string; remotePath?: string; tabId?: string; remoteHost?: SshHostPayload; source?: LocalOpenBrainWorkspace }) => Promise<LocalOpenBrainWorkspace>;
  beginPendingOpenBrainSource: (input: { name: string; path: string; locationKind: 'local' | 'remote'; rebinding: boolean }) => string;
  completePendingOpenBrainSource: (pendingID: string) => void;
  failPendingOpenBrainSource: (pendingID: string, error: string) => void;
  dismissPendingOpenBrainSource: (pendingID: string) => void;
  verifyOpenBrain: (workspace: LocalOpenBrainWorkspace) => Promise<LocalOpenBrainWorkspace>;
  listRecoveryCandidates: (workspace: LocalOpenBrainWorkspace, paths: string[]) => Promise<OpenBrainRecoveryCandidate[]>;
  removeFromDevice: (workspace: LocalOpenBrainWorkspace) => Promise<void>;
  archiveSource: (workspace: LocalOpenBrainWorkspace) => Promise<void>;
  applySourceAction: (workspace: LocalOpenBrainWorkspace, action: OpenBrainSourceActionOptions) => Promise<OpenBrainSourceActionResponse>;
  setSourceLinked: (sourceID: string, linked: boolean) => void;
  isSourceLinked: (sourceID: string) => boolean;
  togglePeerLink: (peerID: string) => void;
  getSourceShare: (workspace: LocalOpenBrainWorkspace) => Promise<OpenBrainSourceShare>;
  shareSourceWithUser: (workspace: LocalOpenBrainWorkspace, email: string) => Promise<void>;
  revokeSourceUserShare: (workspace: LocalOpenBrainWorkspace, uid: string) => Promise<void>;
  setSourcePublic: (workspace: LocalOpenBrainWorkspace) => Promise<void>;
  revokeSourcePublic: (workspace: LocalOpenBrainWorkspace) => Promise<void>;
  getPublicBrainProfile: () => Promise<OpenBrainPublicBrainProfile>;
  updatePublicBrainProfile: (description: string) => Promise<OpenBrainPublicBrainProfile>;
  followPublicBrain: (ownerUID: string) => Promise<void>;
  unfollowPublicBrain: (ownerUID: string) => Promise<void>;
  listPublicBrainDirectory: (query: string) => Promise<PublicBrainDirectoryEntry[]>;
};

function normalizeInstanceIDPart(value: string | null | undefined): string {
  return (value || '').trim().replace(/\\/g, '/').replace(/\/+$/, '');
}

function tabRuntimeReachable(tab: WorkspaceTab | null): boolean {
  if (!tab) {
    return false;
  }
  if (tab.kind !== 'remote') {
    return true;
  }
  return Boolean(useAppStore.getStoreByTabId(tab.id).getState().remoteSession?.localPort);
}

function resolveInstanceIDForWorkspaceTab(runtimeTabId: string, runtimeTab: WorkspaceTab | null): string {
  const appState = useAppStore.getStoreByTabId(runtimeTabId).getState();
  const instanceID = (appState.instanceID || '').trim();
  if (instanceID) {
    return instanceID;
  }
  const session = appState.remoteSession;
  if (session) {
    const hostLabel = normalizeInstanceIDPart(session.hostLabel);
    const remoteHome = normalizeInstanceIDPart(session.remoteHome);
    const installDir = normalizeInstanceIDPart(session.installDir);
    if (hostLabel && remoteHome && installDir) {
      return `remote:${hostLabel}|${remoteHome}|${installDir}`;
    }
  }
  if (runtimeTab?.kind === 'remote') {
    const remoteHostID = (runtimeTab.remoteHost?.id || '').trim();
    if (remoteHostID) {
      return `${runtimeTab.remoteHost?.source || 'remote'}:${remoteHostID}`;
    }
    const remoteLabel = normalizeInstanceIDPart(runtimeTab.remoteHost?.alias || runtimeTab.label);
    if (remoteLabel) {
      return `remote:${remoteLabel}`;
    }
  }
  return 'local:default';
}

function runtimeConnectionForInstanceID(instanceID: string): OpenBrainRuntimeConnection | null {
  const targetInstanceID = instanceID.trim();
  if (!targetInstanceID) {
    return null;
  }
  const tabState = useTabManagerStore.getState();
  for (const tab of tabState.tabs) {
    if (resolveInstanceIDForWorkspaceTab(tab.id, tab) !== targetInstanceID) {
      continue;
    }
    return {
      instanceID: targetInstanceID,
      tabId: tab.id,
      tab,
      reachable: tabRuntimeReachable(tab),
    };
  }
  return null;
}

export function openBrainRuntimeConnectionForWorkspace(workspace: Pick<LocalOpenBrainWorkspace, 'instanceID'>): OpenBrainRuntimeConnection | null {
  return runtimeConnectionForInstanceID((workspace.instanceID || '').trim());
}

function runtimeTabForWorkspace(workspace: LocalOpenBrainWorkspace): WorkspaceTab | null {
  const connection = openBrainRuntimeConnectionForWorkspace(workspace);
  return connection?.reachable ? connection.tab : null;
}

function routeTabIDForWorkspace(workspace: LocalOpenBrainWorkspace): string | undefined {
  const instanceID = (workspace.instanceID || '').trim();
  if (!instanceID) {
    return undefined;
  }
  const connection = runtimeConnectionForInstanceID(instanceID);
  if (connection?.reachable) {
    return connection.tabId;
  }
  const error = new Error('OpenBrain runtime is not connected.') as OpenBrainRuntimeError;
  error.code = 'runtime_unreachable';
  throw error;
}

function createOpenBrainRequestID(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return `src-${crypto.randomUUID()}`;
  }
  return `src-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

function createPendingOpenBrainSourceID(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return `pending-${crypto.randomUUID()}`;
  }
  return `pending-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

function normalizePendingOpenBrainPath(value: string): string {
  return value.trim().replace(/\\/g, '/').replace(/\/+$/, '');
}

function normalizeSources(value: unknown, workspaceTabId?: string): LocalOpenBrainWorkspace[] {
  if (!Array.isArray(value)) {
    return [];
  }
  const tabState = useTabManagerStore.getState();
  const runtimeTabId = workspaceTabId || tabState.activeTabId;
  const runtimeTab = tabState.tabs.find((tab) => tab.id === runtimeTabId) || null;
  const runtimeRemoteHost = runtimeTab?.kind === 'remote' ? runtimeTab.remoteHost : undefined;
  const runtimeLabel = runtimeTab?.label || (runtimeTab?.kind === 'remote' ? runtimeTab.remoteHost?.alias : 'Local') || 'Local';
  const instanceID = resolveInstanceIDForWorkspaceTab(runtimeTabId, runtimeTab);
  return value.reduce<LocalOpenBrainWorkspace[]>((acc, raw) => {
    const item = raw as Partial<LocalOpenBrainWorkspace>;
    const sourceID = (item.sourceID || item.workspaceID || '').trim();
    if (!sourceID) {
      return acc;
    }
    const sourcePath = (item.path || '').trim() || undefined;
    const name = (item.name || '').trim() || sourcePath?.split('/').filter(Boolean).pop() || sourceID;
    acc.push({
      sourceID,
      name,
      path: sourcePath,
      workspaceID: (item.workspaceID || sourceID).trim() || undefined,
      orgID: (item.orgID || '').trim() || undefined,
      brainID: (item.brainID || '').trim() || undefined,
      updatedAt: (item.updatedAt || '').trim() || undefined,
      pageCount: typeof item.pageCount === 'number' ? item.pageCount : undefined,
      federated: item.federated === true,
      remoteURL: item.remoteURL || null,
      openable: item.openable !== false && Boolean(sourcePath),
      locationKind: runtimeRemoteHost || item.locationKind === 'remote' ? 'remote' : sourcePath ? 'local' : undefined,
      remoteHost: item.remoteHost || runtimeRemoteHost,
      instanceID,
      runtimeLabel,
      runtimeReachable: tabRuntimeReachable(runtimeTab),
      disabledQueries: item.disabledQueries === true,
      publicAccess: typeof item.publicAccess === 'boolean' ? item.publicAccess : undefined,
      effectivePermission: item.effectivePermission === 'read' || item.effectivePermission === 'write' || item.effectivePermission === 'admin' ? item.effectivePermission : undefined,
      canMutateSource: typeof item.canMutateSource === 'boolean' ? item.canMutateSource : undefined,
      publicOwnerUID: (item.publicOwnerUID || '').trim() || undefined,
      bindingMode: item.bindingMode === 'granted' ? 'granted' : item.bindingMode === 'own' ? 'own' : undefined,
      bindingStatus: item.bindingStatus === 'needs_binding' ? 'needs_binding' : 'connected',
      bindingReason: item.bindingReason === 'unbound' || item.bindingReason === 'moved' || item.bindingReason === 'mismatch' ? item.bindingReason : undefined,
      lastVerifiedAt: (item.lastVerifiedAt || '').trim() || undefined,
      lastVerifyReason: (item.lastVerifyReason || '').trim() || undefined,
      linked: typeof item.linked === 'boolean' ? item.linked : undefined,
    });
    return acc;
  }, []);
}

function withoutWorkspace(sources: LocalOpenBrainWorkspace[], workspace: LocalOpenBrainWorkspace): LocalOpenBrainWorkspace[] {
  const ids = new Set([workspace.sourceID, workspace.workspaceID].filter(Boolean));
  return sources.filter((source) => {
    if (workspace.instanceID && !sourceMatchesRuntime(source, workspace.instanceID)) {
      return true;
    }
    return !ids.has(source.sourceID) && !ids.has(source.workspaceID);
  });
}

function mergeRuntimeSources(
  current: LocalOpenBrainWorkspace[],
  runtimeTabId: string | undefined,
  nextRuntimeSources: LocalOpenBrainWorkspace[],
): LocalOpenBrainWorkspace[] {
  if (!runtimeTabId) {
    return nextRuntimeSources;
  }
  const runtimeTab = useTabManagerStore.getState().tabs.find((tab) => tab.id === runtimeTabId) || null;
  const instanceID = nextRuntimeSources[0]?.instanceID || resolveInstanceIDForWorkspaceTab(runtimeTabId, runtimeTab);
  const currentRuntimeSources = current.filter((source) => sourceMatchesRuntime(source, instanceID));
  const byKey = new Map<string, LocalOpenBrainWorkspace>();
  for (const source of current) {
    if (sourceMatchesRuntime(source, instanceID)) {
      continue;
    }
    byKey.set(sourceKey(source), source);
  }
  for (const source of nextRuntimeSources) {
    const existing = currentRuntimeSources.find((candidate) => sameOpenBrainSource(candidate, source));
    const key = sourceKey(source);
    byKey.set(key, mergeOpenBrainSource(source, byKey.get(key) || existing));
  }
  return Array.from(byKey.values()).sort((a, b) => a.name.localeCompare(b.name));
}

function upsertRuntimeSource(
  current: LocalOpenBrainWorkspace[],
  nextSource: LocalOpenBrainWorkspace,
): LocalOpenBrainWorkspace[] {
  const byKey = new Map(current.map((source) => [sourceKey(source), source]));
  byKey.set(sourceKey(nextSource), nextSource);
  return Array.from(byKey.values()).sort((a, b) => a.name.localeCompare(b.name));
}

function sourceKey(source: LocalOpenBrainWorkspace): string {
  const cloudKey = cloudSourceKey(source);
  if (cloudKey) {
    return cloudKey;
  }
  const runtime = source.instanceID || 'active';
  return `${runtime}:${source.sourceID || source.workspaceID || source.path || source.name}`;
}

function cloudSourceKey(source: LocalOpenBrainWorkspace): string {
  const orgID = (source.orgID || '').trim();
  const sourceID = (source.workspaceID || source.sourceID || '').trim();
  if (!orgID || orgID === 'local' || !sourceID) {
    return '';
  }
  return `cloud:${orgID}:${sourceID}`;
}

function sourceMatchesRuntime(source: LocalOpenBrainWorkspace, instanceID?: string): boolean {
  return Boolean(source.instanceID && instanceID && source.instanceID === instanceID);
}

function bindingScore(source: LocalOpenBrainWorkspace): number {
  let score = 0;
  if (source.runtimeReachable !== false) {
    score += 8;
  }
  if (source.bindingStatus !== 'needs_binding') {
    score += 4;
  }
  if (source.path) {
    score += 2;
  }
  if (source.openable) {
    score += 1;
  }
  return score;
}

function betterRuntimeBinding(
  next: LocalOpenBrainWorkspace,
  existing: LocalOpenBrainWorkspace,
): LocalOpenBrainWorkspace {
  return bindingScore(next) >= bindingScore(existing) ? next : existing;
}

function mergeOpenBrainSource(
  next: LocalOpenBrainWorkspace,
  existing: LocalOpenBrainWorkspace | undefined,
): LocalOpenBrainWorkspace {
  if (!existing) {
    return next;
  }
  const binding = cloudSourceKey(next) && cloudSourceKey(next) === cloudSourceKey(existing)
    ? betterRuntimeBinding(next, existing)
    : next;
  return {
    ...next,
    path: binding.path || next.path || existing.path,
    openable: binding.openable || next.openable || existing.openable,
    locationKind: binding.locationKind || next.locationKind || existing.locationKind,
    remoteHost: binding.remoteHost || next.remoteHost || existing.remoteHost,
    instanceID: binding.instanceID || next.instanceID || existing.instanceID,
    runtimeLabel: binding.runtimeLabel || next.runtimeLabel || existing.runtimeLabel,
    runtimeReachable: binding.runtimeReachable,
    bindingStatus: binding.bindingStatus || next.bindingStatus || existing.bindingStatus,
    bindingReason: binding.bindingReason || next.bindingReason || existing.bindingReason,
    remoteURL: next.remoteURL || existing.remoteURL,
    publicAccess: typeof next.publicAccess === 'boolean' ? next.publicAccess : existing.publicAccess,
    effectivePermission: next.effectivePermission || existing.effectivePermission,
    canMutateSource: typeof next.canMutateSource === 'boolean' ? next.canMutateSource : existing.canMutateSource,
    publicOwnerUID: next.publicOwnerUID || existing.publicOwnerUID,
    bindingMode: next.bindingMode || existing.bindingMode,
    lastVerifiedAt: next.lastVerifiedAt || existing.lastVerifiedAt,
    lastVerifyReason: next.lastVerifyReason || existing.lastVerifyReason,
  };
}

function sameOpenBrainSource(a: LocalOpenBrainWorkspace, b: LocalOpenBrainWorkspace): boolean {
  const aIDs = [a.sourceID, a.workspaceID].map((value) => (value || '').trim()).filter(Boolean);
  const bIDs = [b.sourceID, b.workspaceID].map((value) => (value || '').trim()).filter(Boolean);
  if (aIDs.some((id) => bIDs.includes(id))) {
    return true;
  }
  const aRemote = normalizeOpenBrainRemote(a.remoteURL);
  const bRemote = normalizeOpenBrainRemote(b.remoteURL);
  if (aRemote && bRemote && aRemote === bRemote) {
    return true;
  }
  return Boolean(a.path && b.path && normalizeOpenBrainPath(a.path) === normalizeOpenBrainPath(b.path));
}

function markRuntimeReachability(sources: LocalOpenBrainWorkspace[]): LocalOpenBrainWorkspace[] {
  const byKey = new Map<string, LocalOpenBrainWorkspace>();
  for (const source of sources) {
    const next = {
      ...source,
      runtimeReachable: source.instanceID
        ? Boolean(runtimeTabForWorkspace(source))
        : true,
    };
    const key = sourceKey(next);
    byKey.set(key, mergeOpenBrainSource(next, byKey.get(key)));
  }
  return Array.from(byKey.values()).sort((a, b) => a.name.localeCompare(b.name));
}

function openBrainRuntimeError(message: string, code?: string, extra?: Partial<OpenBrainRuntimeError>): OpenBrainRuntimeError {
  const error = new Error(message) as OpenBrainRuntimeError;
  error.code = code;
  Object.assign(error, extra);
  return error;
}

function openBrainCreateSourceRuntimeError(
  result: OpenBrainCreateSourceResponse | null | undefined,
  fallbackProvider: OpenBrainProviderMode,
): OpenBrainRuntimeError {
  const cleanupError = (result?.cleanupError || '').trim();
  const requestID = (result?.requestID || '').trim();
  let message = result?.error || 'Failed to create OpenBrain source.';
  if (cleanupError) {
    message = `${message} Cleanup rollback failed: ${cleanupError}`;
    if (requestID) {
      message = `${message} Request ID: ${requestID}.`;
    }
  }
  return openBrainRuntimeError(message, result?.code, {
    authRequired: result?.authRequired === true,
    provider: normalizeProviderMode(result?.provider, fallbackProvider),
    requestID: result?.requestID,
    cleanupAttempted: result?.cleanupAttempted === true,
    cleanupSucceeded: result?.cleanupSucceeded === true,
    cleanupError: result?.cleanupError,
    pathOwnerUID: result?.pathOwnerUID,
  });
}

function normalizeOpenBrainRemote(value: string | null | undefined): string {
  return (value || '').trim().replace(/\/+$/, '').replace(/\.git$/i, '').toLowerCase();
}

function normalizeOpenBrainPath(value: string | null | undefined): string {
  return (value || '').trim().replace(/\/+$/, '');
}

function workspaceIdentityPayload(workspace: LocalOpenBrainWorkspace) {
  return {
    sourceID: workspace.sourceID,
    workspaceID: workspace.workspaceID,
    orgID: workspace.orgID,
    path: workspace.path,
  };
}

function applySourceLinkSettings(
  sources: LocalOpenBrainWorkspace[],
  settings: SourceLinkSettings,
): LocalOpenBrainWorkspace[] {
  return sources.map((source) => ({
    ...source,
    linked: source.disabledQueries === true ? false : resolveSourceLinked(settings, source.sourceID),
  }));
}

function updateWorkspaceQueriesDisabled(
  sources: LocalOpenBrainWorkspace[],
  workspace: LocalOpenBrainWorkspace,
  disabledQueries: boolean,
  settings: SourceLinkSettings,
): LocalOpenBrainWorkspace[] {
  const ids = new Set([workspace.sourceID, workspace.workspaceID].filter(Boolean));
  return sources.map((source) => {
    if (workspace.instanceID && !sourceMatchesRuntime(source, workspace.instanceID)) {
      return source;
    }
    if (!ids.has(source.sourceID) && !ids.has(source.workspaceID)) {
      return source;
    }
    return {
      ...source,
      disabledQueries,
      linked: disabledQueries ? false : resolveSourceLinked(settings, source.sourceID),
    };
  });
}

function updateWorkspacePublicAccess(
  sources: LocalOpenBrainWorkspace[],
  workspace: LocalOpenBrainWorkspace,
  publicAccess: boolean,
): LocalOpenBrainWorkspace[] {
  const ids = new Set([workspace.sourceID, workspace.workspaceID].filter(Boolean));
  return sources.map((source) => {
    if (workspace.instanceID && !sourceMatchesRuntime(source, workspace.instanceID)) {
      return source;
    }
    if (!ids.has(source.sourceID) && !ids.has(source.workspaceID)) {
      return source;
    }
    return {
      ...source,
      publicAccess,
    };
  });
}

function normalizePublicBrainEntry(entry: Partial<PublicBrainDirectoryEntry>): PublicBrainDirectoryEntry | null {
  const brainID = (entry.brainID || '').trim();
  const ownerUID = (entry.ownerUID || '').trim();
  const name = (entry.name || '').trim();
  const username = (entry.username || '').trim();
  if (!brainID || !ownerUID || !name || !username) {
    return null;
  }
  return {
    brainID,
    ownerUID,
    name,
    username,
    ownerInitial: (entry.ownerInitial || '').trim() || undefined,
    avatar: (entry.avatar || '').trim() || undefined,
    activeSourceCount: typeof entry.activeSourceCount === 'number' ? entry.activeSourceCount : 0,
    followed: entry.followed === true,
    owned: entry.owned === true,
    description: (entry.description || '').trim() || undefined,
    member: entry.member === true,
    offer: entry.offer,
    membership: entry.membership,
    accessMode: entry.accessMode,
  };
}

function publicBrainHasActiveSources(brain: { activeSourceCount?: number }): boolean {
  return (brain.activeSourceCount || 0) > 0;
}

function followedBrainFromEntry(entry: PublicBrainDirectoryEntry): FollowedPublicBrain {
  return {
    brainID: entry.brainID,
    ownerUID: entry.ownerUID,
    name: entry.name,
    username: entry.username,
    ownerInitial: entry.ownerInitial,
    avatar: entry.avatar,
    colorKey: `brain:${entry.ownerUID}`,
    activeSourceCount: entry.activeSourceCount,
    description: entry.description,
    owned: entry.owned,
    member: entry.member,
    offer: entry.offer,
    membership: entry.membership,
    accessMode: entry.accessMode,
    linked: true,
  };
}

function followedBrainsFromDirectory(entries: PublicBrainDirectoryEntry[]): FollowedPublicBrain[] {
  return entries
    .filter((entry) => (entry.followed || entry.owned || entry.member) && publicBrainHasActiveSources(entry))
    .map(followedBrainFromEntry);
}

function providerStatusState(providerStatus: OpenBrainProviderStatus, fallbackProvider: OpenBrainProviderMode = 'cloud') {
  return {
    provider: providerStatus?.provider || fallbackProvider,
    authRequired: providerStatus?.authRequired === true,
    githubConnected: providerStatus?.githubConnected === true,
    cloudReady: providerStatus?.cloudReady === true,
    providerStatusChecked: true,
    githubCheckError: providerStatus?.githubCheckError || null,
  };
}

function normalizeProviderMode(value: unknown, fallback: OpenBrainProviderMode = 'cloud'): OpenBrainProviderMode {
  if (value === 'local' || value === 'cloud') {
    return value;
  }
  return fallback;
}

async function loadFollowedPublicBrains(
  provider: OpenBrainProviderMode,
  workspaceTabId: string | undefined,
): Promise<FollowedPublicBrain[]> {
  if (provider === 'local') {
    return [];
  }
  const entries = (await listOpenBrainPublicBrains('', workspaceTabId, { includeSelf: true }))
    .map(normalizePublicBrainEntry)
    .filter((entry): entry is PublicBrainDirectoryEntry => Boolean(entry));
  return followedBrainsFromDirectory(entries);
}

async function loadFollowedPublicBrainsForStore(
  provider: OpenBrainProviderMode,
  workspaceTabId: string | undefined,
  current: FollowedPublicBrain[],
): Promise<{ publicBrains: FollowedPublicBrain[]; loaded: boolean }> {
  if (provider === 'local') {
    return { publicBrains: [], loaded: true };
  }
  try {
    return {
      publicBrains: await loadFollowedPublicBrains(provider, workspaceTabId),
      loaded: true,
    };
  } catch {
    return {
      publicBrains: current,
      loaded: false,
    };
  }
}

function mergeLoadedSources(
  current: LocalOpenBrainWorkspace[],
  rawSources: unknown,
  workspaceTabId: string | undefined,
  sourceLinkSettings: SourceLinkSettings,
): LocalOpenBrainWorkspace[] {
  const sources = normalizeSources(rawSources, workspaceTabId);
  const runtimeTabId = workspaceTabId || useTabManagerStore.getState().activeTabId;
  return applySourceLinkSettings(
    mergeRuntimeSources(current, runtimeTabId, sources),
    sourceLinkSettings,
  );
}

function runtimeTabIDsForOpenBrainRefresh(workspaceTabId?: string): string[] {
  if (workspaceTabId) {
    return [workspaceTabId];
  }
  const tabState = useTabManagerStore.getState();
  const ids: string[] = [];
  let localAdded = false;
  for (const tab of tabState.tabs) {
    if (tab.kind === 'local') {
      if (!localAdded) {
        ids.push(tab.id);
        localAdded = true;
      }
      continue;
    }
    const appState = useAppStore.getStoreByTabId(tab.id).getState();
    if (appState.remoteSession?.localPort) {
      ids.push(tab.id);
    }
  }
  if (ids.length === 0) {
    ids.push(tabState.activeTabId);
  }
  return ids;
}

async function loadSourcesForRuntime(
  workspaceTabId: string,
  provider: OpenBrainProviderMode,
  cached = false,
): Promise<OpenBrainListSourcesResponse | null> {
  try {
    if (cached && provider !== 'local') {
      return await listCachedOpenBrainSources(workspaceTabId);
    }
    return await listGBrainSources(workspaceTabId, { provider });
  } catch {
    return null;
  }
}

export type RefreshInBackgroundOptions = {
  /**
   * When false (the default), background refresh is skipped if the last successful
   * load is fresher than OPENBRAIN_REFRESH_TTL_MS. Pass force: true for explicit
   * triggers (window focus, cloud-ready transition, account switch).
   */
  force?: boolean;
};

const OPENBRAIN_REFRESH_TTL_MS = 60_000;

const PUBLIC_BRAINS_CACHE_PREFIX = 'openbrain.graph.followedPublicBrains.v2';
const PUBLIC_BRAIN_PROFILE_CACHE_PREFIX = 'openbrain.graph.publicBrainProfile.v1';

function currentOpenBrainUid(): string | undefined {
  const uid = useAuthStore.getState().uid;
  return uid && uid.trim() ? uid.trim() : undefined;
}

function publicBrainsCacheKey(uid: string): string {
  return `${PUBLIC_BRAINS_CACHE_PREFIX}.${uid}`;
}

function publicBrainProfileCacheKey(uid: string): string {
  return `${PUBLIC_BRAIN_PROFILE_CACHE_PREFIX}.${uid}`;
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function readCachedPublicBrains(uid: string): FollowedPublicBrain[] | null {
  try {
    const raw = window.localStorage.getItem(publicBrainsCacheKey(uid));
    if (!raw) {
      return null;
    }
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) {
      return null;
    }
    return parsed.filter((entry): entry is FollowedPublicBrain => (
      isPlainObject(entry)
      && typeof entry.brainID === 'string'
      && typeof entry.ownerUID === 'string'
      && typeof entry.name === 'string'
      && typeof entry.username === 'string'
    ));
  } catch {
    return null;
  }
}

function writeCachedPublicBrains(uid: string, brains: FollowedPublicBrain[]): void {
  try {
    window.localStorage.setItem(publicBrainsCacheKey(uid), JSON.stringify(brains));
  } catch {
    // localStorage may be unavailable
  }
}

function readCachedPublicBrainProfile(uid: string): OpenBrainPublicBrainProfile | null {
  try {
    const raw = window.localStorage.getItem(publicBrainProfileCacheKey(uid));
    if (!raw) {
      return null;
    }
    const parsed = JSON.parse(raw) as unknown;
    if (!isPlainObject(parsed)) {
      return null;
    }
    if (typeof parsed.ownerUID !== 'string' || typeof parsed.name !== 'string') {
      return null;
    }
    return parsed as unknown as OpenBrainPublicBrainProfile;
  } catch {
    return null;
  }
}

function writeCachedPublicBrainProfile(uid: string, profile: OpenBrainPublicBrainProfile | null): void {
  try {
    if (!profile) {
      window.localStorage.removeItem(publicBrainProfileCacheKey(uid));
      return;
    }
    window.localStorage.setItem(publicBrainProfileCacheKey(uid), JSON.stringify(profile));
  } catch {
    // localStorage may be unavailable
  }
}

function persistLoadedPublicBrains(provider: OpenBrainProviderMode, brains: FollowedPublicBrain[]): void {
  if (provider === 'local') {
    return;
  }
  const uid = currentOpenBrainUid();
  if (!uid) {
    return;
  }
  writeCachedPublicBrains(uid, brains);
}

export const useOpenBrainStore = create<OpenBrainStoreState>((set, get) => ({
  provider: 'cloud',
  authRequired: false,
  githubConnected: false,
  cloudReady: false,
  providerStatusChecked: false,
  githubCheckError: null,
  sources: [],
  pendingSources: [],
  publicBrains: [],
  publicBrainProfile: null,
  sourceLinkSettings: readSourceLinkSettings(),
  peerLinks: { ...DEFAULT_OPENBRAIN_PEER_LINKS },
  loading: false,
  refreshing: false,
  error: null,
  lastLoadedAt: null,

  refreshProviderStatus: async () => {
    try {
      const providerStatus = await getOpenBrainProviderStatus();
      set(providerStatusState(providerStatus, get().provider));
      return providerStatus;
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to check OpenBrain Cloud status.';
      set({ providerStatusChecked: true, githubCheckError: message });
      throw new Error(message);
    }
  },

  hydrateCachedSources: async (workspaceTabId?: string) => {
    const provider = get().provider;
    try {
      let merged = markRuntimeReachability(get().sources);
      let resolvedProvider = provider;
      for (const runtimeTabId of runtimeTabIDsForOpenBrainRefresh(workspaceTabId)) {
        const result = await loadSourcesForRuntime(runtimeTabId, provider, true);
        if (!result?.success) {
          continue;
        }
        resolvedProvider = normalizeProviderMode(result.provider, resolvedProvider);
        merged = mergeLoadedSources(
          merged,
          result.sources,
          runtimeTabId,
          get().sourceLinkSettings,
        );
      }
      set({
        provider: resolvedProvider,
        sources: merged,
        publicBrains: provider === 'local' ? [] : get().publicBrains,
        error: null,
        // Cache hydration must not refresh the TTL window; otherwise a subsequent
        // non-force background refresh would be skipped and never reach the cloud.
        lastLoadedAt: get().lastLoadedAt,
      });
      // Restore per-account public brain cache so the graph can render the public
      // OpenBrain peer node on the first frame instead of waiting for the cloud.
      if (provider !== 'local') {
        const uid = currentOpenBrainUid();
        if (uid) {
          if (get().publicBrains.length === 0) {
            const cachedBrains = readCachedPublicBrains(uid);
            if (cachedBrains && cachedBrains.length > 0) {
              set({ publicBrains: cachedBrains });
            }
          }
          if (!get().publicBrainProfile) {
            const cachedProfile = readCachedPublicBrainProfile(uid);
            if (cachedProfile) {
              set({ publicBrainProfile: cachedProfile });
            }
          }
        }
      }
      return merged;
    } catch {
      return get().sources;
    }
  },

  refreshInBackground: async (workspaceTabId?: string, options?: RefreshInBackgroundOptions) => {
    if (get().refreshing || get().loading) {
      return get().sources;
    }
    const lastLoadedAt = get().lastLoadedAt;
    if (!options?.force && lastLoadedAt && Date.now() - lastLoadedAt < OPENBRAIN_REFRESH_TTL_MS) {
      return get().sources;
    }
    set({ refreshing: true });
    try {
      const providerStatus = await getOpenBrainProviderStatus().catch(() => null);
      const provider = normalizeProviderMode(providerStatus?.provider, get().provider);
      let merged = markRuntimeReachability(get().sources);
      let authRequired = providerStatus?.authRequired === true;
      let loadedAny = false;
      let lastError = 'Failed to list OpenBrain sources.';
      for (const runtimeTabId of runtimeTabIDsForOpenBrainRefresh(workspaceTabId)) {
        const result = await loadSourcesForRuntime(runtimeTabId, provider);
        if (!result?.success) {
          lastError = result?.error || lastError;
          authRequired = authRequired || result?.authRequired === true;
          continue;
        }
        loadedAny = true;
        authRequired = authRequired || result.authRequired === true;
        merged = mergeLoadedSources(
          merged,
          result.sources,
          runtimeTabId,
          get().sourceLinkSettings,
        );
      }
      const publicBrainLoad = await loadFollowedPublicBrainsForStore(
        provider,
        workspaceTabId,
        get().publicBrains,
      );
      if (publicBrainLoad.loaded) {
        persistLoadedPublicBrains(provider, publicBrainLoad.publicBrains);
      }
      if (!loadedAny) {
        if (publicBrainLoad.loaded) {
          const current = get();
          const statusState = providerStatus
            ? providerStatusState(providerStatus, provider)
            : {
              provider,
              authRequired,
              githubConnected: provider === 'local' || current.githubConnected,
              cloudReady: provider === 'local' || provider === 'cloud',
              providerStatusChecked: true,
              githubCheckError: current.githubCheckError,
            };
          set({
            ...statusState,
            authRequired,
            sources: markRuntimeReachability(current.sources),
            publicBrains: publicBrainLoad.publicBrains,
            refreshing: false,
            error: current.sources.length === 0 ? lastError : current.error,
            lastLoadedAt: current.lastLoadedAt,
          });
          return current.sources;
        }
        const error = new Error(lastError) as Error & {
          authRequired?: boolean;
          provider?: OpenBrainProviderMode;
        };
        error.authRequired = authRequired;
        error.provider = provider;
        throw error;
      }
      const statusState = providerStatus
        ? providerStatusState(providerStatus, provider)
        : {
          provider,
          authRequired,
          githubConnected: provider === 'local' || get().githubConnected,
          cloudReady: provider === 'local' || provider === 'cloud',
          providerStatusChecked: true,
          githubCheckError: get().githubCheckError,
        };
      set({
        ...statusState,
        authRequired,
        sources: merged,
        publicBrains: publicBrainLoad.publicBrains,
        refreshing: false,
        error: null,
        lastLoadedAt: Date.now(),
      });
      return merged;
    } catch (err) {
      const error = err instanceof Error ? err.message : 'Failed to list OpenBrain sources.';
      const current = get();
      set({
        provider: (err as { provider?: OpenBrainProviderMode })?.provider || current.provider,
        authRequired: Boolean((err as { authRequired?: unknown })?.authRequired),
        githubConnected: current.githubConnected,
        cloudReady: current.cloudReady,
        providerStatusChecked: current.providerStatusChecked,
        githubCheckError: current.githubCheckError,
        sources: markRuntimeReachability(current.sources),
        publicBrains: current.publicBrains,
        refreshing: false,
        error: current.sources.length === 0 ? error : current.error,
        lastLoadedAt: current.lastLoadedAt,
      });
      return current.sources;
    }
  },

  refresh: async (workspaceTabId?: string) => {
    set({ loading: true, refreshing: false, error: null });
    try {
      const providerStatus = await getOpenBrainProviderStatus().catch(() => null);
      const provider = normalizeProviderMode(providerStatus?.provider, get().provider);
      let merged = markRuntimeReachability(get().sources);
      let authRequired = providerStatus?.authRequired === true;
      let loadedAny = false;
      let lastError = 'Failed to list OpenBrain sources.';
      for (const runtimeTabId of runtimeTabIDsForOpenBrainRefresh(workspaceTabId)) {
        const result = await loadSourcesForRuntime(runtimeTabId, provider);
        if (!result?.success) {
          lastError = result?.error || lastError;
          authRequired = authRequired || result?.authRequired === true;
          continue;
        }
        loadedAny = true;
        authRequired = authRequired || result.authRequired === true;
        merged = mergeLoadedSources(
          merged,
          result.sources,
          runtimeTabId,
          get().sourceLinkSettings,
        );
      }
      const publicBrainLoad = await loadFollowedPublicBrainsForStore(
        provider,
        workspaceTabId,
        get().publicBrains,
      );
      if (publicBrainLoad.loaded) {
        persistLoadedPublicBrains(provider, publicBrainLoad.publicBrains);
      }
      if (!loadedAny) {
        const error = new Error(lastError) as Error & {
          authRequired?: boolean;
          provider?: OpenBrainProviderMode;
        };
        error.authRequired = authRequired;
        error.provider = provider;
        const current = get();
        const statusState = providerStatus
          ? providerStatusState(providerStatus, provider)
          : {
            provider,
            authRequired,
            githubConnected: provider === 'local' || current.githubConnected,
            cloudReady: provider === 'local' || provider === 'cloud',
            providerStatusChecked: true,
            githubCheckError: current.githubCheckError,
          };
        set({
          ...statusState,
          authRequired,
          sources: markRuntimeReachability(current.sources),
          publicBrains: publicBrainLoad.publicBrains,
          loading: false,
          refreshing: false,
          error: lastError,
          lastLoadedAt: current.lastLoadedAt,
        });
        throw error;
      }
      const statusState = providerStatus
        ? providerStatusState(providerStatus, provider)
        : {
          provider,
          authRequired,
          githubConnected: provider === 'local' || get().githubConnected,
          cloudReady: provider === 'local' || provider === 'cloud',
          providerStatusChecked: true,
          githubCheckError: get().githubCheckError,
        };
      set({
        ...statusState,
        authRequired,
        sources: merged,
        publicBrains: publicBrainLoad.publicBrains,
        loading: false,
        refreshing: false,
        error: null,
        lastLoadedAt: Date.now(),
      });
      return merged;
    } catch (err) {
      const error = err instanceof Error ? err.message : 'Failed to list OpenBrain sources.';
      const current = get();
      set({
        provider: (err as { provider?: OpenBrainProviderMode })?.provider || current.provider,
        authRequired: Boolean((err as { authRequired?: unknown })?.authRequired),
        githubConnected: current.githubConnected,
        cloudReady: current.cloudReady,
        providerStatusChecked: current.providerStatusChecked,
        githubCheckError: current.githubCheckError,
        sources: markRuntimeReachability(current.sources),
        publicBrains: current.publicBrains,
        loading: false,
        refreshing: false,
        error,
        lastLoadedAt: current.lastLoadedAt,
      });
      throw new Error(error);
    }
  },

  createOpenBrain: async (input: { name?: string; localPath?: string; remotePath?: string; tabId?: string; remoteHost?: SshHostPayload; source?: LocalOpenBrainWorkspace }) => {
    const localPath = (input.localPath || '').trim();
    const remotePath = (input.remotePath || '').trim();
    const targetPath = remotePath || localPath;
    if (!targetPath) {
      throw new Error(remotePath ? 'Select a remote workspace directory first.' : 'Select a local workspace directory first.');
    }
    const trimmedName = (input.name || targetPath.split(/[\\/]+/).filter(Boolean).pop() || '').trim();
    if (!trimmedName) {
      throw new Error('OpenBrain source name is required.');
    }
    const workspaceTabId = input.tabId;
    const provider = get().provider;
    if (provider === 'local' && remotePath) {
      throw new Error('Local OpenBrain sources can only be created from local folders. Switch OpenBrain to Cloud for remote folders.');
    }
    const request = {
      name: trimmedName,
      localPath: targetPath,
      sourceID: input.source?.sourceID,
      workspaceID: input.source?.workspaceID,
      orgID: input.source?.orgID,
      createRequestID: createOpenBrainRequestID(),
    };
    let result = await createOpenBrainSource(request, workspaceTabId, { provider });
    if (provider !== 'local' && result?.code === 'path_owned_by_other_account') {
      const owner = result.pathOwnerUID ? ` by account ${result.pathOwnerUID}` : '';
      const confirmed = window.confirm(`This folder is already bound${owner}. Move the binding to your current OpenBrain account?`);
      if (confirmed) {
        result = await createOpenBrainSource({
          ...request,
          takeover: true,
        }, workspaceTabId, { provider });
      }
    }
    if (!result?.success || !result.workspace) {
      throw openBrainCreateSourceRuntimeError(result, provider);
    }
    const workspace = normalizeSources([result.workspace], workspaceTabId)[0];
    if (!workspace) {
      throw new Error('Failed to create OpenBrain source.');
    }
    const sources = upsertRuntimeSource(get().sources, workspace);
    const linkedSources = applySourceLinkSettings(sources, get().sourceLinkSettings);
    set({
      sources: linkedSources,
      error: null,
      lastLoadedAt: Date.now(),
    });
    await get().refresh(workspaceTabId).catch(() => {
      set({
        sources: linkedSources,
        loading: false,
        lastLoadedAt: Date.now(),
      });
      return linkedSources;
    });
    return workspace;
  },

  beginPendingOpenBrainSource: ({ name, path, locationKind, rebinding }) => {
    const normalizedPath = normalizePendingOpenBrainPath(path);
    const trimmedName = name.trim() || normalizedPath.split('/').filter(Boolean).pop() || 'OpenBrain source';
    const pendingID = createPendingOpenBrainSourceID();
    set((state) => ({
      pendingSources: [
        ...state.pendingSources.filter((item) => normalizePendingOpenBrainPath(item.path) !== normalizedPath),
        {
          pendingID,
          name: trimmedName,
          path: normalizedPath,
          locationKind,
          rebinding,
          status: 'creating',
        },
      ],
    }));
    return pendingID;
  },

  completePendingOpenBrainSource: (pendingID) => {
    const normalizedID = pendingID.trim();
    if (!normalizedID) {
      return;
    }
    set((state) => ({
      pendingSources: state.pendingSources.filter((item) => item.pendingID !== normalizedID),
    }));
  },

  failPendingOpenBrainSource: (pendingID, error) => {
    const normalizedID = pendingID.trim();
    const message = error.trim() || 'Failed to create OpenBrain source.';
    if (!normalizedID) {
      return;
    }
    set((state) => ({
      pendingSources: state.pendingSources.map((item) => (
        item.pendingID === normalizedID
          ? { ...item, status: 'failed', error: message }
          : item
      )),
    }));
  },

  dismissPendingOpenBrainSource: (pendingID) => {
    const normalizedID = pendingID.trim();
    if (!normalizedID) {
      return;
    }
    set((state) => ({
      pendingSources: state.pendingSources.filter((item) => item.pendingID !== normalizedID),
    }));
  },

  verifyOpenBrain: async (workspace) => {
    const routeTabId = routeTabIDForWorkspace(workspace);
    const result = await verifyOpenBrainSource(workspaceIdentityPayload(workspace), routeTabId);
    if (!result?.success || !result.workspace) {
      const updated = result?.workspace ? normalizeSources([result.workspace], routeTabId)[0] : null;
      if (updated) {
        set({
          sources: applySourceLinkSettings(upsertRuntimeSource(get().sources, updated), get().sourceLinkSettings),
          lastLoadedAt: Date.now(),
        });
      }
      throw openBrainRuntimeError(
        result?.error || 'OpenBrain source needs to be bound on this runtime.',
        result?.code,
        { authRequired: result?.authRequired === true, provider: normalizeProviderMode(result?.provider, get().provider) },
      );
    }
    const verified = normalizeSources([result.workspace], routeTabId)[0];
    if (!verified) {
      throw new Error('Failed to verify OpenBrain source.');
    }
    set({
      sources: applySourceLinkSettings(upsertRuntimeSource(get().sources, verified), get().sourceLinkSettings),
      error: null,
      lastLoadedAt: Date.now(),
    });
    return verified;
  },

  listRecoveryCandidates: async (workspace, paths) => {
    const routeTabId = routeTabIDForWorkspace(workspace);
    const result = await listOpenBrainRecoveryCandidates({
      ...workspaceIdentityPayload(workspace),
      paths,
    }, routeTabId);
    if (!result?.success) {
      throw openBrainRuntimeError(
        result?.error || 'Failed to check OpenBrain source recovery candidates.',
        result?.code,
        { authRequired: result?.authRequired === true, provider: normalizeProviderMode(result?.provider, get().provider) },
      );
    }
    return result.candidates;
  },

  setSourceLinked: (sourceID, linked) => {
    const trimmed = sourceID.trim();
    if (!trimmed) {
      return;
    }
    const sourceLinkSettings = {
      ...get().sourceLinkSettings,
      [trimmed]: linked,
    };
    writeSourceLinkSettings(sourceLinkSettings);
    set({
      sourceLinkSettings,
      sources: applySourceLinkSettings(get().sources, sourceLinkSettings),
    });
  },

  isSourceLinked: (sourceID) => resolveSourceLinked(get().sourceLinkSettings, sourceID.trim()),

  togglePeerLink: (peerID) => {
    set((state) => ({
      peerLinks: {
        ...state.peerLinks,
        [peerID]: !state.peerLinks[peerID],
      },
    }));
  },

  getSourceShare: async (workspace) => {
    if (!canManageOpenBrainSource(workspace)) {
      throw new Error('This source is read-only on this brain.');
    }
    const share = await getOpenBrainSourceShare(workspaceIdentityPayload(workspace), routeTabIDForWorkspace(workspace));
    set({
      sources: updateWorkspacePublicAccess(get().sources, workspace, share?.public?.status === 'active'),
    });
    return share;
  },

  shareSourceWithUser: async (workspace, email) => {
    if (!canManageOpenBrainSource(workspace)) {
      throw new Error('This source is read-only on this brain.');
    }
    await shareOpenBrainSourceWithUser({ ...workspaceIdentityPayload(workspace), email }, routeTabIDForWorkspace(workspace));
  },

  revokeSourceUserShare: async (workspace, uid) => {
    if (!canManageOpenBrainSource(workspace)) {
      throw new Error('This source is read-only on this brain.');
    }
    await revokeOpenBrainSourceUserShare({ ...workspaceIdentityPayload(workspace), uid }, routeTabIDForWorkspace(workspace));
  },

  setSourcePublic: async (workspace) => {
    if (!canManageOpenBrainSource(workspace)) {
      throw new Error('This source is read-only on this brain.');
    }
    await setOpenBrainSourcePublic(workspaceIdentityPayload(workspace), routeTabIDForWorkspace(workspace));
    set({
      sources: updateWorkspacePublicAccess(get().sources, workspace, true),
    });
    await get().getPublicBrainProfile().catch(() => null);
  },

  revokeSourcePublic: async (workspace) => {
    if (!canManageOpenBrainSource(workspace)) {
      throw new Error('This source is read-only on this brain.');
    }
    await revokeOpenBrainSourcePublic(workspaceIdentityPayload(workspace), routeTabIDForWorkspace(workspace));
    set({
      sources: updateWorkspacePublicAccess(get().sources, workspace, false),
    });
    await get().getPublicBrainProfile().catch(() => null);
  },

  getPublicBrainProfile: async () => {
    if (get().provider === 'local') {
      throw new Error('Public brains are available only with OpenBrain Cloud.');
    }
    const profile = await getOpenBrainPublicBrainProfile();
    set({ publicBrainProfile: profile });
    const uid = currentOpenBrainUid();
    if (uid) {
      writeCachedPublicBrainProfile(uid, profile);
    }
    return profile;
  },

  updatePublicBrainProfile: async (description) => {
    if (get().provider === 'local') {
      throw new Error('Public brains are available only with OpenBrain Cloud.');
    }
    const profile = await updateOpenBrainPublicBrainProfile({ description });
    set({ publicBrainProfile: profile });
    const uid = currentOpenBrainUid();
    if (uid) {
      writeCachedPublicBrainProfile(uid, profile);
    }
    return profile;
  },

  followPublicBrain: async (ownerUID) => {
    if (get().provider === 'local') {
      throw new Error('Public brains are available only with OpenBrain Cloud.');
    }
    const trimmed = ownerUID.trim();
    if (!trimmed) {
      throw new Error('Public brain owner is required.');
    }
    const rawEntry = await followOpenBrainPublicBrain(trimmed);
    const entry = normalizePublicBrainEntry({ ...rawEntry, followed: true });
    if (!entry) {
      throw new Error('Failed to add public brain.');
    }
    const nextBrain = followedBrainFromEntry(entry);
    set((state) => ({
      publicBrains: publicBrainHasActiveSources(nextBrain)
        ? [
          ...state.publicBrains.filter((brain) => brain.ownerUID !== trimmed),
          nextBrain,
        ]
        : state.publicBrains.filter((brain) => brain.ownerUID !== trimmed),
      peerLinks: {
        ...state.peerLinks,
        [`public:${trimmed}`]: true,
      },
    }));
  },

  unfollowPublicBrain: async (ownerUID) => {
    if (get().provider === 'local') {
      return;
    }
    const trimmed = ownerUID.trim();
    if (!trimmed) {
      return;
    }
    await unfollowOpenBrainPublicBrain(trimmed);
    set((state) => ({
      publicBrains: state.publicBrains.filter((brain) => brain.ownerUID !== trimmed),
      peerLinks: {
        ...state.peerLinks,
        [`public:${trimmed}`]: false,
      },
    }));
  },

  listPublicBrainDirectory: async (query) => {
    if (get().provider === 'local') {
      set({ publicBrains: [] });
      return [];
    }
    const entries = (await listOpenBrainPublicBrains(query, undefined, { includeSelf: true }))
      .map(normalizePublicBrainEntry)
      .filter((entry): entry is PublicBrainDirectoryEntry => Boolean(entry));
    set((state) => ({
      publicBrains: followedBrainsFromDirectory(entries),
      peerLinks: {
        ...state.peerLinks,
        ...Object.fromEntries(entries.filter((entry) => entry.followed || entry.owned).map((entry) => [`public:${entry.ownerUID}`, true])),
      },
    }));
    return entries;
  },

  removeFromDevice: async (workspace: LocalOpenBrainWorkspace) => {
    const result = await removeOpenBrainSourceFromDevice(workspaceIdentityPayload(workspace), routeTabIDForWorkspace(workspace));
    if (!result?.success) {
      throw new Error(result?.error || 'Failed to remove OpenBrain source from this device.');
    }
    set({
      sources: withoutWorkspace(get().sources, workspace),
      error: null,
      lastLoadedAt: Date.now(),
    });
  },

  archiveSource: async (workspace: LocalOpenBrainWorkspace) => {
    if (!canManageOpenBrainSource(workspace)) {
      throw new Error('This source is read-only on this brain.');
    }
    const result = await archiveOpenBrainSource(workspaceIdentityPayload(workspace), routeTabIDForWorkspace(workspace));
    if (!result?.success) {
      throw new Error(result?.error || 'Failed to stop OpenBrain Cloud queries for this source.');
    }
    set({
      sources: withoutWorkspace(get().sources, workspace),
      error: null,
      lastLoadedAt: Date.now(),
    });
  },

  applySourceAction: async (workspace: LocalOpenBrainWorkspace, action: OpenBrainSourceActionOptions) => {
    if (!canManageOpenBrainSource(workspace)) {
      throw new Error('This source is read-only on this brain.');
    }
    const togglesQueries = action.disableQueries === true || action.enableQueries === true;
    const previousSources = get().sources;
    if (togglesQueries) {
      set({
        sources: updateWorkspaceQueriesDisabled(
          previousSources,
          workspace,
          action.disableQueries === true,
          get().sourceLinkSettings,
        ),
        error: null,
        lastLoadedAt: Date.now(),
      });
    }

    let result: OpenBrainSourceActionResponse;
    try {
      result = await applyOpenBrainSourceAction({
        ...workspaceIdentityPayload(workspace),
        ...action,
      }, routeTabIDForWorkspace(workspace));
    } catch (err) {
      if (togglesQueries) {
        set({ sources: previousSources });
      }
      throw err;
    }
    if (!result?.success) {
      if (togglesQueries) {
        set({ sources: previousSources });
      }
      throw new Error(result?.error || 'Failed to update OpenBrain Cloud source.');
    }
    const removesFromDevice = action.hardDelete || result.hardDeleted;
    if (removesFromDevice) {
      set({
        sources: withoutWorkspace(get().sources, workspace),
        error: null,
        lastLoadedAt: Date.now(),
      });
      return result;
    }
    if (action.disableQueries || action.enableQueries || result.disabledQueries || result.enabledQueries) {
      const disabledQueries = action.disableQueries || result.disabledQueries === true;
      set({
        sources: updateWorkspaceQueriesDisabled(get().sources, workspace, disabledQueries, get().sourceLinkSettings),
        error: null,
        lastLoadedAt: Date.now(),
      });
      return result;
    }
    set({
      error: null,
      lastLoadedAt: Date.now(),
    });
    return result;
  },
}));
