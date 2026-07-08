import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const appPath = path.join(__dirname, 'App.tsx');

function read(filePath) {
  return readFileSync(filePath, 'utf8');
}

test('app layout applies hover delay to sidebar and conversation resize dividers', () => {
  const source = read(appPath);

  assert.match(source, /const RESIZE_DIVIDER_HOVER_DELAY_MS = 80;/);
  assert.match(
    source,
    /<ResizeDivider\s+direction="vertical"\s+onResizeStart=\{handleSidebarResizeStart\}\s+activeColor="var\(--color-highlight\)"\s+restingColor="var\(--op-sidebar-resize-divider\)"\s+hoverDelayMs=\{RESIZE_DIVIDER_HOVER_DELAY_MS\}\s*\/>/m,
  );
  assert.match(
    source,
    /<ResizeDivider\s+direction="horizontal"\s+onResizeStart=\{handleConversationComposerDockResizeStart\}\s+activeColor="var\(--color-highlight\)"\s+hoverDelayMs=\{RESIZE_DIVIDER_HOVER_DELAY_MS\}\s*\/>/m,
  );
});
