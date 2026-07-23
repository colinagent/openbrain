import type { AuthConfig } from '../auth/authStore';
import { isAuthInvalidResponse } from '../auth/authErrors';
import { resolveOpenBrainAPIBase } from '../auth/openbrainApiBase';
import {
  applyWorkspaceBrainSourceAction,
  archiveWorkspaceBrain,
  createLocalIndexWorkspace,
  isOpenBrainWorkspaceHidden,
  listWorkspaceTemplates,
  listIndexedOpenBrainWorkspaces,
  listConfiguredGBrainSourceWorkspaces,
  registerGBrainSourceForWorkspace,
  removeOpenBrainWorkspaceFromDevice,
  type LocalOpenBrainWorkspace,
  type WorkspaceBrainSourceActionResult,
  type WorkspaceIndexView,
} from '../workspace/openbrainWorkspace';
import {
  normalizeOpenBrainUserSettings,
  type LocalGBrainSettings,
  type OpenBrainUserSettings,
} from '../settings/settingsStore';
import type { SshHost, SshHostWithSecrets } from '../ssh/sshTypes';

export type OpenBrainProviderStatus = {
  provider: 'cloud' | 'local';
  authRequired?: boolean;
  configured: boolean;
  githubConnected?: boolean;
  cloudReady?: boolean;
  githubCheckError?: string;
};

export type OpenBrainListSourcesResponse = {
  success: boolean;
  code?: string;
  error?: string;
  provider?: 'cloud' | 'local';
  authRequired?: boolean;
  sources: LocalOpenBrainWorkspace[];
};

export type OpenBrainQueryInput = {
  brainID?: string;
  scope?: 'brain' | 'workspace';
  workspaceID?: string;
  orgID?: string;
  publicOwnerUID?: string;
  query?: string;
  limit?: number;
};

export type OpenBrainQueryResult = {
  chunkID: string;
  workspaceID: string;
  workspaceName: string;
  path?: string;
  relativePath: string;
  title: string;
  text: string;
  score: number;
};

export type OpenBrainQueryResponse = {
  success: boolean;
  code?: string;
  error?: string;
  provider?: 'cloud' | 'local';
  authRequired?: boolean;
  results: OpenBrainQueryResult[];
};

export type OpenBrainCreateResponse = {
  success: boolean;
  code?: string;
  error?: string;
  provider?: 'cloud' | 'local';
  authRequired?: boolean;
  workspace?: LocalOpenBrainWorkspace & {
    localName?: string;
    templateID?: string;
    templateVersion?: number;
    backupEnabled?: boolean;
    defaultLocalName?: string;
  };
};

export type OpenBrainMutationResponse = {
  success: boolean;
  code?: string;
  error?: string;
  provider?: 'cloud' | 'local';
  authRequired?: boolean;
  sourceID?: string;
  workspaceID?: string;
  orgID?: string;
  disabledQueries?: boolean;
  enabledQueries?: boolean;
  disabledSync?: boolean;
  hardDeleted?: boolean;
  syncJobsRemoved?: number;
  status?: string;
};

const CLOUD_WORKSPACE_TEMPLATE_ID = 'openbrain-cloud';

type CloudBrainWorkspace = {
  id?: string;
  orgID?: string;
  name?: string;
  slug?: string;
  repoProvider?: string;
  repoOwner?: string;
  repoName?: string;
  repoURL?: string;
  repoWebURL?: string;
  repoExternalID?: string;
  storageBackend?: string;
  storageProvider?: string;
  storageRemoteURL?: string;
  defaultBranch?: string;
  disabledQueries?: boolean;
  publicAccess?: boolean;
  effectivePermission?: 'read' | 'write' | 'admin';
  canMutateSource?: boolean;
  publicOwnerUID?: string;
  bindingMode?: 'own' | 'granted';
  status?: string;
  updatedAt?: string;
};

type CloudBrainSearchResult = {
  sourceID?: string;
  workspaceID?: string;
  path?: string;
  slug?: string;
  title?: string;
  chunkID?: string | number;
  chunkIndex?: number;
  chunkText?: string;
  score?: number;
};

class OpenBrainCloudAuthRequiredError extends Error {
  constructor(message = 'Sign in required to use OpenBrain Cloud.') {
    super(message);
    this.name = 'OpenBrainCloudAuthRequiredError';
  }
}

function providerSettings(settings?: OpenBrainUserSettings | null): Required<Pick<OpenBrainUserSettings, 'provider'>> & { local: LocalGBrainSettings } {
  const normalized = normalizeOpenBrainUserSettings(settings);
  return {
    provider: normalized.provider || 'cloud',
    local: normalized.local || {},
  };
}

function authHeaders(auth: AuthConfig): Record<string, string> {
  return {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${auth.token}`,
  };
}

function cloudBaseURL(auth: AuthConfig): string {
  return resolveOpenBrainAPIBase(auth);
}

async function parseJSON<T>(res: Response, requestURL: string): Promise<T> {
  const text = await res.text();
  let body = {} as T & { error?: string };
  if (text) {
    try {
      body = JSON.parse(text) as T & { error?: string };
    } catch {
      const snippet = text.trim().slice(0, 240);
      if (!res.ok) {
        throw new Error(`OpenBrain request failed: ${res.status} ${requestURL}${snippet ? ` - ${snippet}` : ''}`);
      }
      throw new Error(`OpenBrain request returned non-JSON response from ${requestURL}${snippet ? `: ${snippet}` : ''}`);
    }
  }
  if (!res.ok) {
    if (isAuthInvalidResponse(res.status, body.error)) {
      throw new OpenBrainCloudAuthRequiredError();
    }
    throw new Error(body.error || `OpenBrain request failed: ${res.status} ${requestURL}`);
  }
  return body as T;
}

function isOpenBrainCloudAuthRequiredError(error: unknown): error is OpenBrainCloudAuthRequiredError {
  return error instanceof OpenBrainCloudAuthRequiredError
    || (error instanceof Error && error.name === 'OpenBrainCloudAuthRequiredError');
}

function isOpenBrainCloudAuthRequiredFailure(error: unknown): boolean {
  if (isOpenBrainCloudAuthRequiredError(error)) {
    return true;
  }
  const message = error instanceof Error ? error.message.trim().toLowerCase() : '';
  if (!message) {
    return false;
  }
  if (message.includes('openbrain request failed: 401')) {
    return true;
  }
  if (!message.includes('openbrain request failed: 403')) {
    return false;
  }
  return message.includes('unauthorized')
    || message.includes('invalid session')
    || message.includes('missing authorization')
    || message.includes('authentication required');
}

function cloudAuthRequired(provider: 'cloud' | 'local' = 'cloud'): OpenBrainListSourcesResponse {
  return {
    success: false,
    code: 'auth_required',
    error: 'Sign in required to use OpenBrain Cloud.',
    provider,
    authRequired: true,
    sources: [],
  };
}

function cloudUnauthorized(provider: 'cloud' | 'local' = 'cloud'): OpenBrainListSourcesResponse {
  return {
    success: false,
    code: 'cloud_unauthorized',
    error: 'OpenBrain Cloud is not available for this account. Check your organization or cloud access.',
    provider,
    authRequired: false,
    sources: [],
  };
}

function localWorkspaceToSource(workspace: Awaited<ReturnType<typeof registerGBrainSourceForWorkspace>>): OpenBrainCreateResponse['workspace'] {
  return {
    sourceID: workspace.sourceID,
    workspaceID: workspace.workspaceID,
    orgID: workspace.orgID,
    brainID: 'personal',
    name: workspace.localName || workspace.defaultLocalName || workspace.sourceID,
    path: workspace.path,
    openable: true,
    locationKind: workspace.locationKind || 'local',
    remoteHost: workspace.remoteHost,
    localName: workspace.localName,
    templateID: workspace.templateID,
    templateVersion: workspace.templateVersion,
    backupEnabled: workspace.backupEnabled,
    defaultLocalName: workspace.defaultLocalName,
  };
}

function cloudWorkspaceName(workspace: CloudBrainWorkspace): string {
  return (workspace.name || workspace.slug || workspace.id || 'OpenBrain workspace').trim();
}

function mapCloudWorkspace(workspace: CloudBrainWorkspace, indexByID: Map<string, WorkspaceIndexView>): LocalOpenBrainWorkspace | null {
  const workspaceID = (workspace.id || '').trim();
  if (!workspaceID) {
    return null;
  }
  const indexed = indexByID.get(workspaceID);
  const indexedPath = indexed?.path || '';
  return {
    sourceID: workspaceID,
    workspaceID,
    orgID: (workspace.orgID || '').trim() || undefined,
    brainID: 'personal',
    name: cloudWorkspaceName(workspace),
    path: indexedPath || undefined,
    updatedAt: (workspace.updatedAt || '').trim() || undefined,
    remoteURL: workspace.storageRemoteURL || null,
    federated: true,
    openable: Boolean(indexedPath) && (indexed?.locationKind !== 'remote' || Boolean(indexed.remoteHost)),
    disabledQueries: workspace.disabledQueries === true,
    publicAccess: workspace.publicAccess === true,
    effectivePermission: workspace.effectivePermission,
    canMutateSource: typeof workspace.canMutateSource === 'boolean' ? workspace.canMutateSource : undefined,
    publicOwnerUID: (workspace.publicOwnerUID || '').trim() || undefined,
    bindingMode: workspace.bindingMode,
    locationKind: indexed?.locationKind || (indexedPath ? 'local' : undefined),
    remoteHost: indexed?.remoteHost,
  };
}

function mapCloudQueryResult(result: CloudBrainSearchResult, sourceNames: Map<string, string>): OpenBrainQueryResult | null {
  const chunkID = String(result.chunkID ?? '').trim();
  const workspaceID = (result.workspaceID || result.sourceID || '').trim();
  const text = (result.chunkText || '').trim();
  if (!chunkID || !workspaceID || !text) {
    return null;
  }
  const relativePath = (result.path || result.slug || '').trim();
  return {
    chunkID,
    workspaceID,
    workspaceName: sourceNames.get(workspaceID) || workspaceID,
    path: relativePath || undefined,
    relativePath,
    title: (result.title || result.slug || relativePath || 'OpenBrain note').trim(),
    text,
    score: typeof result.score === 'number' && Number.isFinite(result.score) ? result.score : 0,
  };
}

export async function listOpenBrainSources(params: {
  homeDir: string;
  settings?: OpenBrainUserSettings | null;
  auth?: AuthConfig | null;
}): Promise<OpenBrainListSourcesResponse> {
  const settings = providerSettings(params.settings);
  if (settings.provider === 'local') {
    const sources = await listConfiguredGBrainSourceWorkspaces(params.homeDir, settings.local);
    return { success: true, provider: 'local', sources };
  }
  if (!params.auth) {
    return cloudAuthRequired('cloud');
  }
  let workspaces: CloudBrainWorkspace[];
  try {
    workspaces = await listCloudBrainWorkspaces(params.auth);
  } catch (error) {
    if (isOpenBrainCloudAuthRequiredFailure(error)) {
      return cloudUnauthorized('cloud');
    }
    throw error;
  }
  const indexed = await listIndexedOpenBrainWorkspaces(params.homeDir, params.auth);
  const indexByID = new Map(indexed.map((entry) => [entry.workspaceID, entry]));
  const visibleWorkspaces: CloudBrainWorkspace[] = [];
  for (const workspace of workspaces) {
    if (await isOpenBrainWorkspaceHidden(params.homeDir, workspace.id || '', workspace.orgID, params.auth)) {
      continue;
    }
    visibleWorkspaces.push(workspace);
  }
  const sources = visibleWorkspaces
    .map((workspace) => mapCloudWorkspace(workspace, indexByID))
    .filter((source): source is LocalOpenBrainWorkspace => Boolean(source))
    .sort((a, b) => a.name.localeCompare(b.name));
  return { success: true, provider: 'cloud', sources };
}

async function listCloudBrainWorkspaces(auth: AuthConfig): Promise<CloudBrainWorkspace[]> {
  const base = cloudBaseURL(auth);
  if (!base) {
    throw new Error('OpenBrain API URL is not configured.');
  }
  const requestURL = `${base}/v1/me/brain/workspaces`;
  const res = await fetch(requestURL, {
    method: 'GET',
    headers: authHeaders(auth),
  });
  const payload = await parseJSON<{ workspaces?: CloudBrainWorkspace[] }>(res, requestURL);
  return payload.workspaces || [];
}

export async function queryOpenBrain(params: {
  homeDir: string;
  settings?: OpenBrainUserSettings | null;
  auth?: AuthConfig | null;
  input?: OpenBrainQueryInput | null;
}): Promise<OpenBrainQueryResponse> {
  const settings = providerSettings(params.settings);
  const query = (params.input?.query || '').trim();
  if (!query) {
    return { success: false, code: 'invalid_request', error: 'query is required', provider: settings.provider, results: [] };
  }
  if (settings.provider === 'local') {
    return queryLocalOpenBrain(params.homeDir, settings.local, params.input || { query });
  }
  if (!params.auth) {
    return {
      success: false,
      code: 'auth_required',
      error: 'Sign in required to use OpenBrain Cloud.',
      provider: 'cloud',
      authRequired: true,
      results: [],
    };
  }
  const base = cloudBaseURL(params.auth);
  if (!base) {
    return {
      success: false,
      code: 'cloud_unconfigured',
      error: 'OpenBrain API URL is not configured.',
      provider: 'cloud',
      results: [],
    };
  }
  const sourcesResponse = await listOpenBrainSources({ homeDir: params.homeDir, settings: { provider: 'cloud' }, auth: params.auth });
  if (!sourcesResponse.success && sourcesResponse.authRequired) {
    return {
      success: false,
      code: 'auth_required',
      error: 'Sign in required to use OpenBrain Cloud.',
      provider: 'cloud',
      authRequired: true,
      results: [],
    };
  }
  if (!sourcesResponse.success) {
    return {
      success: false,
      code: sourcesResponse.code || 'cloud_unavailable',
      error: sourcesResponse.error || 'OpenBrain Cloud is not available.',
      provider: 'cloud',
      authRequired: false,
      results: [],
    };
  }
  const sourceNames = new Map((sourcesResponse.sources || []).map((source) => [source.workspaceID || source.sourceID, source.name]));
  const workspaceID = (params.input?.workspaceID || '').trim();
  const workspaceOrgID = (params.input?.orgID || params.auth.orgID || '').trim();
  if (workspaceOrgID !== params.auth.orgID) {
    throw new Error('Workspace organization must match the token-bound organization.');
  }
  const endpoint = params.input?.scope === 'workspace' && workspaceID
    ? `/v1/orgs/${encodeURIComponent(workspaceOrgID || 'cloud')}/workspaces/${encodeURIComponent(workspaceID)}/brain/search`
    : '/v1/me/brain/search';
  const requestURL = `${base}${endpoint}`;
  const res = await fetch(requestURL, {
    method: 'POST',
    headers: authHeaders(params.auth),
    body: JSON.stringify({
      query,
      limit: params.input?.limit,
      orgID: params.auth.orgID,
      publicOwnerUID: params.input?.publicOwnerUID || undefined,
    }),
  });
  let payload: { results?: CloudBrainSearchResult[] };
  try {
    payload = await parseJSON<{ results?: CloudBrainSearchResult[] }>(res, requestURL);
  } catch (error) {
    if (isOpenBrainCloudAuthRequiredFailure(error)) {
      return {
        success: false,
        code: 'cloud_unauthorized',
        error: 'OpenBrain Cloud is not available for this account. Check your organization or cloud access.',
        provider: 'cloud',
        authRequired: false,
        results: [],
      };
    }
    throw error;
  }
  const results = (payload.results || [])
    .map((item) => mapCloudQueryResult(item, sourceNames))
    .filter((item): item is OpenBrainQueryResult => Boolean(item));
  return { success: true, provider: 'cloud', results };
}

async function queryLocalOpenBrain(
  homeDir: string,
  local: LocalGBrainSettings,
  input: OpenBrainQueryInput,
): Promise<OpenBrainQueryResponse> {
  const { queryConfiguredGBrain } = await import('./localGBrainQuery');
  return queryConfiguredGBrain(homeDir, local, input);
}

export async function createOpenBrainSource(params: {
  homeDir: string;
  settings?: OpenBrainUserSettings | null;
  auth?: AuthConfig | null;
  name?: string;
  localPath?: string;
  remotePath?: string;
  remoteHost?: SshHostWithSecrets | null;
  remoteHostView?: SshHost | null;
}): Promise<OpenBrainCreateResponse> {
  const settings = providerSettings(params.settings);
  if (settings.provider !== 'local') {
    return {
      success: false,
      code: 'openbrain_source_flow_required',
      error: 'Create or bind OpenBrain Cloud sources through the active runtime server.',
      provider: 'cloud',
    };
  }
  const workspacePath = (params.localPath || '').trim();
  const name = (params.name || pathName(workspacePath) || '').trim();
  if (!name) {
    return { success: false, code: 'invalid_request', error: 'GBrain source name is required.', provider: settings.provider };
  }
  if (!workspacePath) {
    return {
      success: false,
      code: 'invalid_request',
      error: 'Select a local workspace directory first.',
      provider: 'local',
    };
  }
  if ((settings.local.remoteMcpUrl || '').trim()) {
    return {
      success: false,
      code: 'remote_mcp_create_unsupported',
      error: 'Remote MCP GBrain is configured, but creating OpenBrain sources from desktop currently requires a local PGLite or Postgres GBrain database.',
      provider: 'local',
    };
  }
  const materialized = await createLocalIndexWorkspace(params.homeDir, { name, localPath: workspacePath }, params.auth);
  const registered = await registerGBrainSourceForWorkspace(params.homeDir, materialized, settings.local);
  return { success: true, provider: 'local', workspace: localWorkspaceToSource(registered) };
}

export async function removeOpenBrainSourceFromDevice(params: {
  homeDir: string;
  settings?: OpenBrainUserSettings | null;
  auth?: AuthConfig | null;
  workspaceID?: string;
  orgID?: string;
  path?: string;
}): Promise<OpenBrainMutationResponse> {
  const settings = providerSettings(params.settings);
  await removeOpenBrainWorkspaceFromDevice(params.homeDir, {
    workspaceID: params.workspaceID,
    orgID: params.orgID,
    path: params.path,
  }, params.auth);
  return { success: true, provider: settings.provider };
}

export async function archiveOpenBrainSource(params: {
  homeDir: string;
  settings?: OpenBrainUserSettings | null;
  auth?: AuthConfig | null;
  workspaceID?: string;
  orgID?: string;
  path?: string;
}): Promise<OpenBrainMutationResponse> {
  const settings = providerSettings(params.settings);
  if (settings.provider !== 'cloud') {
    return {
      success: false,
      code: 'provider_unsupported',
      error: 'Cloud archive is only available for OpenBrain Cloud sources.',
      provider: settings.provider,
    };
  }
  if (!params.auth) {
    return {
      success: false,
      code: 'auth_required',
      error: 'Sign in required to archive an OpenBrain Cloud workspace.',
      provider: 'cloud',
      authRequired: true,
    };
  }
  try {
    await archiveWorkspaceBrain(params.auth, {
      orgID: params.orgID,
      workspaceID: params.workspaceID,
    });
    await removeOpenBrainWorkspaceFromDevice(params.homeDir, {
      workspaceID: params.workspaceID,
      orgID: params.orgID,
      path: params.path,
    }, params.auth);
    return { success: true, provider: 'cloud' };
  } catch (error) {
    if (isOpenBrainCloudAuthRequiredFailure(error)) {
      return {
        success: false,
        code: 'cloud_unauthorized',
        error: 'OpenBrain Cloud is not available for this account. Check your organization or cloud access.',
        provider: 'cloud',
        authRequired: false,
      };
    }
    throw error;
  }
}

export async function applyOpenBrainSourceAction(params: {
  homeDir: string;
  settings?: OpenBrainUserSettings | null;
  auth?: AuthConfig | null;
  workspaceID?: string;
  orgID?: string;
  path?: string;
  disableQueries?: boolean;
  enableQueries?: boolean;
  disableSync?: boolean;
  hardDelete?: boolean;
  confirmWorkspaceID?: string;
  confirmName?: string;
}): Promise<OpenBrainMutationResponse> {
  const settings = providerSettings(params.settings);
  if (settings.provider !== 'cloud') {
    return {
      success: false,
      code: 'provider_unsupported',
      error: 'Cloud source actions are only available for OpenBrain Cloud sources.',
      provider: settings.provider,
    };
  }
  if (!params.auth) {
    return {
      success: false,
      code: 'auth_required',
      error: 'Sign in required to update an OpenBrain Cloud workspace.',
      provider: 'cloud',
      authRequired: true,
    };
  }
  try {
    const result: WorkspaceBrainSourceActionResult = await applyWorkspaceBrainSourceAction(params.auth, {
      orgID: params.orgID,
      workspaceID: params.workspaceID,
      disableQueries: params.disableQueries,
      enableQueries: params.enableQueries,
      disableSync: params.disableSync,
      hardDelete: params.hardDelete,
      confirmWorkspaceID: params.confirmWorkspaceID,
      confirmName: params.confirmName,
    });
    if (params.hardDelete || result.hardDeleted) {
      await removeOpenBrainWorkspaceFromDevice(params.homeDir, {
        workspaceID: params.workspaceID,
        orgID: params.orgID,
        path: params.path,
      }, params.auth);
    }
    return {
      success: true,
      provider: 'cloud',
      sourceID: result.sourceID,
      workspaceID: result.workspaceID,
      orgID: result.orgID,
      disabledQueries: result.disabledQueries === true,
      enabledQueries: result.enabledQueries === true,
      disabledSync: result.disabledSync === true,
      hardDeleted: result.hardDeleted === true,
      syncJobsRemoved: result.syncJobsRemoved,
      status: result.status,
    };
  } catch (error) {
    if (isOpenBrainCloudAuthRequiredFailure(error)) {
      return {
        success: false,
        code: 'cloud_unauthorized',
        error: 'OpenBrain Cloud is not available for this account. Check your organization or cloud access.',
        provider: 'cloud',
        authRequired: false,
      };
    }
    throw error;
  }
}

function isUsableGitHubAccount(account: {
  owner?: string;
  canCreateRepository?: boolean;
  canSyncRepository?: boolean;
}): boolean {
  if (!account.owner?.trim()) {
    return false;
  }
  if (account.canCreateRepository === false || account.canSyncRepository === false) {
    return false;
  }
  return true;
}

function firstUsableGitHubOwnerFromProviders(
  providers: Array<{
    provider?: string;
    accounts?: Array<{
      owner?: string;
      canCreateRepository?: boolean;
      canSyncRepository?: boolean;
    }>;
  }>,
): string {
  for (const provider of providers) {
    if ((provider.provider || '').trim().toLowerCase() !== 'github') {
      continue;
    }
    for (const account of provider.accounts || []) {
      if (isUsableGitHubAccount(account)) {
        return account.owner!.trim();
      }
    }
  }
  return '';
}

async function defaultGitHubRepositoryOwner(auth: AuthConfig): Promise<string> {
  const templates = await listWorkspaceTemplates(auth);
  const cloudTemplate = templates.templates.find((template) => template.templateID === CLOUD_WORKSPACE_TEMPLATE_ID);
  const providers = [
    ...(cloudTemplate?.storage?.providers || []),
    ...(cloudTemplate?.repository?.providers || []),
  ];
  return firstUsableGitHubOwnerFromProviders(providers);
}

function pathName(localPath: string): string {
  const normalized = localPath.trim().replace(/[\\/]+$/, '');
  if (!normalized) {
    return '';
  }
  return normalized.split(/[\\/]/).filter(Boolean).pop() || normalized;
}

export async function getOpenBrainProviderStatus(
  settings?: OpenBrainUserSettings | null,
  auth?: AuthConfig | null,
): Promise<OpenBrainProviderStatus> {
  const normalized = providerSettings(settings);
  const authRequired = normalized.provider === 'cloud' && !auth;
  const configured = normalized.provider === 'local' || Boolean(auth);

  if (normalized.provider === 'local') {
    return {
      provider: normalized.provider,
      authRequired,
      configured,
      githubConnected: true,
      cloudReady: true,
    };
  }

  if (!auth) {
    return {
      provider: normalized.provider,
      authRequired,
      configured,
      githubConnected: false,
      cloudReady: false,
    };
  }

  try {
    const owner = await defaultGitHubRepositoryOwner(auth);
    const githubConnected = Boolean(owner);
    return {
      provider: normalized.provider,
      authRequired,
      configured,
      githubConnected,
      cloudReady: githubConnected,
    };
  } catch (error) {
    return {
      provider: normalized.provider,
      authRequired,
      configured,
      githubConnected: false,
      cloudReady: false,
      githubCheckError: error instanceof Error ? error.message : 'Failed to check GitHub connection.',
    };
  }
}
