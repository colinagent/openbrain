import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const source = readFileSync(path.join(__dirname, 'AddAgentPopup.tsx'), 'utf8');

test('add agent selector uses popup positioning instead of a centered blocking modal', () => {
  assert.doesNotMatch(source, /useBlockingModal/);
  assert.doesNotMatch(source, /left-1\/2/);
  assert.doesNotMatch(source, /top-1\/2/);
  assert.doesNotMatch(source, /w-\[520px\]/);
  assert.match(source, /w-\[360px\]/);
  assert.match(source, /AddAgentPopup/);
  assert.match(source, /PopupMenu/);
  assert.match(source, /getAddAgentPopupPosition/);
});
