import { safeStorage } from 'electron';
import * as fs from 'fs/promises';
import * as path from 'path';

type StoredCredential = {
  encrypted: string;
  updatedAt: number;
};

type CredentialFile = {
  version: 1;
  credentials: Record<string, StoredCredential>;
};

export type SshSecretPayload = {
  version: 1;
  password?: string;
  passphrase?: string;
};

const CREDENTIALS_FILE = 'ssh-host-credentials.json';

function secretsRoot(homeDir: string) {
  return path.join(homeDir, '.openbrain', 'configs', 'secrets');
}

function credentialsPath(homeDir: string) {
  return path.join(secretsRoot(homeDir), CREDENTIALS_FILE);
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

async function loadCredentialFile(homeDir: string): Promise<CredentialFile> {
  const parsed = await readJsonFile<CredentialFile>(credentialsPath(homeDir), { version: 1, credentials: {} });
  return {
    version: 1,
    credentials: parsed.credentials && typeof parsed.credentials === 'object' ? parsed.credentials : {},
  };
}

async function saveCredentialFile(homeDir: string, file: CredentialFile): Promise<void> {
  await writeJsonFile(credentialsPath(homeDir), { version: 1, credentials: file.credentials });
}

function encryptSecret(secret: SshSecretPayload): string {
  if (!safeStorage.isEncryptionAvailable()) {
    throw new Error('System credential encryption is unavailable');
  }
  return safeStorage.encryptString(JSON.stringify(secret)).toString('base64');
}

function decryptSecret(credential: StoredCredential): SshSecretPayload {
  const raw = safeStorage.decryptString(Buffer.from(credential.encrypted, 'base64'));
  const parsed = JSON.parse(raw) as Partial<SshSecretPayload>;
  return {
    version: 1,
    password: typeof parsed.password === 'string' ? parsed.password : undefined,
    passphrase: typeof parsed.passphrase === 'string' ? parsed.passphrase : undefined,
  };
}

export async function saveSshCredential(
  homeDir: string,
  credentialID: string,
  secret: SshSecretPayload,
): Promise<void> {
  const credentials = await loadCredentialFile(homeDir);
  credentials.credentials[credentialID] = {
    encrypted: encryptSecret(secret),
    updatedAt: Date.now(),
  };
  await saveCredentialFile(homeDir, credentials);
}

export async function deleteSshCredential(homeDir: string, credentialID: string): Promise<void> {
  const credentials = await loadCredentialFile(homeDir);
  delete credentials.credentials[credentialID];
  await saveCredentialFile(homeDir, credentials);
}

export async function loadSshCredential(homeDir: string, credentialID: string): Promise<SshSecretPayload | null> {
  const credentials = await loadCredentialFile(homeDir);
  const credential = credentials.credentials[credentialID];
  return credential ? decryptSecret(credential) : null;
}
