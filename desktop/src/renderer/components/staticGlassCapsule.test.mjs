import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const stylesSource = readFileSync(path.join(__dirname, '../styles/index.css'), 'utf8');
const capsuleSource = readFileSync(path.join(__dirname, 'staticGlassCapsule.ts'), 'utf8');

test('static glass capsule exports shared class names', () => {
  assert.match(capsuleSource, /export const OP_SG_CAPSULE = 'op-sg-capsule';/);
  assert.match(capsuleSource, /export const OP_SG_FROST_SURFACE = 'op-sg-frost-surface';/);
  assert.match(capsuleSource, /export const OP_SG_CAPSULE_ON_SIDEBAR = 'op-sg-capsule--on-sidebar';/);
  assert.match(capsuleSource, /export const OP_SG_CAPSULE_ON_TITLEBAR = 'op-sg-capsule--on-titlebar';/);
  assert.match(capsuleSource, /export const OP_SG_CAPSULE_ON_EDITOR = 'op-sg-capsule--on-editor';/);
  assert.match(capsuleSource, /export const OPENBRAIN_GRAPH_CAPSULE = /);
  assert.match(capsuleSource, /export const OP_SG_CAPSULE_ON_ACTIVITY_HEADER = 'op-sg-capsule--on-activity-header';/);
  assert.match(capsuleSource, /export const UI_PILL_BTN_PRIMARY = 'ui-pill-btn-primary';/);
  assert.match(capsuleSource, /export const UI_PILL_BTN_SECONDARY = 'ui-pill-btn-secondary';/);
  assert.match(capsuleSource, /export const UI_PILL_BTN_COMPACT = 'ui-pill-btn--compact';/);
  assert.match(capsuleSource, /export const UI_PILL_BTN_DIALOG = 'ui-pill-btn--dialog';/);
  assert.match(capsuleSource, /export const UI_PILL_BTN_FIT = 'ui-pill-btn--fit';/);
});

test('static glass surface block follows layout shells and wins cascade', () => {
  const uiCapsuleIndex = stylesSource.indexOf('.ui-capsule-pill {');
  const workspaceShellIndex = stylesSource.indexOf('.workspace-tab-shell {');
  const frostSurfaceIndex = stylesSource.indexOf('/* Static Frost surface');

  assert.ok(uiCapsuleIndex >= 0, 'expected .ui-capsule-pill rule');
  assert.ok(workspaceShellIndex >= 0, 'expected .workspace-tab-shell rule');
  assert.ok(frostSurfaceIndex >= 0, 'expected static frost surface block');
  assert.ok(frostSurfaceIndex > uiCapsuleIndex, 'frost surface must follow ui-capsule-pill');
  assert.ok(frostSurfaceIndex > workspaceShellIndex, 'frost surface must follow workspace-tab-shell');

  assert.match(
    stylesSource,
    /\.op-sg-frost-surface:not\(\.op-popup-menu\),\s*\n\.op-sg-capsule\s*\{[^}]*background-color:\s*color-mix\(\s*in srgb,\s*var\(--op-sg-frost\) var\(--op-sg-milk\),\s*transparent\s*\);/m,
  );
  assert.match(
    stylesSource,
    /\.op-sg-frost-surface\.op-popup-menu\s*\{[^}]*background-color:\s*var\(--color-overlay-bg\);/m,
  );
  assert.match(
    stylesSource,
    /\.op-activity-panel-header,\s*\n\.op-activity-panel-body\s*\{[^}]*background-color:\s*color-mix\(\s*in srgb,\s*var\(--op-sg-frost\) var\(--op-sg-panel-milk\),\s*transparent\s*\);/m,
  );
  assert.match(
    stylesSource,
    /\.op-sg-capsule::before[\s\S]*?background-image:\s*var\(--op-sg-noise-image\);[\s\S]*?mix-blend-mode:\s*var\(--op-sg-noise-blend\);/m,
  );
  assert.doesNotMatch(stylesSource, /--op-sg-panel-header-solid:/);
  assert.doesNotMatch(stylesSource, /--op-sg-panel-body-solid:/);
});

test('activity panel shell border is a lighter mix of the shared capsule border', () => {
  assert.match(stylesSource, /--op-glass-border:\s*color-mix\(in srgb, var\(--op-sg-border\) 65%, transparent\);/g);
  assert.match(
    stylesSource,
    /:root\[data-color-scheme='dark'\]\s*\{[^}]*--op-sg-panel-border:\s*var\(--op-sg-border\);/m,
  );
  assert.match(
    stylesSource,
    /:root\[data-color-scheme='dark'\]\s*\{[^}]*--op-sg-border:\s*color-mix\(in srgb, var\(--color-border\) 45%, white 55%\);/m,
  );
});

test('static frost tokens define milk, noise, and edge shadow stack', () => {
  assert.match(stylesSource, /--op-sg-frost:\s*color-mix\(in srgb, var\(--color-editor-bg\) 90%, white 10%\);/);
  assert.match(stylesSource, /--op-sg-milk:\s*80%;/);
  assert.match(stylesSource, /--op-sg-panel-milk:\s*100%;/);
  assert.match(stylesSource, /--op-sg-noise:\s*0\.08;/);
  assert.match(stylesSource, /--op-sg-noise-blend:\s*overlay;/);
  assert.match(
    stylesSource,
    /:root\[data-color-scheme='dark'\]\s*\{[^}]*--op-sg-frost:\s*color-mix\(in srgb, var\(--color-editor-bg\) 60%, white 40%\);[^}]*--op-sg-noise:\s*0\.16;[^}]*--op-sg-noise-blend:\s*soft-light;/m,
  );
  assert.match(
    stylesSource,
    /--op-sg-shadow:\s*var\(--op-sg-shadow-inner\),\s*var\(--op-sg-elevation\);/m,
  );
  assert.match(
    stylesSource,
    /\.op-activity-panel-header:not\(\.is-expanded\)\s*\{[^}]*box-shadow:\s*var\(--op-sg-shadow-inner\);/m,
  );
  assert.match(stylesSource, /inset 0 -1px 0 var\(--op-sg-rim\)/);
  assert.match(
    stylesSource,
    /\.op-activity-panel-body\s*\{[^}]*padding-bottom:\s*var\(--op-activity-panel-content-bottom-clearance\);/m,
  );
  assert.match(
    stylesSource,
    /\.op-activity-panel\s*\{[^}]*background:\s*var\(--op-activity-panel-shell-bg\);[^}]*box-shadow:\s*var\(--op-glass-panel-shadow\);/m,
  );
  assert.doesNotMatch(stylesSource, /--op-glass-header-floor:/);
  assert.doesNotMatch(stylesSource, /--op-activity-panel-body-fill:/);
});

test('static glass capsule substrate modifiers are defined once', () => {
  assert.match(
    stylesSource,
    /\.op-sg-capsule--on-activity-header\s*\{[^}]*--op-sg-capsule-substrate:\s*var\(--op-sg-panel-solid\);/m,
  );
  assert.match(
    stylesSource,
    /\.openbrain-graph-capsule\.op-sg-capsule\s*\{/,
  );
});

test('clickable pill buttons keep hover background stable', () => {
  assert.match(stylesSource, /\.ui-pill-btn-primary,\s*\n\.ui-pill-btn-secondary\s*\{[^}]*transition:\s*color 0\.15s ease;/m);
  assert.match(
    stylesSource,
    /\.ui-pill-btn-primary:hover:not\(:disabled\),[\s\S]*?\.ui-pill-btn-secondary:focus-visible:not\(:disabled\)\s*\{\s*color:\s*var\(--color-highlight\);\s*\}/m,
  );
  assert.doesNotMatch(stylesSource, /\.ui-pill-btn-primary:hover[^{]*\{[^}]*background/m);
  assert.doesNotMatch(stylesSource, /\.ui-pill-btn-secondary:hover[^{]*\{[^}]*background/m);
});

test('pill buttons enforce silhouette min-width tiers', () => {
  assert.match(
    stylesSource,
    /\.ui-pill-btn-primary,\s*\n\.ui-pill-btn-secondary\s*\{[^}]*min-width:\s*4\.5rem;/m,
  );
  assert.match(stylesSource, /\.ui-pill-btn--compact\s*\{[^}]*min-width:\s*3\.25rem;/m);
  assert.match(stylesSource, /\.ui-pill-btn--dialog\s*\{[^}]*min-width:\s*7rem;/m);
  assert.match(stylesSource, /\.ui-pill-btn--fit\s*\{[^}]*min-width:\s*0;/m);
  assert.match(stylesSource, /\.file-tree-agent-pill\s*\{[^}]*min-width:\s*0;/m);
});

test('status bar controls use plain text triggers without pill chrome', () => {
  const zoomSource = readFileSync(path.join(__dirname, 'ZoomStatusControl.tsx'), 'utf8');
  const syncSource = readFileSync(path.join(__dirname, 'WorkspaceSyncStatusControl.tsx'), 'utf8');
  const branchSource = readFileSync(path.join(__dirname, 'BranchStatusControl.tsx'), 'utf8');

  assert.match(stylesSource, /\.ui-statusbar-control\s*\{[^}]*border:\s*none;/m);
  assert.match(zoomSource, /className="ui-statusbar-control"/);
  assert.doesNotMatch(zoomSource, /ui-pill-btn-secondary px-1\.5 py-0\.5 text-xs/);
  assert.match(syncSource, /className="ui-statusbar-control max-w-\[180px\]"/);
  assert.doesNotMatch(syncSource, /ui-pill-btn-secondary max-w-\[180px\]/);
  assert.match(branchSource, /className="ui-statusbar-control max-w-\[220px\]"/);
});
