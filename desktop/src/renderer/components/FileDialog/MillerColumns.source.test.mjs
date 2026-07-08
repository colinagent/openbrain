import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const source = readFileSync(path.join(__dirname, 'MillerColumns.tsx'), 'utf8');

test('miller column rows do not use hover background for normal entries', () => {
  assert.doesNotMatch(source, /isActive \? 'bg-hover-bg text-prime-text font-medium' : 'text-prime-text hover:bg-hover-bg'/);
  assert.match(source, /isActive \? 'bg-hover-bg text-prime-text font-medium' : 'text-prime-text'/);
});
