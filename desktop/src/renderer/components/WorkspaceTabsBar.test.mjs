import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const workspaceTabsBarPath = path.join(__dirname, 'WorkspaceTabsBar.tsx');
const tabLayoutPath = path.join(__dirname, 'tabLayout.ts');

function read(filePath) {
  return readFileSync(filePath, 'utf8');
}

test('WorkspaceTabsBar uses a flat titlebar workspace tab with delayed close reveal', () => {
  const workspaceTabsBarSource = read(workspaceTabsBarPath);
  const tabLayoutSource = read(tabLayoutPath);

  assert.doesNotMatch(workspaceTabsBarSource, /getTabShellClassName/);
  assert.match(workspaceTabsBarSource, /workspace-tab-shell/);
  assert.match(workspaceTabsBarSource, /OP_SG_CAPSULE_ON_TITLEBAR/);
  assert.match(
    workspaceTabsBarSource,
    /isActive \? `is-active \$\{OP_SG_CAPSULE\} \$\{OP_SG_CAPSULE_ON_TITLEBAR\}` : ''/,
  );
  assert.match(workspaceTabsBarSource, /text-secondary-text/);
  assert.doesNotMatch(workspaceTabsBarSource, /ACTIVE_TAB_LABEL_CLASS/);
  assert.match(workspaceTabsBarSource, /TAB_CLOSE_BUTTON_DELAYED_REVEAL_CLASS/);
  assert.match(workspaceTabsBarSource, /getTabCloseButtonClassName/);
  assert.match(workspaceTabsBarSource, /<CloseButton/);
  assert.doesNotMatch(workspaceTabsBarSource, /\{isActive \? \([\s\S]*<CloseButton/);
  assert.match(
    workspaceTabsBarSource,
    /getTabCloseButtonClassName\(\s*'bg-secondary-bg',\s*`\$\{TAB_CLOSE_BUTTON_DELAYED_REVEAL_CLASS\} no-drag`,\s*\)/,
  );
  assert.match(
    workspaceTabsBarSource,
    /className="no-drag flex h-full min-w-0 flex-1 items-center truncate"/,
  );
  assert.match(workspaceTabsBarSource, /className="flex h-full w-full min-w-0 items-center"/);
  assert.match(workspaceTabsBarSource, /className="flex h-full min-w-0 items-center gap-1 overflow-x-auto overflow-y-visible"/);
  assert.match(
    workspaceTabsBarSource,
    /menuOpen \? 'bg-hover-bg text-highlight' : 'text-secondary-text'/,
  );
  assert.doesNotMatch(workspaceTabsBarSource, /shadow-\[0_4px_12px/);
  assert.match(tabLayoutSource, /export const TAB_MIN_WIDTH = 52;/);
});

test('WorkspaceTabsBar plus menu stays local workspace only', () => {
  const workspaceTabsBarSource = read(workspaceTabsBarPath);

  assert.match(workspaceTabsBarSource, /onNewFolder\(\);/);
  assert.match(workspaceTabsBarSource, /onNewRemote\(\);/);
  assert.match(workspaceTabsBarSource, /workspaceTab\.openFolder/);
  assert.match(workspaceTabsBarSource, /workspaceTab\.connectRemote/);
  assert.match(workspaceTabsBarSource, /Recent/);
  assert.match(workspaceTabsBarSource, /recentLocal\.map/);
  assert.match(workspaceTabsBarSource, /recentRemoteGroups\.map/);
  assert.doesNotMatch(workspaceTabsBarSource, /Create with Template/);
  assert.doesNotMatch(workspaceTabsBarSource, /WorkspaceTemplate/);
  assert.doesNotMatch(workspaceTabsBarSource, /listTemplates/);
  assert.doesNotMatch(workspaceTabsBarSource, /Repository owner/);
  assert.doesNotMatch(workspaceTabsBarSource, /Manage GitHub/);
  assert.doesNotMatch(workspaceTabsBarSource, /Storage/);
  assert.doesNotMatch(workspaceTabsBarSource, /OpenBrain Cloud/);
  assert.doesNotMatch(workspaceTabsBarSource, /openStorageBackendSettings/);
  assert.doesNotMatch(workspaceTabsBarSource, /createFromTemplate/);
});

test('WorkspaceTabsBar shows OpenBrain logo on OpenBrain workspace tabs', () => {
  const workspaceTabsBarSource = read(workspaceTabsBarPath);

  assert.match(workspaceTabsBarSource, /useOpenBrainStore/);
  assert.match(workspaceTabsBarSource, /isOpenBrainWorkspacePath/);
  assert.doesNotMatch(workspaceTabsBarSource, new RegExp(['open', 'brainsRoot'].join('')));
  assert.doesNotMatch(workspaceTabsBarSource, /rootPath/);
  assert.match(workspaceTabsBarSource, /const openbrainSources = useOpenBrainStore\(\(s\) => s\.sources\);/);
  assert.match(workspaceTabsBarSource, /openbrainSources\.map\(\(brain\) => normalizeWorkspacePath\(brain\.path\)\)/);
  assert.match(workspaceTabsBarSource, /const \{ label, workspacePath \} = useTabDisplayState\(tab\);/);
  assert.match(workspaceTabsBarSource, /const isOpenBrain = tab\.kind === 'local' && isOpenBrainWorkspacePath\(workspacePath, openbrainPaths\);/);
  assert.match(workspaceTabsBarSource, /<OpenBrainLogo[\s\S]*title="OpenBrain workspace"/);
});

test('WorkspaceTabsBar regenerates Untitled labels from current local workspace dir', () => {
  const workspaceTabsBarSource = read(workspaceTabsBarPath);

  assert.match(workspaceTabsBarSource, /function shouldRegenerateWorkspaceTabLabel/);
  assert.match(workspaceTabsBarSource, /const currentDir = useStore\(store, \(s\) => s\.currentDir\);/);
  assert.match(workspaceTabsBarSource, /const workspacePath = tab\.workspacePath \|\| currentDir \|\| undefined;/);
  assert.match(
    workspaceTabsBarSource,
    /shouldRegenerateWorkspaceTabLabel\(tab\.label\)[\s\S]*\? getPathBaseName\(workspacePath\) \|\| tab\.label \|\| 'Untitled'/,
  );
});
