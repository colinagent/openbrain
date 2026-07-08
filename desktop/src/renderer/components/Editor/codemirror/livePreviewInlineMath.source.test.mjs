import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const livePreviewSource = readFileSync(path.join(__dirname, 'livePreviewPlugin.ts'), 'utf8');
const blockDecorationsSource = readFileSync(path.join(__dirname, 'livePreviewBlockDecorations.ts'), 'utf8');
const mathBlocksSource = readFileSync(path.join(__dirname, 'utils/mathBlocks.ts'), 'utf8');

test('single-dollar inline math stays disabled for currency-like markdown', () => {
  assert.doesNotMatch(livePreviewSource, /decorateInlineMath/);
  assert.doesNotMatch(livePreviewSource, /findInlineMathRanges/);
  assert.doesNotMatch(livePreviewSource, /math:inline/);
  assert.doesNotMatch(livePreviewSource, /new MathWidget\(/);
});

test('double-dollar math block decoration remains wired', () => {
  assert.match(blockDecorationsSource, /buildMathBlockDecorationsInRange/);
  assert.match(mathBlocksSource, /indexOf\('\$\$'\)/);
  assert.match(mathBlocksSource, /cm-md-math-block-line/);
});
