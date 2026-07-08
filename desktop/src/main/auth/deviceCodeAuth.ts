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
};

export type DeviceTokenResponse = {
  token: string;
  uid: string;
  email?: string;
  baseUrl?: string;
  gateway?: string;
  aiGateway?: string;
  defaultOrg?: {
    id?: string;
    name?: string;
  };
};

type TokenResponseRecord = Record<string, unknown>;

export type DeviceCodeSession = {
  deviceCode: string;
  userCode: string;
  verificationUri: string;
  expiresAt: number;
  pollInterval: number;
};

/**
 * Request a device code from the server.
 */
export async function requestDeviceCode(gateway?: string): Promise<DeviceCodeSession> {
  const origin = (gateway || DEFAULT_GATEWAY).replace(/\/$/, '');
  const url = `${origin}/v1/identity/device/code`;

  let res: Response;
  try {
    res = await authFetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ client_id: 'openbrain' }),
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
  if (
    !deviceCode ||
    !userCode ||
    !verificationUri ||
    expiresIn <= 0
  ) {
    throw new Error('OpenBrain gateway returned an invalid device code response.');
  }

  return {
    deviceCode,
    userCode,
    verificationUri,
    expiresAt: Date.now() + expiresIn * 1000,
    pollInterval: Math.max(interval * 1000, POLL_INTERVAL_MS),
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
  if (!token || !uid) {
    const missing = [
      token ? '' : 'token',
      uid ? '' : 'uid',
    ].filter(Boolean).join(' and ');
    throw new Error(`OpenBrain gateway completed device authorization but returned no ${missing}.`);
  }

  const defaultOrg = objectValue(record.defaultOrg) || objectValue(record.default_org);
  const defaultOrgID = stringValue(defaultOrg?.id);
  const defaultOrgName = stringValue(defaultOrg?.name);
  return {
    token,
    uid,
    email: stringValue(record.email) || stringValue(user?.email) || undefined,
    baseUrl: stringValue(record.baseUrl) || stringValue(record.base_url) || undefined,
    gateway: stringValue(record.gateway) || stringValue(record.gatewayUrl) || stringValue(record.gateway_url) || undefined,
    aiGateway: stringValue(record.aiGateway) || stringValue(record.ai_gateway) || undefined,
    defaultOrg: defaultOrgID || defaultOrgName
      ? {
          id: defaultOrgID || undefined,
          name: defaultOrgName || undefined,
        }
      : undefined,
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

/**
 * Start the device code login flow.
 * Returns when user authorizes or throws on timeout/error.
 */
export async function startDeviceCodeLogin(
  gateway: string | undefined,
  callbacks: {
    onCode: (session: DeviceCodeSession) => void;
    onPending?: () => void;
  }
): Promise<DeviceTokenResponse> {
  const session = await requestDeviceCode(gateway);
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
