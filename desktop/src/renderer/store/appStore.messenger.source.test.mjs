import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const appStoreSourcePath = new URL('./appStore.ts', import.meta.url);
const appStoreSource = await readFile(appStoreSourcePath, 'utf8');

test('messenger records update both Messenger store and thread snapshots', () => {
  assert.match(appStoreSource, /async function resolveMessengerReplyInput\(input: MessengerReplyInput, workspaceTabId: string\): Promise<MessengerReplyInput> \{/);
  assert.match(appStoreSource, /modelParamsForModelKey\(input\.modelKey\)\s*\|\|\s*modelParamsForModelKey\(selectedModelKey\)\s*\|\|\s*await resolveDefaultChatModelParams\(\)/s);
  assert.match(appStoreSource, /return \{\s*\.\.\.input,\s*\.\.\.modelParams,\s*\};/s);
  assert.match(appStoreSource, /const result = await messengerService\.reply\(await resolveMessengerReplyInput\(input, _tabId\)\);/);
  assert.match(
    appStoreSource,
    /onMessengerMessage: \(message\) => \{\s*useMessengerStore\.getState\(\)\.upsertRecord\(message\);\s*getChatWorkspaceStore\(_tabId\)\.getState\(\)\.upsertThreadMessageRecords\(\[message\]\);\s*\}/s,
  );
  assert.match(
    appStoreSource,
    /const records = result\.resolved\s*\?\s*\[result\.resolved, result\.record\]\s*:\s*\[result\.record\];/s,
  );
  assert.match(
    appStoreSource,
    /for \(const record of records\) \{\s*messengerState\.upsertRecord\(record\);\s*\}/s,
  );
  assert.match(
    appStoreSource,
    /getChatWorkspaceStore\(_tabId\)\.getState\(\)\.upsertThreadMessageRecords\(records\);/s,
  );
});
