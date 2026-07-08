import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';

function resolveConversationComposerDockPath() {
  const fromRepoRoot = path.resolve(
    process.cwd(),
    'desktop/src/renderer/components/Chat/ConversationComposerDock.tsx'
  );
  const fromAppRoot = path.resolve(process.cwd(), 'src/renderer/components/Chat/ConversationComposerDock.tsx');
  try {
    return readFileSync(fromRepoRoot, 'utf8') ? fromRepoRoot : fromAppRoot;
  } catch {
    return fromAppRoot;
  }
}

const conversationComposerDockSource = readFileSync(resolveConversationComposerDockPath(), 'utf8');

test('ConversationComposerDock model picker trigger uses the display provider label when available', () => {
  assert.match(conversationComposerDockSource, /const activeModelTriggerText = useMemo\(\(\) => \{/);
  assert.match(conversationComposerDockSource, /\? `\$\{activeModelDisplay\.triggerText\} \$\{detailParts\.join\(' '\)\}`/);
  assert.match(conversationComposerDockSource, /: activeModelDisplay\.triggerText;/);
  assert.match(conversationComposerDockSource, /<span>\{activeModelTriggerText\}<\/span>/);
  assert.match(conversationComposerDockSource, /activeModelPreference\.serviceTier === 'priority' \? 'Fast' : ''/);
  const triggerTextBlock = conversationComposerDockSource.slice(
    conversationComposerDockSource.indexOf('const activeModelTriggerText = useMemo'),
    conversationComposerDockSource.indexOf('useEffect(() => {', conversationComposerDockSource.indexOf('const activeModelTriggerText = useMemo')),
  );
  assert.doesNotMatch(triggerTextBlock, /formatContextWindowOption/);
  assert.doesNotMatch(triggerTextBlock, /activeModelPreference\.contextWindow/);
});

test('ConversationComposerDock model picker menu shows model labels instead of internal model keys', () => {
  assert.doesNotMatch(conversationComposerDockSource, /function renderModelKeyLabel\(/);
  assert.doesNotMatch(conversationComposerDockSource, /getDisplayModelKeyText\(/);
  assert.match(conversationComposerDockSource, /const isActive = effectiveModelKey === model\.key;/);
  assert.match(conversationComposerDockSource, /\{display\.primaryText\}/);
  assert.match(conversationComposerDockSource, /const descriptionParts = \[/);
  assert.match(conversationComposerDockSource, /display\.secondaryText,/);
  assert.match(conversationComposerDockSource, /display\.providerText,/);
  assert.match(conversationComposerDockSource, /modelPreference\.serviceTier === 'priority' \? 'Fast' : ''/);
});

test('ConversationComposerDock model picker has no Auto model option and marks the Default model', () => {
  assert.doesNotMatch(conversationComposerDockSource, /key="__auto__"/);
  assert.doesNotMatch(conversationComposerDockSource, /isAutoSelected/);
  assert.doesNotMatch(conversationComposerDockSource, /Auto — use/);
  assert.match(conversationComposerDockSource, /resolveDefaultChatModelSelection\(modelsConfig\)/);
  assert.match(conversationComposerDockSource, /const isDefaultModel = defaultChatModelKey === model\.key;/);
  const modelPickerListBlock = conversationComposerDockSource.slice(
    conversationComposerDockSource.indexOf('{selectableModels.map((model) => {'),
    conversationComposerDockSource.indexOf('<div className="border-t border-border mt-1 pt-1">'),
  );
  assert.match(modelPickerListBlock, /<\/PopupMenuItem>\s*\{isDefaultModel \? \([\s\S]*rounded-full[\s\S]*>\s*Default\s*<[\s\S]*\) : null\}\s*<IconButton/);
  assert.match(modelPickerListBlock, /title=\{`Edit \$\{display\.primaryText\} options`\}/);
  assert.match(modelPickerListBlock, /aria-label=\{`Edit \$\{display\.primaryText\} options`\}/);
  assert.match(modelPickerListBlock, /<EditIcon className="h-3\.5 w-3\.5" \/>/);
  assert.doesNotMatch(modelPickerListBlock, />\s*Edit\s*</);
});

test('ConversationComposerDock selects conversation backing docs without owning primary editor state', () => {
  assert.match(conversationComposerDockSource, /const setActiveConversationTab = useAppStore\(\(state\) => state\.setActiveConversationTab\);/);
  assert.doesNotMatch(conversationComposerDockSource, /const pinnedConversationMode = useMemo/);
  assert.doesNotMatch(conversationComposerDockSource, /setActiveTab/);
  assert.match(conversationComposerDockSource, /setActiveConversationTab\(tabId\);/);
  assert.match(
    conversationComposerDockSource,
    /setActiveConversationTab\(tabId\);[\s\S]*selectThreadConversation\(normalizedThreadID, nextPath\);/m,
  );
  assert.doesNotMatch(conversationComposerDockSource, /const setActiveTab = useAppStore\(\(state\) => state\.setActiveTab\);/);
});

test('ConversationComposerDock header tab strip uses ui-tabbar for editor-matching capsule tabs', () => {
  assert.match(
    conversationComposerDockSource,
    /<div className="ui-tabbar flex min-w-0 items-center">[\s\S]*<ConversationTabItem/,
  );
});

test('ConversationComposerDock header does not render the conversation file pin button', () => {
  assert.doesNotMatch(conversationComposerDockSource, /const activateLastNonConversationTab = useAppStore\(\(state\) => state\.activateLastNonConversationTab\);/);
  assert.doesNotMatch(conversationComposerDockSource, /const handleConversationFilePinToggle = useCallback\(\(\) => \{/);
  assert.doesNotMatch(conversationComposerDockSource, /conversationFilePinned/);
});

test('ConversationComposerDock marks the pinned conversation tab with status icon state', () => {
  assert.match(conversationComposerDockSource, /const pinnedTabId = useAppStore\(\(state\) => state\.pinnedTabId\);/);
  assert.match(conversationComposerDockSource, /isPinned=\{tab\.id === pinnedTabId\}/);
});

test('ConversationComposerDock exposes Fast only when the model catalog declares priority support', () => {
  assert.match(conversationComposerDockSource, /modelSupportsPriorityServiceTier/);
  assert.match(conversationComposerDockSource, /serviceTier: enabled \? 'priority' : null/);
  assert.match(conversationComposerDockSource, /preference\.serviceTier !== 'priority'/);
  assert.match(conversationComposerDockSource, />Fast<\/span>/);
  assert.doesNotMatch(conversationComposerDockSource, /activeModel\?\.api === 'openai-responses'/);
});

test('ConversationComposerDock renders thinking levels as pills without a separate Thinking toggle', () => {
  assert.match(conversationComposerDockSource, />Thinking<\/div>/);
  assert.match(conversationComposerDockSource, /rounded-full border px-2\.5 py-1 text-xs font-medium transition-colors/);
  assert.doesNotMatch(conversationComposerDockSource, /modelOptionToggleTrackClass\(editingModelPreference\.thinkingLevel/);
  assert.doesNotMatch(conversationComposerDockSource, /editingModelPreference\.thinkingLevel === 'off' \? \(editingThinkingOptions\.find/);
});

test('ConversationComposerDock only renders context choices when catalog options exist', () => {
  assert.match(conversationComposerDockSource, /editingModelPreference\.contextWindowOptions\.length > 0 \? \(/);
  assert.match(conversationComposerDockSource, /editingModelPreference\.contextWindowOptions\.map\(\(contextWindow\) => \(/);
});

test('ConversationComposerDock exposes current agent subagents in the bottom picker', () => {
  assert.match(conversationComposerDockSource, /const getAgentSubagents = useAppStore\(\(state\) => state\.getAgentSubagents\);/);
  assert.match(conversationComposerDockSource, /const getMountableAgentSubagents = useAppStore\(\(state\) => state\.getMountableAgentSubagents\);/);
  assert.match(conversationComposerDockSource, /const mountAgentSubagent = useAppStore\(\(state\) => state\.mountAgentSubagent\);/);
  assert.match(conversationComposerDockSource, /const unmountAgentSubagent = useAppStore\(\(state\) => state\.unmountAgentSubagent\);/);
  assert.match(conversationComposerDockSource, /const nodeGraphRevision = useAppStore\(\(state\) => state\.nodeGraphRevision\);/);
  assert.match(conversationComposerDockSource, /const mountedSubagents = useMemo/);
  assert.match(conversationComposerDockSource, /const mountableSubagents = useMemo/);
  assert.match(conversationComposerDockSource, /const \[subagentAvailableOpen, setSubagentAvailableOpen\] = useState\(false\);/);
  assert.match(conversationComposerDockSource, /const \[subagentPickerRefreshing, setSubagentPickerRefreshing\] = useState/);
  assert.match(conversationComposerDockSource, /if \(!subagentPickerOpen \|\| !effectiveAgentID\)/);
  assert.match(conversationComposerDockSource, /refreshAgentNodes\(\{ force: true \}\)/);
  assert.match(conversationComposerDockSource, /\[effectiveAgentID, getAgentSubagents, nodeGraphRevision\]/);
  assert.match(conversationComposerDockSource, /\[effectiveAgentID, getMountableAgentSubagents, nodeGraphRevision\]/);
  assert.match(conversationComposerDockSource, /const subagentNames = useMemo/);
  assert.match(conversationComposerDockSource, /const subagentPrimaryName = subagentNames\[0\] \?\? '';/);
  assert.match(conversationComposerDockSource, /const subagentExtraCount = Math\.max\(0, subagentNames\.length - 1\);/);
  assert.match(conversationComposerDockSource, /const subagentDisplayTitle = useMemo/);
  assert.match(conversationComposerDockSource, /const subagentAriaLabel = useMemo/);
  assert.match(conversationComposerDockSource, /title=\{subagentDisplayTitle\}/);
  assert.match(conversationComposerDockSource, /aria-label=\{subagentAriaLabel\}/);
  assert.match(conversationComposerDockSource, /\{subagentExtraCount\}/);
  assert.match(conversationComposerDockSource, /text-xs text-tertiary-text/);
  assert.doesNotMatch(conversationComposerDockSource, /<span>Subagents \{mountedSubagentCount\}<\/span>/);
  assert.match(conversationComposerDockSource, /Refreshing subagents\.\.\./);
  assert.match(conversationComposerDockSource, /SubAgent/);
  assert.match(conversationComposerDockSource, /No subagents attached/);
  assert.match(conversationComposerDockSource, /aria-label="Add subagent"/);
  assert.match(conversationComposerDockSource, /setSubagentAvailableOpen\(\(open\) => !open\)/);
  assert.match(conversationComposerDockSource, /\{subagentAvailableOpen && \(/);
  assert.match(conversationComposerDockSource, /Available/);
  assert.match(conversationComposerDockSource, /No available subagents/);
  assert.match(conversationComposerDockSource, /setSubagentAvailableOpen\(true\);/);
  assert.match(conversationComposerDockSource, /setSubagentAvailableOpen\(false\);/);
  assert.match(conversationComposerDockSource, /aria-label=\{`\$\{disabled \? 'Removing' : 'Remove'\} subagent \$\{label\}`\}/);
  assert.match(conversationComposerDockSource, /aria-label=\{`\$\{disabled \? 'Attaching' : 'Attach'\} subagent \$\{label\}`\}/);
  assert.match(conversationComposerDockSource, /<TrashIcon className="h-3\.5 w-3\.5" \/>/);
  assert.match(conversationComposerDockSource, /<PlusIcon className="h-3\.5 w-3\.5" \/>/);
  assert.doesNotMatch(conversationComposerDockSource, />\s*\{disabled \? 'Removing' : 'Remove'\}\s*</);
  assert.doesNotMatch(conversationComposerDockSource, />\s*\{disabled \? 'Attaching' : 'Attach'\}\s*</);
  assert.doesNotMatch(conversationComposerDockSource, /title=\{`\$\{disabled \? 'Removing' : 'Remove'\} subagent \$\{label\}`\}/);
  assert.doesNotMatch(conversationComposerDockSource, /title=\{`\$\{disabled \? 'Attaching' : 'Attach'\} subagent \$\{label\}`\}/);
  assert.doesNotMatch(conversationComposerDockSource, /'Mount'/);
  assert.doesNotMatch(conversationComposerDockSource, /'Mounting'/);
  assert.doesNotMatch(conversationComposerDockSource, /'Unmount'/);
  assert.doesNotMatch(conversationComposerDockSource, /'Unmounting'/);
  assert.doesNotMatch(conversationComposerDockSource, /No subagents mounted/);
  assert.doesNotMatch(conversationComposerDockSource, /Subagent mounted/);
  assert.doesNotMatch(conversationComposerDockSource, /Subagent unmounted/);
  assert.doesNotMatch(conversationComposerDockSource, /Failed to mount subagent/);
  assert.doesNotMatch(conversationComposerDockSource, /Failed to unmount subagent/);
  assert.match(conversationComposerDockSource, /handleSubagentMount\(subagent\.id\)/);
  assert.match(conversationComposerDockSource, /handleSubagentRemove\(subagent\.id\)/);
});

test('ConversationComposerDock treats the agent picker as a per-conversation selection', () => {
  assert.match(conversationComposerDockSource, /const selectedAgentTarget = useChatWorkspaceStore\(\(state\) => state\.getAgentForTarget\(state\.selectedConversationTarget\)\);/);
  assert.match(conversationComposerDockSource, /selectedAgentTarget \|\| resolveChatAgentTarget/);
  assert.match(conversationComposerDockSource, /setAgentForSelectedTarget\(nextTarget\);/);
  assert.doesNotMatch(conversationComposerDockSource, /resolveThreadMetaAgentTarget/);
  assert.doesNotMatch(conversationComposerDockSource, /switchAgentReference/);
  assert.doesNotMatch(conversationComposerDockSource, /selectedThreadMeta\?\.agentID/);
});

test('ConversationComposerDock inserts source references with a single trailing newline', () => {
  assert.match(
    conversationComposerDockSource,
    /insertBlockMarkdown\(view, markdown, \{ leaveCursorAfterBlock: true, trailingNewlines: 1 \}\);/
  );
});

test('ConversationComposerDock does not render idle continuation hints above the composer', () => {
  assert.doesNotMatch(conversationComposerDockSource, /This thread can be continued/);
  assert.doesNotMatch(conversationComposerDockSource, /pending queued work/);
});

test('ConversationComposerDock does not render durable thread history', () => {
  assert.doesNotMatch(conversationComposerDockSource, /threadSnapshotMessages/);
  assert.doesNotMatch(conversationComposerDockSource, /conversation-thread-render/);
  assert.doesNotMatch(conversationComposerDockSource, /getThreadSnapshotForTarget/);
  assert.doesNotMatch(conversationComposerDockSource, /refreshThreadStateByThreadID/);
  assert.match(conversationComposerDockSource, /<ChatMarkdownComposer/);
});

test('ConversationComposerDock opens GBrain scope source on click instead of hover', () => {
  const scopeBlock = conversationComposerDockSource.slice(
    conversationComposerDockSource.indexOf('{gbrainScopeLabel ? ('),
    conversationComposerDockSource.indexOf('<div ref={composerShellRef}', conversationComposerDockSource.indexOf('{gbrainScopeLabel ? (')),
  );

  assert.match(scopeBlock, /onClick=\{toggleGBrainScopePrompt\}/);
  assert.match(scopeBlock, /onKeyDown=\{handleGBrainScopeKeyDown\}/);
  assert.match(scopeBlock, /aria-expanded=\{gbrainScopePromptVisible\}/);
  assert.match(scopeBlock, /id="gbrain-scope-prompt-popover"/);
  assert.match(scopeBlock, /onClick=\{handleClearGBrainQueryScope\}/);
  assert.doesNotMatch(scopeBlock, /title=\{gbrainScopePrompt\}/);
  assert.doesNotMatch(scopeBlock, /onMouseEnter/);
  assert.doesNotMatch(scopeBlock, /onMouseLeave/);
  assert.doesNotMatch(scopeBlock, /role="tooltip"/);
  assert.match(conversationComposerDockSource, /const gbrainScopePopoverRef = useRef<HTMLSpanElement \| null>\(null\);/);
  assert.match(conversationComposerDockSource, /active: gbrainScopePromptVisible,[\s\S]*insideRefs: \[gbrainScopePopoverRef\],/);
  assert.match(conversationComposerDockSource, /event\.key === 'Enter' \|\| event\.key === ' '/);
  assert.match(conversationComposerDockSource, /event\.key === 'Escape'/);
});
