import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const resizeDividerPath = path.join(__dirname, 'ResizeDivider.tsx');

function read(filePath) {
  return readFileSync(filePath, 'utf8');
}

test('ResizeDivider supports delayed hover activation with timer cleanup', () => {
  const source = read(resizeDividerPath);

  assert.match(source, /hoverDelayMs\?: number;/);
  assert.match(source, /hoverDelayMs = 0,/);
  assert.match(source, /const hoverTimerRef = useRef<number \| null>\(null\);/);
  assert.match(source, /window\.clearTimeout\(hoverTimerRef\.current\);/);
  assert.match(source, /if \(hoverDelayMs <= 0\) \{\s*setHovered\(true\);\s*return;\s*\}/m);
  assert.match(source, /hoverTimerRef\.current = window\.setTimeout\(\(\) => \{\s*hoverTimerRef\.current = null;\s*setHovered\(true\);\s*\}, hoverDelayMs\);/m);
});

test('ResizeDivider supports a faint resting line color', () => {
  const source = read(resizeDividerPath);

  assert.match(source, /restingColor\?: string;/);
  assert.match(source, /restingColor,/);
  assert.match(source, /: restingColor\s*\?\s*\{ backgroundColor: restingColor \}/);
});

test('ResizeDivider can keep its hit target active while visually hidden', () => {
  const source = read(resizeDividerPath);

  assert.match(source, /hitTargetEnabled\?: boolean;/);
  assert.match(source, /const isHitTargetEnabled = hitTargetEnabled \?\? isVisible;/);
  assert.match(source, /\$\{isHitTargetEnabled \? 'pointer-events-auto' : 'pointer-events-none'\}/);
});
