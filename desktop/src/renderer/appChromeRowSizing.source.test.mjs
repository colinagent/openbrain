import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const styles = readFileSync(path.join(__dirname, 'styles/index.css'), 'utf8');
const sidebarSource = readFileSync(path.join(__dirname, 'components/Sidebar/Sidebar.tsx'), 'utf8');
const agentsSidebarSource = readFileSync(path.join(__dirname, 'components/Sidebar/AgentsSidebar.tsx'), 'utf8');
const baseDirResourceSidebarSource = readFileSync(path.join(__dirname, 'components/Sidebar/BaseDirResourceSidebar.tsx'), 'utf8');
const openBrainSidebarSource = readFileSync(path.join(__dirname, 'components/Sidebar/OpenBrainSidebar.tsx'), 'utf8');
const editorTabBarSource = readFileSync(path.join(__dirname, 'components/Editor/EditorTabBar.tsx'), 'utf8');

test('chrome row tokens keep the activity rail as the sizing reference', () => {
  assert.match(styles, /--op-ui-chrome-row-icon-size:\s*20px;/);
  assert.match(styles, /--op-ui-chrome-row-button-size:\s*34px;/);
  assert.match(styles, /\.sidebar-root-header \.ui-chrome-row-label\s*\{\s*font-size:\s*var\(--op-ui-sidebar-font-size\);/m);
  assert.match(styles, /\.sidebar-root-header \.ui-chrome-row-label\s*\{[\s\S]*font-weight:\s*400;[\s\S]*line-height:\s*1\.35;/m);
  assert.match(styles, /\.openbrain-sidebar-header \.ui-chrome-row-label\s*\{\s*font-size:\s*12px;/m);
  assert.match(styles, /\.sidebar-root-header \.sidebar-root-header-actions \.icon-button-toolbar\s*\{\s*width:\s*18px;/m);
});

test('sidebar keeps the root header at file-tree scale while the activity rail stays large', () => {
  assert.match(sidebarSource, /className="ui-tabbar sidebar-root-header flex shrink-0 items-center gap-1/);
  assert.doesNotMatch(sidebarSource, /<FileTreeFolderIcon/);
  assert.match(sidebarSource, /size=\{34\}/);
  assert.match(sidebarSource, /<WorkspaceIcon className="w-5 h-5" \/>/);
  assert.match(sidebarSource, /<OpenBrainLogo className="h-5 w-5" monochrome \/>/);
  assert.match(sidebarSource, /<RefreshIcon className="w-3\.5 h-3\.5" \/>/);
  assert.match(editorTabBarSource, /ui-chrome-row-label flex-1 min-w-0 truncate/);
  assert.match(editorTabBarSource, /<PlusIcon className="w-3\.5 h-3\.5" \/>/);
});

test('secondary sidebar panels reuse the workspace root header chrome', () => {
  for (const source of [agentsSidebarSource, baseDirResourceSidebarSource]) {
    assert.match(source, /ui-tabbar sidebar-root-header flex shrink-0 items-center gap-1/);
    assert.match(source, /sidebar-root-header-actions ml-auto flex shrink-0 items-center gap-0\.5/);
    assert.match(source, /ui-chrome-row-label truncate/);
  }
  assert.match(openBrainSidebarSource, /ui-tabbar sidebar-root-header openbrain-sidebar-header flex shrink-0 items-center gap-1/);
  assert.match(openBrainSidebarSource, /sidebar-root-header-actions ml-auto flex shrink-0 items-center gap-0\.5/);
  assert.match(openBrainSidebarSource, /ui-chrome-row-label truncate/);
  assert.doesNotMatch(agentsSidebarSource, /<FileTreeRow/);
  assert.doesNotMatch(baseDirResourceSidebarSource, /<FileTreeRow/);
  assert.match(agentsSidebarSource, /rootHeaderDropActive/);
  assert.match(baseDirResourceSidebarSource, /rootHeaderDropActive/);
});
