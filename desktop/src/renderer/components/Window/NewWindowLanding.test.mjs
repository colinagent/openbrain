import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const newWindowLandingPath = path.join(__dirname, 'NewWindowLanding.tsx');

test('NewWindowLanding renders the OpenBrain client tagline', () => {
  const source = readFileSync(newWindowLandingPath, 'utf8');

  assert.match(source, /BrandMark/);
});
