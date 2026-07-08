import type { SshHost, SshHostWithSecrets } from './sshTypes';

export function resolveHostLabel(host: SshHost) {
  if (host.user && host.hostname) {
    return `${host.user}@${host.hostname}`;
  }
  if (host.hostname) {
    return host.hostname;
  }
  return host.alias;
}

export function sanitizeSshHost(host: SshHostWithSecrets): SshHost {
  const { password: _password, passphrase: _passphrase, ...safeHost } = host;
  return safeHost;
}
