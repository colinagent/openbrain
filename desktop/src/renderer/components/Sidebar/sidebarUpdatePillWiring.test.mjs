import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const sidebarSource = readFileSync(
  path.join(__dirname, 'Sidebar.tsx'),
  'utf8',
);

const stylesSource = readFileSync(
  path.join(__dirname, '../../styles/index.css'),
  'utf8',
);

test('Sidebar only shows the update pill when the desktop update is ready or installing', () => {
  assert.match(sidebarSource, /desktopUpdate\?\.phase === 'ready' \|\| desktopUpdate\?\.phase === 'installing'/);
});

test('Sidebar routes the update pill click to the desktop update install API', () => {
  assert.match(sidebarSource, /window\.electronAPI\?\.desktopUpdate\?\.install\?\.\(\)/);
  assert.match(sidebarSource, /className="sidebar-update-pill"/);
  assert.match(sidebarSource, /desktopUpdateLabel = desktopUpdate\?\.phase === 'installing' \? 'Updating…' : 'Update'/);
});

test('Sidebar renders workspace actions on the root directory row', () => {
  assert.match(
    sidebarSource,
    /const workspaceToolbarActions = \([\s\S]*className="sidebar-update-pill"[\s\S]*<RefreshIcon className="w-3\.5 h-3\.5" \/>/,
  );
  assert.match(
    sidebarSource,
    /className="ui-tabbar sidebar-root-header flex shrink-0 items-center gap-1[\s\S]*\{workspaceToolbarActions\}/,
  );
  assert.doesNotMatch(
    sidebarSource,
    /data-onboarding-target="workspace-root-agent-pill"/,
  );
  assert.doesNotMatch(
    sidebarSource,
    /<div className="ui-tabbar flex shrink-0 items-center justify-center overflow-hidden px-2">[\s\S]*\{workspaceToolbarActions\}/,
  );
});

test('Sidebar update pill reuses the capsule styling hook', () => {
  assert.match(stylesSource, /\.sidebar-update-pill\s*\{/);
  assert.match(stylesSource, /@apply ui-capsule-pill font-semibold/);
});
