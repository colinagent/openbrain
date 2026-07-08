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

test('selected skill pill keeps hover feedback on the close icon only', () => {
  const conversationComposerDockSource = read(conversationComposerDockPath);
  const stylesSource = read(stylesPath);
  const uiCapsuleRule = stylesSource.match(/\.ui-capsule-pill\s*\{([^}]*)\}/m);

  assert.ok(uiCapsuleRule, 'expected .ui-capsule-pill CSS rule');
  assert.doesNotMatch(uiCapsuleRule[1], /hover:text-link-text-hover/);
  assert.doesNotMatch(stylesSource, /\.file-tree-agent-badge:hover\s*\{/);
  assert.match(stylesSource, /\.file-tree-agent-pill:hover/);
  assert.match(stylesSource, /\.file-tree-agent-pill:focus-visible/);
  assert.match(stylesSource, /file-tree-agent-pill:hover[\s\S]*color:\s*var\(--color-highlight\)/);
  assert.match(
    conversationComposerDockSource,
    /<CloseButton[\s\S]*title="Remove selected skill"[\s\S]*variant="inline"/m,
  );
});

test('selected skill pill displays only the skill name', () => {
  const conversationComposerDockSource = read(conversationComposerDockPath);
  const pendingResourcesStart = conversationComposerDockSource.indexOf('{pendingResources.length > 0 && (');
  const composerStart = conversationComposerDockSource.indexOf('<div ref={composerShellRef}', pendingResourcesStart);
  const pendingResourcesSection = conversationComposerDockSource.slice(
    pendingResourcesStart,
    composerStart,
  );

  assert.ok(pendingResourcesStart >= 0, 'expected pending resources section');
  assert.ok(composerStart > pendingResourcesStart, 'expected composer after pending resources section');
  assert.doesNotMatch(pendingResourcesSection, /\/\{resource\.skill\.slug\}/);
  assert.match(pendingResourcesSection, /\{resource\.skill\.name\}/);
});
