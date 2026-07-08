import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const welcomeEditorPath = path.join(__dirname, 'WelcomeEditor.tsx');

test('WelcomeEditor does not render the site label', () => {
  const source = readFileSync(welcomeEditorPath, 'utf8');

  assert.doesNotMatch(source, /openbrain\.io/);
  assert.doesNotMatch(source, /text-highlight/);
});

test('WelcomeEditor keeps the brand mark centered in the visible editor pane', () => {
  const source = readFileSync(welcomeEditorPath, 'utf8');

  assert.match(source, /chatPanelBottomInset\?: number;/);
  assert.match(source, /chatPanelOpen\?: boolean;/);
  assert.match(source, /chatPanelOpen \? 0 : -chatPanelBottomInset \/ 2/);
  assert.match(source, /transform: `translateY\(\$\{translateY\}px\)`/);
  assert.doesNotMatch(source, /BRAND_MARK_OFFSET_/);
});
