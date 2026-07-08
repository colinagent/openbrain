import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const appRoot = path.join(__dirname, '..', '..');

test('sidebar chrome uses the sidebarBg theme token while titlebar keeps its own surface', () => {
  const source = readFileSync(path.join(__dirname, 'styles/index.css'), 'utf8');

  assert.match(source, /\.ui-titlebar\s*\{\s*background:\s*var\(--color-sidebar-bg\);\s*border-bottom:\s*1px solid var\(--op-titlebar-divider\);/s);
  assert.match(source, /--op-titlebar-divider:\s*var\(--color-border\);/);
  assert.match(source, /\.sidebar-activity-rail,\s*\.ui-sidebar\s*\{\s*background:\s*var\(--color-sidebar-bg\);/s);
  assert.match(source, /\.ui-tabbar \.tab-active-shell,\s*\.ui-tabbar \.tab-active-shell \.tab-hover-bg-sync\s*\{[^}]*background-color:\s*transparent;/s);
  assert.match(source, /\.ui-tabbar \.tab-hover-shell\s*\{[^}]*border-radius:\s*9999px;/s);
  assert.match(source, /--op-faint-divider:\s*color-mix\(in srgb, var\(--color-border\) 50%, transparent\);/);
  assert.match(source, /--op-sidebar-resize-divider:\s*var\(--color-border\);/);
  assert.match(source, /\.sidebar-activity-rail\s*\{\s*border-right:\s*1px solid var\(--op-sidebar-resize-divider\);\s*\}/);
  assert.doesNotMatch(source, /\.ui-tabbar\s*\{[^}]*border-bottom:/m);
});

test('sidebarBg is derived from Brand Core at runtime, not stored in jsonc', () => {
  const presets = readFileSync(path.join(__dirname, 'theme/presets.ts'), 'utf8');
  const brandCore = readFileSync(path.join(__dirname, 'theme/brandCore.ts'), 'utf8');
  const tokens = readFileSync(path.join(__dirname, 'theme/tokens.ts'), 'utf8');
  const settingsStore = readFileSync(path.join(appRoot, 'src/main/settings/settingsStore.ts'), 'utf8');
  const themeTemplate = readFileSync(path.join(appRoot, 'settings/theme.jsonc'), 'utf8');

  assert.match(tokens, /sidebarBg:\s*string;/);
  assert.match(presets, /sidebarBg:\s*palette\.sidebarBg \?\? palette\.secondaryBg \?\? palette\.background/);
  assert.match(brandCore, /expandCoreToPalette/);
  assert.match(settingsStore, /core:\s*ThemeCore;/);
  assert.match(settingsStore, /builtInVersion:\s*29,/);
  assert.match(settingsStore, /id:\s*'default-light'[\s\S]*background:\s*'#fcfaf4'/);
  assert.match(settingsStore, /id:\s*'openbrain-light'[\s\S]*background:\s*'#f4f9f7'/);
  assert.match(themeTemplate, /"builtInVersion":\s*29/);
  assert.match(themeTemplate, /"id":\s*"default-light"[\s\S]*"core"/);
  assert.match(themeTemplate, /"id":\s*"openbrain-light"[\s\S]*"background":\s*"#f4f9f7"/);
  assert.doesNotMatch(themeTemplate, /"palette"/);
});
