import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const source = readFileSync(
  path.resolve(import.meta.dirname, './index.tsx'),
  'utf8',
);

test('SettingsIcon renders a gear rather than a sunburst', () => {
  assert.match(source, /export const SettingsIcon[\s\S]*M19\.4 15/);
  assert.doesNotMatch(source, /M12 1v4M12 19v4/);
});
