import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const panelSource = readFileSync(path.join(__dirname, 'FrontmatterPropertiesPanel.tsx'), 'utf8');
const stylesSource = readFileSync(path.join(__dirname, '../../styles/index.css'), 'utf8');

test('frontmatter tags use borderless colored tag pills, not static glass', () => {
  assert.match(panelSource, /UI_TAG_PILL/);
  assert.match(panelSource, /tagPillStyle\(label\)/);
  assert.doesNotMatch(panelSource, /OP_SG_CAPSULE/);
  assert.doesNotMatch(panelSource, /ui-capsule-pill op-md-frontmatter-tag-chip/);
  assert.match(stylesSource, /\.ui-tag-pill\s*\{[^}]*border:\s*none;/m);
  assert.match(stylesSource, /\.ui-tag-pill\s*\{[^}]*background-color:\s*var\(--ui-tag-bg\);/m);
});
