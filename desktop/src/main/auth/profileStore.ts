import * as fs from 'fs/promises';
import * as path from 'path';
import { normalizeAuthEmail } from './email';
import { AuthInvalidError, isAuthInvalidResponse, readErrorMessage } from './authErrors';
import { authFetch } from './netFetch';
import { writeJsonFileAtomic } from '../shared/jsonFile';

/**
 * User profile stored in ~/.openbrain/configs/user/profile.json
 */
export type UserProfile = {
  version: number;
  uid: string;
  name: string;
  username: string;
  email?: string;
  avatar?: string;
  provider?: string; // google | basic
  updatedAt: number;
};

const CANONICAL_AVATAR_PATH_PREFIX = '/v1/user/avatar/';

export function createFallbackProfile(uid: string, email?: string): UserProfile {
  return {
    version: 1,
    uid,
    name: '',
    username: '',
    email: normalizeAuthEmail(email),
    updatedAt: Date.now(),
  };
}

/**
 * Response from GET /v1/me
 */
type MeResponse = {
  uid: string;
  name?: string;
  username: string;
  provider?: string;
  email?: string;
  avatar?: string;
  avatarUrl?: string;
  avatarUri?: string;
};

function normalizeGatewayBaseUrl(gateway: string): string {
  return (gateway || '').trim().replace(/\/+$/, '');
}

function canonicalAvatarFilename(value: string | undefined): string {
  const raw = (value || '').trim();
  if (!raw) return '';

  let pathOnly = raw;
  if (/^https?:\/\//i.test(raw)) {
    try {
      const parsed = new URL(raw);
      if (parsed.search || parsed.hash) {
        return '';
      }
      pathOnly = parsed.pathname;
    } catch {
      return '';
    }
  }

  if (!pathOnly.startsWith(CANONICAL_AVATAR_PATH_PREFIX)) {
    return '';
  }
  const filename = pathOnly.slice(CANONICAL_AVATAR_PATH_PREFIX.length);
  if (
    !filename ||
    filename.includes('/') ||
    filename.includes('\\') ||
    filename.includes('..') ||
    filename.includes('?') ||
    filename.includes('#')
  ) {
    return '';
  }
  return filename;
}

function normalizeStoredAvatar(value: string | undefined): string | undefined {
  const raw = (value || '').trim();
  if (!raw || !/^https?:\/\//i.test(raw)) {
    return undefined;
  }
  return canonicalAvatarFilename(raw) ? raw : undefined;
}

export function resolveProfileAvatarUrl(gateway: string, value: string | undefined): string | undefined {
  const filename = canonicalAvatarFilename(value);
  if (!filename) {
    return undefined;
  }
  const base = normalizeGatewayBaseUrl(gateway);
  if (!base) {
    return undefined;
  }
  return `${base}${CANONICAL_AVATAR_PATH_PREFIX}${encodeURIComponent(filename)}`;
}

function readMeAvatar(data: MeResponse): string | undefined {
  return data.avatarUrl || data.avatar || data.avatarUri;
}

function toUserProfile(gateway: string, data: MeResponse): UserProfile {
  return {
    version: 1,
    uid: data.uid,
    name: data.name || data.username || '',
    username: data.username || '',
    email: normalizeAuthEmail(data.email),
    avatar: resolveProfileAvatarUrl(gateway, readMeAvatar(data)),
    provider: data.provider,
    updatedAt: Date.now(),
  };
}

/**
 * Get the profile config directory path.
 */
export function getProfileDir(homeDir: string): string {
  return path.join(homeDir, '.openbrain', 'configs', 'user');
}

/**
 * Get the profile config file path.
 */
export function getProfilePath(homeDir: string): string {
  return path.join(getProfileDir(homeDir), 'profile.json');
}

/**
 * Load profile from disk.
 */
export async function loadProfile(homeDir: string): Promise<UserProfile | null> {
  const profilePath = getProfilePath(homeDir);
  try {
    const data = await fs.readFile(profilePath, 'utf8');
    const parsed = JSON.parse(data) as UserProfile;
    if (!parsed || typeof parsed.version !== 'number' || !parsed.uid) {
      return null;
    }
    const normalized: UserProfile = {
      version: parsed.version,
      uid: parsed.uid,
      name: parsed.name || parsed.username || '',
      username: parsed.username || '',
      email: normalizeAuthEmail(parsed.email),
      avatar: normalizeStoredAvatar(parsed.avatar),
      provider: parsed.provider,
      updatedAt: parsed.updatedAt || 0,
    };
    if (parsed.email !== normalized.email || parsed.avatar !== normalized.avatar) {
      try {
        await saveProfile(homeDir, normalized);
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
 * Save profile to disk.
 */
export async function saveProfile(homeDir: string, profile: UserProfile): Promise<void> {
  if (!profile.uid) {
    throw new Error('uid is required');
  }
  const profileDir = getProfileDir(homeDir);
  const profilePath = getProfilePath(homeDir);
  const normalized: UserProfile = {
    version: profile.version,
    uid: profile.uid,
    name: profile.name || profile.username || '',
    username: profile.username || '',
    email: normalizeAuthEmail(profile.email),
    avatar: normalizeStoredAvatar(profile.avatar),
    provider: profile.provider,
    updatedAt: profile.updatedAt || Date.now(),
  };

  await fs.mkdir(profileDir, { recursive: true });
  await writeJsonFileAtomic(profilePath, normalized);
}

/**
 * Clear profile (delete the file).
 */
export async function clearProfile(homeDir: string): Promise<void> {
  const profilePath = getProfilePath(homeDir);
  try {
    await fs.unlink(profilePath);
  } catch {
    // Ignore if file doesn't exist
  }
}

/**
 * Fetch profile from backend /v1/user/me endpoint.
 */
export async function fetchProfile(gateway: string, token: string): Promise<UserProfile | null> {
  try {
    const url = `${gateway.replace(/\/$/, '')}/v1/user/me`;
    const response = await authFetch(url, {
      method: 'GET',
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
    });

    if (!response.ok) {
      const message = await readErrorMessage(response);
      if (isAuthInvalidResponse(response.status, message)) {
        throw new AuthInvalidError(message);
      }
      console.error('[profileStore] Failed to fetch profile:', response.status, message);
      return null;
    }

    const data = (await response.json()) as MeResponse;
    if (!data || !data.uid) {
      return null;
    }
    return toUserProfile(gateway, data);
  } catch (err) {
    if (err instanceof AuthInvalidError) {
      throw err;
    }
    console.error('[profileStore] Error fetching profile:', err);
    return null;
  }
}

/**
 * Update profile via backend /v1/user/profile endpoint.
 */
export async function updateProfile(
  gateway: string,
  token: string,
  params: { name: string; username: string }
): Promise<UserProfile | null> {
  try {
    const url = `${gateway.replace(/\/$/, '')}/v1/user/profile`;
    const response = await authFetch(url, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        name: params.name,
        username: params.username,
      }),
    });

    if (!response.ok) {
      const message = await readErrorMessage(response);
      if (isAuthInvalidResponse(response.status, message)) {
        throw new AuthInvalidError(message);
      }
      console.error('[profileStore] Failed to update profile:', response.status, message);
      return null;
    }

    const data = (await response.json()) as MeResponse;
    if (!data || !data.uid) {
      return null;
    }

    return toUserProfile(gateway, data);
  } catch (err) {
    if (err instanceof AuthInvalidError) {
      throw err;
    }
    console.error('[profileStore] Error updating profile:', err);
    return null;
  }
}

/**
 * Create default empty profile.
 */
export function createEmptyProfile(uid: string, email?: string): UserProfile {
  if (!uid) {
    throw new Error('uid is required');
  }
  return {
    version: 1,
    uid,
    name: '',
    username: '',
    email: normalizeAuthEmail(email),
    updatedAt: Date.now(),
  };
}
