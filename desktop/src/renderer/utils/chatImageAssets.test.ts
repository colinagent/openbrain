import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildChatImageAssetsDir,
  persistChatImageAsset,
  type WriteChatImageAssetFile,
} from './chatImageAssets';
import type { ChatInputImage } from './chatImages';

function image(input?: Partial<ChatInputImage>): ChatInputImage {
  return {
    name: 'Screen Shot 1.png',
    mimeType: 'image/png',
    extension: '.png',
    size: 3,
    file: {} as File,
    base64: 'AAA',
    ...input,
  };
}

test('buildChatImageAssetsDir targets cwd .agent image assets', () => {
  assert.equal(
    buildChatImageAssetsDir('/Users/example/code/sample-workspace/'),
    '/Users/example/code/sample-workspace/.agent/assets/images',
  );
});

test('persistChatImageAsset writes base64 and returns absolute markdown image path', async () => {
  const writes: Array<{ path: string; base64: string; overwrite: boolean | undefined }> = [];
  const writer: WriteChatImageAssetFile = async (path, base64, options) => {
    writes.push({ path, base64, overwrite: options?.overwrite });
    return { success: true };
  };

  const result = await persistChatImageAsset(
    image(),
    '/Users/example/code/sample-workspace',
    writer,
    new Date(2026, 3, 25, 15, 30, 12),
  );

  assert.equal(writes.length, 1);
  assert.equal(writes[0].base64, 'AAA');
  assert.equal(writes[0].overwrite, false);
  assert.match(writes[0].path, /^\/Users\/example\/code\/sample-workspace\/\.agent\/assets\/images\/2026-04-25-153012-[a-f0-9]{8}-Screen-Shot-1\.png$/);
  assert.equal(result.path, writes[0].path);
  assert.equal(result.markdown, `![${writes[0].path.split('/').pop()}](${writes[0].path}){width=10%}`);
});

test('persistChatImageAsset retries with a suffix on name collision', async () => {
  const writes: string[] = [];
  const writer: WriteChatImageAssetFile = async (path) => {
    writes.push(path);
    return { success: writes.length > 1, error: 'file exists' };
  };

  const result = await persistChatImageAsset(
    image(),
    '/Users/example/code/sample-workspace',
    writer,
    new Date(2026, 3, 25, 15, 30, 12),
  );

  assert.equal(writes.length, 2);
  assert.match(writes[1], /-Screen-Shot-1-2\.png$/);
  assert.equal(result.path, writes[1]);
});

test('persistChatImageAsset requires an absolute cwd', async () => {
  await assert.rejects(
    () => persistChatImageAsset(image(), 'relative/workspace', async () => ({ success: true })),
    /Select an agent before adding images/,
  );
});
