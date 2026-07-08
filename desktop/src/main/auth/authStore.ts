import * as fs from 'fs/promises';
import * as path from 'path';
import { normalizeAuthEmail } from './email';
import { writeJsonFileAtomic } from '../shared/jsonFile';

/**
 * Auth configuration stored in ~/.openbrain/configs/user/auth.json
 */
export type AuthConfig = {
  version: number;
  baseUrl: string;
  gateway: string;
  aiGateway: string;
  defaultOrgID?: string;
  defaultOrgName?: string;
  token: string;
  uid: string;
  email?: string;
  activeOrgID?: string;
  activeOrgName?: string;
  updatedAt: number;
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
  return /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(value) && !value.startsWith('org-')
    ? value
    : undefined;
}

function normalizeActiveOrgName(raw?: string | null): string | undefined {
  const value = (raw || '').trim();
  return value || undefined;
}

function normalizeDefaultOrgID(raw?: string | null): string | undefined {
  return normalizeActiveOrgID(raw);
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
  defaultOrgID?: string;
  defaultOrgName?: string;
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
    const uid = params.get('uid');

    if (!token || !uid) {
      return null;
    }

    return {
      token,
      uid,
      email: normalizeAuthEmail(params.get('email')),
      baseUrl: params.get('baseUrl') || undefined,
      gateway: params.get('gateway') || undefined,
      aiGateway: params.get('aiGateway') || undefined,
      defaultOrgID: normalizeDefaultOrgID(params.get('defaultOrgID')),
      defaultOrgName: normalizeActiveOrgName(params.get('defaultOrgName')),
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
    if (!parsed || typeof parsed.version !== 'number' || !parsed.token || !parsed.uid) {
      return null;
    }
    const baseUrl = normalizeBaseUrl(parsed.baseUrl);
    const normalized: AuthConfig = {
      version: parsed.version,
      baseUrl,
      gateway: normalizeGateway(parsed.gateway),
      aiGateway: normalizeAIGateway(parsed.aiGateway, baseUrl),
      defaultOrgID: normalizeDefaultOrgID(parsed.defaultOrgID),
      defaultOrgName: normalizeDefaultOrgID(parsed.defaultOrgID) ? normalizeActiveOrgName(parsed.defaultOrgName) : undefined,
      token: parsed.token,
      uid: parsed.uid,
      email: normalizeAuthEmail(parsed.email),
      activeOrgID: normalizeActiveOrgID(parsed.activeOrgID),
      activeOrgName: normalizeActiveOrgID(parsed.activeOrgID) ? normalizeActiveOrgName(parsed.activeOrgName) : undefined,
      updatedAt: parsed.updatedAt || 0,
    };
    // Best-effort migration: persist newly added fields (e.g. gateway) so that
    // other processes (openbrain-server/openbrain) can scan auth.json directly.
    if (
      !parsed.gateway ||
      normalizeGateway(parsed.gateway) !== normalized.gateway ||
      !parsed.aiGateway ||
      normalizeAIGateway(parsed.aiGateway, baseUrl) !== normalized.aiGateway ||
      parsed.defaultOrgID !== normalized.defaultOrgID ||
      parsed.defaultOrgName !== normalized.defaultOrgName ||
      parsed.email !== normalized.email ||
      parsed.activeOrgID !== normalized.activeOrgID ||
      parsed.activeOrgName !== normalized.activeOrgName
    ) {
      try {
        await saveAuthConfig(homeDir, normalized);
      } catch {
        // ignore write-back failures
      }
    }
    return normalized;
  } catch {
    return null;
  }
}

/**
 * Save auth config to disk.
 */
export async function saveAuthConfig(homeDir: string, config: AuthConfig): Promise<void> {
  if (!config.uid) {
    throw new Error('uid is required');
  }
  const configDir = getAuthConfigDir(homeDir);
  const configPath = getAuthConfigPath(homeDir);

  const baseUrl = normalizeBaseUrl(config.baseUrl);
  const normalized: AuthConfig = {
    ...config,
    baseUrl,
    gateway: normalizeGateway(config.gateway),
    aiGateway: normalizeAIGateway(config.aiGateway, baseUrl),
    defaultOrgID: normalizeDefaultOrgID(config.defaultOrgID),
    defaultOrgName: normalizeDefaultOrgID(config.defaultOrgID) ? normalizeActiveOrgName(config.defaultOrgName) : undefined,
    email: normalizeAuthEmail(config.email),
    activeOrgID: normalizeActiveOrgID(config.activeOrgID),
    activeOrgName: normalizeActiveOrgID(config.activeOrgID) ? normalizeActiveOrgName(config.activeOrgName) : undefined,
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
export function createAuthConfig(
  token: string,
  uid: string,
  email?: string,
  baseUrl?: string,
  gateway?: string,
  aiGateway?: string,
  defaultOrgID?: string,
  defaultOrgName?: string
): AuthConfig {
  if (!uid) {
    throw new Error('uid is required');
  }
  const normalizedBaseUrl = normalizeBaseUrl(baseUrl);
  return {
    version: 1,
    baseUrl: normalizedBaseUrl,
    gateway: normalizeGateway(gateway),
    aiGateway: normalizeAIGateway(aiGateway, normalizedBaseUrl),
    defaultOrgID: normalizeDefaultOrgID(defaultOrgID),
    defaultOrgName: normalizeDefaultOrgID(defaultOrgID) ? normalizeActiveOrgName(defaultOrgName) : undefined,
    token,
    uid,
    email: normalizeAuthEmail(email),
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
