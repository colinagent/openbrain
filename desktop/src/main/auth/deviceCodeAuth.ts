import { shell } from 'electron';
import { authFetch, readableNetworkError } from './netFetch';

// Gateway base used for auth/user APIs.
const DEFAULT_GATEWAY = 'https://api.op-agent.com';
const POLL_INTERVAL_MS = 5000;
const MAX_POLL_ATTEMPTS = 180; // 15 minutes
const MAX_CONSECUTIVE_NETWORK_ERRORS = 3;

export type DeviceCodeResponse = {
  device_code: string;
  user_code: string;
  verification_uri: string;
  expires_in: number;
  interval: number;
  target_kind: 'personal' | 'organization';
  org_id?: string;
  org_slug?: string;
};

export type DeviceTokenResponse = {
  token: string;
  uid: string;
  email?: string;
  deploymentID: string;
  orgID: string;
  identityID: string;
  connectionID: string;
  authMethod: string;
  assurance?: string;
  authTime: string;
  expiresAt: string;
  baseUrl?: string;
  gateway?: string;
  aiGateway?: string;
};

type TokenResponseRecord = Record<string, unknown>;

export type DeviceCodeSession = {
  deviceCode: string;
  userCode: string;
  verificationUri: string;
  expiresAt: number;
  pollInterval: number;
  targetKind: 'personal' | 'organization';
  targetOrgID?: string;
  targetOrgSlug?: string;
};

/**
 * Request a device code from the server.
 */
export async function requestDeviceCode(gateway?: string, orgSlug?: string): Promise<DeviceCodeSession> {
  const origin = (gateway || DEFAULT_GATEWAY).replace(/\/$/, '');
  const url = `${origin}/v1/identity/device/code`;

  let res: Response;
  try {
    res = await authFetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        client_id: 'openbrain',
        org_slug: (orgSlug || '').trim().toLowerCase(),
      }),
    });
  } catch (err) {
    throw new Error(`Failed to reach OpenBrain gateway for device login: ${readableNetworkError(err)}`);
  }

  if (!res.ok) {
    throw new Error(`Failed to request device code: ${res.status}`);
  }

  const data = (await res.json()) as Partial<DeviceCodeResponse>;
  const deviceCode = stringValue(data.device_code);
  const userCode = stringValue(data.user_code);
  const verificationUri = stringValue(data.verification_uri);
  const expiresIn = typeof data.expires_in === 'number' ? data.expires_in : 0;
  const interval = typeof data.interval === 'number' ? data.interval : 0;
  const targetKind = data.target_kind === 'organization' ? 'organization' : data.target_kind === 'personal' ? 'personal' : null;
  const targetOrgID = stringValue(data.org_id);
  const targetOrgSlug = stringValue(data.org_slug).toLowerCase();
  if (
    !deviceCode ||
    !userCode ||
    !verificationUri ||
    expiresIn <= 0 ||
    !targetKind ||
    (targetKind === 'organization' && (!targetOrgID || !targetOrgSlug)) ||
    (targetKind === 'personal' && (targetOrgID || targetOrgSlug))
  ) {
    throw new Error('OpenBrain gateway returned an invalid device code response.');
  }

  return {
    deviceCode,
    userCode,
    verificationUri,
    expiresAt: Date.now() + expiresIn * 1000,
    pollInterval: Math.max(interval * 1000, POLL_INTERVAL_MS),
    targetKind,
    targetOrgID: targetOrgID || undefined,
    targetOrgSlug: targetOrgSlug || undefined,
  };
}

export function normalizeDeviceTokenResponse(payload: unknown): DeviceTokenResponse {
  if (!payload || typeof payload !== 'object') {
    throw new Error('OpenBrain gateway returned an invalid device token response.');
  }

  const record = payload as TokenResponseRecord;
  const user = objectValue(record.user);
  const token = stringValue(record.token) || stringValue(record.access_token) || stringValue(record.accessToken);
  const uid =
    stringValue(record.uid) ||
    stringValue(record.user_id) ||
    stringValue(record.userId) ||
    stringValue(record.sub) ||
    stringValue(user?.uid) ||
    stringValue(user?.id);
  const deploymentID = stringValue(record.deploymentId) || stringValue(record.deployment_id);
  const orgID = stringValue(record.orgId) || stringValue(record.org_id);
  const identityID = stringValue(record.identityId) || stringValue(record.identity_id);
  const connectionID = stringValue(record.connectionId) || stringValue(record.connection_id);
  const authMethod = stringValue(record.authMethod) || stringValue(record.auth_method);
  const assurance = stringValue(record.assurance);
  const authTime = stringValue(record.authTime) || stringValue(record.auth_time);
  const expiresAt = stringValue(record.expiresAt) || stringValue(record.expires_at);
  if (
    !token ||
    !uid ||
    !deploymentID ||
    !orgID ||
    !identityID ||
    !connectionID ||
    !authMethod ||
    !authTime ||
    !expiresAt ||
    !Number.isFinite(Date.parse(authTime)) ||
    !Number.isFinite(Date.parse(expiresAt))
  ) {
    const missing = [
      token ? '' : 'token',
      uid ? '' : 'uid',
      deploymentID ? '' : 'deploymentId',
      orgID ? '' : 'orgId',
      identityID ? '' : 'identityId',
      connectionID ? '' : 'connectionId',
      authMethod ? '' : 'authMethod',
      authTime ? '' : 'authTime',
      expiresAt ? '' : 'expiresAt',
    ].filter(Boolean).join(' and ');
    throw new Error(
      missing
        ? `OpenBrain gateway completed device authorization but returned no ${missing}.`
        : 'OpenBrain gateway returned invalid tenant session timestamps.',
    );
  }

  return {
    token,
    uid,
    email: stringValue(record.email) || stringValue(user?.email) || undefined,
    deploymentID,
    orgID,
    identityID,
    connectionID,
    authMethod,
    assurance: assurance || undefined,
    authTime,
    expiresAt,
    baseUrl: stringValue(record.baseUrl) || stringValue(record.base_url) || undefined,
    gateway: stringValue(record.gateway) || stringValue(record.gatewayUrl) || stringValue(record.gateway_url) || undefined,
    aiGateway: stringValue(record.aiGateway) || stringValue(record.ai_gateway) || undefined,
  };
}

export function deviceVerificationLoginUri(verificationUri: string): string {
  const raw = verificationUri.trim();
  if (!raw) {
    return raw;
  }
  try {
    const url = new URL(raw);
    if (url.protocol !== 'http:' && url.protocol !== 'https:') {
      return raw;
    }
    const redirectTo = `${url.pathname || '/device'}${url.search || ''}${url.hash || ''}`;
    const loginUrl = new URL('/login', url.origin);
    loginUrl.searchParams.set('redirectTo', redirectTo || '/device');
    return loginUrl.toString();
  } catch {
    return raw;
  }
}

/**
 * Poll for device token until authorized or timeout.
 */
export async function pollDeviceToken(
  session: DeviceCodeSession,
  gateway?: string,
  onPending?: () => void
): Promise<DeviceTokenResponse> {
  const origin = (gateway || DEFAULT_GATEWAY).replace(/\/$/, '');
  const url = `${origin}/v1/identity/device/token`;
  let networkErrors = 0;
  let pollInterval = session.pollInterval;

  for (let i = 0; i < MAX_POLL_ATTEMPTS; i++) {
    if (Date.now() > session.expiresAt) {
      throw new Error('Device code expired');
    }

    let res: Response;
    try {
      res = await authFetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          device_code: session.deviceCode,
          grant_type: 'device_code',
        }),
      });
      networkErrors = 0;
    } catch (err) {
      networkErrors += 1;
      if (networkErrors >= MAX_CONSECUTIVE_NETWORK_ERRORS) {
        throw new Error(`Device login token polling failed: ${readableNetworkError(err)}`);
      }
      onPending?.();
      await sleep(session.pollInterval);
      continue;
    }

    if (res.ok) {
      const data = await res.json().catch(() => null);
      return normalizeDeviceTokenResponse(data);
    }

    const error = await readDeviceTokenError(res);

    if (error.error === 'authorization_pending') {
      onPending?.();
      await sleep(pollInterval);
      continue;
    }

    if (error.error === 'slow_down') {
      pollInterval += 5000;
      onPending?.();
      await sleep(pollInterval);
      continue;
    }

    if (error.error === 'expired_token') {
      throw new Error('Device code expired');
    }

    if (error.error === 'access_denied') {
      throw new Error('Device authorization was denied.');
    }

    throw new Error(`Device login token request failed (${res.status}): ${error.message || error.error || 'unknown error'}`);
  }

  throw new Error('Device code polling timeout');
}

export async function exchangeTenantSession(
  gateway: string,
  token: string,
  orgID: string,
): Promise<DeviceTokenResponse> {
  const origin = (gateway || DEFAULT_GATEWAY).replace(/\/$/, '');
  let response: Response;
  try {
    response = await authFetch(`${origin}/v1/identity/session/exchanges`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
        Accept: 'application/json',
      },
      body: JSON.stringify({ orgId: orgID }),
    });
  } catch (err) {
    throw new Error(`Failed to reach OpenBrain gateway for organization switch: ${readableNetworkError(err)}`);
  }
  if (!response.ok) {
    const failure = await readDeviceTokenError(response);
    const message = failure.message || failure.error || `HTTP ${response.status}`;
    if (response.status === 401) {
      const error = new Error(message);
      error.name = 'AuthInvalidError';
      throw error;
    }
    if (failure.error === 'reauth_required') {
      throw new Error('This organization requires a new SSO sign-in.');
    }
    throw new Error(message);
  }
  return normalizeDeviceTokenResponse(await response.json().catch(() => null));
}

/**
 * Start the device code login flow.
 * Returns when user authorizes or throws on timeout/error.
 */
export async function startDeviceCodeLogin(
  gateway: string | undefined,
  orgSlug: string | undefined,
  callbacks: {
    onCode: (session: DeviceCodeSession) => void;
    onPending?: () => void;
  }
): Promise<DeviceTokenResponse> {
  const session = await requestDeviceCode(gateway, orgSlug);
  callbacks.onCode(session);

  // Open the login page first so stale browser sessions from the legacy
  // OpAgent app cannot silently authorize the wrong account.
  shell.openExternal(deviceVerificationLoginUri(session.verificationUri));

  // Poll for token
  return pollDeviceToken(session, gateway, callbacks.onPending);
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function objectValue(value: unknown): TokenResponseRecord | null {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as TokenResponseRecord
    : null;
}

async function readDeviceTokenError(response: Response): Promise<{ error: string; message?: string }> {
  try {
    const data = await response.json() as unknown;
    if (data && typeof data === 'object') {
      const record = data as TokenResponseRecord;
      const error = stringValue(record.error) || stringValue(record.code) || 'unknown';
      const message =
        stringValue(record.error_description) ||
        stringValue(record.message) ||
        stringValue(record.detail) ||
        error;
      return { error, message };
    }
  } catch {
    // fall through to HTTP status text
  }
  return {
    error: 'unknown',
    message: response.statusText || `HTTP ${response.status}`,
  };
}
