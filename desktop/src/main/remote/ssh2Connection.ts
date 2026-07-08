import * as fs from 'node:fs/promises';
import * as os from 'node:os';
import { Client } from 'ssh2';
import type { AnyAuthMethod, AuthenticationType, ConnectConfig } from 'ssh2';
import { resolveHostLabel } from '../ssh/sshHostUtils';
import type { SshHost, SshHostWithSecrets } from '../ssh/sshTypes';

const SSH_READY_TIMEOUT_MS = 30_000;
const SSH_KEEPALIVE_INTERVAL_MS = 15_000;

type SshAuthAttempt = AnyAuthMethod & { keyPath?: string };

const DEFAULT_KEY_PATHS = [
  '~/.ssh/id_ed25519',
  '~/.ssh/id_rsa',
  '~/.ssh/id_ecdsa',
  '~/.ssh/id_dsa',
  '~/.ssh/id_xmss',
];

function expandTilde(input: string) {
  if (input === '~') {
    return os.homedir();
  }
  if (input.startsWith('~/')) {
    return `${os.homedir()}/${input.slice(2)}`;
  }
  return input;
}

function resolveSshHostname(host: SshHost) {
  return host.hostname || host.alias;
}

function resolveSshUsername(host: SshHost) {
  return host.user || os.userInfo().username;
}

function resolveSshPort(host: SshHost) {
  const raw = String(host.port || '').trim();
  if (!raw) {
    return 22;
  }
  const port = Number(raw);
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error(`Invalid SSH port: ${raw}`);
  }
  return port;
}

async function readPrivateKey(keyPath: string): Promise<Buffer | null> {
  try {
    return await fs.readFile(expandTilde(keyPath));
  } catch {
    return null;
  }
}

function authProtocolMethod(attempt: SshAuthAttempt): AuthenticationType {
  return attempt.type === 'agent' ? 'publickey' : attempt.type;
}

function makeAuthHandler(attempts: SshAuthAttempt[]): ConnectConfig['authHandler'] {
  let index = 0;
  return ((methodsLeft: AuthenticationType[] | null, _partialSuccess: boolean, next: (method: AnyAuthMethod | false) => void) => {
    while (index < attempts.length) {
      const attempt = attempts[index++];
      if (methodsLeft && !methodsLeft.includes(authProtocolMethod(attempt))) {
        continue;
      }
      if (attempt.type === 'publickey') {
        const { keyPath: _keyPath, ...payload } = attempt;
        next(payload);
        return;
      }
      next(attempt);
      return;
    }
    next(false);
  }) as ConnectConfig['authHandler'];
}

async function buildAuthAttempts(host: SshHostWithSecrets): Promise<SshAuthAttempt[]> {
  const username = resolveSshUsername(host);
  const attempts: SshAuthAttempt[] = [];

  if (host.authMethod === 'password') {
    if (!host.password) {
      throw new Error(`Password is missing for ${resolveHostLabel(host)}`);
    }
    return [
      { type: 'password', username, password: host.password },
      {
        type: 'keyboard-interactive',
        username,
        prompt: (_name, _instructions, _lang, prompts, finish) => {
          finish(prompts.map(() => host.password || ''));
        },
      },
    ];
  }

  if (host.authMethod === 'keyFile') {
    if (!host.identityFile) {
      throw new Error(`Private key file is missing for ${resolveHostLabel(host)}`);
    }
    const key = await readPrivateKey(host.identityFile);
    if (!key) {
      throw new Error(`Failed to read private key file: ${host.identityFile}`);
    }
    return [{ type: 'publickey', username, key, keyPath: host.identityFile, passphrase: host.passphrase }];
  }

  if (host.identityFile) {
    const key = await readPrivateKey(host.identityFile);
    if (key) {
      attempts.push({ type: 'publickey', username, key, keyPath: host.identityFile, passphrase: host.passphrase });
    }
  }

  const agent = process.env.SSH_AUTH_SOCK;
  if (agent) {
    attempts.push({ type: 'agent', username, agent });
  }

  for (const keyPath of DEFAULT_KEY_PATHS) {
    if (host.identityFile && expandTilde(host.identityFile) === expandTilde(keyPath)) {
      continue;
    }
    const key = await readPrivateKey(keyPath);
    if (key) {
      attempts.push({ type: 'publickey', username, key, keyPath });
    }
  }

  return attempts;
}

export async function connectSshClient(host: SshHostWithSecrets): Promise<Client> {
  const attempts = await buildAuthAttempts(host);
  const config: ConnectConfig = {
    host: resolveSshHostname(host),
    port: resolveSshPort(host),
    username: resolveSshUsername(host),
    readyTimeout: SSH_READY_TIMEOUT_MS,
    keepaliveInterval: SSH_KEEPALIVE_INTERVAL_MS,
    tryKeyboard: host.authMethod === 'password',
    authHandler: makeAuthHandler(attempts),
  };

  return new Promise<Client>((resolve, reject) => {
    const client = new Client();
    let settled = false;

    const finish = (error?: Error) => {
      if (settled) {
        return;
      }
      settled = true;
      client.off('ready', onReady);
      client.off('error', onError);
      if (error) {
        client.end();
        reject(error);
        return;
      }
      resolve(client);
    };

    const onReady = () => finish();
    const onError = (error: Error) => finish(error);

    client.once('ready', onReady);
    client.once('error', onError);
    client.connect(config);
  });
}
