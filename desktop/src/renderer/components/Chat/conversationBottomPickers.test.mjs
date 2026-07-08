import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const conversationComposerDockPath = path.join(__dirname, 'ConversationComposerDock.tsx');

function read(filePath) {
  return readFileSync(filePath, 'utf8');
}

test('conversation bottom pickers use the centered-above popup anchor', () => {
  const source = read(conversationComposerDockPath);

  assert.match(
    source,
    /const CHAT_BOTTOM_PICKER_MENU_CLASS_NAME = 'absolute bottom-full left-1\/2 mb-1 -translate-x-1\/2 z-\[?50\]? overflow-hidden';/,
  );
  assert.match(source, /className=\{`\$\{CHAT_BOTTOM_PICKER_MENU_CLASS_NAME\} w-\[280px\]`\}/);
  assert.match(source, /className=\{`\$\{CHAT_BOTTOM_PICKER_MENU_CLASS_NAME\} w-\[320px\]`\}/);
  assert.match(source, /className=\{`\$\{CHAT_BOTTOM_PICKER_MENU_CLASS_NAME\} w-\[560px\]`\}/);
  assert.doesNotMatch(source, /left-full bottom-0 ml-1/);
});

test('agent picker treats empty cwd as the runtime default workspace', () => {
  const source = read(conversationComposerDockPath);
  const handleAgentSelect = source.slice(
    source.indexOf('const handleAgentSelect = useCallback'),
    source.indexOf('const handleSubagentRemove = useCallback'),
  );

  assert.match(source, /const canSwitchAgent = !isCommandMode && Boolean\(effectiveAgentID\);/);
  assert.match(handleAgentSelect, /const targetDir = \(agentSwitchTargetDir \|\| ''\)\.trim\(\);/);
  assert.match(handleAgentSelect, /agentCwd: targetDir,/);
  assert.doesNotMatch(handleAgentSelect, /if \(!targetDir\) \{\s*return;\s*\}/);
});
