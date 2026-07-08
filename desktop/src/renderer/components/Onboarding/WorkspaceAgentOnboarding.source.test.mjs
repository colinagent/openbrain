import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const onboardingSource = readFileSync(path.join(__dirname, 'WorkspaceAgentOnboarding.tsx'), 'utf8');
const domSource = readFileSync(path.join(__dirname, 'workspaceAgentOnboardingDom.ts'), 'utf8');
const appSource = readFileSync(path.join(__dirname, '../../App.tsx'), 'utf8');
const sidebarSource = readFileSync(path.join(__dirname, '../Sidebar/Sidebar.tsx'), 'utf8');
const fileTreeItemSource = readFileSync(path.join(__dirname, '../FileExplorer/FileTreeItem.tsx'), 'utf8');
const fileTreeChildrenSource = readFileSync(path.join(__dirname, '../FileExplorer/FileTreeChildren.tsx'), 'utf8');
const stylesSource = readFileSync(path.join(__dirname, '../../styles/index.css'), 'utf8');

test('workspace agent onboarding is mounted at the app shell', () => {
  assert.match(appSource, /import \{ WorkspaceAgentOnboarding \} from '\.\/components\/Onboarding\/WorkspaceAgentOnboarding';/);
  assert.match(appSource, /\{!showSpecialPage \? <WorkspaceAgentOnboarding \/> : null\}/);
});

test('workspace agent onboarding targets directory agent pills', () => {
  assert.doesNotMatch(sidebarSource, /data-onboarding-target="workspace-root-agent-pill"/);
  assert.match(fileTreeItemSource, /data-onboarding-target="workspace-dir-agent-pill"/);
  assert.doesNotMatch(domSource, /workspace-root-agent-pill/);
  assert.match(domSource, /workspace-dir-agent-pill/);
});

test('directory agent pill click focuses the conversation composer', () => {
  assert.match(fileTreeChildrenSource, /requestComposerFocus = useChatWorkspaceStore\(\(s\) => s\.requestComposerFocus\)/);
  assert.match(fileTreeChildrenSource, /setAgentForSelectedTarget = useChatWorkspaceStore\(\(s\) => s\.setAgentForSelectedTarget\)/);
  assert.match(fileTreeChildrenSource, /createPendingConversation\(\);\s*setAgentForSelectedTarget\(\{\s*agentID: info\.agentID,\s*agentName: info\.agentName \?\? null,\s*agentCwd: info\.agentCwd,\s*\}\);\s*setAgentInfo\(info\.agentID, info\.agentName \?\? null, info\.agentCwd\);\s*requestComposerFocus\(\);/s);
});

test('workspace agent onboarding persists dismissal through ui settings', () => {
  assert.match(domSource, /workspaceAgentOnboardingSeen\?: unknown/);
  assert.match(onboardingSource, /workspaceAgentOnboardingSeen: true/);
  assert.match(onboardingSource, /settingsApi\.onChanged/);
});

test('workspace agent onboarding keeps the sidebar agent target visible during the guide', () => {
  assert.match(stylesSource, /\.workspace-agent-onboarding-active \.sidebar-hover-area \.file-tree-item-right,/);
  assert.match(stylesSource, /\.workspace-agent-onboarding-active \.sidebar-hover-area \.file-tree-agent-inline-pill,/);
  assert.match(stylesSource, /\.workspace-agent-onboarding-ring\s*\{/);
  assert.match(stylesSource, /\.workspace-agent-onboarding-card\s*\{/);
});
