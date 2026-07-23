import { randomUUID } from 'crypto';
import { spawn } from 'child_process';
import * as os from 'os';
import * as fs from 'fs/promises';
import * as path from 'path';
import type { AuthConfig } from '../auth/authStore';
import { resolveOpenBrainAPIBase } from '../auth/openbrainApiBase';
import { writeJsonFileAtomic } from '../shared/jsonFile';
import { buildPowerShellCommand } from '../remote/remoteRuntimeScripts';
import { runSsh } from '../remote/ssh2Transport';
import type { LocalGBrainSettings } from '../settings/settingsStore';
import type { SshHost, SshHostWithSecrets } from '../ssh/sshTypes';

type WorkspaceTemplateFile = {
  path: string;
  content: string;
};

export type WorkspaceTemplateManifest = {
  directories?: string[];
  files?: WorkspaceTemplateFile[];
};

export type GitHubRepositoryRef = {
  owner: string;
  name: string;
  remoteURL: string;
  externalID?: string;
};

export type WorkspaceRepositoryProviderOption = {
  provider: string;
  name: string;
  enabled: boolean;
  canCreateRepository?: boolean;
  canSyncRepository?: boolean;
  permissionState?: string;
  permissionMessage?: string;
  accounts?: WorkspaceGitHubAccount[];
};

export type WorkspaceGitHubAccount = {
  owner: string;
  accountType?: string;
  installationID?: string;
  connectedAt?: string;
  canCreateRepository?: boolean;
  canSyncRepository?: boolean;
  permissionState?: string;
  permissionMessage?: string;
};

export type WorkspaceRepositoryPolicy = {
  enabled: boolean;
  provider?: string;
  providers?: WorkspaceRepositoryProviderOption[];
  remoteURL?: string;
  webURL?: string;
  owner?: string;
  name?: string;
  defaultBranch?: string;
  externalID?: string;
};

export type WorkspaceSyncPolicy = {
  autoSync?: boolean;
  onOpen?: boolean;
  onLocalChange?: boolean;
  intervalSec?: number;
  conflict?: string;
  deleteMode?: string;
};

export type WorkspaceStorageProviderOption = {
  provider: string;
  backend: string;
  name: string;
  enabled: boolean;
  configured?: boolean;
  connectable?: boolean;
  connected?: boolean;
  connectedAs?: string;
  connectedAt?: string;
  region?: string;
  canCreateRepository?: boolean;
  canSyncRepository?: boolean;
  permissionState?: string;
  permissionMessage?: string;
  accounts?: WorkspaceGitHubAccount[];
};

export type WorkspaceStorageBinding = {
  enabled: boolean;
  backend?: string;
  provider?: string;
  region?: string;
  remoteID?: string;
  remoteName?: string;
  remoteURL?: string;
  connectedAs?: string;
  providers?: WorkspaceStorageProviderOption[];
  syncPolicy?: WorkspaceSyncPolicy;
};

export type WorkspaceTemplateView = {
  templateID: string;
  orgID: string;
  name: string;
  description?: string;
  version: number;
  defaultLocalName: string;
  backupEnabled: boolean;
  repository?: WorkspaceRepositoryPolicy;
  storage?: WorkspaceStorageBinding;
  manifest: WorkspaceTemplateManifest;
};

export type WorkspaceTemplateListResult = {
  templates: WorkspaceTemplateView[];
  defaultID: string;
};

type WorkspaceStorageConnectionsResult = {
  providers?: WorkspaceStorageProviderOption[];
};

export type WorkspaceCreateResult = {
  workspaceID: string;
  orgID: string;
  templateID: string;
  templateVersion: number;
  backupEnabled: boolean;
  defaultLocalName: string;
  repository?: WorkspaceRepositoryPolicy;
  storage?: WorkspaceStorageBinding;
  manifest: WorkspaceTemplateManifest;
};

export type WorkspaceBrainSourceActionInput = {
  orgID?: string;
  workspaceID?: string;
  disableQueries?: boolean;
  enableQueries?: boolean;
  disableSync?: boolean;
  hardDelete?: boolean;
  confirmWorkspaceID?: string;
  confirmName?: string;
};

export type WorkspaceBrainSourceActionResult = {
  ok?: boolean;
  orgID?: string;
  workspaceID?: string;
  sourceID?: string;
  disabledQueries?: boolean;
  enabledQueries?: boolean;
  disabledSync?: boolean;
  hardDeleted?: boolean;
  syncJobsRemoved?: number;
  status?: string;
};

type WorkspaceGitAccessToken = {
  provider?: string;
  username?: string;
  accessToken?: string;
  accessTokenExpiresAt?: string;
  remoteURL?: string;
};

type WorkspaceIndexEntry = {
  workspaceID: string;
  orgID?: string;
  localName: string;
  path: string;
  locationKind?: 'local' | 'remote';
  remoteHost?: SshHost;
  templateID?: string;
  templateVersion?: number;
  backupEnabled: boolean;
  repository?: WorkspaceRepositoryPolicy;
  storage?: WorkspaceStorageBinding;
  syncPolicy?: WorkspaceSyncPolicy;
  createdAt: string;
  updatedAt: string;
};

type HiddenWorkspaceIndexEntry = {
  workspaceID: string;
  orgID?: string;
  hiddenAt: string;
};

type WorkspaceIndexFile = {
  version: 3;
  deployments: Record<string, WorkspaceIndexDeployment>;
  workspaces: WorkspaceIndexEntry[];
  hiddenWorkspaces?: HiddenWorkspaceIndexEntry[];
  activeDeploymentID?: string;
  activeOrgID?: string;
  activeUID?: string;
};

type WorkspaceIndexDeployment = {
  organizations: Record<string, WorkspaceIndexOrganization>;
};

type WorkspaceIndexOrganization = {
  accounts: Record<string, WorkspaceIndexAccount>;
};

type WorkspaceIndexAccount = {
  workspaces: WorkspaceIndexEntry[];
  hiddenWorkspaces?: HiddenWorkspaceIndexEntry[];
};

type MaterializeWorkspaceOptions = {
  localPath: string;
  writeManifest?: boolean;
  existingGitHubRepository?: GitHubRepositoryRef | null;
  pushGit?: boolean;
};

type MaterializeRemoteWorkspaceOptions = {
  remotePath: string;
  remoteHost: SshHostWithSecrets;
  remoteHostView: SshHost;
  writeManifest?: boolean;
  existingGitHubRepository?: GitHubRepositoryRef | null;
  pushGit?: boolean;
};

export type MaterializedWorkspace = WorkspaceCreateResult & {
  localName: string;
  path: string;
  locationKind?: 'local' | 'remote';
  remoteHost?: SshHost;
};

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
  disabledQueries?: boolean;
  publicAccess?: boolean;
  effectivePermission?: 'read' | 'write' | 'admin';
  canMutateSource?: boolean;
  publicOwnerUID?: string;
  bindingMode?: 'own' | 'granted';
  locationKind?: 'local' | 'remote';
  remoteHost?: SshHost;
};

export type WorkspaceIndexView = WorkspaceIndexEntry;

type GBrainSourceEntry = {
  id?: string;
  name?: string;
  local_path?: string | null;
  remote_url?: string | null;
  federated?: boolean;
  page_count?: number;
  last_sync_at?: string | null;
};

type GBrainSourcesPayload = {
  sources?: GBrainSourceEntry[];
};

function openbrainBaseURL(auth: AuthConfig): string {
  return resolveOpenBrainAPIBase(auth);
}

function authHeaders(auth: AuthConfig): Record<string, string> {
  return {
    'Content-Type': 'application/json',
    'X-UID': auth.uid,
    Authorization: `Bearer ${auth.token}`,
  };
}

async function parseJSONResponse<T>(res: Response, requestURL?: string): Promise<T> {
  const text = await res.text();
  let body = {} as T & { error?: string };
  if (text) {
    try {
      body = JSON.parse(text) as T & { error?: string };
    } catch {
      const snippet = text.trim().slice(0, 240);
      if (!res.ok) {
        throw new Error(`OpenBrain request failed: ${res.status}${requestURL ? ` ${requestURL}` : ''}${snippet ? ` - ${snippet}` : ''}`);
      }
      throw new Error(`OpenBrain request returned non-JSON response${requestURL ? ` from ${requestURL}` : ''}${snippet ? `: ${snippet}` : ''}`);
    }
  }
  if (!res.ok) {
    const status = `OpenBrain request failed: ${res.status}${requestURL ? ` ${requestURL}` : ''}`;
    throw new Error(body.error ? `${status} - ${body.error}` : status);
  }
  return body as T;
}

export async function listWorkspaceTemplates(auth: AuthConfig, orgID?: string): Promise<WorkspaceTemplateListResult> {
  const baseURL = openbrainBaseURL(auth);
  if (!baseURL) {
    throw new Error('OpenBrain API URL is not configured.');
  }
  const normalizedOrgID = requireBoundOrgID(auth, orgID);
  const path = `/v1/orgs/${encodeURIComponent(normalizedOrgID)}/workspace-templates`;
  const requestURL = `${baseURL}${path}`;
  const res = await fetch(requestURL, {
    method: 'GET',
    headers: authHeaders(auth),
  });
  const result = await parseJSONResponse<WorkspaceTemplateListResult>(res, requestURL);
  try {
    const connections = await listStorageConnections(auth);
    return mergeStorageConnectionsIntoTemplates(result, connections, { includeGitHub: false });
  } catch {
    return result;
  }
}

async function listStorageConnections(auth: AuthConfig): Promise<WorkspaceStorageConnectionsResult> {
  const baseURL = openbrainBaseURL(auth);
  if (!baseURL) {
    return {};
  }
  const requestURL = `${baseURL}/v1/storage-connections`;
  const res = await fetch(requestURL, {
    method: 'GET',
    headers: authHeaders(auth),
  });
  return parseJSONResponse<WorkspaceStorageConnectionsResult>(res, requestURL);
}

function mergeStorageConnectionsIntoTemplates(
  result: WorkspaceTemplateListResult,
  connections: WorkspaceStorageConnectionsResult,
  options: { includeGitHub: boolean },
): WorkspaceTemplateListResult {
  const providers = new Map<string, WorkspaceStorageProviderOption>();
  for (const provider of connections.providers || []) {
    const key = normalizeStorageProviderKey(provider.provider);
    if (!key || (key === 'github' && !options.includeGitHub)) {
      continue;
    }
    providers.set(key, provider);
  }
  if (providers.size === 0) {
    return result;
  }
  return {
    ...result,
    templates: (result.templates || []).map((template) => ({
      ...template,
      storage: template.storage
        ? {
            ...template.storage,
            providers: mergeTemplateStorageProviders(template.storage.providers || [], providers),
          }
        : template.storage,
    })),
  };
}

function mergeTemplateStorageProviders(
  current: WorkspaceStorageProviderOption[],
  connections: Map<string, WorkspaceStorageProviderOption>,
): WorkspaceStorageProviderOption[] {
  return current.map((provider) => {
    const key = normalizeStorageProviderKey(provider.provider);
    const connection = key ? connections.get(key) : undefined;
    if (!connection) {
      return provider;
    }
    return {
      ...provider,
      enabled: provider.enabled || connection.enabled,
      configured: provider.configured || connection.configured,
      connected: provider.connected || connection.connected,
      connectedAs: provider.connectedAs || connection.connectedAs,
      connectedAt: provider.connectedAt || connection.connectedAt,
      canCreateRepository: provider.canCreateRepository ?? connection.canCreateRepository,
      canSyncRepository: provider.canSyncRepository ?? connection.canSyncRepository,
      permissionState: provider.permissionState || connection.permissionState,
      permissionMessage: provider.permissionMessage || connection.permissionMessage,
      accounts: provider.accounts && provider.accounts.length > 0
        ? provider.accounts
        : connection.accounts && connection.accounts.length > 0
          ? connection.accounts
          : accountsFromConnectedAs(connection.connectedAs),
    };
  });
}

function normalizeStorageProviderKey(provider?: string): string {
  const value = (provider || '').trim().toLowerCase();
  if (value === 'feishu') {
    return 'lark-drive';
  }
  return value;
}

function accountsFromConnectedAs(value?: string): WorkspaceGitHubAccount[] | undefined {
  const accounts = (value || '')
    .split(',')
    .map((owner) => owner.trim())
    .filter(Boolean)
    .map((owner) => ({ owner }));
  return accounts.length > 0 ? accounts : undefined;
}

export async function createOpenbrainWorkspace(
  auth: AuthConfig,
  input: { templateID?: string; provider?: string; storageProvider?: string; repositoryOwner?: string; repositoryName?: string; name?: string; orgID?: string },
): Promise<WorkspaceCreateResult> {
  const baseURL = openbrainBaseURL(auth);
  if (!baseURL) {
    throw new Error('OpenBrain API URL is not configured.');
  }
  const orgID = requireBoundOrgID(auth, input.orgID);
  const path = `/v1/orgs/${encodeURIComponent(orgID)}/workspaces`;
  const requestURL = `${baseURL}${path}`;
  const res = await fetch(requestURL, {
    method: 'POST',
    headers: authHeaders(auth),
    body: JSON.stringify({
      templateID: input.templateID,
      provider: input.provider,
      storageProvider: input.storageProvider,
      repositoryOwner: input.repositoryOwner,
      repositoryName: input.repositoryName,
      name: input.name,
    }),
  });
  const result = await parseJSONResponse<WorkspaceCreateResult>(res, requestURL);
  requireBoundOrgID(auth, result.orgID, true);
  return result;
}

function normalizeOrgID(raw?: string | null): string | undefined {
  const value = (raw || '').trim().toLowerCase();
  if (!value) {
    return undefined;
  }
  return /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(value) ? value : undefined;
}

function requireBoundOrgID(
  auth: AuthConfig,
  requestedOrgID?: string | null,
  requireRequested = false,
): string {
  const boundOrgID = normalizeOrgID(auth.orgID);
  if (!boundOrgID) {
    throw new Error('tenant_context_required: authenticated organization is missing or invalid');
  }
  const requestedRaw = (requestedOrgID || '').trim();
  if (!requestedRaw) {
    if (requireRequested) {
      throw new Error('tenant_context_required: workspace organization is missing');
    }
    return boundOrgID;
  }
  const requested = normalizeOrgID(requestedRaw);
  if (!requested) {
    throw new Error('tenant_context_invalid: workspace organization is invalid');
  }
  if (requested !== boundOrgID) {
    throw new Error('tenant_context_mismatch: workspace organization does not match the authenticated organization');
  }
  return boundOrgID;
}

export async function createLocalIndexWorkspace(
  homeDir: string,
  input: { name?: string; localPath: string },
  auth?: AuthConfig | null,
): Promise<MaterializedWorkspace> {
  return materializeWorkspace(homeDir, {
    workspaceID: `local-${randomUUID()}`,
    orgID: 'local',
    templateID: 'local-index-workspace',
    templateVersion: 1,
    backupEnabled: false,
    defaultLocalName: safeLocalName(input.name || 'workspace'),
    storage: {
      enabled: false,
      backend: 'local',
      syncPolicy: defaultWorkspaceSyncPolicy(false),
    },
    manifest: {
      directories: ['projects', 'concepts', 'decisions', 'systems', 'raw', 'inbox'],
      files: [
        {
          path: 'AGENTS.md',
          content: '# Workspace Instructions\n\nThis is an OpenBrain LLM Wiki workspace. Keep durable work in visible markdown files.\n',
        },
        {
          path: 'index.md',
          content: '# Workspace\n\n- Projects: [projects/](projects/)\n- Concepts: [concepts/](concepts/)\n- Decisions: [decisions/](decisions/)\n- Raw captures: [raw/](raw/)\n',
        },
      ],
    },
  }, auth, { localPath: input.localPath || '', writeManifest: true });
}

export async function createLocalEmptyWorkspace(
  homeDir: string,
  input: { name?: string; localPath: string },
  auth?: AuthConfig | null,
): Promise<MaterializedWorkspace> {
  return materializeWorkspace(homeDir, {
    workspaceID: `local-${randomUUID()}`,
    orgID: 'local',
    templateID: 'empty-workspace',
    templateVersion: 1,
    backupEnabled: false,
    defaultLocalName: safeLocalName(input.name || 'workspace'),
    storage: {
      enabled: false,
      backend: 'local',
      syncPolicy: defaultWorkspaceSyncPolicy(false),
    },
    manifest: {},
  }, auth, { localPath: input.localPath || '', writeManifest: true });
}

export async function materializeWorkspace(
  homeDir: string,
  result: WorkspaceCreateResult,
  auth: AuthConfig | null | undefined,
  options: MaterializeWorkspaceOptions,
): Promise<MaterializedWorkspace> {
  const workspacePath = await normalizeExistingWorkspacePath(options.localPath);
  const localName = localNameFromWorkspacePath(workspacePath, result.defaultLocalName || 'workspace');
  const writeManifest = options?.writeManifest !== false;

  if (writeManifest) {
    for (const dir of result.manifest.directories || []) {
      const targetDir = safeWorkspacePath(workspacePath, dir);
      await fs.mkdir(targetDir, { recursive: true });
    }
    for (const file of result.manifest.files || []) {
      const targetFile = safeWorkspacePath(workspacePath, file.path);
      await fs.mkdir(path.dirname(targetFile), { recursive: true });
      await writeFileIfMissing(targetFile, file.content || '');
    }
  }

  if (result.storage?.backend === 'git' && result.repository?.remoteURL) {
    const gitToken = auth ? await fetchWorkspaceGitToken(auth, result) : null;
    await importGitWorkspace(result.repository.remoteURL, workspacePath, result.repository.defaultBranch || 'main', gitToken, {
      existingGitHubRepository: options.existingGitHubRepository || null,
      push: options.pushGit !== false,
    });
  }

  await upsertWorkspaceIndex(homeDir, {
    workspaceID: result.workspaceID,
    orgID: result.orgID,
    localName,
    path: workspacePath,
    locationKind: 'local',
    templateID: result.templateID,
    templateVersion: result.templateVersion,
    backupEnabled: result.backupEnabled,
    repository: result.repository,
    storage: result.storage,
    syncPolicy: result.storage?.syncPolicy || defaultWorkspaceSyncPolicy(result.backupEnabled),
  }, auth);

  return {
    ...result,
    localName,
    path: workspacePath,
    locationKind: 'local',
  };
}

export async function bindLocalOpenBrainWorkspace(
  homeDir: string,
  result: WorkspaceCreateResult,
  localPath: string,
  auth?: AuthConfig | null,
): Promise<MaterializedWorkspace & { indexEntry: WorkspaceIndexView }> {
  const workspacePath = await normalizeExistingWorkspacePath(localPath);
  const localName = localNameFromWorkspacePath(workspacePath, result.defaultLocalName || 'workspace');

  const indexEntry = await upsertWorkspaceIndex(homeDir, {
    workspaceID: result.workspaceID,
    orgID: result.orgID,
    localName,
    path: workspacePath,
    locationKind: 'local',
    templateID: result.templateID,
    templateVersion: result.templateVersion,
    backupEnabled: result.backupEnabled,
    repository: result.repository,
    storage: result.storage,
    syncPolicy: result.storage?.syncPolicy || defaultWorkspaceSyncPolicy(result.backupEnabled),
  }, auth);

  return {
    ...result,
    localName,
    path: workspacePath,
    locationKind: 'local',
    indexEntry,
  };
}

export async function materializeRemoteWorkspace(
  homeDir: string,
  result: WorkspaceCreateResult,
  auth: AuthConfig | null | undefined,
  options: MaterializeRemoteWorkspaceOptions,
): Promise<MaterializedWorkspace> {
  const remotePath = normalizeRemoteWorkspacePath(options.remotePath);
  const localName = localNameFromRemoteWorkspacePath(remotePath, result.defaultLocalName || 'workspace');
  if (options.writeManifest === true) {
    throw new Error('Remote workspace template file creation is not supported for this flow.');
  }
  if (result.storage?.backend === 'git' && result.repository?.remoteURL) {
    const gitToken = auth ? await fetchWorkspaceGitToken(auth, result) : null;
    await importRemoteGitWorkspace(result.repository.remoteURL, remotePath, result.repository.defaultBranch || 'main', options.remoteHost, gitToken, {
      existingGitHubRepository: options.existingGitHubRepository || null,
      push: options.pushGit !== false,
    });
  }
  await upsertWorkspaceIndex(homeDir, {
    workspaceID: result.workspaceID,
    orgID: result.orgID,
    localName,
    path: remotePath,
    locationKind: 'remote',
    remoteHost: sanitizeWorkspaceRemoteHost(options.remoteHostView),
    templateID: result.templateID,
    templateVersion: result.templateVersion,
    backupEnabled: result.backupEnabled,
    repository: result.repository,
    storage: result.storage,
    syncPolicy: result.storage?.syncPolicy || defaultWorkspaceSyncPolicy(result.backupEnabled),
  }, auth);
  return {
    ...result,
    localName,
    path: remotePath,
    locationKind: 'remote',
    remoteHost: sanitizeWorkspaceRemoteHost(options.remoteHostView),
  };
}

export async function listGBrainSourceWorkspaces(homeDir: string): Promise<LocalOpenBrainWorkspace[]> {
  const sources = await listGBrainSources(homeDir);
  return sources
    .map((source) => {
      const sourceID = (source.id || '').trim();
      const sourcePath = typeof source.local_path === 'string' && source.local_path.trim()
        ? path.resolve(source.local_path.trim())
        : undefined;
      const name = (source.name || '').trim() || sourceID || sourcePath || 'source';
      return {
        sourceID,
        workspaceID: sourceID || undefined,
        orgID: 'local',
        brainID: 'host',
        name,
        path: sourcePath,
        updatedAt: source.last_sync_at || undefined,
        pageCount: typeof source.page_count === 'number' && Number.isFinite(source.page_count) ? source.page_count : undefined,
        federated: source.federated === true,
        remoteURL: source.remote_url || null,
        openable: Boolean(sourcePath),
        locationKind: 'local' as const,
      };
    })
    .filter((workspace) => workspace.sourceID)
    .sort((a, b) => a.name.localeCompare(b.name));
}

export async function listConfiguredGBrainSourceWorkspaces(
  homeDir: string,
  local?: LocalGBrainSettings,
): Promise<LocalOpenBrainWorkspace[]> {
  const remoteMcpUrl = (local?.remoteMcpUrl || '').trim();
  if (remoteMcpUrl) {
    const remote = normalizeRemoteMcpURL(remoteMcpUrl);
    return [{
      sourceID: 'remote-mcp',
      workspaceID: 'remote-mcp',
      orgID: 'local',
      brainID: 'remote',
      name: 'GBrain Remote MCP',
      remoteURL: remote.mcpURL || remoteMcpUrl,
      federated: true,
      openable: false,
    }];
  }
  const sources = await listGBrainSources(homeDir, local);
  return sources
    .map((source) => {
      const sourceID = (source.id || '').trim();
      const sourcePath = typeof source.local_path === 'string' && source.local_path.trim()
        ? path.resolve(source.local_path.trim())
        : undefined;
      const name = (source.name || '').trim() || sourceID || sourcePath || 'source';
      return {
        sourceID,
        workspaceID: sourceID || undefined,
        orgID: 'local',
        brainID: 'personal',
        name,
        path: sourcePath,
        updatedAt: source.last_sync_at || undefined,
        pageCount: typeof source.page_count === 'number' && Number.isFinite(source.page_count) ? source.page_count : undefined,
        federated: source.federated === true,
        remoteURL: source.remote_url || null,
        openable: Boolean(sourcePath),
        locationKind: 'local' as const,
      };
    })
    .filter((workspace) => workspace.sourceID)
    .sort((a, b) => a.name.localeCompare(b.name));
}

export async function registerGBrainSourceForWorkspace(
  homeDir: string,
  workspace: MaterializedWorkspace,
  local?: LocalGBrainSettings,
): Promise<MaterializedWorkspace & { sourceID: string }> {
  const sourceID = await nextAvailableGBrainSourceID(homeDir, workspace.localName || workspace.defaultLocalName, local);
  await runGBrainCommand(homeDir, [
    'sources',
    'add',
    sourceID,
    '--path',
    workspace.path,
    '--name',
    workspace.localName || workspace.defaultLocalName || sourceID,
    '--federated',
  ], local);
  return {
    ...workspace,
    workspaceID: sourceID,
    sourceID,
  };
}

export async function runConfiguredGBrainCommand(
  homeDir: string,
  args: string[],
  local?: LocalGBrainSettings,
): Promise<{ stdout: string; stderr: string }> {
  return runGBrainCommand(homeDir, args, local);
}

export async function listIndexedOpenBrainWorkspaces(homeDir: string, auth?: AuthConfig | null): Promise<WorkspaceIndexView[]> {
  const index = await loadWorkspaceIndex(homeDir, auth);
  return index.workspaces;
}

export async function isOpenBrainWorkspaceHidden(homeDir: string, workspaceID: string, orgID?: string, auth?: AuthConfig | null): Promise<boolean> {
  const key = normalizeWorkspaceIdentity(workspaceID, orgID);
  if (!key.workspaceID) {
    return false;
  }
  const index = await loadWorkspaceIndex(homeDir, auth);
  return (index.hiddenWorkspaces || []).some((entry) => workspaceIdentityEqual(entry, key));
}

export async function removeOpenBrainWorkspaceFromDevice(
  homeDir: string,
  input: { workspaceID?: string; orgID?: string; path?: string },
  auth?: AuthConfig | null,
): Promise<void> {
  const index = await loadWorkspaceIndex(homeDir, auth);
  const key = normalizeWorkspaceIdentity(input.workspaceID || '', input.orgID);
  const normalizedPath = normalizeWorkspacePathForIndex(input.path || '');
  const removed = index.workspaces.filter((entry) =>
    (key.workspaceID && workspaceIdentityEqual(entry, key)) ||
    (normalizedPath && normalizeWorkspacePathForIndex(entry.path) === normalizedPath)
  );
  index.workspaces = index.workspaces.filter((entry) => !removed.includes(entry));
  const hiddenKeys = removed
    .map((entry) => normalizeWorkspaceIdentity(entry.workspaceID, entry.orgID))
    .filter((entry) => entry.workspaceID);
  if (key.workspaceID) {
    hiddenKeys.push(key);
  }
  const uniqueHiddenKeys = hiddenKeys.filter((hidden, index) =>
    hiddenKeys.findIndex((candidate) => workspaceIdentityEqual(candidate, hidden)) === index
  );
  const now = new Date().toISOString();
  const existing = index.hiddenWorkspaces || [];
  const nextHidden = existing.filter((entry) =>
    !uniqueHiddenKeys.some((hidden) => workspaceIdentityEqual(entry, hidden))
  );
  for (const hidden of uniqueHiddenKeys) {
    nextHidden.push({ ...hidden, hiddenAt: now });
  }
  index.hiddenWorkspaces = nextHidden;
  await saveWorkspaceIndex(homeDir, index);
}

export async function findIndexedOpenBrainWorkspaceByPath(
  homeDir: string,
  workspacePath: string,
  auth?: AuthConfig | null,
): Promise<WorkspaceIndexView | null> {
  const normalized = normalizeWorkspacePathForIndex(workspacePath);
  if (!normalized) {
    return null;
  }
  const index = await loadWorkspaceIndex(homeDir, auth);
  return index.workspaces.find((entry) =>
    normalizeWorkspacePathForIndex(entry.path) === normalized
  ) || null;
}

async function listGBrainSources(homeDir: string, local?: LocalGBrainSettings): Promise<GBrainSourceEntry[]> {
  const result = await runGBrainCommand(homeDir, ['sources', 'list', '--json'], local);
  let parsed: GBrainSourcesPayload;
  try {
    parsed = JSON.parse(result.stdout) as GBrainSourcesPayload;
  } catch {
    const snippet = result.stdout.trim().slice(0, 240);
    throw new Error(`GBrain sources list returned non-JSON output${snippet ? `: ${snippet}` : '.'}`);
  }
  return Array.isArray(parsed.sources) ? parsed.sources : [];
}

async function nextAvailableGBrainSourceID(homeDir: string, name: string, local?: LocalGBrainSettings): Promise<string> {
  const sources = await listGBrainSources(homeDir, local);
  const existing = new Set(sources.map((source) => (source.id || '').trim()).filter(Boolean));
  const base = gbrainSourceIDBase(name);
  for (let i = 0; i < 100; i += 1) {
    const suffix = i === 0 ? '' : `-${i + 1}`;
    const candidate = gbrainSourceIDWithSuffix(base, suffix);
    if (!existing.has(candidate)) {
      return candidate;
    }
  }
  throw new Error('Could not allocate a unique GBrain source id.');
}

function gbrainSourceIDBase(value: string): string {
  const base = safeLocalName(value || 'workspace')
    .replace(/^[^a-z0-9]+|[^a-z0-9]+$/g, '')
    .slice(0, 32)
    .replace(/-+$/g, '');
  return base || 'workspace';
}

function gbrainSourceIDWithSuffix(base: string, suffix: string): string {
  const maxBaseLength = Math.max(1, 32 - suffix.length);
  const trimmedBase = base.slice(0, maxBaseLength).replace(/-+$/g, '') || 'workspace';
  return `${trimmedBase}${suffix}`;
}

function gbrainExecutableName(): string {
  return process.platform === 'win32' ? 'gbrain.exe' : 'gbrain';
}

function bundledGBrainBinDir(homeDir: string): string {
  return path.join(homeDir, '.openbrain', 'bin');
}

async function resolveGBrainBinary(homeDir: string, local?: LocalGBrainSettings): Promise<string> {
  const explicit = (local?.cliPath || process.env.GBRAIN_CLI_PATH || '').trim();
  if (explicit) {
    return explicit;
  }
  const bundled = path.join(bundledGBrainBinDir(homeDir), gbrainExecutableName());
  if (await pathExists(bundled)) {
    return bundled;
  }
  return gbrainExecutableName();
}

function gbrainConfigHome(homeDir: string): string {
  return path.join(homeDir, '.openbrain', 'configs', 'gbrain');
}

function defaultGBrainDatabasePath(homeDir: string): string {
  return path.join(gbrainConfigHome(homeDir), '.gbrain', 'brain.pglite');
}

function hasLocalGBrainFileConfig(local?: LocalGBrainSettings): boolean {
  return Boolean(
    (local?.engine && (local.engine === 'pglite' || local.engine === 'postgres')) ||
    (local?.databaseUrl || '').trim() ||
    (local?.databasePath || '').trim() ||
    (local?.remoteMcpUrl || '').trim()
  );
}

function normalizeRemoteMcpURL(rawURL: string): { issuerURL: string; mcpURL: string } {
  const value = rawURL.trim().replace(/\/+$/, '');
  if (!value) {
    return { issuerURL: '', mcpURL: '' };
  }
  const mcpURL = value.endsWith('/mcp') ? value : `${value}/mcp`;
  const issuerURL = value.replace(/\/mcp$/, '');
  return { issuerURL, mcpURL };
}

function normalizeRemoteSecretEnvVar(rawName?: string): string {
  const name = (rawName || 'GBRAIN_REMOTE_CLIENT_SECRET').trim();
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(name) ? name : 'GBRAIN_REMOTE_CLIENT_SECRET';
}

async function ensureLocalGBrainConfig(homeDir: string, local?: LocalGBrainSettings): Promise<string | null> {
  if (!hasLocalGBrainFileConfig(local)) {
    return null;
  }
  const configHome = gbrainConfigHome(homeDir);
  const configDir = path.join(configHome, '.gbrain');
  await fs.mkdir(configDir, { recursive: true });
  const remoteMcpUrl = (local?.remoteMcpUrl || '').trim();
  const databaseUrl = (local?.databaseUrl || '').trim();
  const config: Record<string, unknown> = {
    engine: remoteMcpUrl || databaseUrl || local?.engine === 'postgres' ? 'postgres' : 'pglite',
  };
  const databasePath = (local?.databasePath || '').trim();
  if (databasePath) {
    config.database_path = databasePath;
  } else if (config.engine === 'pglite' && !remoteMcpUrl) {
    config.database_path = defaultGBrainDatabasePath(homeDir);
  }
  if (remoteMcpUrl) {
    const remote = normalizeRemoteMcpURL(remoteMcpUrl);
    const clientID = (local?.remoteMcpClientID || process.env.GBRAIN_REMOTE_CLIENT_ID || '').trim() || 'openbrain-desktop';
    config.remote_mcp = {
      issuer_url: remote.issuerURL,
      mcp_url: remote.mcpURL,
      oauth_client_id: clientID,
    };
  }
  await fs.writeFile(path.join(configDir, 'config.json'), `${JSON.stringify(config, null, 2)}\n`, 'utf8');
  return configHome;
}

async function gbrainCommandEnv(homeDir: string, local?: LocalGBrainSettings): Promise<NodeJS.ProcessEnv> {
  const pathParts = [
    bundledGBrainBinDir(homeDir),
    process.env.PATH || '',
    path.join(homeDir, '.bun', 'bin'),
    '/opt/homebrew/bin',
    '/usr/local/bin',
  ].filter(Boolean);
  const env: NodeJS.ProcessEnv = {
    ...process.env,
    HOME: homeDir,
    PATH: Array.from(new Set(pathParts.flatMap((part) => part.split(path.delimiter)).filter(Boolean))).join(path.delimiter),
  };
  const databaseUrl = (local?.databaseUrl || '').trim();
  if (databaseUrl) {
    env.GBRAIN_DATABASE_URL = databaseUrl;
    delete env.DATABASE_URL;
  }
  const remoteClientID = (local?.remoteMcpClientID || '').trim();
  if (remoteClientID) {
    env.GBRAIN_REMOTE_CLIENT_ID = remoteClientID;
  }
  const remoteSecret = (local?.remoteMcpClientSecret || '').trim();
  const remoteSecretEnvVar = normalizeRemoteSecretEnvVar(local?.remoteMcpClientSecretEnvVar);
  if (remoteSecret) {
    env.GBRAIN_REMOTE_CLIENT_SECRET = remoteSecret;
    if (remoteSecretEnvVar && remoteSecretEnvVar !== 'GBRAIN_REMOTE_CLIENT_SECRET') {
      env[remoteSecretEnvVar] = remoteSecret;
    }
  } else if (remoteSecretEnvVar && remoteSecretEnvVar !== 'GBRAIN_REMOTE_CLIENT_SECRET' && process.env[remoteSecretEnvVar]) {
    env.GBRAIN_REMOTE_CLIENT_SECRET = process.env[remoteSecretEnvVar];
  }
  const configHome = await ensureLocalGBrainConfig(homeDir, local);
  if (configHome) {
    env.GBRAIN_HOME = configHome;
  }
  return env;
}

async function runGBrainCommand(homeDir: string, args: string[], local?: LocalGBrainSettings): Promise<{ stdout: string; stderr: string }> {
  const binary = await resolveGBrainBinary(homeDir, local);
  const env = await gbrainCommandEnv(homeDir, local);
  return new Promise((resolve, reject) => {
    const child = spawn(binary, args, {
      cwd: homeDir,
      stdio: ['ignore', 'pipe', 'pipe'],
      env,
    });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', (chunk) => {
      stdout += String(chunk);
    });
    child.stderr.on('data', (chunk) => {
      stderr += String(chunk);
    });
    child.on('error', (err: NodeJS.ErrnoException) => {
      if (err.code === 'ENOENT') {
        reject(new Error('GBrain CLI is not available. Install gbrain or set GBRAIN_CLI_PATH.'));
        return;
      }
      reject(err);
    });
    child.on('close', (code) => {
      if (code === 0) {
        resolve({ stdout, stderr });
        return;
      }
      const detail = stderr.trim() || stdout.trim();
      reject(new Error(detail || `gbrain ${args.join(' ')} failed with exit code ${code}`));
    });
  });
}

async function fetchWorkspaceGitToken(auth: AuthConfig, result: WorkspaceCreateResult): Promise<WorkspaceGitAccessToken | null> {
  const baseURL = openbrainBaseURL(auth);
  if (!baseURL || !result.orgID || !result.workspaceID) {
    return null;
  }
  const orgID = requireBoundOrgID(auth, result.orgID, true);
  const requestURL = `${baseURL}/v1/orgs/${encodeURIComponent(orgID)}/workspaces/${encodeURIComponent(result.workspaceID)}/git-token`;
  const res = await fetch(requestURL, {
    method: 'POST',
    headers: authHeaders(auth),
  });
  return parseJSONResponse<WorkspaceGitAccessToken>(res, requestURL);
}

export async function queueWorkspaceBrainSync(auth: AuthConfig, result: WorkspaceCreateResult): Promise<void> {
  const baseURL = openbrainBaseURL(auth);
  if (!baseURL || !result.orgID || !result.workspaceID) {
    return;
  }
  const orgID = requireBoundOrgID(auth, result.orgID, true);
  const requestURL = `${baseURL}/v1/orgs/${encodeURIComponent(orgID)}/workspaces/${encodeURIComponent(result.workspaceID)}/brain/sync`;
  const res = await fetch(requestURL, {
    method: 'POST',
    headers: authHeaders(auth),
  });
  await parseJSONResponse<{ ok?: boolean; status?: string }>(res, requestURL);
}

export async function archiveWorkspaceBrain(auth: AuthConfig, input: { orgID?: string; workspaceID?: string }): Promise<void> {
  const baseURL = openbrainBaseURL(auth);
  const workspaceID = (input.workspaceID || '').trim();
  if (!baseURL || !input.orgID || !workspaceID) {
    throw new Error('OpenBrain Cloud workspace identity is missing.');
  }
  const orgID = requireBoundOrgID(auth, input.orgID, true);
  const requestURL = `${baseURL}/v1/orgs/${encodeURIComponent(orgID)}/workspaces/${encodeURIComponent(workspaceID)}/brain/archive`;
  const res = await fetch(requestURL, {
    method: 'POST',
    headers: authHeaders(auth),
  });
  await parseJSONResponse<{ ok?: boolean; status?: string }>(res, requestURL);
}

export async function applyWorkspaceBrainSourceAction(
  auth: AuthConfig,
  input: WorkspaceBrainSourceActionInput,
): Promise<WorkspaceBrainSourceActionResult> {
  const baseURL = openbrainBaseURL(auth);
  const workspaceID = (input.workspaceID || '').trim();
  if (!baseURL || !input.orgID || !workspaceID) {
    throw new Error('OpenBrain Cloud workspace identity is missing.');
  }
  const orgID = requireBoundOrgID(auth, input.orgID, true);
  const requestURL = `${baseURL}/v1/orgs/${encodeURIComponent(orgID)}/workspaces/${encodeURIComponent(workspaceID)}/brain/source-action`;
  const res = await fetch(requestURL, {
    method: 'POST',
    headers: authHeaders(auth),
    body: JSON.stringify({
      disableQueries: input.disableQueries === true,
      enableQueries: input.enableQueries === true,
      disableSync: input.disableSync === true,
      hardDelete: input.hardDelete === true,
      confirmWorkspaceID: input.confirmWorkspaceID,
      confirmName: input.confirmName,
    }),
  });
  return parseJSONResponse<WorkspaceBrainSourceActionResult>(res, requestURL);
}

export async function inspectLocalGitHubRepository(workspacePath: string): Promise<GitHubRepositoryRef | null> {
  await assertExistingDirectory(workspacePath);
  if (!await isGitRepository(workspacePath)) {
    return null;
  }
  let remote = '';
  try {
    remote = (await readGitCommand(workspacePath, ['remote', 'get-url', 'origin'])).trim();
  } catch {
    return null;
  }
  if (!remote) {
    return null;
  }
  const parsed = parseGitHubRemoteURL(remote);
  if (!parsed) {
    throw new Error(`OpenBrain Cloud only supports GitHub origin repositories. Selected directory origin is: ${remote}`);
  }
  return { ...parsed, remoteURL: remote };
}

export async function inspectRemoteGitHubRepository(host: SshHostWithSecrets, remotePath: string): Promise<GitHubRepositoryRef | null> {
  const workspacePath = normalizeRemoteWorkspacePath(remotePath);
  const command = remoteGitInspectCommand(workspacePath);
  let output = '';
  try {
    const result = await runSsh(host, command, 30_000);
    output = result.stdout.trim();
  } catch {
    return null;
  }
  if (!output) {
    return null;
  }
  const parsed = parseGitHubRemoteURL(output);
  if (!parsed) {
    throw new Error(`OpenBrain Cloud only supports GitHub origin repositories. Selected remote directory origin is: ${output}`);
  }
  return { ...parsed, remoteURL: output };
}

async function importGitWorkspace(
  remoteURL: string,
  workspacePath: string,
  defaultBranch: string,
  token?: WorkspaceGitAccessToken | null,
  options?: { existingGitHubRepository?: GitHubRepositoryRef | null; push?: boolean },
): Promise<void> {
  await assertExistingDirectory(workspacePath);
  const branch = (defaultBranch || 'main').trim() || 'main';
  if (!await isGitRepository(workspacePath)) {
    await runGitCommand(workspacePath, ['init']);
    await runGitCommand(workspacePath, ['checkout', '-B', branch]);
  }
  if (!options?.existingGitHubRepository) {
    await ensureGitRemote(workspacePath, remoteURL);
  }
  const status = await readGitCommand(workspacePath, ['status', '--porcelain']);
  const hasCommits = await gitHasCommits(workspacePath);
  if (status.trim() || !hasCommits) {
    await runGitCommand(workspacePath, ['add', '-A']);
    if (status.trim() || !hasCommits) {
      await commitWorkspaceImport(workspacePath, hasCommits);
    }
  }
  if (!await gitHasCommits(workspacePath)) {
    await runGitCommand(workspacePath, gitCommitArgs('Initial workspace import', '--allow-empty'));
  }
  if (options?.push !== false) {
    const pushRemote = options?.existingGitHubRepository ? remoteURL : 'origin';
    await runGitCommand(workspacePath, ['push', '-u', pushRemote, `HEAD:${branch}`], token);
  }
}

async function pathExists(target: string): Promise<boolean> {
  try {
    await fs.access(target);
    return true;
  } catch {
    return false;
  }
}

async function importRemoteGitWorkspace(
  remoteURL: string,
  remotePath: string,
  defaultBranch: string,
  host: SshHostWithSecrets,
  token?: WorkspaceGitAccessToken | null,
  options?: { existingGitHubRepository?: GitHubRepositoryRef | null; push?: boolean },
): Promise<void> {
  const workspacePath = normalizeRemoteWorkspacePath(remotePath);
  const command = remoteGitImportCommand({
    workspacePath,
    remoteURL,
    branch: (defaultBranch || 'main').trim() || 'main',
    token,
    preserveOrigin: Boolean(options?.existingGitHubRepository),
    push: options?.push !== false,
  });
  await runSsh(host, command, 180_000);
}

async function runGitCommand(cwd: string, args: string[], token?: WorkspaceGitAccessToken | null): Promise<void> {
  await readGitCommand(cwd, args, token);
}

async function readGitCommand(cwd: string, args: string[], token?: WorkspaceGitAccessToken | null): Promise<string> {
  const askpass = await createGitAskpass(token);
  try {
    return await new Promise<string>((resolve, reject) => {
      const child = spawn('git', args, {
        cwd,
        stdio: ['ignore', 'pipe', 'pipe'],
        env: {
          ...process.env,
          ...(askpass ? askpass.env : {}),
        },
      });
      let stdout = '';
      let stderr = '';
      child.stdout.on('data', (chunk) => {
        stdout += String(chunk);
      });
      child.stderr.on('data', (chunk) => {
        stderr += String(chunk);
      });
      child.on('error', reject);
      child.on('close', (code) => {
        if (code === 0) {
          resolve(stdout);
          return;
        }
        reject(new Error(stderr.trim() || stdout.trim() || `git ${args.join(' ')} failed with exit code ${code}`));
      });
    });
  } finally {
    if (askpass?.dir) {
      await fs.rm(askpass.dir, { recursive: true, force: true });
    }
  }
}

async function isGitRepository(workspacePath: string): Promise<boolean> {
  try {
    await readGitCommand(workspacePath, ['rev-parse', '--is-inside-work-tree']);
    return true;
  } catch {
    return false;
  }
}

async function gitHasCommits(workspacePath: string): Promise<boolean> {
  try {
    await readGitCommand(workspacePath, ['rev-parse', '--verify', 'HEAD']);
    return true;
  } catch {
    return false;
  }
}

async function ensureGitRemote(workspacePath: string, remoteURL: string): Promise<void> {
  const normalizedRemoteURL = remoteURL.trim();
  if (!normalizedRemoteURL) {
    throw new Error('Git remote URL is missing.');
  }
  let existing = '';
  try {
    existing = (await readGitCommand(workspacePath, ['remote', 'get-url', 'origin'])).trim();
  } catch {
    await runGitCommand(workspacePath, ['remote', 'add', 'origin', normalizedRemoteURL]);
    return;
  }
  if (gitRemoteURLsEqual(existing, normalizedRemoteURL)) {
    return;
  }
  throw new Error(`Selected directory already has a different origin remote: ${existing}`);
}

async function commitWorkspaceImport(workspacePath: string, hasCommits: boolean): Promise<void> {
  const message = hasCommits ? 'Sync workspace' : 'Initial workspace import';
  try {
    await runGitCommand(workspacePath, gitCommitArgs(message));
  } catch (err) {
    const text = (err as Error).message || '';
    if (text.includes('nothing to commit') || text.includes('no changes added to commit')) {
      return;
    }
    throw err;
  }
}

function gitCommitArgs(message: string, ...extra: string[]): string[] {
  return ['-c', 'user.name=OpenBrain', '-c', 'user.email=openbrain@users.noreply.github.com', 'commit', ...extra, '-m', message];
}

function gitRemoteURLsEqual(a: string, b: string): boolean {
  return normalizeGitRemoteURL(a) === normalizeGitRemoteURL(b);
}

function normalizeGitRemoteURL(value: string): string {
  return value.trim().replace(/\.git$/i, '').replace(/\/+$/g, '').toLowerCase();
}

function parseGitHubRemoteURL(remoteURL: string): { owner: string; name: string } | null {
  const value = remoteURL.trim();
  if (!value) {
    return null;
  }
  const httpsMatch = /^https?:\/\/(?:www\.)?github\.com\/([^/\s]+)\/([^/\s]+?)(?:\.git)?\/?$/i.exec(value);
  if (httpsMatch) {
    return { owner: httpsMatch[1], name: httpsMatch[2].replace(/\.git$/i, '') };
  }
  const sshMatch = /^git@github\.com:([^/\s]+)\/([^/\s]+?)(?:\.git)?$/i.exec(value);
  if (sshMatch) {
    return { owner: sshMatch[1], name: sshMatch[2].replace(/\.git$/i, '') };
  }
  const sshURLMatch = /^ssh:\/\/git@github\.com\/([^/\s]+)\/([^/\s]+?)(?:\.git)?\/?$/i.exec(value);
  if (sshURLMatch) {
    return { owner: sshURLMatch[1], name: sshURLMatch[2].replace(/\.git$/i, '') };
  }
  return null;
}

function isRemoteWindowsPath(remotePath: string): boolean {
  return /^[A-Za-z]:[\\/]/.test(remotePath) || remotePath.includes('\\');
}

function remoteGitInspectCommand(workspacePath: string): string {
  if (isRemoteWindowsPath(workspacePath)) {
    return buildPowerShellCommand([
      '$ErrorActionPreference = "Stop"',
      `$workspace = ${psQuote(workspacePath)}`,
      'if (!(Test-Path -LiteralPath $workspace -PathType Container)) { exit 0 }',
      'Set-Location -LiteralPath $workspace',
      'git rev-parse --is-inside-work-tree *> $null',
      'if ($LASTEXITCODE -ne 0) { exit 0 }',
      '$remote = git remote get-url origin 2>$null',
      'if ($LASTEXITCODE -eq 0 -and $remote) { Write-Output $remote.Trim() }',
    ].join('\n'));
  }
  return buildPosixSshCommand([
    `cd ${posixQuote(workspacePath)} 2>/dev/null || exit 0`,
    'git rev-parse --is-inside-work-tree >/dev/null 2>&1 || exit 0',
    'git remote get-url origin 2>/dev/null || true',
  ].join('\n'));
}

function remoteGitImportCommand(input: {
  workspacePath: string;
  remoteURL: string;
  branch: string;
  token?: WorkspaceGitAccessToken | null;
  preserveOrigin: boolean;
  push: boolean;
}): string {
  return isRemoteWindowsPath(input.workspacePath)
    ? remoteGitImportPowerShellCommand(input)
    : remoteGitImportPosixCommand(input);
}

function remoteGitImportPosixCommand(input: {
  workspacePath: string;
  remoteURL: string;
  branch: string;
  token?: WorkspaceGitAccessToken | null;
  preserveOrigin: boolean;
  push: boolean;
}): string {
  const accessToken = input.token?.accessToken?.trim() || '';
  const username = (input.token?.username || 'x-access-token').trim() || 'x-access-token';
  const lines = [
    'set -e',
    `WORKSPACE=${posixQuote(input.workspacePath)}`,
    `REMOTE_URL=${posixQuote(input.remoteURL)}`,
    `BRANCH=${posixQuote(input.branch)}`,
    `PRESERVE_ORIGIN=${input.preserveOrigin ? '1' : '0'}`,
    `GIT_USERNAME_VALUE=${posixQuote(username)}`,
    `GIT_PASSWORD_VALUE=${posixQuote(accessToken)}`,
    '[ -d "$WORKSPACE" ] || { echo "Selected remote workspace directory does not exist: $WORKSPACE" >&2; exit 1; }',
    'cd "$WORKSPACE"',
    'ASKPASS_DIR=""',
    'cleanup() { if [ -n "$ASKPASS_DIR" ]; then rm -rf "$ASKPASS_DIR"; fi; }',
    'trap cleanup EXIT',
    'export GIT_TERMINAL_PROMPT=0',
    'export GCM_INTERACTIVE=never',
    // Disable git credential helpers (e.g. macOS osxkeychain) so OpenBrain's
    // askpass token is used; system helpers run first and may return a token
    // scoped to a different repo, which GitHub reports as Repository not found.
    'export GIT_CONFIG_COUNT=1',
    'export GIT_CONFIG_KEY_0=credential.helper',
    'export GIT_CONFIG_VALUE_0=',
  ];
  if (accessToken) {
    lines.push(
      'ASKPASS_DIR="$(mktemp -d "${TMPDIR:-/tmp}/openbrain-git-askpass.XXXXXX")"',
      'ASKPASS="$ASKPASS_DIR/askpass.sh"',
      'cat > "$ASKPASS" <<\'EOF\'',
      '#!/bin/sh',
      'case "$1" in',
      '  *Username*) printf \'%s\\n\' "$GIT_USERNAME_VALUE" ;;',
      '  *) printf \'%s\\n\' "$GIT_PASSWORD_VALUE" ;;',
      'esac',
      'EOF',
      'chmod +x "$ASKPASS"',
      'export GIT_ASKPASS="$ASKPASS"',
    );
  }
  lines.push(
    'if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then',
    '  git init',
    '  git checkout -B "$BRANCH"',
    'fi',
    'if [ "$PRESERVE_ORIGIN" != "1" ]; then',
    '  if git remote get-url origin >/dev/null 2>&1; then git remote set-url origin "$REMOTE_URL"; else git remote add origin "$REMOTE_URL"; fi',
    'fi',
    'STATUS="$(git status --porcelain)"',
    'if ! git rev-parse --verify HEAD >/dev/null 2>&1; then HAS_COMMITS=0; else HAS_COMMITS=1; fi',
    'if [ -n "$STATUS" ] || [ "$HAS_COMMITS" = "0" ]; then',
    '  git add -A',
    '  if [ "$HAS_COMMITS" = "1" ]; then MSG="Sync workspace"; else MSG="Initial workspace import"; fi',
    '  if ! COMMIT_OUTPUT="$(git -c user.name=OpenBrain -c user.email=openbrain@users.noreply.github.com commit -m "$MSG" 2>&1)"; then',
    '    case "$COMMIT_OUTPUT" in',
    '      *"nothing to commit"*|*"no changes added to commit"*) ;;',
    '      *) printf "%s\\n" "$COMMIT_OUTPUT" >&2; exit 1 ;;',
    '    esac',
    '  fi',
    'fi',
    'if ! git rev-parse --verify HEAD >/dev/null 2>&1; then',
    '  git -c user.name=OpenBrain -c user.email=openbrain@users.noreply.github.com commit --allow-empty -m "Initial workspace import"',
    'fi',
  );
  if (input.push) {
    lines.push(
      'if [ "$PRESERVE_ORIGIN" = "1" ]; then PUSH_REMOTE="$REMOTE_URL"; else PUSH_REMOTE=origin; fi',
      'git push -u "$PUSH_REMOTE" "HEAD:$BRANCH"',
    );
  }
  return buildPosixSshCommand(lines.join('\n'));
}

function remoteGitImportPowerShellCommand(input: {
  workspacePath: string;
  remoteURL: string;
  branch: string;
  token?: WorkspaceGitAccessToken | null;
  preserveOrigin: boolean;
  push: boolean;
}): string {
  const accessToken = input.token?.accessToken?.trim() || '';
  const username = (input.token?.username || 'x-access-token').trim() || 'x-access-token';
  const lines = [
    '$ErrorActionPreference = "Stop"',
    `$workspace = ${psQuote(input.workspacePath)}`,
    `$remoteUrl = ${psQuote(input.remoteURL)}`,
    `$branch = ${psQuote(input.branch)}`,
    `$preserveOrigin = ${input.preserveOrigin ? '$true' : '$false'}`,
    'if (!(Test-Path -LiteralPath $workspace -PathType Container)) { throw "Selected remote workspace directory does not exist: $workspace" }',
    'Set-Location -LiteralPath $workspace',
    '$env:GIT_TERMINAL_PROMPT = "0"',
    '$env:GCM_INTERACTIVE = "never"',
    // Disable git credential helpers (e.g. macOS osxkeychain) so OpenBrain's
    // askpass token is used; system helpers run first and may return a token
    // scoped to a different repo, which GitHub reports as Repository not found.
    '$env:GIT_CONFIG_COUNT = "1"',
    '$env:GIT_CONFIG_KEY_0 = "credential.helper"',
    '$env:GIT_CONFIG_VALUE_0 = ""',
    '$askpassDir = $null',
    'try {',
  ];
  if (accessToken) {
    lines.push(
      `  $env:GIT_USERNAME_VALUE = ${psQuote(username)}`,
      `  $env:GIT_PASSWORD_VALUE = ${psQuote(accessToken)}`,
      '  $askpassDir = Join-Path ([IO.Path]::GetTempPath()) ("openbrain-git-askpass-" + [Guid]::NewGuid().ToString("N"))',
      '  New-Item -ItemType Directory -Force -Path $askpassDir | Out-Null',
      '  $askpass = Join-Path $askpassDir "askpass.bat"',
      '  Set-Content -LiteralPath $askpass -Encoding ASCII -Value "@echo off`r`necho %* | findstr /I ""Username"" >nul && (echo %GIT_USERNAME_VALUE%& exit /b 0)`r`necho %GIT_PASSWORD_VALUE%`r`n"',
      '  $env:GIT_ASKPASS = $askpass',
    );
  }
  lines.push(
    '  git rev-parse --is-inside-work-tree *> $null',
    '  if ($LASTEXITCODE -ne 0) { git init; git checkout -B $branch }',
    '  if (!$preserveOrigin) {',
    '    git remote get-url origin *> $null',
    '    if ($LASTEXITCODE -eq 0) { git remote set-url origin $remoteUrl } else { git remote add origin $remoteUrl }',
    '  }',
    '  $status = git status --porcelain',
    '  git rev-parse --verify HEAD *> $null',
    '  $hasCommits = $LASTEXITCODE -eq 0',
    '  if ($status -or !$hasCommits) {',
    '    git add -A',
    '    $message = if ($hasCommits) { "Sync workspace" } else { "Initial workspace import" }',
    '    $commitOutput = git -c user.name=OpenBrain -c user.email=openbrain@users.noreply.github.com commit -m $message 2>&1 | Out-String',
    '    if ($LASTEXITCODE -ne 0 -and $commitOutput -notmatch "nothing to commit|no changes added to commit") { throw $commitOutput.Trim() }',
    '  }',
    '  git rev-parse --verify HEAD *> $null',
    '  if ($LASTEXITCODE -ne 0) { git -c user.name=OpenBrain -c user.email=openbrain@users.noreply.github.com commit --allow-empty -m "Initial workspace import" }',
  );
  if (input.push) {
    lines.push(
      '  $pushRemote = if ($preserveOrigin) { $remoteUrl } else { "origin" }',
      '  git push -u $pushRemote "HEAD:$branch"',
    );
  }
  lines.push(
    '} finally {',
    '  if ($askpassDir) { Remove-Item -Recurse -Force -LiteralPath $askpassDir -ErrorAction SilentlyContinue }',
    '}',
  );
  return buildPowerShellCommand(lines.join('\n'));
}


async function createGitAskpass(token?: WorkspaceGitAccessToken | null): Promise<{ dir?: string; env: NodeJS.ProcessEnv } | null> {
  const accessToken = token?.accessToken?.trim();
  // Disable git credential helpers (e.g. macOS osxkeychain) so OpenBrain's
  // askpass token is used; system helpers run first and may return a token
  // scoped to a different repo, which GitHub reports as Repository not found.
  // An empty value clears the helper list (git 2.31+).
  const gitCredentialOverride = {
    GIT_CONFIG_COUNT: '1',
    GIT_CONFIG_KEY_0: 'credential.helper',
    GIT_CONFIG_VALUE_0: '',
  };
  if (!accessToken) {
    return {
      dir: '',
      env: {
        GIT_TERMINAL_PROMPT: '0',
        GCM_INTERACTIVE: 'never',
        ...gitCredentialOverride,
      },
    };
  }
  const username = (token?.username || 'x-access-token').trim() || 'x-access-token';
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), 'openbrain-git-askpass-'));
  const scriptPath = path.join(dir, process.platform === 'win32' ? 'askpass.bat' : 'askpass.sh');
  if (process.platform === 'win32') {
    await fs.writeFile(scriptPath, `@echo off\r\nset prompt=%*\r\necho %prompt% | findstr /I "Username" >nul && (echo ${escapeWindowsAskpass(username)}& exit /b 0)\r\necho ${escapeWindowsAskpass(accessToken)}\r\n`, { mode: 0o700 });
  } else {
    await fs.writeFile(scriptPath, `#!/bin/sh\ncase "$1" in\n  *Username*) printf '%s\\n' '${escapeShellSingleQuoted(username)}' ;;\n  *) printf '%s\\n' '${escapeShellSingleQuoted(accessToken)}' ;;\nesac\n`, { mode: 0o700 });
    await fs.chmod(scriptPath, 0o700);
  }
  return {
    dir,
    env: {
      GIT_ASKPASS: scriptPath,
      GIT_TERMINAL_PROMPT: '0',
      GCM_INTERACTIVE: 'never',
      ...gitCredentialOverride,
    },
  };
}

function escapeShellSingleQuoted(value: string): string {
  return value.replace(/'/g, `'\\''`);
}

function escapeWindowsAskpass(value: string): string {
  return value.replace(/%/g, '%%').replace(/\^/g, '^^').replace(/&/g, '^&').replace(/\|/g, '^|').replace(/</g, '^<').replace(/>/g, '^>');
}

function defaultWorkspaceSyncPolicy(enabled: boolean): WorkspaceSyncPolicy {
  return {
    autoSync: enabled,
    onOpen: false,
    onLocalChange: false,
    intervalSec: enabled ? 300 : 0,
    conflict: 'keep-both',
    deleteMode: 'trash',
  };
}

function safeLocalName(value: string): string {
  const normalized = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, '-')
    .replace(/^-+|-+$/g, '');
  return normalized || 'workspace';
}

function sanitizeWorkspaceRemoteHost(host: SshHost): SshHost {
  const { password: _password, passphrase: _passphrase, ...safeHost } = host as SshHost & {
    password?: string;
    passphrase?: string;
  };
  return { ...safeHost };
}

function normalizeRemoteWorkspacePath(workspacePath: string): string {
  const trimmed = (workspacePath || '').trim();
  if (!trimmed) {
    throw new Error('Select a remote workspace directory first.');
  }
  if (!trimmed.startsWith('/') && !/^[A-Za-z]:[\\/]/.test(trimmed)) {
    throw new Error('Remote workspace directory must be an absolute path.');
  }
  return trimmed;
}

function localNameFromRemoteWorkspacePath(workspacePath: string, fallback: string): string {
  const normalized = workspacePath.trim().replace(/[\\/]+$/g, '');
  const parts = normalized.split(/[\\/]/).filter(Boolean);
  return parts[parts.length - 1] || safeLocalName(fallback || 'workspace');
}

async function normalizeExistingWorkspacePath(workspacePath: string): Promise<string> {
  const trimmed = (workspacePath || '').trim();
  if (!trimmed) {
    throw new Error('Select a local workspace directory first.');
  }
  if (!path.isAbsolute(trimmed)) {
    throw new Error('Workspace directory must be an absolute path.');
  }
  const resolved = path.resolve(trimmed);
  await assertExistingDirectory(resolved);
  return resolved;
}

async function assertExistingDirectory(workspacePath: string): Promise<void> {
  let stat;
  try {
    stat = await fs.stat(workspacePath);
  } catch {
    throw new Error('Selected workspace directory does not exist.');
  }
  if (!stat.isDirectory()) {
    throw new Error('Selected workspace path is not a directory.');
  }
}

function localNameFromWorkspacePath(workspacePath: string, fallback: string): string {
  return path.basename(path.resolve(workspacePath)) || safeLocalName(fallback || 'workspace');
}

function normalizeWorkspacePathForIndex(workspacePath: string): string {
  const trimmed = (workspacePath || '').trim();
  return trimmed ? path.resolve(trimmed) : '';
}

function buildPosixSshCommand(script: string): string {
  return `sh -lc ${posixQuote(script)}`;
}

function posixQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}

function psQuote(value: string): string {
  return `'${value.replace(/'/g, "''")}'`;
}

function safeWorkspacePath(root: string, relativePath: string): string {
  const trimmed = relativePath.trim();
  if (!trimmed || path.isAbsolute(trimmed)) {
    throw new Error(`Invalid workspace template path: ${relativePath}`);
  }
  const target = path.resolve(root, trimmed);
  const resolvedRoot = path.resolve(root);
  if (target !== resolvedRoot && !target.startsWith(resolvedRoot + path.sep)) {
    throw new Error(`Workspace template path escapes root: ${relativePath}`);
  }
  return target;
}

async function writeFileIfMissing(filePath: string, content: string): Promise<void> {
  try {
    await fs.writeFile(filePath, content, { encoding: 'utf8', flag: 'wx' });
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code === 'EEXIST') {
      return;
    }
    throw err;
  }
}

function workspaceIndexPath(homeDir: string): string {
  return path.join(homeDir, '.openbrain', 'index', 'workspaces.json');
}

async function loadWorkspaceIndex(homeDir: string, auth?: AuthConfig | null): Promise<WorkspaceIndexFile> {
  const deploymentID = (auth?.deploymentID || '').trim();
  const orgID = (auth?.orgID || '').trim();
  const uid = (auth?.uid || '').trim();
  try {
    const raw = await fs.readFile(workspaceIndexPath(homeDir), 'utf8');
    const parsed = JSON.parse(raw) as Partial<WorkspaceIndexFile>;
    if (parsed.version !== 3 || !parsed.deployments || !deploymentID || !orgID || !uid) {
      return createActiveWorkspaceIndex(deploymentID, orgID, uid);
    }
    return activateWorkspaceIndex({
      version: 3,
      deployments: parsed.deployments,
      workspaces: [],
      hiddenWorkspaces: [],
    }, deploymentID, orgID, uid);
  } catch {
    return createActiveWorkspaceIndex(deploymentID, orgID, uid);
  }
}

async function saveWorkspaceIndex(homeDir: string, index: WorkspaceIndexFile): Promise<void> {
  const target = workspaceIndexPath(homeDir);
  const deploymentID = (index.activeDeploymentID || '').trim();
  const orgID = (index.activeOrgID || '').trim();
  const uid = (index.activeUID || '').trim();
  const deployments = { ...(index.deployments || {}) };
  if (deploymentID && orgID && uid) {
    const deployment = deployments[deploymentID] || { organizations: {} };
    const organizations = { ...(deployment.organizations || {}) };
    const organization = organizations[orgID] || { accounts: {} };
    const accounts = { ...(organization.accounts || {}) };
    accounts[uid] = {
      workspaces: sortWorkspaceIndexEntries(index.workspaces || []),
      hiddenWorkspaces: sortHiddenWorkspaceEntries(index.hiddenWorkspaces || []),
    };
    organizations[orgID] = { accounts };
    deployments[deploymentID] = { organizations };
  }
  normalizeWorkspaceIndexDeployments(deployments);
  await fs.mkdir(path.dirname(target), { recursive: true });
  await writeJsonFileAtomic(target, {
    version: 3,
    deployments,
  });
}

function createActiveWorkspaceIndex(deploymentID: string, orgID: string, uid: string): WorkspaceIndexFile {
  const normalizedDeploymentID = deploymentID.trim();
  const normalizedOrgID = orgID.trim();
  const normalizedUID = uid.trim();
  const index: WorkspaceIndexFile = {
    version: 3,
    deployments: {},
    workspaces: [],
    hiddenWorkspaces: [],
    activeDeploymentID: normalizedDeploymentID,
    activeOrgID: normalizedOrgID,
    activeUID: normalizedUID,
  };
  if (normalizedDeploymentID && normalizedOrgID && normalizedUID) {
    index.deployments[normalizedDeploymentID] = {
      organizations: {
        [normalizedOrgID]: {
          accounts: {
            [normalizedUID]: { workspaces: [], hiddenWorkspaces: [] },
          },
        },
      },
    };
  }
  return index;
}

function activateWorkspaceIndex(
  index: WorkspaceIndexFile,
  deploymentID: string,
  orgID: string,
  uid: string,
): WorkspaceIndexFile {
  const normalizedDeploymentID = deploymentID.trim();
  const normalizedOrgID = orgID.trim();
  const normalizedUID = uid.trim();
  const deployments = { ...(index.deployments || {}) };
  const deployment = deployments[normalizedDeploymentID] || { organizations: {} };
  const organizations = { ...(deployment.organizations || {}) };
  const organization = organizations[normalizedOrgID] || { accounts: {} };
  const accounts = { ...(organization.accounts || {}) };
  const account = accounts[normalizedUID] || { workspaces: [], hiddenWorkspaces: [] };
  if (normalizedDeploymentID && normalizedOrgID && normalizedUID) {
    accounts[normalizedUID] = account;
    organizations[normalizedOrgID] = { accounts };
    deployments[normalizedDeploymentID] = { organizations };
  }
  return {
    version: 3,
    deployments,
    workspaces: Array.isArray(account.workspaces) ? [...account.workspaces] : [],
    hiddenWorkspaces: Array.isArray(account.hiddenWorkspaces) ? [...account.hiddenWorkspaces] : [],
    activeDeploymentID: normalizedDeploymentID,
    activeOrgID: normalizedOrgID,
    activeUID: normalizedUID,
  };
}

function normalizeWorkspaceIndexDeployments(
  deployments: Record<string, WorkspaceIndexDeployment>,
): void {
  for (const deployment of Object.values(deployments)) {
    for (const organization of Object.values(deployment?.organizations || {})) {
      for (const [uid, account] of Object.entries(organization?.accounts || {})) {
        organization.accounts[uid] = {
          workspaces: sortWorkspaceIndexEntries(account?.workspaces || []),
          hiddenWorkspaces: sortHiddenWorkspaceEntries(account?.hiddenWorkspaces || []),
        };
      }
    }
  }
}

function workspaceIndexAccounts(
  index: WorkspaceIndexFile,
): Array<{ deploymentID: string; orgID: string; uid: string; account: WorkspaceIndexAccount }> {
  const result: Array<{ deploymentID: string; orgID: string; uid: string; account: WorkspaceIndexAccount }> = [];
  for (const [deploymentID, deployment] of Object.entries(index.deployments || {})) {
    for (const [orgID, organization] of Object.entries(deployment?.organizations || {})) {
      for (const [uid, account] of Object.entries(organization?.accounts || {})) {
        if (account) {
          result.push({ deploymentID, orgID, uid, account });
        }
      }
    }
  }
  return result;
}

function sortWorkspaceIndexEntries(entries: WorkspaceIndexEntry[]): WorkspaceIndexEntry[] {
  return [...entries].sort((a, b) => {
    const byName = (a.localName || '').localeCompare(b.localName || '');
    if (byName !== 0) {
      return byName;
    }
    return (a.workspaceID || '').localeCompare(b.workspaceID || '');
  });
}

function sortHiddenWorkspaceEntries(entries: HiddenWorkspaceIndexEntry[]): HiddenWorkspaceIndexEntry[] {
  return [...entries].sort((a, b) => {
    const left = `${a.orgID || ''}:${a.workspaceID || ''}`;
    const right = `${b.orgID || ''}:${b.workspaceID || ''}`;
    return left.localeCompare(right);
  });
}

async function upsertWorkspaceIndex(
  homeDir: string,
  entry: Omit<WorkspaceIndexEntry, 'createdAt' | 'updatedAt'>,
  auth?: AuthConfig | null,
  options?: { takeover?: boolean },
): Promise<WorkspaceIndexEntry> {
  const authUID = (auth?.uid || '').trim();
  const authDeploymentID = (auth?.deploymentID || '').trim();
  const authOrgID = (auth?.orgID || '').trim();
  if (!authUID) {
    throw new Error('auth_required: sign in before binding an OpenBrain workspace on this device');
  }
  if (!authDeploymentID || !authOrgID) {
    throw new Error('tenant_context_required: deployment and organization are required');
  }
  if (entry.orgID && entry.orgID !== authOrgID && entry.orgID !== 'local') {
    throw new Error('tenant_context_mismatch: workspace organization does not match the authenticated organization');
  }
  const index = await loadWorkspaceIndex(homeDir, auth);
  const now = new Date().toISOString();
  const existing = index.workspaces.find((item) => item.workspaceID === entry.workspaceID);
  const normalizedPath = normalizeWorkspacePathForIndex(entry.path);
  if (normalizedPath) {
    const activeDeploymentID = (index.activeDeploymentID || '').trim();
    const activeOrgID = (index.activeOrgID || '').trim();
    const activeUID = (index.activeUID || '').trim();
    for (const owner of workspaceIndexAccounts(index)) {
      if (
        owner.deploymentID === activeDeploymentID &&
        owner.orgID === activeOrgID &&
        owner.uid === activeUID
      ) {
        continue;
      }
      const otherPathEntry = (owner.account.workspaces || []).find((item) =>
        normalizeWorkspacePathForIndex(item.path) === normalizedPath
      );
      if (!otherPathEntry) {
        continue;
      }
      if (!options?.takeover) {
        throw new Error(
          `path_owned_by_other_tenant: ${entry.path} is already bound to workspace ${otherPathEntry.workspaceID} by ${owner.deploymentID}/${owner.orgID}/${owner.uid}`,
        );
      }
      owner.account.workspaces = (owner.account.workspaces || []).filter((item) =>
        normalizeWorkspacePathForIndex(item.path) !== normalizedPath
      );
    }
  }
  const pathConflict = index.workspaces.find((item) =>
    item.workspaceID !== entry.workspaceID
    && normalizedPath
    && normalizeWorkspacePathForIndex(item.path) === normalizedPath
  );
  if (pathConflict) {
    throw new Error(`workspace_path_conflict: ${entry.path} is already bound to workspace ${pathConflict.workspaceID}`);
  }
  const nextEntry: WorkspaceIndexEntry = {
    ...entry,
    createdAt: existing?.createdAt || now,
    updatedAt: now,
  };
  index.workspaces = [
    ...index.workspaces.filter((item) => item.workspaceID !== entry.workspaceID),
    nextEntry,
  ].sort((a, b) => a.localName.localeCompare(b.localName));
  const key = normalizeWorkspaceIdentity(entry.workspaceID, entry.orgID);
  index.hiddenWorkspaces = (index.hiddenWorkspaces || []).filter((hidden) => !workspaceIdentityEqual(hidden, key));
  await saveWorkspaceIndex(homeDir, index);
  return nextEntry;
}

function normalizeWorkspaceIdentity(workspaceID: string | undefined, orgID?: string): { workspaceID: string; orgID?: string } {
  return {
    workspaceID: (workspaceID || '').trim(),
    orgID: (orgID || '').trim() || undefined,
  };
}

function workspaceIdentityEqual(
  entry: { workspaceID?: string; orgID?: string },
  key: { workspaceID?: string; orgID?: string },
): boolean {
  const entryWorkspaceID = (entry.workspaceID || '').trim();
  const keyWorkspaceID = (key.workspaceID || '').trim();
  if (!entryWorkspaceID || !keyWorkspaceID || entryWorkspaceID !== keyWorkspaceID) {
    return false;
  }
  const entryOrgID = (entry.orgID || '').trim();
  const keyOrgID = (key.orgID || '').trim();
  return !entryOrgID || !keyOrgID || entryOrgID === keyOrgID;
}
