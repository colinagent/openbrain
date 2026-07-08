import assert from 'node:assert/strict';
import test from 'node:test';

import {
  ACTIVITY_PANEL_MIN_WIDTH,
  PINNED_FILE_PANEL_MAX_WIDTH,
  PINNED_FILE_PANEL_MIN_WIDTH,
  CONVERSATION_COMPOSER_DOCK_MIN_HEIGHT,
  DEFAULT_PINNED_FILE_PANEL_WIDTH,
  DEFAULT_CONVERSATION_COMPOSER_DOCK_HEIGHT,
  DEFAULT_MARKDOWN_CONTENT_WIDTH,
  DEFAULT_MARKDOWN_TEXT_OFFSET,
  MARKDOWN_CONTENT_WIDTH_MIN,
  MARKDOWN_TEXT_OFFSET_MIN,
  normalizePinnedFilePanelWidth,
  normalizeUiSettings,
  normalizeActivityPanelWidth,
  normalizeConversationComposerDockHeight,
  normalizeMarkdownContentWidth,
  normalizeMarkdownTextOffset,
  migrateThemeId,
  migrateLegacyThemeId,
  normalizeIdleSleepPolicy,
  getIdleSleepPolicy,
} from './settingsStore';
import {
  DEFAULT_WINDOW_ZOOM_LEVEL,
  formatWindowZoomPercent,
  MAX_WINDOW_ZOOM_PERCENT,
  MIN_WINDOW_ZOOM_PERCENT,
} from '../shared/windowZoom';

test('markdownTextOffset falls back to default for invalid values', () => {
  assert.equal(normalizeMarkdownTextOffset(undefined), DEFAULT_MARKDOWN_TEXT_OFFSET);
  assert.equal(normalizeMarkdownTextOffset(null), DEFAULT_MARKDOWN_TEXT_OFFSET);
  assert.equal(normalizeMarkdownTextOffset('60'), DEFAULT_MARKDOWN_TEXT_OFFSET);
  assert.equal(normalizeMarkdownTextOffset(Number.NaN), DEFAULT_MARKDOWN_TEXT_OFFSET);
});

test('markdownTextOffset clamps to the configured minimum', () => {
  assert.equal(normalizeMarkdownTextOffset(MARKDOWN_TEXT_OFFSET_MIN - 1), MARKDOWN_TEXT_OFFSET_MIN);
  assert.equal(normalizeMarkdownTextOffset(-100), MARKDOWN_TEXT_OFFSET_MIN);
});

test('markdownTextOffset keeps in-range and larger finite values unchanged', () => {
  assert.equal(normalizeMarkdownTextOffset(DEFAULT_MARKDOWN_TEXT_OFFSET), DEFAULT_MARKDOWN_TEXT_OFFSET);
  assert.equal(normalizeMarkdownTextOffset(120), 120);
  assert.equal(normalizeMarkdownTextOffset(999), 999);
});

test('markdownContentWidth falls back to default for invalid values', () => {
  assert.equal(normalizeMarkdownContentWidth(undefined), DEFAULT_MARKDOWN_CONTENT_WIDTH);
  assert.equal(normalizeMarkdownContentWidth(null), DEFAULT_MARKDOWN_CONTENT_WIDTH);
  assert.equal(normalizeMarkdownContentWidth('882'), DEFAULT_MARKDOWN_CONTENT_WIDTH);
  assert.equal(normalizeMarkdownContentWidth(Number.NaN), DEFAULT_MARKDOWN_CONTENT_WIDTH);
});

test('markdownContentWidth clamps to the configured minimum', () => {
  assert.equal(normalizeMarkdownContentWidth(MARKDOWN_CONTENT_WIDTH_MIN - 1), MARKDOWN_CONTENT_WIDTH_MIN);
  assert.equal(normalizeMarkdownContentWidth(-100), MARKDOWN_CONTENT_WIDTH_MIN);
});

test('markdownContentWidth keeps in-range and larger finite values unchanged', () => {
  assert.equal(normalizeMarkdownContentWidth(DEFAULT_MARKDOWN_CONTENT_WIDTH), DEFAULT_MARKDOWN_CONTENT_WIDTH);
  assert.equal(normalizeMarkdownContentWidth(1200), 1200);
  assert.equal(normalizeMarkdownContentWidth(4096), 4096);
});

test('conversationComposerDockHeight falls back to default for invalid values', () => {
  assert.equal(normalizeConversationComposerDockHeight(undefined), DEFAULT_CONVERSATION_COMPOSER_DOCK_HEIGHT);
  assert.equal(normalizeConversationComposerDockHeight(null), DEFAULT_CONVERSATION_COMPOSER_DOCK_HEIGHT);
  assert.equal(normalizeConversationComposerDockHeight('160'), DEFAULT_CONVERSATION_COMPOSER_DOCK_HEIGHT);
  assert.equal(normalizeConversationComposerDockHeight(Number.NaN), DEFAULT_CONVERSATION_COMPOSER_DOCK_HEIGHT);
});

test('conversationComposerDockHeight clamps to the configured minimum', () => {
  assert.equal(normalizeConversationComposerDockHeight(CONVERSATION_COMPOSER_DOCK_MIN_HEIGHT - 1), CONVERSATION_COMPOSER_DOCK_MIN_HEIGHT);
  assert.equal(normalizeConversationComposerDockHeight(-100), CONVERSATION_COMPOSER_DOCK_MIN_HEIGHT);
});

test('conversationComposerDockHeight keeps in-range and larger finite values unchanged', () => {
  assert.equal(normalizeConversationComposerDockHeight(DEFAULT_CONVERSATION_COMPOSER_DOCK_HEIGHT), DEFAULT_CONVERSATION_COMPOSER_DOCK_HEIGHT);
  assert.equal(normalizeConversationComposerDockHeight(240), 240);
  assert.equal(normalizeConversationComposerDockHeight(999), 999);
});

test('pinnedFilePanelWidth falls back to default for invalid values', () => {
  assert.equal(normalizePinnedFilePanelWidth(undefined), DEFAULT_PINNED_FILE_PANEL_WIDTH);
  assert.equal(normalizePinnedFilePanelWidth(null), DEFAULT_PINNED_FILE_PANEL_WIDTH);
  assert.equal(normalizePinnedFilePanelWidth('420'), DEFAULT_PINNED_FILE_PANEL_WIDTH);
  assert.equal(normalizePinnedFilePanelWidth(Number.NaN), DEFAULT_PINNED_FILE_PANEL_WIDTH);
});

test('pinnedFilePanelWidth clamps to configured bounds', () => {
  assert.equal(
    normalizePinnedFilePanelWidth(PINNED_FILE_PANEL_MIN_WIDTH - 1),
    PINNED_FILE_PANEL_MIN_WIDTH
  );
  assert.equal(
    normalizePinnedFilePanelWidth(PINNED_FILE_PANEL_MAX_WIDTH + 1),
    PINNED_FILE_PANEL_MAX_WIDTH
  );
});

test('pinnedFilePanelWidth keeps in-range finite values unchanged', () => {
  assert.equal(normalizePinnedFilePanelWidth(DEFAULT_PINNED_FILE_PANEL_WIDTH), DEFAULT_PINNED_FILE_PANEL_WIDTH);
  assert.equal(normalizePinnedFilePanelWidth(560), 560);
});

test('activityPanelWidth falls back to auto width for invalid values', () => {
  assert.equal(normalizeActivityPanelWidth(undefined), undefined);
  assert.equal(normalizeActivityPanelWidth(null), undefined);
  assert.equal(normalizeActivityPanelWidth('640'), undefined);
  assert.equal(normalizeActivityPanelWidth(Number.NaN), undefined);
});

test('activityPanelWidth clamps to the configured minimum', () => {
  assert.equal(normalizeActivityPanelWidth(ACTIVITY_PANEL_MIN_WIDTH - 1), ACTIVITY_PANEL_MIN_WIDTH);
  assert.equal(normalizeActivityPanelWidth(-100), ACTIVITY_PANEL_MIN_WIDTH);
});

test('activityPanelWidth keeps in-range and larger finite values unchanged', () => {
  assert.equal(normalizeActivityPanelWidth(ACTIVITY_PANEL_MIN_WIDTH), ACTIVITY_PANEL_MIN_WIDTH);
  assert.equal(normalizeActivityPanelWidth(640), 640);
  assert.equal(normalizeActivityPanelWidth(999), 999);
});

test('ui settings default activityPanelMaxHeight is 400', () => {
  assert.equal(normalizeUiSettings({ version: 1, themeId: 'default-light' }).activityPanelMaxHeight, 400);
});

test('workspace agent onboarding defaults to unseen and preserves seen state', () => {
  assert.equal(normalizeUiSettings({ version: 1, themeId: 'default-light' }).workspaceAgentOnboardingSeen, false);
  assert.equal(
    normalizeUiSettings({
      version: 1,
      themeId: 'default-light',
      workspaceAgentOnboardingSeen: true,
    }).workspaceAgentOnboardingSeen,
    true
  );
});

test('chatThinkingLevel falls back to off for invalid values', () => {
  assert.equal(normalizeUiSettings({ version: 1, themeId: 'default-light' }).chatThinkingLevel, 'off');
  assert.equal(normalizeUiSettings({ version: 1, themeId: 'default-light', chatThinkingLevel: 1 as never }).chatThinkingLevel, 'off');
});

test('chatThinkingLevel preserves raw configured values', () => {
  assert.equal(normalizeUiSettings({ version: 1, themeId: 'default-light', chatThinkingLevel: 'minimal' }).chatThinkingLevel, 'minimal');
  assert.equal(normalizeUiSettings({ version: 1, themeId: 'default-light', chatThinkingLevel: 'xhigh' }).chatThinkingLevel, 'xhigh');
  assert.equal(normalizeUiSettings({ version: 1, themeId: 'default-light', chatThinkingLevel: ' custom ' }).chatThinkingLevel, 'custom');
});

test('ui zoomLevel falls back to default for invalid values', () => {
  assert.equal(normalizeUiSettings({ version: 1, themeId: 'default-light' }).zoomLevel, DEFAULT_WINDOW_ZOOM_LEVEL);
  assert.equal(normalizeUiSettings({ version: 1, themeId: 'default-light', zoomLevel: '1' as never }).zoomLevel, DEFAULT_WINDOW_ZOOM_LEVEL);
  assert.equal(normalizeUiSettings({ version: 1, themeId: 'default-light', zoomLevel: Number.NaN }).zoomLevel, DEFAULT_WINDOW_ZOOM_LEVEL);
});

test('ui zoomLevel clamps to stepped window zoom bounds', () => {
  assert.equal(
    formatWindowZoomPercent(normalizeUiSettings({ version: 1, themeId: 'default-light', zoomLevel: 100 }).zoomLevel),
    `${MAX_WINDOW_ZOOM_PERCENT}%`
  );
  assert.equal(
    formatWindowZoomPercent(normalizeUiSettings({ version: 1, themeId: 'default-light', zoomLevel: -100 }).zoomLevel),
    `${MIN_WINDOW_ZOOM_PERCENT}%`
  );
});

test('ui zoomLevel snaps in-range finite values to 10 percent steps', () => {
  assert.equal(formatWindowZoomPercent(normalizeUiSettings({ version: 1, themeId: 'default-light', zoomLevel: 2 }).zoomLevel), '140%');
  assert.equal(formatWindowZoomPercent(normalizeUiSettings({ version: 1, themeId: 'default-light', zoomLevel: -1.5 }).zoomLevel), '80%');
});

test('ui fontFamily drops the legacy global monospace default', () => {
  assert.equal(
    normalizeUiSettings({
      version: 1,
      themeId: 'default-light',
      fontFamily: '"JetBrains Mono", SF Mono, Menlo, Monaco, Consolas, "Liberation Mono", monospace',
    }).fontFamily,
    ''
  );
});

test('ui fontFamily keeps explicit custom fonts', () => {
  assert.equal(
    normalizeUiSettings({
      version: 1,
      themeId: 'default-light',
      fontFamily: '"IBM Plex Sans", sans-serif',
    }).fontFamily,
    '"IBM Plex Sans", sans-serif'
  );
});

test('migrateThemeId maps deprecated opagent ids to default ids', () => {
  assert.equal(migrateThemeId('opagent-light'), 'default-light');
  assert.equal(migrateThemeId('opagent-dark'), 'default-dark');
  assert.equal(migrateThemeId('default-light'), 'default-light');
  assert.equal(migrateThemeId('openbrain-light'), 'openbrain-light');
});

test('migrateLegacyThemeId maps pre-rename ids for one-time upgrade', () => {
  assert.equal(migrateLegacyThemeId('default-light'), 'openbrain-light');
  assert.equal(migrateLegacyThemeId('default-dark'), 'openbrain-dark');
  assert.equal(migrateLegacyThemeId('opagent-light'), 'default-light');
  assert.equal(migrateLegacyThemeId('opagent-dark'), 'default-dark');
});

test('normalizeUiSettings migrates opagent theme ids', () => {
  assert.equal(
    normalizeUiSettings({ version: 1, themeId: 'opagent-light' }).themeId,
    'default-light',
  );
  assert.equal(
    normalizeUiSettings({ version: 1, themeId: 'default-light' }).themeId,
    'default-light',
  );
});

test('normalizeIdleSleepPolicy prefers explicit idleSleepPolicy values', () => {
  assert.equal(normalizeIdleSleepPolicy({ idleSleepPolicy: 'off' }), 'off');
  assert.equal(normalizeIdleSleepPolicy({ idleSleepPolicy: 'whileAgentRunning' }), 'whileAgentRunning');
  assert.equal(normalizeIdleSleepPolicy({ idleSleepPolicy: 'whileAppRunning' }), 'whileAppRunning');
  assert.equal(
    normalizeIdleSleepPolicy({ preventSleepWhileAgentRunning: true }),
    'whileAgentRunning',
  );
  assert.equal(normalizeIdleSleepPolicy({ preventSleepWhileAgentRunning: false }), 'off');
  assert.equal(
    getIdleSleepPolicy({ version: 1, power: { idleSleepPolicy: 'whileAppRunning' } }),
    'whileAppRunning',
  );
});
