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

export type OpenBrainPublicBrainEntry = {
	brainID: string;
	ownerUID: string;
  name: string;
  username: string;
  ownerInitial?: string;
  avatar?: string;
  activeSourceCount: number;
	followed: boolean;
	owned?: boolean;
	description?: string;
  accessMode?: 'public' | 'members_only';
  member?: boolean;
  offer?: { offerID: string; unitAmountU: string; currency: 'usd'; interval: 'month'; checkoutAvailable: boolean; includesAIUsage: false };
  membership?: { membershipID: string; status: string; currentPeriodEnd?: string; cancelAtPeriodEnd: boolean; includesAIUsage: false };
};

export type OpenBrainPublicBrainsResponse = {
  brains: OpenBrainPublicBrainEntry[];
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
    const errorPayload = payload as { code?: unknown; error?: unknown };
    const message = typeof errorPayload.error === 'string'
      ? errorPayload.error
      : (trimmed || `Request failed: ${res.status}`);
    const code = typeof errorPayload.code === 'string' ? errorPayload.code.trim() : '';
    throw new Error(code ? `${code}: ${message}` : message);
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

export async function followOpenBrainPublicBrain(
	ownerUID: string,
	workspaceTabId?: string,
): Promise<OpenBrainPublicBrainEntry> {
	return putJSON<OpenBrainPublicBrainEntry>(
		`/v1/openbrain/cloud/public-brains/${encodeURIComponent(ownerUID.trim())}/follow`,
    {},
    workspaceTabId,
  );
}

export async function unfollowOpenBrainPublicBrain(
  ownerUID: string,
  workspaceTabId?: string,
): Promise<OpenBrainMutationResponse> {
  return deleteJSON<OpenBrainMutationResponse>(
		`/v1/openbrain/cloud/public-brains/${encodeURIComponent(ownerUID.trim())}/follow`,
    workspaceTabId,
  );
}

export type OpenBrainPublicBrainConversation = {
	conversationId: string;
	brainId: string;
	executionMode: 'hosted' | 'runtime_byok';
	profileId: string;
	expiresAt: string;
};

export type OpenBrainPublicBrainFunding = {
	kind: 'free_daily' | 'ai_balance' | 'owner_preview';
	authorizedMaxDebitU?: string;
	actualDebitU?: string;
	retailCostU?: string;
	balanceU?: string;
	remaining?: number;
	limit?: number;
	resetsAt?: string;
};

export type OpenBrainPublicBrainQuote = {
	quoteId: string;
	conversationId: string;
	profileId: string;
	pricingVersion: string;
	currency: 'usd';
	maxAuthorizedDebitU: string;
	maxAuthorizedDebitMicrousd: number;
	expiresAt: string;
	funding: OpenBrainPublicBrainFunding;
};

export type OpenBrainPublicBrainTurnEvent = {
	type: 'accepted' | 'retrieving' | 'synthesizing' | 'complete' | 'error';
	turnId?: string;
	answer?: string;
	citations?: Array<{ citationId: string; title: string; excerpt?: string }>;
	funding?: OpenBrainPublicBrainFunding;
	code?: string;
	retryable?: boolean;
};

export type OpenBrainPublicBrainBYOKResult = {
	answer: string;
	citations: Array<{ citationId: string; title: string; excerpt?: string }>;
	funding: OpenBrainPublicBrainFunding;
	executionMode: 'runtime_byok';
	modelKey?: string;
	modelRan: boolean;
	billingResponsibility: 'external_provider';
	evidenceCompleted: boolean;
};

export type OpenBrainRuntimeModels = {
	models: Array<{ key: string; name: string; provider: string }>;
	defaultModelKey: string;
};

export async function listOpenBrainRuntimeModels(workspaceTabId?: string): Promise<OpenBrainRuntimeModels> {
	return getJSON<OpenBrainRuntimeModels>('/v1/openbrain/runtime/models', workspaceTabId);
}

export async function createOpenBrainPublicBrainConversation(
	brainID: string,
	executionMode: 'hosted' | 'runtime_byok' = 'hosted',
	workspaceTabId?: string,
): Promise<OpenBrainPublicBrainConversation> {
	return postJSON<OpenBrainPublicBrainConversation>(
		`/v1/openbrain/cloud/public-brains/${encodeURIComponent(brainID.trim())}/conversations`,
		{ executionMode },
		workspaceTabId,
	);
}

export async function runOpenBrainPublicBrainBYOKTurn(
	brainID: string,
	conversationID: string,
	input: {
		turnId: string;
		quoteId: string;
		question: string;
		maxAuthorizedDebitMicrousd: number;
		modelKey: string;
		history?: Array<{ role: 'user' | 'assistant'; text: string }>;
	},
	workspaceTabId?: string,
): Promise<OpenBrainPublicBrainBYOKResult> {
	return postJSON<OpenBrainPublicBrainBYOKResult>(
		`/v1/openbrain/cloud/public-brains/${encodeURIComponent(brainID.trim())}/conversations/${encodeURIComponent(conversationID.trim())}/byok-turns`,
		input,
		workspaceTabId,
	);
}

export async function quoteOpenBrainPublicBrainTurn(
	brainID: string,
	conversationID: string,
	question: string,
	workspaceTabId?: string,
): Promise<OpenBrainPublicBrainQuote> {
	return postJSON<OpenBrainPublicBrainQuote>(
		`/v1/openbrain/cloud/public-brains/${encodeURIComponent(brainID.trim())}/conversations/${encodeURIComponent(conversationID.trim())}/turn-quotes`,
		{ question },
		workspaceTabId,
	);
}

export async function runOpenBrainPublicBrainTurn(
	brainID: string,
	conversationID: string,
	input: { turnId: string; quoteId: string; question: string; maxAuthorizedDebitMicrousd: number },
	onEvent: (event: OpenBrainPublicBrainTurnEvent) => void,
	workspaceTabId?: string,
): Promise<void> {
	const response = await fetch(
		`${resolveOpenBrainBaseUrl(workspaceTabId)}/v1/openbrain/cloud/public-brains/${encodeURIComponent(brainID.trim())}/conversations/${encodeURIComponent(conversationID.trim())}/turns`,
		{ method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' }, body: JSON.stringify(input) },
	);
	if (!response.ok || !response.body) {
		await readJSONResponse(response);
		throw new Error('Public brain turn stream is unavailable.');
	}
	const reader = response.body.getReader();
	const decoder = new TextDecoder();
	let buffer = '';
	try {
		for (;;) {
			const { done, value } = await reader.read();
			buffer += decoder.decode(value || new Uint8Array(), { stream: !done }).replace(/\r\n/g, '\n');
			let boundary = buffer.indexOf('\n\n');
			while (boundary >= 0) {
				const block = buffer.slice(0, boundary);
				buffer = buffer.slice(boundary + 2);
				const data = block.split('\n').filter((line) => line.startsWith('data:')).map((line) => line.slice(5).trim()).join('\n');
				if (data) onEvent(JSON.parse(data) as OpenBrainPublicBrainTurnEvent);
				boundary = buffer.indexOf('\n\n');
			}
			if (done) break;
		}
	} finally {
		reader.releaseLock();
	}
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
