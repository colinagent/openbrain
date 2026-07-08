import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const livePreviewSource = readFileSync(path.join(__dirname, 'livePreviewPlugin.ts'), 'utf8');
const parseRefreshSource = readFileSync(path.join(__dirname, 'livePreviewParseRefresh.ts'), 'utf8');
const blockDecorationsSource = readFileSync(path.join(__dirname, 'livePreviewBlockDecorations.ts'), 'utf8');

test('live preview guards replace decorations behind syntaxTreeAvailable', () => {
  assert.match(livePreviewSource, /allowReplaceDecorations/);
  assert.match(livePreviewSource, /if \(decoContext\.allowReplaceDecorations\)/);
  assert.match(livePreviewSource, /resolveLivePreviewReplacePolicy/);
});

test('scroll parse refresh wires forceParsing and viewport decoration refresh', () => {
  assert.match(parseRefreshSource, /ViewPlugin\.fromClass/);
  assert.match(parseRefreshSource, /addEventListener\('scroll'/);
  assert.match(parseRefreshSource, /forceParsing/);
  assert.match(parseRefreshSource, /refreshLivePreviewDecorationsEffect/);
  assert.match(parseRefreshSource, /refreshLivePreviewViewportDecorationsEffect/);
  assert.match(parseRefreshSource, /viewportChanged/);
  assert.match(parseRefreshSource, /syntaxTree\(update\.state\)[\s\S]*this\.scheduleRefresh\(false\)/);
  assert.match(parseRefreshSource, /viewportChanged[\s\S]*this\.scheduleRefresh\(true\)/);
});

test('live preview plugin rebuilds on explicit scroll refresh effects', () => {
  assert.match(livePreviewSource, /refreshLivePreviewDecorationsEffect/);
  assert.match(livePreviewSource, /refreshLivePreviewViewportDecorationsEffect/);
  assert.match(livePreviewSource, /livePreviewRefreshRequested/);
});

test('block cursor decorations refresh on viewport effect', () => {
  assert.match(blockDecorationsSource, /refreshLivePreviewViewportDecorationsEffect/);
  assert.match(blockDecorationsSource, /updateCursorDecorationsForRanges/);
});
