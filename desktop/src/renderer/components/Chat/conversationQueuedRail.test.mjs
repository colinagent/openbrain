import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const conversationComposerDockPath = path.join(__dirname, 'ConversationComposerDock.tsx');
const stylesPath = path.join(__dirname, '../../styles/index.css');

function read(filePath) {
  return readFileSync(filePath, 'utf8');
}

test('queued messages render as a compact single-line capsule rail', () => {
  const source = read(conversationComposerDockPath);

  assert.match(source, /className="conversation-queued-rail"/);
  assert.match(source, /className="conversation-queued-count"/);
  assert.match(source, /className="conversation-queued-scroll"/);
  assert.match(source, /className="conversation-queued-list"/);
  assert.match(source, /className="conversation-queued-chip"/);
  assert.match(source, /className="conversation-queued-chip-text"/);
  assert.match(source, /Queued \{flattenedQueuedMessages\.length\}/);
  assert.match(source, /message\.kind === 'follow_up' && \(/);
  assert.match(source, /const actionsDisabled = message\.pending === true;/);
  assert.match(source, /disabled=\{actionsDisabled\}/);
  assert.match(source, /return 'Queued message';/);
  assert.doesNotMatch(source, /conversation-queued-panel/);
  assert.doesNotMatch(source, /conversation-queued-images/);
});

test('queued rail styles enforce nowrap scrolling and hover-only actions', () => {
  const styles = read(stylesPath);

  assert.match(
    styles,
    /\.conversation-queued-scroll\s*\{[\s\S]*overflow-x:\s*auto;[\s\S]*overflow-y:\s*hidden;/m,
  );
  assert.match(
    styles,
    /\.conversation-queued-list\s*\{[\s\S]*flex-wrap:\s*nowrap;[\s\S]*width:\s*max-content;/m,
  );
  assert.match(
    styles,
    /\.conversation-queued-chip\s*\{[\s\S]*@apply ui-capsule-pill;[\s\S]*max-width:\s*min\(320px, calc\(100vw - 180px\)\);/m,
  );
  assert.match(
    styles,
    /\.conversation-queued-chip-text\s*\{[\s\S]*text-overflow:\s*ellipsis;[\s\S]*white-space:\s*nowrap;/m,
  );
  assert.match(
    styles,
    /\.conversation-queued-actions\s*\{[\s\S]*opacity:\s*0;[\s\S]*pointer-events:\s*none;/m,
  );
  assert.match(
    styles,
    /\.conversation-queued-item:hover \.conversation-queued-actions,\s*\.conversation-queued-item:focus-within \.conversation-queued-actions\s*\{[\s\S]*opacity:\s*1;[\s\S]*pointer-events:\s*auto;/m,
  );
  assert.doesNotMatch(styles, /\.conversation-queued-panel\s*\{/m);
});
