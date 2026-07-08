import type { LocalOpenBrainWorkspace } from '../store/openBrainStore';
import { useAppStore } from '../store/appStore';
import { useTabManagerStore } from '../store/tabManagerStore';

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

export type OpenBrainListSourcesResponse = {
  success: boolean;
  code?: string;
  error?: string;
  provider?: 'cloud' | 'local';
  authRequired?: boolean;
  sources: LocalOpenBrainWorkspace[];
};

export type OpenBrainQueryResponse = {
  success: boolean;
  code?: string;
  error?: string;
  provider?: 'cloud' | 'local';
  authRequired?: boolean;
  results: OpenBrainQueryResult[];
};

export type OpenBrainStatusResponse = {
  success: boolean;
  code?: string;
  error?: string;
  status?: Record<string, unknown>;
};

export type OpenBrainCreateSourceInput = {
  name?: string;
  localPath?: string;
  path?: string;
  sourceID?: string;
  workspaceID?: string;
  orgID?: string;
  takeover?: boolean;
  createRequestID?: string;
};

export type OpenBrainMutationInput = {
  sourceID?: string;
  workspaceID?: string;
  orgID?: string;
  path?: string;
};

export type OpenBrainSourceActionInput = OpenBrainMutationInput & {
  disableQueries?: boolean;
  enableQueries?: boolean;
  disableSync?: boolean;
  hardDelete?: boolean;
  confirmWorkspaceID?: string;
  confirmName?: string;
};

export type OpenBrainCreateSourceResponse = {
  success: boolean;
  code?: string;
  error?: string;
  provider?: 'cloud' | 'local';
  authRequired?: boolean;
  pathOwnerUID?: string;
  requestID?: string;
  cleanupAttempted?: boolean;
  cleanupSucceeded?: boolean;
  cleanupError?: string;
  workspace?: LocalOpenBrainWorkspace;
};

export type OpenBrainVerifySourceResponse = OpenBrainCreateSourceResponse;

export type OpenBrainRecoveryCandidate = {
  path: string;
  name?: string;
};

export type OpenBrainRecoveryCandidatesResponse = {
  success: boolean;
  code?: string;
  error?: string;
  provider?: 'cloud' | 'local';
  authRequired?: boolean;
  candidates: OpenBrainRecoveryCandidate[];
};

export type OpenBrainMutationResponse = {
  success: boolean;
  code?: string;
  error?: string;
  provider?: 'cloud' | 'local';
  authRequired?: boolean;
};

export type OpenBrainSourceActionResponse = OpenBrainMutationResponse & {
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

export type OpenBrainSourceShareUser = {
  uid: string;
  name?: string;
  username?: string;
  email?: string;
  permission: string;
};

export type OpenBrainSourceShare = {
  orgID: string;
  resourceID: string;
  sourceID: string;
  users: OpenBrainSourceShareUser[];
  public?: {
    id: string;
    ownerUID: string;
    orgID: string;
    resourceID: string;
    sourceID: string;
    status: string;
    riskAckVersion?: string;
  } | null;
};

export type OpenBrainPublicBrainSource = {
  sourceID: string;
  name?: string;
  workspaceID?: string;
  orgID?: string;
};

export type OpenBrainPublicBrainEntry = {
  ownerUID: string;
  name: string;
  username: string;
  ownerInitial?: string;
  avatar?: string;
  activeSourceCount: number;
  subscribed: boolean;
  owned?: boolean;
  description?: string;
  sources?: OpenBrainPublicBrainSource[];
};

export type OpenBrainPublicBrainsResponse = {
  brains: OpenBrainPublicBrainEntry[];
};

export type OpenBrainPublicBrainSourcesResponse = {
  sources: OpenBrainPublicBrainSource[];
};

export type OpenBrainPublicBrainProfile = {
  ownerUID: string;
  name: string;
  username: string;
  ownerInitial?: string;
  avatar?: string;
  activeSourceCount: number;
  description?: string;
};

export type OpenBrainProviderMode = 'cloud' | 'local';

type OpenBrainRequestOptions = {
  provider?: OpenBrainProviderMode;
};

function getActiveWorkspaceTabId(): string {
  return useTabManagerStore.getState().activeTabId;
}

function resolveOpenBrainBaseUrl(workspaceTabId?: string): string {
  const tabState = useTabManagerStore.getState();
  const targetTabId = workspaceTabId || getActiveWorkspaceTabId();
  const targetTab = tabState.tabs.find((tab) => tab.id === targetTabId) || null;
  if (workspaceTabId && !targetTab) {
    throw new Error('OpenBrain runtime is not connected.');
  }
  const ws = useAppStore.getStoreByTabId(targetTabId).getState();
  const port = ws.remoteSession?.localPort;
  if (targetTab?.kind === 'remote') {
    if (!port) {
      throw new Error('OpenBrain remote runtime is not connected.');
    }
    return `http://127.0.0.1:${port}`;
  }
  return port ? `http://127.0.0.1:${port}` : 'http://127.0.0.1:19530';
}

async function readJSONResponse<T>(res: Response): Promise<T> {
  const text = await res.text();
  const trimmed = text.trim();
  let payload: unknown = {};
  if (trimmed) {
    try {
      payload = JSON.parse(trimmed);
    } catch {
      throw new Error(trimmed);
    }
  }
  if (!res.ok) {
    const message = typeof (payload as { error?: unknown }).error === 'string'
      ? (payload as { error: string }).error
      : (trimmed || `Request failed: ${res.status}`);
    throw new Error(message);
  }
  return payload as T;
}

async function getJSON<T>(path: string, workspaceTabId?: string): Promise<T> {
  const res = await fetch(`${resolveOpenBrainBaseUrl(workspaceTabId)}${path}`);
  return readJSONResponse<T>(res);
}

async function postJSON<T>(path: string, payload: unknown, workspaceTabId?: string): Promise<T> {
  const res = await fetch(`${resolveOpenBrainBaseUrl(workspaceTabId)}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload || {}),
  });
  return readJSONResponse<T>(res);
}

async function putJSON<T>(path: string, payload: unknown, workspaceTabId?: string): Promise<T> {
  const res = await fetch(`${resolveOpenBrainBaseUrl(workspaceTabId)}${path}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload || {}),
  });
  return readJSONResponse<T>(res);
}

async function deleteJSON<T>(path: string, workspaceTabId?: string): Promise<T> {
  const res = await fetch(`${resolveOpenBrainBaseUrl(workspaceTabId)}${path}`, { method: 'DELETE' });
  return readJSONResponse<T>(res);
}

export async function listGBrainSources(
  workspaceTabId?: string,
  options?: OpenBrainRequestOptions,
): Promise<OpenBrainListSourcesResponse> {
  const path = options?.provider === 'local'
    ? '/v1/openbrain/sources'
    : '/v1/openbrain/cloud/sources';
  return listOpenBrainSourcesFromPath(path, workspaceTabId);
}

export async function listCachedOpenBrainSources(workspaceTabId?: string): Promise<OpenBrainListSourcesResponse> {
  return listOpenBrainSourcesFromPath('/v1/openbrain/cached-sources', workspaceTabId);
}

async function listOpenBrainSourcesFromPath(
  path: string,
  workspaceTabId?: string,
): Promise<OpenBrainListSourcesResponse> {
  const result = await getJSON<OpenBrainListSourcesResponse>(path, workspaceTabId);
  return {
    success: result?.success === true,
    code: result?.code,
    error: result?.error,
    provider: result?.provider,
    authRequired: result?.authRequired,
    sources: Array.isArray(result?.sources) ? result.sources : [],
  };
}

export async function queryOpenBrain(
  input: OpenBrainQueryInput,
  workspaceTabId?: string,
  options?: OpenBrainRequestOptions,
): Promise<OpenBrainQueryResponse> {
  const path = options?.provider === 'local'
    ? '/v1/openbrain/query'
    : '/v1/openbrain/cloud/query';
  const result = await postJSON<OpenBrainQueryResponse>(path, input, workspaceTabId);
  return {
    success: result?.success === true,
    code: result?.code,
    error: result?.error,
    provider: result?.provider,
    authRequired: result?.authRequired,
    results: Array.isArray(result?.results) ? result.results : [],
  };
}

export async function createOpenBrainSource(
  input: OpenBrainCreateSourceInput,
  workspaceTabId?: string,
  options?: OpenBrainRequestOptions,
): Promise<OpenBrainCreateSourceResponse> {
  if (options?.provider === 'local') {
    const localPath = (input.localPath || input.path || '').trim();
    const result = await window.electronAPI?.openBrain?.createSource?.({
      name: input.name,
      localPath,
    });
    if (!result) {
      throw new Error('OpenBrain provider API is not available.');
    }
    return result as OpenBrainCreateSourceResponse;
  }
  return postJSON<OpenBrainCreateSourceResponse>('/v1/openbrain/cloud/sources', input, workspaceTabId);
}

export async function verifyOpenBrainSource(
  input: OpenBrainMutationInput,
  workspaceTabId?: string,
): Promise<OpenBrainVerifySourceResponse> {
  return postJSON<OpenBrainVerifySourceResponse>('/v1/openbrain/cloud/sources/verify', input, workspaceTabId);
}

export async function listOpenBrainRecoveryCandidates(
  input: OpenBrainMutationInput & { paths?: string[] },
  workspaceTabId?: string,
): Promise<OpenBrainRecoveryCandidatesResponse> {
  const result = await postJSON<OpenBrainRecoveryCandidatesResponse>(
    '/v1/openbrain/cloud/sources/recovery-candidates',
    input,
    workspaceTabId,
  );
  return {
    success: result?.success === true,
    code: result?.code,
    error: result?.error,
    provider: result?.provider,
    authRequired: result?.authRequired,
    candidates: Array.isArray(result?.candidates) ? result.candidates : [],
  };
}

export async function removeOpenBrainSourceFromDevice(
  input: OpenBrainMutationInput,
  workspaceTabId?: string,
): Promise<OpenBrainMutationResponse> {
  return postJSON<OpenBrainMutationResponse>('/v1/openbrain/cloud/sources/remove', input, workspaceTabId);
}

export async function archiveOpenBrainSource(
  input: OpenBrainMutationInput,
  workspaceTabId?: string,
): Promise<OpenBrainMutationResponse> {
  return postJSON<OpenBrainMutationResponse>('/v1/openbrain/cloud/sources/archive', input, workspaceTabId);
}

export async function applyOpenBrainSourceAction(
  input: OpenBrainSourceActionInput,
  workspaceTabId?: string,
): Promise<OpenBrainSourceActionResponse> {
  return postJSON<OpenBrainSourceActionResponse>('/v1/openbrain/cloud/sources/action', input, workspaceTabId);
}

function sourceSharePath(input: OpenBrainMutationInput): string {
  const orgID = (input.orgID || '').trim();
  const resourceID = (input.workspaceID || input.sourceID || '').trim();
  if (!orgID || !resourceID) {
    throw new Error('OpenBrain Cloud source identity is required.');
  }
  return `/v1/openbrain/cloud/orgs/${encodeURIComponent(orgID)}/resources/${encodeURIComponent(resourceID)}/source-share`;
}

export async function getOpenBrainSourceShare(
  input: OpenBrainMutationInput,
  workspaceTabId?: string,
): Promise<OpenBrainSourceShare> {
  const result = await getJSON<OpenBrainSourceShare>(sourceSharePath(input), workspaceTabId);
  return {
    ...result,
    users: Array.isArray(result?.users) ? result.users : [],
    public: result?.public || null,
  };
}

export async function setOpenBrainSourcePublic(
  input: OpenBrainMutationInput,
  workspaceTabId?: string,
): Promise<OpenBrainSourceShare['public']> {
  return putJSON<OpenBrainSourceShare['public']>(`${sourceSharePath(input)}/public`, {
    riskAcknowledged: true,
    riskAckVersion: 'public-source-v1',
  }, workspaceTabId);
}

export async function revokeOpenBrainSourcePublic(
  input: OpenBrainMutationInput,
  workspaceTabId?: string,
): Promise<OpenBrainMutationResponse> {
  return deleteJSON<OpenBrainMutationResponse>(`${sourceSharePath(input)}/public`, workspaceTabId);
}

export async function shareOpenBrainSourceWithUser(
  input: OpenBrainMutationInput & { email: string },
  workspaceTabId?: string,
): Promise<OpenBrainSourceShareUser> {
  return putJSON<OpenBrainSourceShareUser>(`${sourceSharePath(input)}/users`, { email: input.email }, workspaceTabId);
}

export async function revokeOpenBrainSourceUserShare(
  input: OpenBrainMutationInput & { uid: string },
  workspaceTabId?: string,
): Promise<OpenBrainMutationResponse> {
  return deleteJSON<OpenBrainMutationResponse>(
    `${sourceSharePath(input)}/users/${encodeURIComponent(input.uid.trim())}`,
    workspaceTabId,
  );
}

export async function getOpenBrainPublicBrainProfile(
  workspaceTabId?: string,
): Promise<OpenBrainPublicBrainProfile> {
  return getJSON<OpenBrainPublicBrainProfile>('/v1/openbrain/cloud/public-profile', workspaceTabId);
}

export async function updateOpenBrainPublicBrainProfile(
  input: { description: string },
  workspaceTabId?: string,
): Promise<OpenBrainPublicBrainProfile> {
  return putJSON<OpenBrainPublicBrainProfile>('/v1/openbrain/cloud/public-profile', {
    description: input.description,
  }, workspaceTabId);
}

export async function listOpenBrainPublicBrains(
  query: string,
  workspaceTabId?: string,
  options?: { includeSelf?: boolean },
): Promise<OpenBrainPublicBrainEntry[]> {
  const params = new URLSearchParams();
  const search = query.trim();
  if (search) {
    params.set('query', search);
  }
  if (options?.includeSelf) {
    params.set('includeSelf', 'true');
  }
  const encoded = params.toString();
  const path = `/v1/openbrain/cloud/public-brains${encoded ? `?${encoded}` : ''}`;
  const result = await getJSON<OpenBrainPublicBrainsResponse>(path, workspaceTabId);
  return Array.isArray(result?.brains) ? result.brains : [];
}

export async function resolveOpenBrainPublicBrainSources(
  ownerUID: string,
  workspaceTabId?: string,
): Promise<OpenBrainPublicBrainSource[]> {
  const trimmed = ownerUID.trim();
  if (!trimmed) {
    return [];
  }
  const result = await getJSON<OpenBrainPublicBrainSourcesResponse>(
    `/v1/openbrain/cloud/public-brains/${encodeURIComponent(trimmed)}/sources`,
    workspaceTabId,
  );
  return Array.isArray(result?.sources) ? result.sources : [];
}

export async function subscribeOpenBrainPublicBrain(
  ownerUID: string,
  workspaceTabId?: string,
): Promise<OpenBrainPublicBrainEntry> {
  return putJSON<OpenBrainPublicBrainEntry>(
    `/v1/openbrain/cloud/public-brains/${encodeURIComponent(ownerUID.trim())}/subscription`,
    {},
    workspaceTabId,
  );
}

export async function unsubscribeOpenBrainPublicBrain(
  ownerUID: string,
  workspaceTabId?: string,
): Promise<OpenBrainMutationResponse> {
  return deleteJSON<OpenBrainMutationResponse>(
    `/v1/openbrain/cloud/public-brains/${encodeURIComponent(ownerUID.trim())}/subscription`,
    workspaceTabId,
  );
}

export type OpenBrainProviderStatus = {
  provider: 'cloud' | 'local';
  authRequired?: boolean;
  configured?: boolean;
  githubConnected?: boolean;
  cloudReady?: boolean;
  githubCheckError?: string;
};

export async function getOpenBrainProviderStatus(): Promise<OpenBrainProviderStatus> {
  const provider = await window.electronAPI?.openBrain?.getProvider?.();
  return {
    provider: provider?.provider === 'local' ? 'local' : 'cloud',
    authRequired: provider?.authRequired === true,
    configured: provider?.configured === true,
    githubConnected: provider?.githubConnected === true,
    cloudReady: provider?.cloudReady === true,
    githubCheckError: provider?.githubCheckError,
  };
}

export async function getGBrainStatus(workspaceTabId?: string): Promise<OpenBrainStatusResponse> {
  void workspaceTabId;
  const provider = await getOpenBrainProviderStatus();
  return {
    success: true,
    status: {
      provider: provider.provider,
      configured: provider.configured === true,
      authRequired: provider.authRequired === true,
      githubConnected: provider.githubConnected === true,
      cloudReady: provider.cloudReady === true,
      githubCheckError: provider.githubCheckError,
    },
  };
}
