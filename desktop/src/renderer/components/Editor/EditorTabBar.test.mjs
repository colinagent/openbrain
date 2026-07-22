import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const editorTabBarPath = path.join(__dirname, 'EditorTabBar.tsx');
const tabLayoutPath = path.join(__dirname, '../tabLayout.ts');
const stylesPath = path.join(__dirname, '../../styles/index.css');

function read(filePath) {
  return readFileSync(filePath, 'utf8');
}

test('EditorTabBar uses the shared tab shell and no separator pseudo-element', () => {
  const editorTabBarSource = read(editorTabBarPath);
  const tabLayoutSource = read(tabLayoutPath);

  assert.match(editorTabBarSource, /getTabShellClassName/);
  assert.match(editorTabBarSource, /className=\{getTabShellClassName\(isActive, 'cursor-pointer'\)\}/);
  assert.match(editorTabBarSource, /getTabCloseButtonClassName\(/);
  assert.match(editorTabBarSource, /TAB_CLOSE_BUTTON_DELAYED_REVEAL_CLASS/);
  assert.match(editorTabBarSource, /className="ui-tabbar editor-tabbar flex items-center gap-2 px-2/);
  assert.doesNotMatch(editorTabBarSource, /bg-sidebar-bg/);
  assert.match(editorTabBarSource, /className="flex min-w-0 items-center gap-1 overflow-x-auto"/);
  assert.doesNotMatch(editorTabBarSource, /after:bg-border/);
  assert.doesNotMatch(editorTabBarSource, /last:after:hidden/);
  assert.match(tabLayoutSource, /tab-hover-shell/);
  assert.match(tabLayoutSource, /tab-hover-bg-sync/);
  assert.match(
    read(stylesPath),
    /\.ui-tabbar \.tab-hover-shell\s*\{[^}]*border-radius:\s*9999px;[^}]*\}/m,
  );
  assert.doesNotMatch(
    read(stylesPath),
    /\.ui-tabbar \.tab-hover-shell\.tab-active-shell\s*\{/,
  );
});

test('EditorTabBar uses editor surface with a bottom divider', () => {
  const styles = read(stylesPath);

  assert.match(
    styles,
    /\.editor-tabbar\s*\{\s*background:\s*var\(--color-editor-bg\);\s*border-bottom:\s*1px solid var\(--op-faint-divider\);\s*\}/s,
  );
  assert.match(
    styles,
    /\.ui-tabbar \.tab-active-shell:hover,\s*\.ui-tabbar \.tab-active-shell:hover \.tab-hover-bg-sync[\s\S]*background-color:\s*var\(--op-tab-hover-bg\);/s,
  );
  assert.doesNotMatch(styles, /\.ui-tabbar\s*\{[^}]*border-bottom:/m);
  assert.doesNotMatch(styles, /\.ui-tabbar\s*\{[^}]*linear-gradient\(/m);
});

test('EditorTabBar renders primary editor tabs without conversation tabs', () => {
  const source = read(editorTabBarPath);

  assert.match(source, /const documents = useAppStore\(\(state\) => state\.documents\);/);
  assert.match(source, /getEditorDocuments\(documents\)/);
  assert.doesNotMatch(source, /const tabs = documents;/);
  assert.match(source, /tabs\.map\(\(tab\) => \{/);
  assert.doesNotMatch(source, />\s*Save\s*</);
  assert.doesNotMatch(source, /Save \(Ctrl\+S\)/);
  assert.doesNotMatch(source, /activeVisible && isDirty/);
});

test('EditorTabBar shows a status pin icon for the pinned editor tab', () => {
  const source = read(editorTabBarPath);

  assert.match(source, /const pinnedTabId = useAppStore\(\(state\) => state\.pinnedTabId\);/);
  assert.match(source, /const isPinned = tab\.id === pinnedTabId;/);
  assert.match(source, /\{isPinned && \(/);
  assert.match(source, /<PinIcon/);
});

test('EditorTabBar does not render a pin action in the tab strip', () => {
  const source = read(editorTabBarPath);

  assert.doesNotMatch(source, /Pin file to right/);
  assert.doesNotMatch(source, /Unpin file/);
  assert.doesNotMatch(source, /togglePinnedTab/);
});

test('EditorTabBar empty state keeps only the new tab action', () => {
  const source = read(editorTabBarPath);

  assert.match(source, /if \(tabs\.length === 0\)/);
  assert.doesNotMatch(source, /No editors/);
  assert.match(source, /aria-label="New tab"/);
});
