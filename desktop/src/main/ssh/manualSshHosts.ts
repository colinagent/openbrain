import * as fs from 'fs/promises';
import * as path from 'path';
import { randomUUID } from 'crypto';
import {
  deleteSshCredential,
  loadSshCredential,
  saveSshCredential,
  type SshSecretPayload,
} from './manualSshCredentials';
import type {
  ManualSshHostInput,
  ManualSshHostRecord,
  SshHost,
  SshHostWithSecrets,
} from './sshTypes';

type ManualHostsFile = {
  version: 1;
  hosts: ManualSshHostRecord[];
};

const HOSTS_FILE = 'ssh-hosts.json';

function settingsRoot(homeDir: string) {
  return path.join(homeDir, '.openbrain', 'configs', 'settings');
}

function hostsPath(homeDir: string) {
  return path.join(settingsRoot(homeDir), HOSTS_FILE);
}

function trimString(value: unknown): string | undefined {
  if (typeof value !== 'string') {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed || undefined;
}

export function normalizeSshPort(value: unknown): string | undefined {
  const raw = trimString(value);
  if (!raw) {
    return undefined;
  }
  if (!/^\d+$/.test(raw)) {
    throw new Error('SSH port must be a number');
  }
  const port = Number(raw);
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error('SSH port must be between 1 and 65535');
  }
  return String(port);
}

function normalizeManualHost(value: unknown): ManualSshHostRecord | null {
  if (!value || typeof value !== 'object') {
    return null;
  }
  const raw = value as Record<string, unknown>;
  const id = trimString(raw.id);
  const alias = trimString(raw.alias);
  const hostname = trimString(raw.hostname);
  const user = trimString(raw.user);
  const authMethod = raw.authMethod === 'keyFile' || raw.authMethod === 'password'
    ? raw.authMethod
    : undefined;
  if (!id || !alias || !hostname || !user || !authMethod) {
    return null;
  }
  let port: string | undefined;
  try {
    port = normalizeSshPort(raw.port);
  } catch {
    return null;
  }
  const identityFile = trimString(raw.identityFile);
  if (authMethod === 'keyFile' && !identityFile) {
    return null;
  }
  const credentialID = trimString(raw.credentialID);
  return {
    id,
    alias,
    hostname,
    user,
    port,
    identityFile: authMethod === 'keyFile' ? identityFile : undefined,
    source: 'manual',
    authMethod,
    credentialID,
    hasPassword: raw.hasPassword === true,
    hasPassphrase: raw.hasPassphrase === true,
    createdAt: typeof raw.createdAt === 'number' && Number.isFinite(raw.createdAt) ? raw.createdAt : Date.now(),
    updatedAt: typeof raw.updatedAt === 'number' && Number.isFinite(raw.updatedAt) ? raw.updatedAt : Date.now(),
  };
}

async function readJsonFile<T>(filePath: string, fallback: T): Promise<T> {
  try {
    return JSON.parse(await fs.readFile(filePath, 'utf8')) as T;
  } catch {
    return fallback;
  }
}

async function writeJsonFile(filePath: string, value: unknown): Promise<void> {
  await fs.mkdir(path.dirname(filePath), { recursive: true });
  const tmpPath = `${filePath}.${process.pid}.${Date.now()}.tmp`;
  await fs.writeFile(tmpPath, JSON.stringify(value, null, 2), 'utf8');
  await fs.rename(tmpPath, filePath);
}

async function loadHostsFile(homeDir: string): Promise<ManualHostsFile> {
  const parsed = await readJsonFile<ManualHostsFile>(hostsPath(homeDir), { version: 1, hosts: [] });
  const hosts = Array.isArray(parsed.hosts)
    ? parsed.hosts.map(normalizeManualHost).filter((host): host is ManualSshHostRecord => Boolean(host))
    : [];
  return { version: 1, hosts };
}

async function saveHostsFile(homeDir: string, file: ManualHostsFile): Promise<void> {
  await writeJsonFile(hostsPath(homeDir), { version: 1, hosts: file.hosts });
}

function buildHostRecord(
  input: ManualSshHostInput,
  existing: ManualSshHostRecord | undefined,
  credentialID: string | undefined,
  flags: { hasPassword: boolean; hasPassphrase: boolean },
): ManualSshHostRecord {
  const alias = trimString(input.alias);
  const hostname = trimString(input.hostname);
  const user = trimString(input.user);
  const authMethod = input.authMethod;
  if (!alias) {
    throw new Error('Alias is required');
  }
  if (!hostname) {
    throw new Error('Host is required');
  }
  if (!user) {
    throw new Error('User is required');
  }
  if (authMethod !== 'password' && authMethod !== 'keyFile') {
    throw new Error('Unsupported SSH authentication method');
  }
  const identityFile = trimString(input.identityFile);
  if (authMethod === 'keyFile' && !identityFile) {
    throw new Error('Private key file is required');
  }
  const now = Date.now();
  return {
    id: existing?.id || input.id || `ssh-host-${randomUUID()}`,
    alias,
    hostname,
    user,
    port: normalizeSshPort(input.port),
    identityFile: authMethod === 'keyFile' ? identityFile : undefined,
    source: 'manual',
    authMethod,
    credentialID,
    hasPassword: flags.hasPassword,
    hasPassphrase: flags.hasPassphrase,
    createdAt: existing?.createdAt || now,
    updatedAt: now,
  };
}

export async function listManualSshHosts(homeDir: string): Promise<SshHost[]> {
  const file = await loadHostsFile(homeDir);
  return file.hosts.map((host) => ({ ...host }));
}

export async function saveManualSshHost(homeDir: string, input: ManualSshHostInput): Promise<SshHost> {
  const file = await loadHostsFile(homeDir);
  const existing = input.id ? file.hosts.find((host) => host.id === input.id) : undefined;
  const credentialID = existing?.credentialID || `ssh-cred-${randomUUID()}`;
  const secret: SshSecretPayload = { version: 1 };
  let shouldWriteSecret = false;
  let shouldDeleteSecret = false;
  let hasPassword = false;
  let hasPassphrase = false;

  if (input.authMethod === 'password') {
    if (typeof input.password === 'string' && input.password.length > 0) {
      secret.password = input.password;
      shouldWriteSecret = true;
      hasPassword = true;
    } else if (existing?.authMethod === 'password' && existing.hasPassword && existing.credentialID) {
      hasPassword = true;
    } else {
      throw new Error('Password is required');
    }
  } else {
    if (typeof input.passphrase === 'string' && input.passphrase.length > 0) {
      secret.passphrase = input.passphrase;
      shouldWriteSecret = true;
      hasPassphrase = true;
    } else if (input.passphrase === undefined && existing?.authMethod === 'keyFile' && existing.hasPassphrase && existing.credentialID) {
      hasPassphrase = true;
    } else {
      shouldDeleteSecret = Boolean(existing?.credentialID);
    }
  }

  const nextCredentialID = hasPassword || hasPassphrase ? credentialID : undefined;
  const record = buildHostRecord(input, existing, nextCredentialID, { hasPassword, hasPassphrase });
  if (shouldWriteSecret) {
    await saveSshCredential(homeDir, credentialID, secret);
  }

  file.hosts = existing
    ? file.hosts.map((host) => (host.id === existing.id ? record : host))
    : [...file.hosts, record];
  await saveHostsFile(homeDir, file);

  if (!shouldWriteSecret && shouldDeleteSecret && existing?.credentialID) {
    await deleteSshCredential(homeDir, existing.credentialID);
  }

  return { ...record };
}

export async function deleteManualSshHost(homeDir: string, hostID: string): Promise<{ success: boolean }> {
  const id = trimString(hostID);
  if (!id) {
    return { success: false };
  }
  const file = await loadHostsFile(homeDir);
  const existing = file.hosts.find((host) => host.id === id);
  if (!existing) {
    return { success: false };
  }
  file.hosts = file.hosts.filter((host) => host.id !== id);
  await saveHostsFile(homeDir, file);
  if (existing.credentialID) {
    await deleteSshCredential(homeDir, existing.credentialID);
  }
  return { success: true };
}

export async function resolveSshHostForConnect(homeDir: string, host: SshHost): Promise<SshHostWithSecrets> {
  if (host.source !== 'manual' || !host.id) {
    return { ...host };
  }
  const file = await loadHostsFile(homeDir);
  const stored = file.hosts.find((entry) => entry.id === host.id);
  if (!stored) {
    throw new Error('Saved SSH host was not found');
  }
  if (!stored.credentialID) {
    return { ...stored };
  }
  const secret = await loadSshCredential(homeDir, stored.credentialID);
  if (!secret) {
    throw new Error('Saved SSH credential was not found');
  }
  return {
    ...stored,
    password: stored.authMethod === 'password' ? secret.password : undefined,
    passphrase: stored.authMethod === 'keyFile' ? secret.passphrase : undefined,
  };
}
