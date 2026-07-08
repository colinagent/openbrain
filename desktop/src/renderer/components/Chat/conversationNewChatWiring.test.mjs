import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const conversationComposerDockPath = path.join(__dirname, 'ConversationComposerDock.tsx');

test('new chat stays pending and does not create a backing chat on click', () => {
  const source = readFileSync(conversationComposerDockPath, 'utf8');
  const handler = source.slice(
    source.indexOf('const handleCreateConversation = useCallback'),
    source.indexOf('const handleAgentSelect = useCallback'),
  );

  assert.match(handler, /createPendingConversation\(\);/);
  assert.match(handler, /focusComposer\(\);/);
  assert.doesNotMatch(handler, /\bcreateChat\b/);
  assert.doesNotMatch(handler, /ensureThreadTab/);
  assert.doesNotMatch(handler, /consumePendingConversation/);
  assert.doesNotMatch(handler, /upsertThreadMeta/);
});
