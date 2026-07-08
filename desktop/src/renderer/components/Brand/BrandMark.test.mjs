import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const brandMarkPath = path.join(__dirname, 'BrandMark.tsx');

test('BrandMark renders a soft 80/20 canvas-white monochrome mark (no glass block)', () => {
  const source = readFileSync(brandMarkPath, 'utf8');

  assert.doesNotMatch(source, /text-logo-light/);
  assert.doesNotMatch(source, /text-logo-dark/);
  assert.match(source, /<OpenBrainLogo/);
  assert.match(source, /className="op-brand-mark-fg block h-28 w-28 shrink-0"/);
  assert.match(source, /monochrome/);
  assert.doesNotMatch(source, /variant=/);
  assert.match(source, /<h1 className="op-brand-mark-fg op-brand-mark-title m-0 text-center text-2xl">/);
  assert.doesNotMatch(source, /text-secondary-text/);
  assert.doesNotMatch(source, /font-semibold/);
  assert.doesNotMatch(source, /tracking-\[0\.1em\]/);
  assert.doesNotMatch(source, /text-prime-text/);
  assert.doesNotMatch(source, /text-highlight/);
  assert.doesNotMatch(source, /text-tertiary-text/);
  assert.doesNotMatch(source, /text-\[var\(--op-opagent\)\]/);
  assert.doesNotMatch(source, /opacity-50/);
  assert.doesNotMatch(source, /OP_SG_CAPSULE/);
  assert.doesNotMatch(source, /op-sg-capsule/);
  assert.doesNotMatch(source, /brand-mark-tagline-capsule/);
  assert.doesNotMatch(source, /uppercase/);
  assert.doesNotMatch(source, /backdrop-blur|backdrop-filter/);
  assert.match(source, /op-brand-mark-fg/);
  assert.match(source, /A GUI and agent runtime for GBrain/);
  assert.match(source, /\{PRODUCT_TAGLINE\}/);
  assert.match(source, /\{PRODUCT_NAME\}/);
  assert.match(source, /PRODUCT_NAME\s*=\s*'OPENBRAIN'/);
});
