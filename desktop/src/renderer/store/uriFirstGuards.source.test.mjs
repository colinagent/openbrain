import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const rendererRoot = path.resolve(__dirname, '..');

function read(relativePath) {
  return readFileSync(path.join(rendererRoot, relativePath), 'utf8');
}

test('conversation targets no longer expose path-keyed chat identity', () => {
  const files = [
    'App.tsx',
    'components/Chat/ActivityPanel.tsx',
    'components/Chat/ConversationComposerDock.tsx',
    'components/Editor/MarkdownEditor.tsx',
    'components/Editor/TextEditor.tsx',
    'services/chatService.ts',
    'store/chatWorkspaceStore.ts',
    'utils/chatRetarget.ts',
    'utils/chatSelectionSync.ts',
  ];

  for (const file of files) {
    const source = read(file);
    assert.doesNotMatch(source, /kind:\s*['"]chat['"]/, `${file} must not construct path-keyed chat targets`);
    assert.doesNotMatch(source, /\.kind\s*={2,3}\s*['"]chat['"]/, `${file} must not branch on path-keyed chat targets`);
    assert.doesNotMatch(source, /\.kind\s*!={1,2}\s*['"]chat['"]/, `${file} must not branch on path-keyed chat targets`);
  }
});

test('resource render physical path escape hatch is explicitly named', () => {
  assert.doesNotMatch(read('services/resourceService.ts'), /getRenderUrlForPath/);
  assert.match(read('components/Editor/ImageEditor.tsx'), /getRenderUrlForPhysicalPath\(filePath\)/);
});
