import assert from 'node:assert/strict';
import { mkdtemp, readFile, rm, stat } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { writeJsonFileAtomic } from './jsonFile';

test('atomic JSON files are private to the current OS user', async (t) => {
  if (process.platform === 'win32') {
    t.skip('Windows does not expose POSIX file modes');
    return;
  }
  const dir = await mkdtemp(path.join(os.tmpdir(), 'openbrain-json-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  const target = path.join(dir, 'auth.json');

  await writeJsonFileAtomic(target, { token: 'test-only' });

  assert.deepEqual(JSON.parse(await readFile(target, 'utf8')), { token: 'test-only' });
  assert.equal((await stat(target)).mode & 0o777, 0o600);
});
