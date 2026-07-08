import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const tabLayoutPath = path.join(__dirname, 'tabLayout.ts');
const stylesPath = path.join(__dirname, '../styles/index.css');

function read(filePath) {
  return readFileSync(filePath, 'utf8');
}

test('shared tab layout keeps a fixed Chrome-style width before shrinking', () => {
  const source = read(tabLayoutPath);

  assert.match(source, /export const TAB_PREFERRED_WIDTH = 160;/);
  assert.match(source, /export const TAB_MIN_WIDTH = 52;/);
  assert.match(source, /export const TAB_MAX_WIDTH = TAB_PREFERRED_WIDTH;/);
  assert.match(source, /flex:\s*`0 1 \$\{TAB_PREFERRED_WIDTH\}px`,/);
  assert.match(source, /width:\s*TAB_PREFERRED_WIDTH,/);
  assert.match(source, /minWidth:\s*TAB_MIN_WIDTH,/);
  assert.match(source, /maxWidth:\s*TAB_MAX_WIDTH,/);
  assert.doesNotMatch(source, /flex:\s*'1 1 0'/);
});

test('tab close button stays overlayed and does not reserve width in CSS', () => {
  const tabLayoutSource = read(tabLayoutPath);
  const stylesSource = read(stylesPath);
  const closeRule = stylesSource.match(/\.tab-close-btn\s*\{([^}]*)\}/m);
  const toolbarHoverRule = stylesSource.match(
    /\.icon-button-toolbar:hover,\s*\.icon-button-toolbar:focus-visible,\s*\.icon-button-toolbar\.is-active-hover\s*\{([^}]*)\}/m,
  );
  const tabInlineHoverRule = stylesSource.match(
    /\.tab-icon-hover-lift\.icon-button-inline:hover,\s*\.tab-icon-hover-lift\.icon-button-inline:focus-visible\s*\{([^}]*)\}/m,
  );

  assert.match(
    tabLayoutSource,
    /export const TAB_CLOSE_BUTTON_DELAYED_REVEAL_CLASS = 'tab-close-btn-delayed';/,
  );
  assert.match(
    tabLayoutSource,
    /export const TAB_ICON_HOVER_LIFT_CLASS = 'tab-icon-hover-lift';/,
  );
  assert.match(
    tabLayoutSource,
    /export const TAB_CLOSE_BUTTON_BACKGROUND_SYNC_CLASS =\s*'tab-hover-bg-sync';/,
  );
  assert.match(
    tabLayoutSource,
    /export const TAB_CLOSE_BUTTON_CLASS =\s*'tab-close-btn absolute inset-y-0 right-1 z-10 my-auto';/,
  );
  assert.match(
    tabLayoutSource,
    /TAB_CLOSE_BUTTON_BACKGROUND_SYNC_CLASS,/,
  );
  assert.ok(closeRule, 'expected .tab-close-btn CSS rule');
  assert.ok(toolbarHoverRule, 'expected toolbar hover rule');
  assert.ok(tabInlineHoverRule, 'expected tab inline hover rule');
  assert.match(stylesSource, /\.tab-close-btn-delayed\s*\{[^}]*transition:\s*opacity 0\.12s ease;[^}]*transition-delay:\s*0s;[^}]*\}/m);
  assert.match(stylesSource, /\.tab-hover-shell:hover \.tab-close-btn-delayed,\s*\.workspace-tab-shell:hover \.tab-close-btn-delayed\s*\{[^}]*opacity:\s*1;[^}]*transition-delay:\s*0\.08s;[^}]*\}/m);
  assert.match(
    stylesSource,
    /\.tab-hover-shell:has\(:focus-visible\) \.tab-close-btn-delayed,\s*\.workspace-tab-shell:has\(:focus-visible\) \.tab-close-btn-delayed,\s*\.tab-close-btn-delayed:focus-visible\s*\{[^}]*opacity:\s*1;[^}]*transition-delay:\s*0s;[^}]*\}/m,
  );
  assert.match(
    stylesSource,
    /\.tab-hover-shell:hover:not\(\.tab-active-shell\) \.tab-hover-bg-sync\s*\{[^}]*transition-delay:\s*0\.08s;[^}]*\}/m,
  );
  assert.match(
    stylesSource,
    /\.tab-hover-shell:has\(:focus-visible\):not\(\.tab-active-shell\) \.tab-hover-bg-sync,\s*\.tab-hover-bg-sync:focus-visible\s*\{[^}]*transition-delay:\s*0s;[^}]*\}/m,
  );
  assert.doesNotMatch(stylesSource, /\.group:focus-within \.tab-close-btn-delayed/);
  assert.doesNotMatch(stylesSource, /\.group:focus-within \.tab-hover-bg-sync/);
  assert.doesNotMatch(closeRule[1], /(?:^|[\s;])width\s*:/m);
  assert.doesNotMatch(closeRule[1], /(?:^|[\s;])min-width\s*:/m);
  assert.doesNotMatch(toolbarHoverRule[1], /transform\s*:/m);
  assert.match(toolbarHoverRule[1], /box-shadow\s*:/m);
  assert.doesNotMatch(tabInlineHoverRule[1], /transform\s*:/m);
  assert.match(tabInlineHoverRule[1], /box-shadow\s*:/m);
});

test('shared tab shell uses hoverBg without separator pseudo-elements', () => {
  const source = read(tabLayoutPath);
  const stylesSource = read(stylesPath);

  assert.match(
    source,
    /export const TAB_SHELL_CLASS =\s*'tab-hover-shell group relative flex h-\[30px\] self-center items-center overflow-hidden rounded-full px-3 bg-transparent';/,
  );
  assert.match(source, /export function getTabShellClassName\(isActive: boolean, extraClassName = ''\): string \{/);
  assert.match(source, /TAB_SHELL_CLASS,/);
  assert.match(source, /isActive \? 'tab-active-shell text-highlight' : 'text-secondary-text'/);
  assert.doesNotMatch(source, /after:bg-border/);
  assert.match(stylesSource, /\.tab-active-shell,\s*\.tab-active-shell \.tab-hover-bg-sync\s*\{[^}]*background-color:\s*var\(--color-editor-bg\);[^}]*\}/m);
  assert.match(stylesSource, /\.ui-tabbar \.tab-hover-shell\s*\{[^}]*border-radius:\s*9999px;[^}]*\}/m);
  assert.match(stylesSource, /\.tab-hover-shell,\s*\.tab-hover-bg-sync\s*\{[^}]*border-radius:\s*9999px;[^}]*\}/m);
  assert.match(stylesSource, /--op-tab-hover-bg:\s*color-mix\(in srgb, var\(--color-hover-bg\) 72%, var\(--color-secondary-bg\) 28%\);/);
  assert.match(stylesSource, /\.tab-hover-shell:hover:not\(\.tab-active-shell\),\s*\.tab-hover-shell:hover:not\(\.tab-active-shell\) \.tab-hover-bg-sync\s*\{[^}]*background-color:\s*var\(--op-tab-hover-bg\);[^}]*transition-delay:\s*0\.08s;[^}]*\}/m);
  assert.match(stylesSource, /\.ui-tabbar \.tab-active-shell:hover,\s*\.ui-tabbar \.tab-active-shell:hover \.tab-hover-bg-sync[\s\S]*background-color:\s*var\(--op-tab-hover-bg\);/m);
  assert.match(stylesSource, /\.tab-hover-shell:has\(:focus-visible\):not\(\.tab-active-shell\),\s*\.tab-hover-shell:has\(:focus-visible\):not\(\.tab-active-shell\) \.tab-hover-bg-sync,\s*\.tab-hover-bg-sync:focus-visible\s*\{[^}]*background-color:\s*var\(--op-tab-hover-bg\);[^}]*transition-delay:\s*0s;[^}]*\}/m);
  assert.match(stylesSource, /\.workspace-tab-shell:not\(\.is-active\):hover,\s*\.workspace-tab-shell:not\(\.is-active\):has\(:focus-visible\)\s*\{[^}]*background-color:\s*var\(--op-tab-hover-bg\);[^}]*\}/m);
  assert.match(stylesSource, /--op-sg-accent-ratio:\s*76%;/);
  assert.match(
    stylesSource,
    /\.op-sg-capsule--on-titlebar\s*\{[^}]*--op-sg-capsule-substrate:\s*var\(--color-titlebar-bg\);[^}]*--op-sg-capsule-shadow:\s*var\(--op-sg-shadow-inner\);/m,
  );
  assert.match(
    stylesSource,
    /:root\[data-color-scheme='dark'\]\s*\{[^}]*--op-sg-accent-ratio:\s*22%;[^}]*--op-sg-substrate-ratio:\s*78%;[^}]*--op-sg-text:\s*var\(--color-prime-text\);/m,
  );
  assert.doesNotMatch(
    stylesSource,
    /--op-workspace-tab-active-bg:\s*var\(--op-glass-header-floor-collapsed\);/m,
  );
  assert.match(
    stylesSource,
    /\.workspace-tab-shell\.op-sg-capsule\s*\{[^}]*--op-sg-frost:\s*color-mix\(in srgb, var\(--color-editor-bg\) 80%, white 20%\);[^}]*--op-sg-milk:\s*90%;[^}]*--op-sg-capsule-bg:\s*color-mix\(in srgb, var\(--op-sg-frost\) var\(--op-sg-milk\), transparent\);[^}]*background:\s*var\(--op-sg-capsule-bg\);/m,
  );
  assert.match(
    stylesSource,
    /\.workspace-tab-shell\.op-sg-capsule\s*\{[^}]*border-color:\s*var\(--op-sg-capsule-border\);/m,
  );
});
