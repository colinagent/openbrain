import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const hostsSource = readFileSync(path.join(__dirname, 'manualSshHosts.ts'), 'utf8');
const credentialsSource = readFileSync(path.join(__dirname, 'manualSshCredentials.ts'), 'utf8');

test('manual SSH hosts store secrets separately with safeStorage encryption', () => {
  assert.match(hostsSource, /const HOSTS_FILE = 'ssh-hosts\.json'/);
  assert.match(hostsSource, /credentialID/);
  assert.match(credentialsSource, /const CREDENTIALS_FILE = 'ssh-host-credentials\.json'/);
  assert.match(credentialsSource, /safeStorage\.isEncryptionAvailable\(\)/);
  assert.match(credentialsSource, /safeStorage\.encryptString/);
  assert.match(credentialsSource, /safeStorage\.decryptString/);
});
