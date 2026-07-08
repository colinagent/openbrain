import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

test('sidebar mounts Messenger as a dedicated rail item and sidebar view', () => {
  const source = readFileSync(path.join(__dirname, 'Sidebar.tsx'), 'utf8');
  const tabsSource = readFileSync(path.join(__dirname, 'sidebarTabs.ts'), 'utf8');
  const stylesSource = readFileSync(path.join(__dirname, '../../styles/index.css'), 'utf8');

  assert.match(source, /useMessengerStore/);
  assert.match(source, /selectMessengerPendingRequestTotal/);
  assert.match(source, /case 'messenger':/);
  assert.match(source, /<MessengerIcon className="w-5 h-5" \/>/);
  assert.match(source, /item\.key === 'messenger' && pendingRequestCount > 0/);
  assert.match(source, /<span className="sidebar-messenger-badge">\{formatMessengerPendingBadgeCount\(pendingRequestCount\)\}<\/span>/);
  assert.match(stylesSource, /--op-messenger-pending:\s*#cf222e;/);
  assert.match(stylesSource, /\.sidebar-messenger-badge\s*\{[^}]*background:\s*var\(--op-messenger-pending\);/m);
  assert.doesNotMatch(stylesSource, /\.sidebar-messenger-badge\s*\{[^}]*background:\s*var\(--color-accent\);/m);
  assert.doesNotMatch(source, /item\.key === 'messenger' && unreadCount > 0/);
  assert.doesNotMatch(source, /Math\.min\(unreadCount, 9\)/);
  assert.match(source, /view === 'messenger'[\s\S]*<MessengerSidebar \/>/);
  assert.doesNotMatch(source, /MessengerPanel/);
  assert.doesNotMatch(source, /messengerOpen/);
  assert.match(tabsSource, /\{ key: 'messenger', label: 'Messenger' \}/);
});

test('MessengerSidebar memoizes derived messenger summaries outside Zustand subscriptions', () => {
  const source = readFileSync(path.join(__dirname, 'MessengerSidebar.tsx'), 'utf8');

  assert.doesNotMatch(source, /useMessengerStore\(\(state\) => selectMessengerAgentSummaries/);
  assert.match(source, /useMemo\(\s*\(\) => selectMessengerAgentSummaries/);
});

test('MessengerSidebar opens only selected thread rows through workspace thread conversations', () => {
  const source = readFileSync(path.join(__dirname, 'MessengerSidebar.tsx'), 'utf8');

  assert.match(source, /useChatWorkspaceStore/);
  assert.match(source, /openThreadConversation/);
  assert.match(source, /selectMessengerChannelsForAgent/);
  assert.match(source, /openThreadSummary/);
  assert.match(source, /handleSelectThread/);
  assert.match(source, /loadMessengerChannel\(thread\.channelID\)/);
  assert.doesNotMatch(source, /openComposerThreadTarget/);
  assert.doesNotMatch(source, /setActivityExpanded/);
  assert.match(source, /threadID/);
  assert.doesNotMatch(source, /openAgentThread/);
  assert.doesNotMatch(source, /lastAutoOpenedAgentRef/);
  assert.doesNotMatch(source, /loadMessengerAgent/);
  assert.doesNotMatch(source, /selectMessengerRecordsForAgent/);
});

test('MessengerSidebar renders selected agent threads as conversation targets', () => {
  const source = readFileSync(path.join(__dirname, 'MessengerSidebar.tsx'), 'utf8');

  assert.match(source, /type MessengerThreadSummary = \{/);
  assert.match(source, /pendingRequestCount: number;/);
  assert.match(source, /buildMessengerThreadSummariesByAgent/);
  assert.match(source, /getMessengerChannelPendingRequestCount/);
  assert.match(source, /selectedChannelID/);
  assert.match(source, /selectChannel/);
  assert.match(source, /handleSelectThread/);
  assert.match(source, /openThreadSummary\(thread\)/);
  assert.match(source, /openThreadConversation\(normalizedThreadID/);
  assert.match(source, /expanded && threadSummaries\.length > 0/);
  assert.match(source, /selectedConversationTarget\?\.kind === 'thread'/);
});

test('MessengerSidebar keeps agent thread groups collapsed by default with a latest-ten preview', () => {
  const source = readFileSync(path.join(__dirname, 'MessengerSidebar.tsx'), 'utf8');

  assert.match(source, /const MESSENGER_THREAD_PREVIEW_LIMIT = 10;/);
  assert.match(source, /const MESSENGER_THREAD_MAX_VISIBLE = 99;/);
  assert.match(source, /const \[expandedAgentIDs, setExpandedAgentIDs\] = useState<Record<string, boolean>>\(\{\}\);/);
  assert.match(source, /const \[showAllThreadAgentIDs, setShowAllThreadAgentIDs\] = useState<Record<string, boolean>>\(\{\}\);/);
  assert.match(source, /threadSummaries\.slice\(0, MESSENGER_THREAD_PREVIEW_LIMIT\)/);
  assert.match(source, /threadSummaries\.slice\(0, MESSENGER_THREAD_MAX_VISIBLE\)/);
  assert.match(source, /hiddenThreadCount > 0/);
  assert.match(source, /<MoreHorizontalIcon className="h-4 w-4 shrink-0" \/>/);
  assert.match(source, /setShowAllThreadAgentIDs\(\(current\) => \(\{/);
});

test('MessengerSidebar thread rows render only thread titles from channel summaries', () => {
  const source = readFileSync(path.join(__dirname, 'MessengerSidebar.tsx'), 'utf8');
  const stylesSource = readFileSync(path.join(__dirname, '../../styles/index.css'), 'utf8');

  assert.match(source, /const title = compactTitle\(channel\.title \|\| channel\.lastMessage\?\.title \|\| threadID\) \|\| 'Thread';/);
  assert.match(source, /<span className="min-w-0 flex-1 truncate text-xs font-medium">\s*\{thread\.title\}\s*<\/span>/);
  assert.match(source, /thread\.pendingRequestCount > 0\s*\?\s*'messenger-pending-dot'/);
  assert.match(stylesSource, /\.messenger-pending-dot\s*\{[^}]*background:\s*var\(--op-messenger-pending\);/m);
  assert.doesNotMatch(source, /thread\.pendingRequestCount > 0\s*\?\s*'bg-accent'/);
  assert.doesNotMatch(source, /thread\.body/);
  assert.doesNotMatch(source, /thread\.openCount/);
  assert.doesNotMatch(source, /thread\.unreadCount/);
  assert.doesNotMatch(source, /formatSidebarTime\(thread\.lastUpdatedAt\)/);
});

test('MessengerSidebar agent rows show the agent name and id instead of message body copy', () => {
  const source = readFileSync(path.join(__dirname, 'MessengerSidebar.tsx'), 'utf8');

  assert.match(source, /const resolveAgentByID = useAppStore\(\(state\) => state\.resolveAgentByID\);/);
  assert.match(source, /function resolveMessengerAgentName\(/);
  assert.match(source, /const agentName = resolveMessengerAgentName\(summary\.agentID, summary\.title, resolveAgentByID\);/);
  assert.match(source, /<span className="truncate text-sm font-medium">\{agentName\}<\/span>/);
  assert.match(source, /<span className="mt-0\.5 block truncate text-xs text-tertiary-text">\s*\{summary\.agentID\}\s*<\/span>/);
  assert.match(source, /summary\.pendingRequestCount > 0/);
  assert.match(source, /\{formatMessengerPendingBadgeCount\(summary\.pendingRequestCount\)\}/);
  assert.match(source, /messenger-pending-badge min-w-4/);
  assert.doesNotMatch(source, /summary\.unreadCount > 0/);
  assert.doesNotMatch(source, /Math\.min\(summary\.unreadCount, 9\)/);
  assert.doesNotMatch(source, /summary\.openCount > 0/);
  assert.doesNotMatch(source, /summary\.pendingRequestCount[\s\S]*?bg-accent/);
  assert.match(source, /title=\{summary\.agentID\}/);
  assert.doesNotMatch(source, /\{summary\.lastBody \|\| summary\.subtitle\}/);
});

test('MessengerSidebar clears pending requests from an agent row context menu', () => {
  const source = readFileSync(path.join(__dirname, 'MessengerSidebar.tsx'), 'utf8');
  const appStoreSource = readFileSync(path.join(__dirname, '../../store/appStore.ts'), 'utf8');
  const serviceSource = readFileSync(path.join(__dirname, '../../services/messengerService.ts'), 'utf8');

  assert.match(source, /onContextMenu=\{\(event\) => handleAgentContextMenu\(event, summary\)\}/);
  assert.match(source, /Clear pending requests/);
  assert.match(source, /Clear all messages/);
  assert.match(source, /archiveMessengerAgentPendingRequests\(menu\.agentID\)/);
  assert.match(source, /archiveMessengerAgentMessages\(menu\.agentID\)/);
  assert.doesNotMatch(source, /summary\.pendingRequestCount <= 0[\s\S]*closeAgentContextMenu\(\);\s*return;/);
  assert.match(source, /const agentMenuRef = useRef<HTMLDivElement \| null>\(null\);/);
  assert.match(source, /window\.addEventListener\('mousedown', handleAgentMenuMouseDown, true\);/);
  assert.match(source, /window\.addEventListener\('keydown', handleAgentMenuKeyDown, true\);/);
  assert.match(source, /ref=\{agentMenuRef\}/);
  assert.doesNotMatch(source, /aria-label="Close messenger agent menu"/);
  assert.doesNotMatch(source, /fixed inset-0 z-\[60\][^"]*bg-transparent/);
  assert.match(source, /formatMessengerPendingBadgeCount\(agentContextMenu\.pendingRequestCount\)/);
  const clearHandlerMatch = source.match(/const handleClearAgentPendingRequests = useCallback\(async \(\) => \{[\s\S]*?\n  \}, \[/);
  assert.ok(clearHandlerMatch, 'handleClearAgentPendingRequests should exist');
  assert.match(
    clearHandlerMatch[0],
    /setClearingAgentID\(menu\.agentID\);\s*closeAgentContextMenu\(\);\s*try \{\s*const archived = await archiveMessengerAgentPendingRequests\(menu\.agentID\);/,
  );
  assert.match(clearHandlerMatch[0], /Cleared \$\{archived\} pending request/);
  const clearAllHandlerMatch = source.match(/const handleClearAgentAllMessages = useCallback\(async \(\) => \{[\s\S]*?\n  \}, \[/);
  assert.ok(clearAllHandlerMatch, 'handleClearAgentAllMessages should exist');
  assert.match(
    clearAllHandlerMatch[0],
    /setClearingAgentID\(menu\.agentID\);\s*closeAgentContextMenu\(\);\s*try \{\s*const archived = await archiveMessengerAgentMessages\(menu\.agentID\);/,
  );
  assert.match(clearAllHandlerMatch[0], /Cleared \$\{archived\} message/);
  assert.match(appStoreSource, /archiveMessengerAgentPendingRequests: async \(agentID: string\) => \{/);
  assert.match(appStoreSource, /archiveMessengerAgentMessages: async \(agentID: string\) => \{/);
  assert.match(appStoreSource, /archiveMessengerAgentPendingRequests: \(agentID: string\) => Promise<number>;/);
  assert.match(appStoreSource, /archiveMessengerAgentMessages: \(agentID: string\) => Promise<number>;/);
  assert.match(appStoreSource, /return result\?\.archived \?\? 0;/);
  assert.match(appStoreSource, /pendingRequestsOnly: true/);
  assert.match(appStoreSource, /archiveAgentPendingRequests\(normalizedAgentID\)/);
  assert.match(appStoreSource, /archiveAgentMessages\(normalizedAgentID\)/);
  assert.match(serviceSource, /agentID\?: string;/);
  assert.match(serviceSource, /pendingRequestsOnly\?: boolean;/);
});

test('messenger reply returns resolved request and user reply records', () => {
  const appStoreSource = readFileSync(path.join(__dirname, '../../store/appStore.ts'), 'utf8');
  const serviceSource = readFileSync(path.join(__dirname, '../../services/messengerService.ts'), 'utf8');
  const serverSource = readFileSync(path.resolve(__dirname, '../../../../../server/internal/server/ws/messenger_handler.go'), 'utf8');

  assert.match(serviceSource, /export type MessengerReplyResult = \{/);
  assert.match(serviceSource, /record: MessengerRecord;/);
  assert.match(serviceSource, /resolved\?: MessengerRecord \| null;/);
  assert.match(serviceSource, /Promise<MessengerReplyResult>/);
  assert.match(serverSource, /out\.Dispatch = nil/);
  assert.match(serverSource, /out\.Queue = nil/);
  assert.match(serverSource, /return out, nil/);
  assert.match(appStoreSource, /const result = await messengerService\.reply\(await resolveMessengerReplyInput\(input, _tabId\)\);/);
  assert.match(appStoreSource, /const records = result\.resolved\s*\?\s*\[result\.resolved, result\.record\]\s*:\s*\[result\.record\];/);
  assert.match(appStoreSource, /for \(const record of records\) \{\s*messengerState\.upsertRecord\(record\);\s*\}/s);
  assert.match(appStoreSource, /getChatWorkspaceStore\(_tabId\)\.getState\(\)\.upsertThreadMessageRecords\(records\);/);
});

test('MessengerSidebar does not open a thread when the messenger view mounts or an agent row is selected', () => {
  const source = readFileSync(path.join(__dirname, 'MessengerSidebar.tsx'), 'utf8');

  assert.match(source, /const selectChatConversation = useChatWorkspaceStore\(\(state\) => state\.selectChatConversation\);/);
  assert.match(source, /const hideComposer = useChatWorkspaceStore\(\(state\) => state\.hideComposer\);/);
  assert.match(source, /selectChatConversation\(null\);\s*hideComposer\(\);/);
  assert.match(source, /const handleSelect = \(summary: MessengerAgentSummary\) => \{/);
  const handleSelectMatch = source.match(/const handleSelect = \(summary: MessengerAgentSummary\) => \{[\s\S]*?\n  \};/);
  assert.ok(handleSelectMatch, 'handleSelect should exist');
  assert.doesNotMatch(handleSelectMatch[0], /openThreadConversation/);
  assert.doesNotMatch(handleSelectMatch[0], /loadMessengerChannel/);
  assert.doesNotMatch(handleSelectMatch[0], /setActivityExpanded/);
});

test('app store loads messenger details by channel instead of by agent', () => {
  const appStoreSource = readFileSync(path.join(__dirname, '../../store/appStore.ts'), 'utf8');

  assert.match(appStoreSource, /refreshMessenger: async \(\) => \{/);
  assert.match(appStoreSource, /messengerService\.list\(\{ limit: 100 \}\)/);
  assert.match(appStoreSource, /loadMessengerChannel: async \(channelID: string\) => \{/);
  assert.match(appStoreSource, /channelID: normalizedChannelID,/);
  assert.match(appStoreSource, /setChannelMessages\(\s*returnedChannelID,/);
  assert.doesNotMatch(appStoreSource, /loadMessengerAgent/);
  assert.doesNotMatch(appStoreSource, /Promise\.all\(channelIDs\.map/);
});

test('MessengerSidebar does not gate Messenger behind auth state', () => {
  const source = readFileSync(path.join(__dirname, 'MessengerSidebar.tsx'), 'utf8');

  assert.doesNotMatch(source, /useAuthStore/);
  assert.doesNotMatch(source, /LoginRequiredPrompt/);
  assert.doesNotMatch(source, /authLoggedIn/);
});
