import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const source = readFileSync(path.join(__dirname, 'useSelectedThreadSnapshotSync.ts'), 'utf8');

test('selected thread snapshot sync runs from App layer and is independent of composer visibility', () => {
  assert.match(source, /export function useSelectedThreadSnapshotSync\(\)/);
  assert.match(source, /selectedConversationTarget\?\.kind !== 'thread'/);
  assert.match(source, /refreshThreadStateByThreadID\(threadID, \{/);
  assert.match(source, /chatPath: selectedChatPath \|\| selectedConversationTarget\.chatPath \|\| null,/);
  assert.match(source, /modelKey: selectedModelKey \|\| null,/);
  assert.doesNotMatch(source, /composerVisible/);
});
