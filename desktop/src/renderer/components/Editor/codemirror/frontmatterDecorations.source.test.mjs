import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const sourcePath = new URL('./frontmatterDecorations.ts', import.meta.url);
const source = await readFile(sourcePath, 'utf8');

test('frontmatter bind agent values render through the shared agent mention widget', () => {
  assert.match(source, /parseAgentMentionValue/);
  assert.match(source, /key === 'bind'/);
  assert.match(source, /kind: 'agent'/);
  assert.match(source, /new AgentMentionWidget/);
});

test('frontmatter yaml collapses while properties panel is active', () => {
  assert.match(source, /shouldCollapseFrontmatterYaml/);
  assert.match(source, /cm-md-frontmatter-collapsed/);
  assert.match(source, /toggleFrontmatterSourceModeEffect/);
});
