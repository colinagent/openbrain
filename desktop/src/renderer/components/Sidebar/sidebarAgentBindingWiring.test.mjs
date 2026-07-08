import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const source = readFileSync(
  path.resolve(import.meta.dirname, './Sidebar.tsx'),
  'utf8',
);

test('Sidebar root add menu subscribes to root agent binding updates', () => {
  assert.match(source, /agentBindingByCwd = useAppStore\(\(state\) => state\.agentBindingByCwd\)/);
  assert.match(source, /currentDir \? agentBindingByCwd\.has\(currentDir\) : false/);
  assert.match(source, /\[agentBindingByCwd, currentDir\]/);
});
