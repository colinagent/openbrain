import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const appPath = path.join(__dirname, 'App.tsx');

function read(filePath) {
  return readFileSync(filePath, 'utf8');
}

test('composer dock host stays above the resize divider for upward menus', () => {
  const source = read(appPath);

  assert.match(
    source,
    /Keep upward-opening conversation menus above the resize divider and editor-local overlays\./,
  );
  assert.match(
    source,
    /className="relative z-\[40\] shrink-0 min-h-0 overflow-visible"/,
  );
  assert.doesNotMatch(
    source,
    /className="relative z-\[1\] shrink-0 min-h-0 overflow-visible"/,
  );
});

test('pinned file pane is separate from activity panel host', () => {
  const source = read(appPath);

  assert.match(source, /const showPinnedFilePane = Boolean\(pinnedTab\?\.filePath\);/);
  assert.match(source, /const editorContentRowRef = useRef<HTMLDivElement \| null>\(null\);/);
  assert.match(source, /const sourceWidth = editorContentRowRef\.current\?\.clientWidth/);
  assert.match(source, /sourceWidth - PINNED_FILE_PANEL_PRIMARY_MIN_WIDTH/);
  assert.match(source, /className="op-pinned-conversation-file op-md-outline-shell is-expanded relative shrink-0 min-w-0 h-full min-h-0 flex flex-col overflow-hidden"/);
  assert.match(source, /renderEditorForTab\(pinnedTab, \{/);
  assert.match(source, /autoFocus: false/);
  assert.match(source, /suppressOutlineToggle: true/);
  assert.match(source, /textOffsetEnabled: false/);
  assert.match(source, /compactMarkdown: true/);
  assert.match(source, /onResizeStart=\{handlePinnedFilePanelResizeStart\}/);
  assert.doesNotMatch(source, /variant="glass"/);
  assert.match(source, /pinnedFilePanelWidth: finalWidth/);
});

test('pinned files are hidden from the primary editor area while pinned', () => {
  const source = read(appPath);

  assert.match(source, /const pinnedTab = pinnedTabId\s*\?\s*workspaceDocuments\.find\(\(tab\) => tab\.id === pinnedTabId\) \|\| null\s*: null;/m);
  assert.doesNotMatch(source, /pinnedConversationMode/);
  assert.match(source, /const primaryEditorTab = showPendingConversationPlaceholder/);
  assert.match(source, /: pinnedTab/);
  assert.match(source, /activeDocument\.id === pinnedTab\.id/);
  assert.match(source, /\) : primaryEditorTab \? \(/);
});

test('primary editor shows a pinned placeholder instead of generic empty state while a file is pinned', () => {
  const source = read(appPath);

  assert.match(source, /\) : pinnedTab \? \(/);
  assert.match(source, /title=\{`Return \$\{pinnedTab\.title\} to editor`\}/);
  assert.match(source, /<p className="text-base text-prime-text">Pinned file<\/p>/);
  assert.match(source, /handleReturnPinnedFileToEditor/);
});

test('pending new chat hides stale conversation backing docs from the primary editor', () => {
  const source = read(appPath);

  assert.match(source, /const pendingConversationSelected = selectedConversationTarget\?\.kind === 'pending';/);
  assert.match(source, /const showPendingConversationPlaceholder = Boolean\(/);
  assert.match(source, /activeDocument\?\.documentRole === 'conversation'/);
  assert.match(source, /const primaryEditorTab = showPendingConversationPlaceholder/);
  assert.match(
    source,
    /\{showPendingConversationPlaceholder \? \(\s*<WelcomeEditor[\s\S]*?\) : primaryEditorTab \? \(/m,
  );
  assert.match(
    source,
    /if \(!pendingConversationSelected\) \{[\s\S]*activateLastNonConversationTab\(\);[\s\S]*\}, \[[\s\S]*pendingConversationSelected,/m,
  );
});

test('welcome editor tab receives composer dock layout props', () => {
  const source = read(appPath);

  assert.match(source, /if \(tab\.editorId === 'welcome'\) \{/);
  assert.match(
    source,
    /if \(tab\.editorId === 'welcome'\) \{[\s\S]*chatPanelBottomInset=\{conversationComposerDockHeight \+ 1\}/m,
  );
  assert.match(
    source,
    /if \(tab\.editorId === 'welcome'\) \{[\s\S]*chatPanelOpen=\{composerVisible\}/m,
  );
});

test('primary editor falls back to the welcome page when no editor is open', () => {
  const source = read(appPath);

  assert.match(source, /import \{ WelcomeEditor \} from '\.\/components\/Editor\/WelcomeEditor';/);
  assert.match(
    source,
    /chatPanelBottomInset=\{conversationComposerDockHeight \+ 1\}/,
  );
  assert.match(source, /chatPanelOpen=\{composerVisible\}/);
  assert.doesNotMatch(source, /No editor open/);
  assert.doesNotMatch(source, /Select a file from the sidebar/);
});

test('activity panel attaches to the active editor or OpenBrain page area', () => {
  const source = read(appPath);

  assert.match(source, /const getActivityPanelAreaElement = \(\) => \{/);
  assert.match(source, /if \(showOpenBrainPage\) \{\s*return openBrainPagePaneRef\.current;\s*\}/);
  assert.match(source, /if \(showMessengerView\) \{\s*return messengerConversationPaneRef\.current;\s*\}/);
  assert.match(source, /return primaryEditorPaneRef\.current;/);
  assert.match(source, /ref=\{pinnedEditorPaneRef\}/);
  assert.match(source, /renderActivityPanel\(\)/);
  assert.doesNotMatch(source, /showActivityPanelInPinnedPane/);
});

test('activity panel stays visible for pending user questions', () => {
  const source = read(appPath);

  assert.match(source, /const selectedAwaitingUser = useChatWorkspaceStore\(\(state\) => \(/);
  assert.match(source, /state\.getAwaitingUser\(state\.getTargetChatPath\(state\.selectedConversationTarget\)\)/);
  assert.match(source, /liveOverlay\.errorMessage\s*\|\|\s*selectedAwaitingUser\s*\|\|\s*hasContextUsage/);
});

test('activity panel stays visible for durable thread entries', () => {
  const source = read(appPath);

  assert.match(source, /\(threadSnapshot\?\.entries\?\.length \|\| 0\) > 0/);
});

test('activity panel stays visible for live answer segments', () => {
  const source = read(appPath);

  assert.match(source, /liveOverlay\.streamingSegments\.length > 0/);
});

test('conversation selection sync follows the primary editor without overriding explicit tab picks', () => {
  const source = read(appPath);

  assert.match(
    source,
    /const activeChatPath = normalizeChatSessionPath\(currentFilePath\);[\s\S]*const selectedTarget = getChatWorkspaceStore\(activeTabId\)\.getState\(\)\.selectedConversationTarget;[\s\S]*\}, \[activeTabId, currentFilePath, workspaceConversationDocs\]\);/m,
  );
  assert.doesNotMatch(source, /setTargetChatPath\(null\)/);
  assert.doesNotMatch(source, /const exists = workspaceConversationDocs\.some\(\(tab\) => tab\.filePath === targetChatPath\)/);
});

test('conversation documents are not persisted as editor session paths', () => {
  const source = read(appPath);

  assert.match(source, /function getWorkspaceOpenEditorFilePaths\(tabId: string\): string\[\] \{/);
  assert.match(source, /path && isPlanDocumentPath\(path\)/);
  assert.doesNotMatch(
    source,
    /isConversationDocumentPath\(path, workspaceState\.currentDir\)\s*\|\|\s*isPlanDocumentPath\(path\)/,
  );
});
