export type SshAuthMethod = 'agent' | 'keyFile' | 'password';

export type SshHost = {
  id?: string;
  alias: string;
  hostname?: string;
  user?: string;
  port?: string;
  identityFile?: string;
  source?: string;
  authMethod?: SshAuthMethod;
  credentialID?: string;
  hasPassword?: boolean;
  hasPassphrase?: boolean;
};

export type SshHostWithSecrets = SshHost & {
  password?: string;
  passphrase?: string;
};

export type ManualSshHostInput = {
  id?: string;
  alias: string;
  hostname: string;
  user: string;
  port?: string;
  identityFile?: string;
  authMethod: Extract<SshAuthMethod, 'keyFile' | 'password'>;
  password?: string;
  passphrase?: string;
};

export type ManualSshHostRecord = SshHost & {
  id: string;
  hostname: string;
  user: string;
  source: 'manual';
  authMethod: Extract<SshAuthMethod, 'keyFile' | 'password'>;
  createdAt: number;
  updatedAt: number;
};

