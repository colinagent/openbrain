import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const diffSource = readFileSync(path.join(__dirname, 'utils/reviewTableDiff.ts'), 'utf8');
const widgetSource = readFileSync(path.join(__dirname, 'widgets/ReviewTableDiffWidget.ts'), 'utf8');
const livePreviewSource = readFileSync(path.join(__dirname, 'livePreviewBlockDecorations.ts'), 'utf8');
const cssSource = readFileSync(path.join(__dirname, '../../../styles/index.css'), 'utf8');

test('review table diff is computed from existing hunk removed and added rows', () => {
  assert.match(diffSource, /buildReviewTableDiffForBlock/);
  assert.match(diffSource, /removedLines/);
  assert.match(diffSource, /addedLines/);
  assert.match(diffSource, /buildPairedRow/);
  assert.match(diffSource, /buildRemovedRow/);
  assert.match(diffSource, /buildAddedRow/);
});

test('live preview uses a single review table diff widget instead of a separate old table', () => {
  assert.match(livePreviewSource, /ReviewTableDiffWidget/);
  assert.match(livePreviewSource, /buildReviewTableDiffForBlock\(normalizeReviewHunks\(reviewOverlay\), parsed, block\)/);
  assert.match(widgetSource, /cm-md-table-review-block/);
  assert.match(widgetSource, /cm-review-table-cell-old/);
  assert.match(widgetSource, /cm-review-table-cell-new/);
});

test('review table diff has added removed and modified visual states', () => {
  assert.match(cssSource, /cm-review-table-cell-added/);
  assert.match(cssSource, /cm-review-table-cell-removed/);
  assert.match(cssSource, /cm-review-table-cell-modified/);
});
