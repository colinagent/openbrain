import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const appPath = path.join(__dirname, 'App.tsx');
const source = readFileSync(appPath, 'utf8');

test('activity panel seeds and persists an 85%-width default once settings hydrate', () => {
  assert.match(source, /const DEFAULT_ACTIVITY_PANEL_WIDTH_RATIO = 0\.85;/);
  assert.match(source, /const activityVisible = Boolean\(selectedConversationTarget\)/m);
  assert.match(source, /const showActivityPanel = activityVisible;/);
  assert.match(source, /const defaultActivityPanelWidth = activityPanelWidth == null/);
  assert.match(source, /!uiSettingsHydrated\s*\|\|\s*!showActivityPanel\s*\|\|\s*activityPanelWidth != null\s*\|\|\s*defaultActivityPanelWidth == null/);
  assert.match(source, /setActivityPanelWidth\(defaultActivityPanelWidth\);/);
  assert.match(source, /activityPanelWidth: defaultActivityPanelWidth,/);
});

test('pinned conversation files do not move the activity panel into the pinned pane', () => {
  assert.doesNotMatch(source, /showActivityPanelInPinnedPane/);
  assert.doesNotMatch(source, /PINNED_ACTIVITY_PANEL_WIDTH_RATIO/);
  assert.doesNotMatch(source, /resolvePinnedActivityPanelWidth/);
  assert.doesNotMatch(source, /pinnedEditorViewportStyle/);
  assert.match(source, /const getActivityPanelAreaElement = \(\) => \{/);
  assert.match(source, /if \(showOpenBrainPage\) \{\s*return openBrainPagePaneRef\.current;\s*\}/);
  assert.match(source, /if \(showMessengerView\) \{\s*return messengerConversationPaneRef\.current;\s*\}/);
  assert.match(source, /return primaryEditorPaneRef\.current;/);
  assert.match(source, /return getActivityPanelAreaElement\(\)\?\.clientWidth\s*\|\|\s*editorAreaRef\.current\?\.clientWidth\s*\|\|\s*window\.innerWidth;/);
  assert.match(source, /const renderActivityPanel = \(\) => \(/);
  assert.equal(source.match(/<ActivityPanel\b/g)?.length ?? 0, 1);
  assert.match(source, /showActivityPanel && \(\s*<div\s*ref={activityPanelStackRef}[\s\S]*?className=\{`absolute bottom-0 left-0 right-0 z-\[50\] overflow-visible pb-3 \$\{showOpenBrainPage \? 'pointer-events-auto' : 'pointer-events-none'\}`\}/);
  assert.match(source, /onTopLeftResizeStart=\{\(event\) => handleActivityPanelCornerResizeStart\('left', event\)\}/);
  assert.match(source, /onHeaderPointerDown=\{handleActivityPanelHeaderPointerDown\}/);
});

test('primary editor bottom safe area is reserved only while the activity panel is collapsed', () => {
  assert.match(source, /const shouldReserveActivityPanelEditorSafeArea = showActivityPanel && !showSpecialPage && !liveOverlay\.expanded;/);
  assert.match(source, /const ACTIVITY_PANEL_COLLAPSED_STACK_MIN_HEIGHT = 62;/);
  assert.match(source, /const activityPanelBottomSafeArea = shouldReserveActivityPanelEditorSafeArea\s*\?\s*Math\.max\(activityPanelCoveredBottom, ACTIVITY_PANEL_COLLAPSED_STACK_MIN_HEIGHT\)\s*\+\s*ACTIVITY_PANEL_EDITOR_TEXT_CLEARANCE\s*:\s*0;/);
  assert.match(source, /ref={activityPanelStackRef}/);
  assert.match(source, /const primaryEditorViewportStyle = \{\s*'--op-editor-bottom-safe-area': `\$\{activityPanelBottomSafeArea\}px`,\s*\} as React\.CSSProperties;/);
  assert.doesNotMatch(source, /pinnedEditorViewportStyle/);
});

test('OpenBrain graph page renders the activity panel overlay', () => {
  assert.match(source, /const activityVisible = Boolean\(selectedConversationTarget\)/m);
  assert.match(
    source,
    /<OpenBrainPage onOpenWorkspace=\{handleOpenBrainWorkspace\} onCreateSource=\{handleCreateOpenBrainSource\} onBindSource=\{handleBindOpenBrainSource\} \/>\s*\{renderActivityPanel\(\)\}/m,
  );
});
