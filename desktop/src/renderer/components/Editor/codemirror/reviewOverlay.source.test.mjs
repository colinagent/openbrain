import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const source = readFileSync(path.join(__dirname, 'reviewOverlay.ts'), 'utf8');
const themeSource = readFileSync(path.join(__dirname, 'theme.ts'), 'utf8');

test('review overlay renders structured hunks and a single floating file toolbar', () => {
  assert.match(source, /export type ReviewHunk = \{/);
  assert.match(source, /class ReviewHunkWidget extends WidgetType/);
  assert.match(source, /class ReviewFileToolbarPlugin/);
  assert.match(source, /cm-review-removed-block/);
  assert.match(source, /cm-review-hunk-count/);
  assert.match(source, /cm-review-file-toolbar/);
  assert.match(source, /ViewPlugin\.fromClass\(ReviewFileToolbarPlugin\)/);
  assert.match(source, /view\.dom\.appendChild\(this\.dom\)/);
  assert.match(source, /this\.createDecisionButton\('Undo file', 'undoFile'\)/);
  assert.match(source, /this\.createDecisionButton\('Keep file', 'keepFile'\)/);
  assert.doesNotMatch(source, /Add to Chat/);
  assert.doesNotMatch(source, /onAddToChat/);
  assert.doesNotMatch(source, /class ReviewFileToolbarWidget extends WidgetType/);
});

test('review overlay file action toolbar is fixed at the editor top center', () => {
  assert.match(themeSource, /'\.cm-review-file-toolbar': \{/);
  assert.match(themeSource, /position: 'absolute'/);
  assert.match(themeSource, /top: '8px'/);
  assert.match(themeSource, /left: '50%'/);
  assert.match(themeSource, /justifyContent: 'center'/);
  assert.match(themeSource, /transform: 'translateX\(-50%\)'/);
  assert.match(themeSource, /backgroundColor: 'transparent'/);
  assert.match(themeSource, /boxShadow: 'none'/);
  assert.match(themeSource, /'\.cm-review-file-toolbar\.is-visible': \{/);
});

test('review overlay file action hover highlights text without changing background', () => {
  const hoverStart = themeSource.indexOf("'.cm-review-action:hover:not(:disabled)': {");
  const disabledStart = themeSource.indexOf("'.cm-review-action:disabled': {");
  assert.ok(hoverStart >= 0, 'review action hover style should exist');
  assert.ok(disabledStart > hoverStart, 'disabled style should follow hover style');
  const hoverSource = themeSource.slice(hoverStart, disabledStart);
  assert.match(hoverSource, /color: cssVar\('highlight'\)/);
  assert.doesNotMatch(hoverSource, /backgroundColor/);
});

test('review overlay hunk widgets do not add dark panel backgrounds', () => {
  const widgetStart = themeSource.indexOf("'.cm-review-hunk-widget': {");
  const headerStart = themeSource.indexOf("'.cm-review-hunk-header': {");
  const countStart = themeSource.indexOf("'.cm-review-hunk-count': {");
  assert.ok(widgetStart >= 0, 'hunk widget style should exist');
  assert.ok(headerStart > widgetStart, 'hunk header style should follow widget style');
  assert.ok(countStart > headerStart, 'hunk count style should follow header style');
  const widgetSource = themeSource.slice(widgetStart, headerStart);
  const headerSource = themeSource.slice(headerStart, countStart);
  assert.match(widgetSource, /backgroundColor: 'transparent'/);
  assert.match(headerSource, /backgroundColor: 'transparent'/);
  assert.doesNotMatch(widgetSource, /backgroundColor: 'rgba\(32, 32, 36,/);
  assert.doesNotMatch(headerSource, /backgroundColor: 'rgba\(0, 0, 0,/);
});

test('review overlay hunk widgets avoid vertical margins that break editor coordinates', () => {
  const widgetStart = themeSource.indexOf("'.cm-review-hunk-widget': {");
  const headerStart = themeSource.indexOf("'.cm-review-hunk-header': {");
  assert.ok(widgetStart >= 0, 'hunk widget style should exist');
  assert.ok(headerStart > widgetStart, 'hunk header style should follow widget style');
  const widgetSource = themeSource.slice(widgetStart, headerStart);
  assert.match(widgetSource, /margin: '0'/);
  assert.doesNotMatch(widgetSource, /margin: '\d+px 0/);
  assert.doesNotMatch(widgetSource, /margin: '0 0 \d+px/);
  assert.doesNotMatch(widgetSource, /var\(--op-md-line-padding-x\)/);
});

test('review overlay removed hunks use a light red background without dark panels', () => {
  const removedBlockStart = themeSource.indexOf("'.cm-review-removed-block': {");
  const removedLineStart = themeSource.indexOf("'.cm-review-removed-line': {");
  assert.ok(removedBlockStart >= 0, 'removed block style should exist');
  assert.ok(removedLineStart > removedBlockStart, 'removed line style should follow removed block style');
  const removedBlockSource = themeSource.slice(removedBlockStart, removedLineStart);
  assert.match(removedBlockSource, /backgroundColor: 'rgba\(185, 75, 95, 0\.14\)'/);
  assert.match(removedBlockSource, /boxShadow: 'inset 3px 0 0 0 rgba\(185, 75, 95,/);
  assert.doesNotMatch(removedBlockSource, /backgroundColor: 'rgba\(32, 32, 36,/);
});

test('review overlay does not render removed markdown table rows as separate table fragments', () => {
  assert.doesNotMatch(source, /buildRemovedSegments/);
  assert.doesNotMatch(source, /cm-review-removed-table-wrapper/);
  assert.doesNotMatch(themeSource, /'\.cm-review-removed-table-wrapper': \{/);
  assert.doesNotMatch(themeSource, /'\.cm-review-removed-table td': \{/);
  assert.match(source, /isReviewHunkHandledByTablePreview/);
});

test('review overlay keeps changed lines highlighted and falls back to changed ranges', () => {
  assert.match(source, /cm-review-added-line cm-review-new-line/);
  assert.match(source, /normalizeReviewRanges\(overlay\.changedRanges\)\.map/);
  assert.match(source, /Decoration\.set\(ranges, true\)/);
});
