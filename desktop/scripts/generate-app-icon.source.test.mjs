import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

const repoRoot = process.cwd().endsWith('desktop')
  ? process.cwd()
  : path.join(process.cwd(), 'desktop');

const iconSource = readFileSync(path.join(repoRoot, 'build/icon.svg'), 'utf8');

test('macOS app icon shell keeps the measured system rounded shape', () => {
  assert.match(
    iconSource,
    /measured from Apple Notes\/Reminders: 25px inset and 45\.75px radius at 256px/,
  );
  assert.match(iconSource, /<rect x="100" y="100" width="824" height="824" rx="183" ry="183"\/>/);
  assert.match(iconSource, /<rect x="100" y="100" width="824" height="824" rx="183" ry="183" fill="url\(#cardGrad\)"\/>/);
  assert.match(iconSource, /<rect x="102\.5" y="102\.5" width="819" height="819" rx="181" ry="181"/);
});

test('macOS app icon uses the light OpenBrain logo mark on a white shell', () => {
  assert.match(iconSource, /id="openbrainGradient"/);
  assert.match(iconSource, /OpenBrain logo mark \(light/);
  assert.match(iconSource, /stop offset="1" stop-color="#FFFFFF"/);
  assert.doesNotMatch(iconSource, /Logo bars/);
});
