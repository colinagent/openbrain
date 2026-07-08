import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const source = readFileSync(path.join(__dirname, 'tagPill.ts'), 'utf8');

test('tag pill exports shared class and css variable helper', () => {
  assert.match(source, /export const UI_TAG_PILL = 'ui-tag-pill';/);
  assert.match(source, /export function tagPillBackground\(label: string\): string/);
  assert.match(source, /export type TagPillStyle = CSSProperties & \{ '--ui-tag-bg': string \};/);
  assert.match(source, /export function tagPillStyle\(label: string\): TagPillStyle/);
});

test('tag pill background is stable and theme-aware', () => {
  assert.match(source, /label\.trim\(\)\.toLowerCase\(\)/);
  assert.match(source, /fnv1a\(normalized\) % TAG_PILL_HUES\.length/);
  assert.match(
    source,
    /return `color-mix\(in srgb, \$\{hue\} \$\{TAG_PILL_MIX\}, var\(--color-editor-bg\)\)`;/,
  );
  assert.match(source, /const TAG_PILL_MIX = '22%';/);
});
