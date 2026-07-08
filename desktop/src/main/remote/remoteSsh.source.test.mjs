import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const source = [
  'remoteSsh.ts',
  'ssh2Connection.ts',
  'ssh2Transport.ts',
].map((file) => readFileSync(path.join(__dirname, file), 'utf8')).join('\n');

test('Remote SSH uses ssh2 exec and forwardOut instead of shelling out to ssh -L', () => {
  assert.match(source, /from 'ssh2'/);
  assert.match(source, /client\.exec/);
  assert.match(source, /client\.forwardOut/);
  assert.doesNotMatch(source, /spawn\('ssh'/);
  assert.doesNotMatch(source, /execFile\('ssh'/);
});
