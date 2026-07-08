import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const source = [
  'RemoteConnectModal.tsx',
  'RemoteConnectForm.tsx',
].map((file) => readFileSync(path.join(__dirname, file), 'utf8')).join('\n');

test('RemoteConnectModal exposes manual SSH host password and private-key flows', () => {
  assert.match(source, /saveHost/);
  assert.match(source, /deleteHost/);
  assert.match(source, /pickIdentityFile/);
  assert.match(source, /authMethod: 'password'/);
  assert.match(source, /authMethod: 'keyFile'/);
  assert.match(source, /type="password"/);
});
