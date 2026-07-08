import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const markdownPath = path.join(__dirname, 'ActivityMarkdown.tsx');
const stylesPath = path.join(__dirname, '../../styles/index.css');
const markdownSource = readFileSync(markdownPath, 'utf8');
const stylesSource = readFileSync(stylesPath, 'utf8');

test('activity markdown code blocks wrap inside the activity panel', () => {
  assert.match(markdownSource, /<pre className="op-activity-panel-md-pre">\{children\}<\/pre>/);
  assert.match(stylesSource, /\.op-activity-panel-markdown pre\s*\{[\s\S]*max-width: 100%;[\s\S]*overflow-x: hidden;[\s\S]*box-sizing: border-box;[\s\S]*\}/);
  assert.match(stylesSource, /\.op-activity-panel-markdown pre code\s*\{[\s\S]*white-space: pre-wrap;[\s\S]*overflow-wrap: anywhere;[\s\S]*\}/);
  assert.match(stylesSource, /\.op-activity-panel-md-pre\s*\{[\s\S]*max-width: 100%;[\s\S]*overflow-x: hidden;[\s\S]*box-sizing: border-box;[\s\S]*\}/);
});

test('activity markdown supports highlight syntax through a mark component', () => {
  assert.match(markdownSource, /function remarkHighlight\(\)/);
  assert.match(markdownSource, /findMarkdownHighlightRanges/);
  assert.match(markdownSource, /className=\{buildClassName\('op-md-highlight', className\)\}/);
  assert.match(stylesSource, /\.op-md-highlight,\s*\.op-activity-panel-markdown mark\s*\{[\s\S]*background:\s*var\(--color-preview-highlight-bg\);/);
});

test('activity markdown prefers emoji presentation for symbolic emoji', () => {
  assert.match(stylesSource, /\.op-activity-panel-markdown\s*\{[^}]*--op-emoji-font-family/m);
  assert.match(stylesSource, /\.op-activity-panel-markdown\s*\{[^}]*font-variant-emoji:\s*emoji;/m);
});
