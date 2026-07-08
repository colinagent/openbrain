import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const source = readFileSync(path.join(__dirname, 'FileTreeChildren.tsx'), 'utf8');

test('file tree agent labels subscribe to async binding and node map updates', () => {
  assert.match(source, /agentBindingByCwd = useAppStore\(\(state\) => state\.agentBindingByCwd\)/);
  assert.match(source, /nodesByID = useAppStore\(\(state\) => state\.nodesByID\)/);
  assert.match(source, /\[visibleChildren, dir, getChatAgentForCwd, agentBindingByCwd, nodesByID\]/);
});

test('.agent agent pill clicks focus the conversation composer', () => {
  assert.match(source, /requestComposerFocus = useChatWorkspaceStore\(\(s\) => s\.requestComposerFocus\)/);
  assert.match(source, /if \(!entry\.isDir \|\| entry\.name !== '\.agent'\)/);
  assert.match(source, /const info = getChatAgentForCwd\(dir\)/);
  assert.match(source, /rightLabel=\{agentLabel\}/);
  assert.match(source, /agentLabelPlacement=\{isAgentMarker \? 'inline' : 'right'\}/);
  assert.match(source, /showConversationComposerDock\(\);\s*setInputMode\('chat'\);\s*createPendingConversation\(\);\s*setAgentForSelectedTarget\(\{\s*agentID: info\.agentID,\s*agentName: info\.agentName \?\? null,\s*agentCwd: info\.agentCwd,\s*\}\);\s*setAgentInfo\(info\.agentID, info\.agentName \?\? null, info\.agentCwd\);\s*requestComposerFocus\(\);/s);
});
