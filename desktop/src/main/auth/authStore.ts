import * as fs from 'fs/promises';
import * as path from 'path';
import { normalizeAuthEmail } from './email';
import { writeJsonFileAtomic } from '../shared/jsonFile';

/**
 * Auth configuration stored in ~/.openbrain/configs/user/auth.json
 */
export type AuthConfig = {
  version: 2;
  baseUrl: string;
  gateway: string;
  aiGateway: string;
  token: string;
  uid: string;
  email?: string;
  deploymentID: string;
  orgID: string;
  orgSlug?: string;
  orgName?: string;
  identityID: string;
  connectionID: string;
  authMethod: string;
  assurance?: string;
  authTime: string;
  expiresAt: string;
  updatedAt: number;
};

export type CreateAuthConfigInput = Omit<AuthConfig, 'version' | 'updatedAt' | 'baseUrl' | 'gateway' | 'aiGateway'> & {
  baseUrl?: string;
  gateway?: string;
  aiGateway?: string;
};

const DEFAULT_BASE_URL = 'https://openbrain.chat';
const DEFAULT_GATEWAY = 'https://api.op-agent.com';
const DEFAULT_AI_GATEWAY = DEFAULT_GATEWAY;
const DESKTOP_AUTH_CALLBACK_URL = 'openbrain://auth/callback';

export { DEFAULT_AI_GATEWAY, DEFAULT_GATEWAY };

function normalizeBaseUrl(raw?: string): string {
  const next = (raw || DEFAULT_BASE_URL).trim() || DEFAULT_BASE_URL;
  try {
    const u = new URL(next);
    u.pathname = u.pathname.replace(/\/+$/, '');
    u.search = '';
    u.hash = '';
    return u.toString().replace(/\/$/, '');
  } catch {
    return next.replace(/\/$/, '');
  }
}

function normalizeGateway(raw?: string): string {
  const next = (raw || DEFAULT_GATEWAY).trim() || DEFAULT_GATEWAY;
  try {
    const u = new URL(next);
    u.pathname = u.pathname.replace(/\/+$/, '');
    u.search = '';
    u.hash = '';
    return u.toString().replace(/\/$/, '');
  } catch {
    return next.replace(/\/$/, '');
  }
}

function defaultAIGateway(baseUrl?: string): string {
  void baseUrl;
  return DEFAULT_AI_GATEWAY;
}

function normalizeAIGateway(raw?: string, baseUrl?: string): string {
  const next = (raw || '').trim();
  if (next) {
    try {
      const u = new URL(next);
      u.pathname = u.pathname.replace(/\/+$/, '');
      u.search = '';
      u.hash = '';
      return u.toString().replace(/\/$/, '');
    } catch {
      return next.replace(/\/$/, '');
    }
  }
  return defaultAIGateway(baseUrl);
}

export function normalizeActiveOrgID(raw?: string | null): string | undefined {
  const value = (raw || '').trim().toLowerCase();
  if (!value) {
    return undefined;
  }
  return /^[a-z0-9](?:[a-z0-9-]{0,126}[a-z0-9])?$/.test(value) ? value : undefined;
}

function normalizeDisplayValue(raw?: string | null): string | undefined {
  const value = (raw || '').trim();
  return value || undefined;
}

function normalizeRequiredID(raw?: string | null): string | undefined {
  const value = (raw || '').trim();
  if (!value || value.length > 128 || !/^[A-Za-z0-9][A-Za-z0-9._:-]*$/.test(value)) {
    return undefined;
  }
  return value;
}

function normalizeTimestamp(raw?: string | null): string | undefined {
  const value = (raw || '').trim();
  if (!value || !Number.isFinite(Date.parse(value))) {
    return undefined;
  }
  return value;
}

/**
 * Get the auth config directory path.
 */
export function getAuthConfigDir(homeDir: string): string {
  return path.join(homeDir, '.openbrain', 'configs', 'user');
}

/**
 * Get the auth config file path.
 */
export function getAuthConfigPath(homeDir: string): string {
  return path.join(getAuthConfigDir(homeDir), 'auth.json');
}

/**
 * Parse auth callback URL params to extract token, uid, email.
 * Supported URL formats:
 *   openbrain://auth/callback#token=...&uid=...&email=...
 *   openbrain://auth/callback?token=...&uid=...&email=...
 */
export function parseAuthCallbackUrl(url: string): {
  token: string;
  uid: string;
  email?: string;
  baseUrl?: string;
  gateway?: string;
  aiGateway?: string;
  deploymentID: string;
  orgID: string;
  identityID: string;
  connectionID: string;
  authMethod: string;
  assurance?: string;
  authTime: string;
  expiresAt: string;
} | null {
  try {
    const parsedUrl = new URL(url);
    if (
      parsedUrl.protocol !== 'openbrain:' ||
      parsedUrl.hostname !== 'auth' ||
      parsedUrl.pathname !== '/callback'
    ) {
      return null;
    }

    let params: URLSearchParams | null = null;

    const hashIndex = url.indexOf('#');
    if (hashIndex !== -1) {
      params = new URLSearchParams(url.slice(hashIndex + 1));
    }

    if (!params || !params.get('token') || !params.get('uid')) {
      params = parsedUrl.searchParams;
    }

    const token = params.get('token');
    const uid = normalizeRequiredID(params.get('uid'));
    const deploymentID = normalizeRequiredID(params.get('deploymentId'));
    const orgID = normalizeActiveOrgID(params.get('orgId'));
    const identityID = normalizeRequiredID(params.get('identityId'));
    const connectionID = normalizeRequiredID(params.get('connectionId'));
    const authMethod = normalizeDisplayValue(params.get('authMethod'));
    const authTime = normalizeTimestamp(params.get('authTime'));
    const expiresAt = normalizeTimestamp(params.get('expiresAt'));

    if (
      !token ||
      !uid ||
      !deploymentID ||
      !orgID ||
      !identityID ||
      !connectionID ||
      !authMethod ||
      !authTime ||
      !expiresAt
    ) {
      return null;
    }

    return {
      token,
      uid,
      email: normalizeAuthEmail(params.get('email')),
      baseUrl: params.get('baseUrl') || undefined,
      gateway: params.get('gateway') || undefined,
      aiGateway: params.get('aiGateway') || undefined,
      deploymentID,
      orgID,
      identityID,
      connectionID,
      authMethod,
      assurance: normalizeDisplayValue(params.get('assurance')),
      authTime,
      expiresAt,
    };
  } catch {
    return null;
  }
}

/**
 * Load auth config from disk.
 */
export async function loadAuthConfig(homeDir: string): Promise<AuthConfig | null> {
  const configPath = getAuthConfigPath(homeDir);
  try {
    const data = await fs.readFile(configPath, 'utf8');
    const parsed = JSON.parse(data) as Partial<AuthConfig>;
    const deploymentID = normalizeRequiredID(parsed?.deploymentID);
    const orgID = normalizeActiveOrgID(parsed?.orgID);
    const uid = normalizeRequiredID(parsed?.uid);
    const identityID = normalizeRequiredID(parsed?.identityID);
    const connectionID = normalizeRequiredID(parsed?.connectionID);
    const authMethod = normalizeDisplayValue(parsed?.authMethod);
    const authTime = normalizeTimestamp(parsed?.authTime);
    const expiresAt = normalizeTimestamp(parsed?.expiresAt);
    if (
      !parsed ||
      parsed.version !== 2 ||
      !parsed.token ||
      !uid ||
      !deploymentID ||
      !orgID ||
      !identityID ||
      !connectionID ||
      !authMethod ||
      !authTime ||
      !expiresAt
    ) {
      return null;
    }
    const baseUrl = normalizeBaseUrl(parsed.baseUrl);
    return {
      version: 2,
      baseUrl,
      gateway: normalizeGateway(parsed.gateway),
      aiGateway: normalizeAIGateway(parsed.aiGateway, baseUrl),
      token: parsed.token,
      uid,
      email: normalizeAuthEmail(parsed.email),
      deploymentID,
      orgID,
      orgSlug: normalizeDisplayValue(parsed.orgSlug)?.toLowerCase(),
      orgName: normalizeDisplayValue(parsed.orgName),
      identityID,
      connectionID,
      authMethod,
      assurance: normalizeDisplayValue(parsed.assurance),
      authTime,
      expiresAt,
      updatedAt: parsed.updatedAt || 0,
    };
  } catch {
    return null;
  }
}

/**
 * Save auth config to disk.
 */
export async function saveAuthConfig(homeDir: string, config: AuthConfig): Promise<void> {
  if (config.version !== 2) {
    throw new Error('auth config version 2 is required');
  }
  const configDir = getAuthConfigDir(homeDir);
  const configPath = getAuthConfigPath(homeDir);

  const baseUrl = normalizeBaseUrl(config.baseUrl);
  const uid = normalizeRequiredID(config.uid);
  const deploymentID = normalizeRequiredID(config.deploymentID);
  const orgID = normalizeActiveOrgID(config.orgID);
  const identityID = normalizeRequiredID(config.identityID);
  const connectionID = normalizeRequiredID(config.connectionID);
  const authMethod = normalizeDisplayValue(config.authMethod);
  const authTime = normalizeTimestamp(config.authTime);
  const expiresAt = normalizeTimestamp(config.expiresAt);
  if (
    !config.token ||
    !uid ||
    !deploymentID ||
    !orgID ||
    !identityID ||
    !connectionID ||
    !authMethod ||
    !authTime ||
    !expiresAt
  ) {
    throw new Error('tenant-bound auth config is incomplete');
  }
  const normalized: AuthConfig = {
    ...config,
    version: 2,
    baseUrl,
    gateway: normalizeGateway(config.gateway),
    aiGateway: normalizeAIGateway(config.aiGateway, baseUrl),
    uid,
    email: normalizeAuthEmail(config.email),
    deploymentID,
    orgID,
    orgSlug: normalizeDisplayValue(config.orgSlug)?.toLowerCase(),
    orgName: normalizeDisplayValue(config.orgName),
    identityID,
    connectionID,
    authMethod,
    assurance: normalizeDisplayValue(config.assurance),
    authTime,
    expiresAt,
  };

  await fs.mkdir(configDir, { recursive: true });
  await writeJsonFileAtomic(configPath, normalized);
}

/**
 * Clear auth config (delete the file).
 */
export async function clearAuthConfig(homeDir: string): Promise<void> {
  const configPath = getAuthConfigPath(homeDir);
  try {
    await fs.unlink(configPath);
  } catch {
    // Ignore if file doesn't exist
  }
}

/**
 * Create auth config from parsed callback params.
 */
export function createAuthConfig(input: CreateAuthConfigInput): AuthConfig {
  const normalizedBaseUrl = normalizeBaseUrl(input.baseUrl);
  return {
    ...input,
    version: 2,
    baseUrl: normalizedBaseUrl,
    gateway: normalizeGateway(input.gateway),
    aiGateway: normalizeAIGateway(input.aiGateway, normalizedBaseUrl),
    updatedAt: Date.now(),
  };
}

/**
 * Get the login URL for the auth base.
 */
export function getLoginUrl(baseUrl?: string): string {
  const rawBase = (baseUrl || DEFAULT_BASE_URL).trim();
  let origin = rawBase.replace(/\/$/, '');
  try {
    origin = new URL(origin).origin;
  } catch {
    // keep as-is
  }
  return `${origin}/login?redirectTo=${encodeURIComponent(DESKTOP_AUTH_CALLBACK_URL)}`;
}

/**
 * Get the login URL with a custom redirectTo target.
 * Used for dev-mode localhost callback where custom protocols don't work on macOS.
 */
export function getLoginUrlWithRedirect(baseUrl: string | undefined, redirectTo: string): string {
  const rawBase = (baseUrl || DEFAULT_BASE_URL).trim();
  let origin = rawBase.replace(/\/$/, '');
  try {
    origin = new URL(origin).origin;
  } catch {
    // keep as-is
  }
  return `${origin}/login?redirectTo=${encodeURIComponent(redirectTo)}`;
}
