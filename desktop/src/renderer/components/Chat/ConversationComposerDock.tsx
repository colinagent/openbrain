import React, { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { useAuthStore } from '../../store/authStore';
import type { EditorView } from '@codemirror/view';
import { useAppStore } from '../../store/appStore';
import { useUiStore } from '../../store/uiStore';
import {
  useChatWorkspaceStore,
  getConversationTargetKey,
  type ConversationTarget,
  type QueuedMessage,
  type PendingConversation,
} from '../../store/chatWorkspaceStore';
import { useModelsStore } from '../../store/modelsStore';
import { useToastStore } from '../../store/toastStore';
import {
  compactCurrentThread,
  editQueuedMessage,
  queueFollowUp,
  promoteQueuedMessage,
  refreshThreadState,
  removeQueuedMessage,
  submitChatTurn,
  stopCommand,
  stopStream,
} from '../../services/chatService';
import { writeClipboardImageFromElement, writeClipboardText } from '../../services/clipboardService';
import { ArrowUpTinyIcon, ChatLineIcon, CheckTinyIcon, ChevronDownIcon, CloseButton, EditIcon, PlusIcon, SendArrowSimpleIcon, TerminalIcon, TrashIcon } from '../Icons';
import { IconButton } from '../IconButton';
import { useDismissOnOutsideInteraction } from '../../hooks/useDismissOnOutsideInteraction';
import { PopupMenu, PopupMenuItem } from '../PopupMenu';
import {
  TAB_ICON_HOVER_LIFT_CLASS,
} from '../tabLayout';
import { ImageContextMenu } from '../Editor/ImageContextMenu';
import { resolveImageMenuTarget, useMarkdownImageMenu } from '../Editor/useMarkdownImageMenu';
import {
  getChatWorkdir,
  isThreadChatPath,
  resolveChatAgentTarget,
} from '../../utils/chatAgentTarget';
import {
  extractDroppedImageFiles,
  hasClipboardImage,
  readChatImages,
  readClipboardImages,
  type ChatInputImage,
} from '../../utils/chatImages';
import { persistChatImageAssets } from '../../utils/chatImageAssets';
import {
  type ThinkingLevel,
} from '../../services/chatInput';
import {
  resolveChatModelSelection,
  resolveDefaultChatModelSelection,
} from '../../utils/chatModelSelection';
import {
  getThinkingPickerLevels,
  normalizeThinkingLevelForModel,
  UI_CHAT_THINKING_ON_LEVEL,
} from '../../utils/chatThinking';
import {
  formatContextWindowOption,
  modelSupportsPriorityServiceTier,
  resolveChatModelPreference,
} from '../../utils/chatModelPreferences';
import {
  getConversationPrimaryButtonMode,
  hasConversationSubmissionContent,
  type ConversationPrimaryButtonMode,
} from '../../utils/conversationComposerDockState';
import { ChatMarkdownComposer, type ChatMarkdownComposerHandle } from './ChatMarkdownComposer';
import { dirnamePosix, parseMarkdownImage, resolveMarkdownPath } from '../../utils/markdownMedia';
import { buildFileReferenceLink } from '../../utils/chatReferenceLinks';
import {
  filterBuiltInSlashCommands,
  filterSlashSkillOptions,
  removeSlashTokenFromDraft,
  resolveSlashMenuState,
  type BuiltInSlashCommand,
  type SlashMenuItem,
  type SkillOption,
} from '../../utils/chatSlash';
import { resolvePlanSkillShortcutAction } from '../../utils/chatPlanSkillHotkey';
import {
  activateComposerPlanBlock,
  removeComposerPlanBlock,
  type ComposerPlanState,
} from '../../utils/chatPlanBlock';
import {
  buildGBrainQueryScopePrompt,
  gbrainQueryScopeLabel,
} from '../../utils/gbrainQueryScope';
import {
  buildAgentSwitchOptions,
  formatAgentTargetDisplayLabel,
  formatAgentTargetDisplayTitle,
} from '../../utils/agentSwitch';
import {
  getModelDisplayInfo,
  getModelEntryDisplay,
  getVisibleProviderLabel,
} from '../../utils/modelDisplay';
import { ConversationTabItem } from './ConversationTabItem';
import {
  getDesktopBillingSubscription,
  modelRequiresBundledTokenValue,
  type DesktopBillingSubscription,
} from '../../services/billingAccess';
import { useBillingReminderStore } from '../../store/billingReminderStore';
import { rendererI18n } from '../../../main/i18n/renderer';

const SLASH_MENU_WIDTH = 288;
const SLASH_MENU_VERTICAL_GAP = 4;
const SLASH_MENU_EDGE_GAP = 8;
const SLASH_MENU_MAX_HEIGHT = 224;
const SLASH_MENU_ITEM_HEIGHT = 40;
const SLASH_MENU_MESSAGE_HEIGHT = 40;
const CHAT_BOTTOM_PICKER_MENU_CLASS_NAME = 'absolute bottom-full left-1/2 mb-1 -translate-x-1/2 z-50 overflow-hidden';

function modelOptionToggleTrackClass(enabled: boolean): string {
  return `relative inline-flex h-5 w-9 shrink-0 items-center rounded-full border transition-colors ${enabled ? 'border-highlight bg-highlight' : 'border-border bg-hover-bg'}`;
}

function modelOptionToggleThumbClass(enabled: boolean): string {
  return `block h-4 w-4 rounded-full border border-border bg-editor-bg shadow-sm transition-transform ${enabled ? 'translate-x-4' : 'translate-x-0.5'}`;
}

function newChatTitle(): string {
  return rendererI18n.t('chat:newChat');
}

function flattenQueuedMessages(messages: {
  steering: QueuedMessage[];
  followUp: QueuedMessage[];
}): QueuedMessage[] {
  return [...messages.steering, ...messages.followUp];
}

function buildQueuedMessageSummary(message: QueuedMessage): string {
  const text = message.text.trim().replace(/\s+/g, ' ');
  if (text) {
    return text;
  }
  return 'Queued message';
}

function getQueuedMessageStateLabel(message: QueuedMessage): string {
  return message.kind === 'steering' ? 'Steering' : 'Follow-up';
}

function getQueuedMessageStateTitle(message: QueuedMessage): string {
  return message.kind === 'steering' ? 'Steering' : 'Follow-up';
}

function getLastPathSegment(path: string | null): string {
  const normalized = (path || '').trim().replace(/\/+$/, '');
  if (!normalized) return '';
  const parts = normalized.split('/').filter(Boolean);
  return parts.length > 0 ? parts[parts.length - 1] : '';
}

function getConversationTitle(): string {
  return newChatTitle();
}

function getThinkingLevelLabel(level: ThinkingLevel): string {
  if (level === UI_CHAT_THINKING_ON_LEVEL) {
    return 'on';
  }
  return level;
}

function isImeComposingEvent(event: { isComposing?: boolean; keyCode?: number }): boolean {
  return Boolean(event.isComposing || event.keyCode === 229);
}

type PendingResource =
  | {
    kind: 'skill';
    key: string;
    skill: {
      id: string;
      slug: string;
      name: string;
    };
  };

function buildBlockInsertion(view: EditorView, markdown: string): string {
  const selection = view.state.selection.main;
  const hasSelection = !selection.empty;
  const from = selection.from;
  const to = selection.to;
  const before = from > 0 ? view.state.doc.sliceString(from - 1, from) : '';
  const after = to < view.state.doc.length ? view.state.doc.sliceString(to, to + 1) : '';
  let insert = markdown;
  if (!hasSelection && from > 0 && before !== '\n') {
    insert = `\n${insert}`;
  }
  if (!hasSelection && to < view.state.doc.length && after !== '\n') {
    insert = `${insert}\n`;
  }
  return insert;
}

function withTrailingNewlines(value: string, count: number): string {
  const trailingNewlines = Math.max(0, Math.floor(count));
  const trimmed = value.replace(/\n+$/, '');
  return trailingNewlines > 0
    ? `${trimmed}${'\n'.repeat(trailingNewlines)}`
    : trimmed;
}

function insertBlockMarkdown(
  view: EditorView,
  markdown: string,
  options?: { leaveCursorAfterBlock?: boolean; trailingNewlines?: number },
): void {
  const selection = view.state.selection.main;
  let insert = buildBlockInsertion(view, markdown);
  if (options?.leaveCursorAfterBlock) {
    insert = withTrailingNewlines(insert, options.trailingNewlines ?? 2);
  }
  const cursor = selection.from + insert.length;
  view.dispatch({
    changes: { from: selection.from, to: selection.to, insert },
    selection: { anchor: cursor },
    scrollIntoView: true,
    userEvent: 'input',
  });
  view.focus();
}

function replaceComposerContent(view: EditorView, content: string, selection: number): void {
  const nextSelection = Math.max(0, Math.min(selection, content.length));
  view.dispatch({
    changes: {
      from: 0,
      to: view.state.doc.length,
      insert: content,
    },
    selection: { anchor: nextSelection },
    scrollIntoView: true,
    userEvent: 'input',
  });
  view.focus();
}

function contentReferencesImagePath(content: string, documentPath: string, targetPath: string): boolean {
  const normalizedDocumentPath = (documentPath || '').trim();
  const normalizedTargetPath = (targetPath || '').trim();
  if (!normalizedDocumentPath || !normalizedTargetPath) {
    return false;
  }
  for (const rawLine of content.replace(/\r\n/g, '\n').split('\n')) {
    const parsed = parseMarkdownImage(rawLine.trim());
    if (!parsed) {
      continue;
    }
    const resolvedPath = resolveMarkdownPath(normalizedDocumentPath, parsed.url, false);
    if (resolvedPath === normalizedTargetPath) {
      return true;
    }
  }
  return false;
}

function getDroppedEntryLabel(path: string, isDir: boolean): string {
  const normalized = path.trim().replace(/\/+$/, '');
  const parts = normalized.split('/').filter(Boolean);
  const last = parts[parts.length - 1] || normalized;
  if (last) {
    return last;
  }
  return isDir ? 'folder' : 'file';
}

function resolveSkillSlug(
  meta: Record<string, unknown>,
  cwd: string | null | undefined,
  uri: string | null | undefined,
): string {
  const rawSlug = typeof meta.slug === 'string' ? meta.slug.trim() : '';
  if (rawSlug) {
    return rawSlug;
  }
  const normalizedCwd = (cwd || '').trim().replace(/\/+$/, '');
  if (normalizedCwd) {
    const parts = normalizedCwd.split('/').filter(Boolean);
    if (parts.length > 0) {
      return parts[parts.length - 1];
    }
  }
  const normalizedUri = (uri || '').trim();
  if (normalizedUri.startsWith('file://')) {
    const decoded = normalizedUri.slice('file://'.length).replace(/\/+$/, '');
    const parts = decoded.split('/').filter(Boolean);
    if (parts.length >= 2) {
      return parts[parts.length - 2];
    }
  }
  return '';
}

type ConversationListItem =
  | { kind: 'thread'; key: string; threadID: string; chatPath?: string; tabId?: string; title?: string }
  | { kind: 'command'; key: string; path: string; tabId: string }
  | { kind: 'pending'; key: string; id: string };

function toConversationTarget(item: ConversationListItem | null): ConversationTarget {
  if (!item) {
    return null;
  }
  switch (item.kind) {
    case 'thread':
      return { kind: 'thread', threadID: item.threadID, ...(item.chatPath ? { chatPath: item.chatPath } : {}) };
    case 'command':
      return { kind: 'command', path: item.path };
    case 'pending':
      return { kind: 'pending', id: item.id };
  }
}

function getFallbackConversationTarget(items: ConversationListItem[], removedKey: string): ConversationTarget {
  const removedIndex = items.findIndex((item) => item.key === removedKey);
  if (removedIndex < 0) {
    return null;
  }
  const remaining = items.filter((item) => item.key !== removedKey);
  if (remaining.length === 0) {
    return null;
  }
  return toConversationTarget(remaining[Math.min(removedIndex, remaining.length - 1)] || null);
}

function ConversationCloseButton({
  onClick,
  title = 'Close composer dock',
}: {
  onClick: (e: React.MouseEvent) => void;
  title?: string;
}) {
  return (
    <IconButton variant="inline" className="group relative" onClick={onClick} title={title} aria-label={title}>
      <span className="absolute h-2.5 w-2.5 rounded-full bg-tertiary-text opacity-50 transition-opacity duration-150 group-hover:opacity-0" />
      <svg
        className="w-3.5 h-3.5 opacity-0 transition-opacity duration-150 group-hover:opacity-100"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <line x1="6" y1="6" x2="18" y2="18" />
        <line x1="18" y1="6" x2="6" y2="18" />
      </svg>
    </IconButton>
  );
}

export function ConversationComposerDock({
  showTopBorder = true,
}: {
  showTopBorder?: boolean;
}) {
  const selectedConversationTarget = useChatWorkspaceStore((state) => state.selectedConversationTarget);
  const composerFocusRequestSeq = useChatWorkspaceStore((state) => state.composerFocusRequestSeq);
  const selectedTargetInProgress = useChatWorkspaceStore((state) => state.isTargetInProgress(state.selectedConversationTarget));
  const selectedThreadState = useChatWorkspaceStore((state) => state.getThreadStateForTarget(state.selectedConversationTarget));
  const pendingConversations = useChatWorkspaceStore((state) => state.pendingConversations);
  const openComposerTargets = useChatWorkspaceStore((state) => state.openComposerTargets);
  const pendingComposerInsertQueue = useChatWorkspaceStore((state) => state.pendingComposerInsertQueue);
  const queuedMessages = useChatWorkspaceStore((state) => state.getQueuedMessagesForTarget(state.selectedConversationTarget));
  const selectedSkill = useChatWorkspaceStore((state) => state.selectedSkill);
  const selectedPlanBlock = useChatWorkspaceStore((state) => state.getComposerPlanStateForTarget(state.selectedConversationTarget));
  const rawDraft = useChatWorkspaceStore((state) => state.getDraftForTarget(state.selectedConversationTarget));
  const setDraftForSelectedTarget = useChatWorkspaceStore((state) => state.setDraftForSelectedTarget);
  const createPendingConversation = useChatWorkspaceStore((state) => state.createPendingConversation);
  const selectPendingConversation = useChatWorkspaceStore((state) => state.selectPendingConversation);
  const closeComposerTarget = useChatWorkspaceStore((state) => state.closeComposerTarget);
  const removePendingConversation = useChatWorkspaceStore((state) => state.removePendingConversation);
  const requestComposerBlockInsert = useChatWorkspaceStore((state) => state.requestComposerBlockInsert);
  const consumeComposerBlockInsert = useChatWorkspaceStore((state) => state.consumeComposerBlockInsert);
  const agentID = useChatWorkspaceStore((state) => state.agentID);
  const agentName = useChatWorkspaceStore((state) => state.agentName);
  const agentCwd = useChatWorkspaceStore((state) => state.agentCwd);
  const selectedAgentTarget = useChatWorkspaceStore((state) => state.getAgentForTarget(state.selectedConversationTarget));
  const gbrainQueryScope = useChatWorkspaceStore((state) => state.gbrainQueryScope);
  const clearGBrainQueryScope = useChatWorkspaceStore((state) => state.clearGBrainQueryScope);
  const setAgentInfo = useChatWorkspaceStore((state) => state.setAgentInfo);
  const setAgentForSelectedTarget = useChatWorkspaceStore((state) => state.setAgentForSelectedTarget);
  const selectedModelKey = useChatWorkspaceStore((state) => state.getSelectedModelKey());
  const inputMode = useChatWorkspaceStore((state) => state.inputMode);
  const activeCommand = useChatWorkspaceStore((state) => state.activeCommand);
  const setInputMode = useChatWorkspaceStore((state) => state.setInputMode);
  const setSelectedSkill = useChatWorkspaceStore((state) => state.setSelectedSkill);
  const clearSelectedSkill = useChatWorkspaceStore((state) => state.clearSelectedSkill);
  const setComposerPlanStateForTarget = useChatWorkspaceStore((state) => state.setComposerPlanStateForTarget);
  const clearComposerPlanStateForTarget = useChatWorkspaceStore((state) => state.clearComposerPlanStateForTarget);
  const setSelectedModelKey = useChatWorkspaceStore((state) => state.setSelectedModelKey);
  const setRememberedModelKey = useChatWorkspaceStore((state) => state.setRememberedModelKey);
  const syncChatSettings = useChatWorkspaceStore((state) => state.syncChatSettings);
  const hideComposer = useChatWorkspaceStore((state) => state.hideComposer);
  const targetChatPath = useChatWorkspaceStore((state) => state.targetChatPath);
  const pathToThreadID = useChatWorkspaceStore((state) => state.pathToThreadID);
  const selectChatConversation = useChatWorkspaceStore((state) => state.selectChatConversation);
  const selectThreadConversation = useChatWorkspaceStore((state) => state.selectThreadConversation);
  const removeLiveOverlay = useChatWorkspaceStore((state) => state.removeLiveOverlay);
  const getConversationRunStatus = useChatWorkspaceStore((state) => state.getConversationRunStatus);
  const getAwaitingUser = useChatWorkspaceStore((state) => state.getAwaitingUser);
  const hasBlockingModal = useUiStore((state) => state.hasBlockingModal);
  const [gbrainScopePromptVisible, setGBrainScopePromptVisible] = useState(false);

  const documents = useAppStore((state) => state.documents);
  const activeEditorTabId = useAppStore((state) => state.activeTabId);
  const pinnedTabId = useAppStore((state) => state.pinnedTabId);
  const tabs = useMemo(
    () => documents.filter((tab): tab is typeof tab & { filePath: string } => (
      tab.documentRole === 'conversation' && Boolean((tab.filePath || '').trim())
    )),
    [documents],
  );
  const skillNodes = useAppStore((state) => state.skillNodes);
  const setActiveConversationTab = useAppStore((state) => state.setActiveConversationTab);
  const closeTab = useAppStore((state) => state.closeTab);
  const deleteEntry = useAppStore((state) => state.deleteEntry);
  const writeBase64File = useAppStore((state) => state.writeBase64File);
  const getChatAgentForCwd = useAppStore((state) => state.getChatAgentForCwd);
  const agentNodes = useAppStore((state) => state.agentNodes);
  const agentsRootDir = useAppStore((state) => state.agentsRootDir);
  const currentDir = useAppStore((state) => state.currentDir);
  const resolveAgentByID = useAppStore((state) => state.resolveAgentByID);
  const ensureDerivedDirs = useAppStore((state) => state.ensureDerivedDirs);
  const agentNodesLoading = useAppStore((state) => state.agentNodesLoading);
  const refreshAgentNodes = useAppStore((state) => state.refreshAgentNodes);
  const getAgentSubagents = useAppStore((state) => state.getAgentSubagents);
  const getMountableAgentSubagents = useAppStore((state) => state.getMountableAgentSubagents);
  const mountAgentSubagent = useAppStore((state) => state.mountAgentSubagent);
  const unmountAgentSubagent = useAppStore((state) => state.unmountAgentSubagent);
  const nodeGraphRevision = useAppStore((state) => state.nodeGraphRevision);

  const loadModels = useModelsStore((state) => state.load);
  const modelsConfig = useModelsStore((state) => state.config);
  const setModelPreference = useModelsStore((state) => state.setModelPreference);
  const pushToast = useToastStore((state) => state.pushToast);
  const openModelsTab = useAppStore((state) => state.openModelsTab);
  const showBillingReminder = useBillingReminderStore((state) => state.show);

  const [agentPickerOpen, setAgentPickerOpen] = useState(false);
  const [modelPickerOpen, setModelPickerOpen] = useState(false);
  const [thinkingPickerOpen, setThinkingPickerOpen] = useState(false);
  const [editingModelKey, setEditingModelKey] = useState<string | null>(null);
  const [agentPickerRefreshing, setAgentPickerRefreshing] = useState(false);
  const [subagentPickerOpen, setSubagentPickerOpen] = useState(false);
  const [subagentAvailableOpen, setSubagentAvailableOpen] = useState(false);
  const [subagentPickerRefreshing, setSubagentPickerRefreshing] = useState(false);
  const [mountingSubagentID, setMountingSubagentID] = useState<string | null>(null);
  const [removingSubagentID, setRemovingSubagentID] = useState<string | null>(null);
  const [billingSubscription, setBillingSubscription] = useState<DesktopBillingSubscription | null>(null);
  const agentPickerRef = useRef<HTMLDivElement | null>(null);
  const subagentPickerRef = useRef<HTMLDivElement | null>(null);
  const modelPickerRef = useRef<HTMLDivElement | null>(null);
  const thinkingPickerRef = useRef<HTMLDivElement | null>(null);
  const gbrainScopePopoverRef = useRef<HTMLSpanElement | null>(null);
  const composerRef = useRef<ChatMarkdownComposerHandle | null>(null);
  const lastComposerFocusRequestSeqRef = useRef(0);
  const imageInputRef = useRef<HTMLInputElement | null>(null);
  const [isDragOver, setIsDragOver] = useState(false);
  const [slashCursorPos, setSlashCursorPos] = useState<number | null>(null);
  const [slashHighlightedIndex, setSlashHighlightedIndex] = useState(0);
  const [dismissedSlashToken, setDismissedSlashToken] = useState<string | null>(null);
  const [slashMenuPosition, setSlashMenuPosition] = useState<{ left: number; top: number } | null>(null);
  const requestedSlashRefreshRef = useRef(false);
  const composerShellRef = useRef<HTMLDivElement | null>(null);

  const chatTabs = tabs;
  const conversationItems = useMemo<ConversationListItem[]>(() => {
    const items: ConversationListItem[] = chatTabs
      .filter((tab): tab is typeof tab & { filePath: string } => Boolean(tab.filePath))
      .map((tab) => {
        const threadID = (tab.threadID || pathToThreadID[tab.filePath] || '').trim();
        return threadID
          ? {
            kind: 'thread' as const,
            key: `thread:${threadID}`,
            threadID,
            chatPath: tab.filePath,
            tabId: tab.id,
          }
          : {
            kind: 'command' as const,
            key: `command:${tab.filePath}`,
            path: tab.filePath,
            tabId: tab.id,
          };
      });
    const existingThreadIDs = new Set(
      items
        .filter((item): item is Extract<ConversationListItem, { kind: 'thread' }> => item.kind === 'thread')
        .map((item) => item.threadID),
    );
    for (const thread of openComposerTargets) {
      const threadID = (thread.threadID || '').trim();
      if (!threadID || existingThreadIDs.has(threadID)) {
        continue;
      }
      items.push({
        kind: 'thread',
        key: `thread:${threadID}`,
        threadID,
        ...(thread.chatPath ? { chatPath: thread.chatPath } : {}),
        title: thread.title || threadID,
      });
    }
    items.push(...pendingConversations.map((pending: PendingConversation) => ({ kind: 'pending' as const, key: `pending:${pending.id}`, id: pending.id })));
    return items.filter((item) => item.key);
  }, [chatTabs, openComposerTargets, pathToThreadID, pendingConversations]);

  const focusComposer = useCallback(() => {
    requestAnimationFrame(() => {
      composerRef.current?.focus();
    });
  }, []);

  const insertComposerBlockMarkdown = useCallback((markdown: string): boolean => {
    const view = composerRef.current?.getView();
    if (!view) {
      return false;
    }
    insertBlockMarkdown(view, markdown, { leaveCursorAfterBlock: true, trailingNewlines: 1 });
    return true;
  }, []);

  useEffect(() => {
    if (composerFocusRequestSeq === 0 || composerFocusRequestSeq === lastComposerFocusRequestSeqRef.current) {
      return;
    }
    lastComposerFocusRequestSeqRef.current = composerFocusRequestSeq;
    focusComposer();
  }, [composerFocusRequestSeq, focusComposer]);

  useEffect(() => {
    if (pendingComposerInsertQueue.length === 0) {
      return;
    }

    let cancelled = false;
    let frame = 0;

    const attemptInsert = () => {
      if (cancelled) {
        return;
      }
      const nextInsert = useChatWorkspaceStore.getState().pendingComposerInsertQueue[0];
      if (!nextInsert) {
        return;
      }
      if (!insertComposerBlockMarkdown(nextInsert.markdown)) {
        frame = requestAnimationFrame(attemptInsert);
        return;
      }
      consumeComposerBlockInsert(nextInsert.id);
      focusComposer();
    };

    attemptInsert();
    return () => {
      cancelled = true;
      if (frame) {
        cancelAnimationFrame(frame);
      }
    };
  }, [consumeComposerBlockInsert, focusComposer, insertComposerBlockMarkdown, pendingComposerInsertQueue]);

  const draft = rawDraft;
  const displayedSelectedSkill = selectedSkill;
  const selectedTargetKey = useMemo(
    () => getConversationTargetKey(selectedConversationTarget),
    [selectedConversationTarget],
  );
  const selectedChatPath = useChatWorkspaceStore((state) => state.getTargetChatPath(state.selectedConversationTarget));
  const selectedThreadStateKey = selectedConversationTarget?.kind === 'thread'
    ? selectedChatPath || selectedConversationTarget.threadID
    : null;
  const selectedAwaitingUser = selectedConversationTarget?.kind === 'thread'
    ? getAwaitingUser(selectedThreadStateKey)
    : null;
  const hasPendingUserRequest = Boolean(selectedAwaitingUser);
  const authLoggedIn = useAuthStore((state) => state.loggedIn);
  const hasQueuedMessages = queuedMessages.steering.length > 0 || queuedMessages.followUp.length > 0;
  const canContinueSelectedThread = hasQueuedMessages || selectedThreadState.tailStatus === 'needs_continuation';
  const flattenedQueuedMessages = useMemo(
    () => flattenQueuedMessages(queuedMessages),
    [queuedMessages],
  );

  const isCommandMode = inputMode === 'command';
  const isComposerReadOnly = hasPendingUserRequest || (isCommandMode && selectedTargetInProgress);
  const pendingResources = useMemo<PendingResource[]>(() => {
    if (isCommandMode) {
      return [];
    }
    const resources: PendingResource[] = [];
    if (displayedSelectedSkill && displayedSelectedSkill.slug !== 'plan') {
      resources.push({
        kind: 'skill',
        key: `skill:${displayedSelectedSkill.id}`,
        skill: displayedSelectedSkill,
      });
    }
    return resources;
  }, [displayedSelectedSkill, isCommandMode]);

  const skillOptions = useMemo<SkillOption[]>(() => {
    return skillNodes
      .filter((node) => (node.kind || '').trim().toLowerCase() === 'skill')
      .map((node) => {
        const meta = ((node.meta as Record<string, unknown> | undefined) || {});
        const slug = resolveSkillSlug(meta, node.cwd, node.uri);
        const name = (typeof meta.name === 'string' ? meta.name : '').trim() || slug;
        const description = (typeof meta.description === 'string' ? meta.description : '').trim();
        return {
          id: (node.id || '').trim(),
          slug,
          name,
          description,
        };
      })
      .filter((option) => option.id && option.slug && option.name)
      .sort((a, b) => a.slug.localeCompare(b.slug));
  }, [skillNodes]);
  const builtInSlashCommands = useMemo<BuiltInSlashCommand[]>(() => ([
    {
      key: 'builtin-compact',
      slug: 'compact',
      name: 'Compact',
      description: 'Compact the current thread context',
    },
  ]), []);
  const slashMenuState = useMemo(() => resolveSlashMenuState({
    draft,
    cursorPos: slashCursorPos,
    isCommandMode,
    isQueuedReadOnly: isComposerReadOnly,
    dismissedSlashToken,
    skillOptions,
    builtInCommands: builtInSlashCommands,
    agentNodesLoading,
  }), [
    agentNodesLoading,
    dismissedSlashToken,
    builtInSlashCommands,
    draft,
    isCommandMode,
    isComposerReadOnly,
    skillOptions,
    slashCursorPos,
  ]);
  const slashState = slashMenuState.slashState;
  const filteredSlashItems = slashMenuState.filteredItems;
  const slashMenuVisible = slashMenuState.status !== 'hidden';
  const slashMenuHasResults = slashMenuState.status === 'results';
  const slashSendItems = useMemo<SlashMenuItem[]>(() => {
    if (filteredSlashItems.length > 0) {
      return filteredSlashItems;
    }
    if (!slashState || isCommandMode || isComposerReadOnly) {
      return [];
    }
    return [
      ...filterBuiltInSlashCommands(builtInSlashCommands, slashState.query).map((command) => ({ kind: 'command' as const, ...command })),
      ...filterSlashSkillOptions(skillOptions, slashState.query).map((option) => ({ kind: 'skill' as const, ...option })),
    ];
  }, [builtInSlashCommands, filteredSlashItems, isCommandMode, isComposerReadOnly, skillOptions, slashState]);

  const applyConversationTarget = useCallback((target: ConversationTarget) => {
    if (!target) {
      selectChatConversation(null);
      return;
    }
    if (target.kind === 'thread') {
      selectThreadConversation(target.threadID, target.chatPath);
      const nextTab = chatTabs.find((tab) => tab.threadID === target.threadID || tab.filePath === target.chatPath);
      if (nextTab) {
        setActiveConversationTab(nextTab.id);
      }
      return;
    }
    if (target.kind === 'command') {
      selectChatConversation(target.path);
      const nextTab = chatTabs.find((tab) => tab.filePath === target.path);
      if (nextTab) {
        setActiveConversationTab(nextTab.id);
      }
      return;
    }
    selectPendingConversation(target.id);
    focusComposer();
  }, [chatTabs, focusComposer, selectChatConversation, selectPendingConversation, selectThreadConversation, setActiveConversationTab]);

  const effectiveTarget = useMemo(() => (
    selectedAgentTarget || resolveChatAgentTarget({
      selectedChatPath: targetChatPath,
      explicitAgentID: agentID,
      explicitAgentName: agentName,
      explicitAgentCwd: agentCwd,
      currentDir,
      resolveChatAgentForCwd: getChatAgentForCwd,
    })
  ), [agentCwd, getChatAgentForCwd, agentID, agentName, currentDir, selectedAgentTarget, targetChatPath]);
  const resolvedAgent = useMemo(
    () => (effectiveTarget?.agentID ? resolveAgentByID(effectiveTarget.agentID) : null),
    [effectiveTarget?.agentID, nodeGraphRevision, resolveAgentByID]
  );
  const effectiveAgentID = effectiveTarget?.agentID ?? null;
  const effectiveAgentName = effectiveTarget?.agentName ?? null;
  const effectiveAgentCwd = effectiveTarget?.agentCwd ?? null;
  const canAttachImages = inputMode === 'chat' && !hasPendingUserRequest && Boolean(effectiveAgentCwd);
  const agentSwitchTargetDir = useMemo(() => {
    const chatTargetDir = getChatWorkdir(targetChatPath);
    return effectiveAgentCwd || chatTargetDir || null;
  }, [effectiveAgentCwd, targetChatPath]);
  const canSwitchAgent = !isCommandMode && Boolean(effectiveAgentID);
  const agentPickerOptions = useMemo(
    () => buildAgentSwitchOptions({
      agentNodes,
      agentsRootDir,
      currentAgentID: effectiveAgentID,
    }),
    [agentNodes, agentsRootDir, effectiveAgentID]
  );
  const mountedSubagents = useMemo(
    () => (effectiveAgentID ? getAgentSubagents(effectiveAgentID) : []),
    [effectiveAgentID, getAgentSubagents, nodeGraphRevision]
  );
  const mountableSubagents = useMemo(
    () => (effectiveAgentID ? getMountableAgentSubagents(effectiveAgentID) : []),
    [effectiveAgentID, getMountableAgentSubagents, nodeGraphRevision]
  );
  const mountedSubagentCount = mountedSubagents.length;
  const mountableSubagentCount = mountableSubagents.length;
  const subagentNames = useMemo(
    () => mountedSubagents
      .map((subagent) => (subagent.name || subagent.id).trim())
      .filter(Boolean),
    [mountedSubagents],
  );
  const subagentPrimaryName = subagentNames[0] ?? '';
  const subagentExtraCount = Math.max(0, subagentNames.length - 1);
  const subagentDisplayTitle = useMemo(() => {
    if (subagentNames.length === 0) {
      return 'Subagents';
    }
    return mountedSubagents
      .map((subagent) => {
        const label = (subagent.name || subagent.id).trim();
        const path = (subagent.path || '').trim();
        return path ? `${label}\n${path}` : label;
      })
      .join('\n\n');
  }, [mountedSubagents, subagentNames.length]);
  const subagentAriaLabel = useMemo(() => {
    if (subagentNames.length === 0) {
      return 'Subagents';
    }
    if (subagentNames.length === 1) {
      return `Subagents: ${subagentNames[0]}`;
    }
    if (subagentNames.length === 2) {
      return `Subagents: ${subagentNames[0]} and 1 more`;
    }
    return `Subagents: ${subagentNames[0]} and ${subagentExtraCount} more`;
  }, [subagentExtraCount, subagentNames]);
  const composerDocumentPath = selectedChatPath;

  useEffect(() => {
    void loadModels();
  }, [loadModels]);

  useEffect(() => {
    let cancelled = false;
    if (!authLoggedIn) {
      setBillingSubscription(null);
      return undefined;
    }
    void getDesktopBillingSubscription()
      .then((subscription) => {
        if (!cancelled) {
          setBillingSubscription(subscription);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setBillingSubscription(null);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [authLoggedIn, modelPickerOpen]);

  const enabledModels = useMemo(() => modelsConfig.models.filter((model) => model.enabled), [modelsConfig.models]);
  const selectableModels = enabledModels;
  const defaultChatModelSelection = useMemo(
    () => resolveDefaultChatModelSelection(modelsConfig),
    [modelsConfig],
  );
  const defaultChatModelKey = defaultChatModelSelection.modelKey;
  const displayedModelKey = selectedModelKey || defaultChatModelKey;
  const modelSelection = useMemo(
    () => resolveChatModelSelection(enabledModels, displayedModelKey),
    [displayedModelKey, enabledModels]
  );
  const activeModel = modelSelection.effectiveModel;
  const effectiveModelKey = modelSelection.effectiveModelKey;
  const activeModelDisplay = useMemo(
    () => getModelDisplayInfo(
      activeModel?.id || effectiveModelKey || '',
      activeModel?.label,
      getVisibleProviderLabel(activeModel?.provider, activeModel?.providerLabel),
    ),
    [activeModel?.id, activeModel?.label, activeModel?.provider, activeModel?.providerLabel, effectiveModelKey]
  );
  const accountNeedsProviderModel = Boolean(billingSubscription)
    && billingSubscription?.bundledTokenEligible !== true;
  const activeModelPreference = useMemo(
    () => resolveChatModelPreference(modelsConfig, activeModel),
    [activeModel, modelsConfig],
  );
  const activeModelNeedsBundledTokenValue = useMemo(
    () => (effectiveModelKey ? modelRequiresBundledTokenValue(effectiveModelKey, activeModel) : false),
    [activeModel, effectiveModelKey]
  );
  const activeModelTriggerText = useMemo(() => {
    if (!effectiveModelKey) {
      return 'Select Model';
    }
    const detailParts = [
      activeModelPreference.thinkingLevel && activeModelPreference.thinkingLevel !== 'off'
        ? activeModelPreference.thinkingLevel
        : '',
      activeModelPreference.serviceTier === 'priority' ? 'Fast' : '',
    ].filter(Boolean);
    return detailParts.length > 0
      ? `${activeModelDisplay.triggerText} ${detailParts.join(' ')}`
      : activeModelDisplay.triggerText;
  }, [
    activeModelDisplay.triggerText,
    activeModelPreference.serviceTier,
    activeModelPreference.thinkingLevel,
    effectiveModelKey,
  ]);
  useEffect(() => {
    if (!selectedTargetKey || selectedModelKey || !defaultChatModelKey) {
      return;
    }
    setSelectedModelKey(defaultChatModelKey);
  }, [
    defaultChatModelKey,
    selectedModelKey,
    selectedTargetKey,
    setSelectedModelKey,
  ]);
  const editingModel = useMemo(
    () => selectableModels.find((model) => model.key === (editingModelKey || effectiveModelKey)) || activeModel,
    [activeModel, editingModelKey, effectiveModelKey, selectableModels],
  );
  const editingModelPreference = useMemo(
    () => resolveChatModelPreference(modelsConfig, editingModel),
    [editingModel, modelsConfig],
  );
  const editingThinkingOptions = useMemo(
    () => getThinkingPickerLevels(editingModel).map((level) => ({ level })),
    [editingModel],
  );
  useEffect(() => {
    if (!agentPickerOpen || !canSwitchAgent) {
      setAgentPickerRefreshing(false);
      return;
    }
    let active = true;
    setAgentPickerRefreshing(true);
    void ensureDerivedDirs()
      .then(() => refreshAgentNodes({ force: true }))
      .finally(() => {
        if (active) {
          setAgentPickerRefreshing(false);
        }
      });
    return () => {
      active = false;
    };
  }, [agentPickerOpen, canSwitchAgent, ensureDerivedDirs, refreshAgentNodes]);

  useEffect(() => {
    if (!subagentPickerOpen || !effectiveAgentID) {
      setSubagentPickerRefreshing(false);
      return;
    }
    let active = true;
    setSubagentPickerRefreshing(true);
    void ensureDerivedDirs()
      .then(() => refreshAgentNodes({ force: true }))
      .finally(() => {
        if (active) {
          setSubagentPickerRefreshing(false);
        }
      });
    return () => {
      active = false;
    };
  }, [effectiveAgentID, ensureDerivedDirs, refreshAgentNodes, subagentPickerOpen]);

  useEffect(() => {
    if (!subagentPickerOpen) {
      setSubagentAvailableOpen(false);
    }
  }, [subagentPickerOpen]);

  useEffect(() => {
    if (selectedConversationTarget?.kind !== 'thread' || !isThreadChatPath(selectedChatPath)) {
      return;
    }
    if (!selectedChatPath) {
      return;
    }
    void refreshThreadState(selectedChatPath).catch(() => {
      // Keep selection lightweight; main submit paths refresh thread state explicitly.
    });
  }, [selectedConversationTarget?.kind === 'thread' ? selectedConversationTarget.threadID : null, selectedChatPath]);

  const persistChatSettings = useCallback(async (
    chatPath: string,
    settings: { modelKey?: string | null }
  ) => {
    if (!chatPath) {
      return;
    }
    syncChatSettings(chatPath, settings);
  }, [syncChatSettings]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const isToggle = (event.metaKey || event.ctrlKey) && event.shiftKey && event.key === ';';
      if (!isToggle) return;
      event.preventDefault();
      setInputMode(inputMode === 'chat' ? 'command' : 'chat');
    };
    window.addEventListener('keydown', onKeyDown, true);
    return () => window.removeEventListener('keydown', onKeyDown, true);
  }, [inputMode, setInputMode]);

  const dismissPickers = useCallback(() => {
    setAgentPickerOpen(false);
    setSubagentPickerOpen(false);
    setModelPickerOpen(false);
    setThinkingPickerOpen(false);
  }, []);

  const anyPickerOpen = agentPickerOpen || subagentPickerOpen || modelPickerOpen || thinkingPickerOpen;

  useDismissOnOutsideInteraction({
    active: anyPickerOpen,
    onDismiss: dismissPickers,
    insideRefs: [agentPickerRef, subagentPickerRef, modelPickerRef, thinkingPickerRef],
  });

  const dismissGBrainScopePrompt = useCallback(() => {
    setGBrainScopePromptVisible(false);
  }, []);

  useDismissOnOutsideInteraction({
    active: gbrainScopePromptVisible,
    onDismiss: dismissGBrainScopePrompt,
    insideRefs: [gbrainScopePopoverRef],
  });

  useEffect(() => {
    if (!isCommandMode) {
      return;
    }
    setAgentPickerOpen(false);
    setSubagentPickerOpen(false);
  }, [isCommandMode]);

  useEffect(() => {
    if (!slashState) {
      requestedSlashRefreshRef.current = false;
      if (dismissedSlashToken != null) {
        setDismissedSlashToken(null);
      }
      return;
    }
    if (dismissedSlashToken && dismissedSlashToken !== slashState.token) {
      setDismissedSlashToken(null);
    }
  }, [dismissedSlashToken, slashState]);

  useEffect(() => {
    if (!slashMenuVisible || !slashMenuHasResults) {
      if (slashHighlightedIndex !== 0) {
        setSlashHighlightedIndex(0);
      }
      return;
    }
    if (slashHighlightedIndex >= filteredSlashItems.length) {
      setSlashHighlightedIndex(0);
    }
  }, [filteredSlashItems.length, slashHighlightedIndex, slashMenuHasResults, slashMenuVisible]);

  useEffect(() => {
    if (!slashState) {
      return;
    }
    if (skillOptions.length > 0 || agentNodesLoading || requestedSlashRefreshRef.current) {
      return;
    }
    requestedSlashRefreshRef.current = true;
    void refreshAgentNodes({ force: true }).catch(() => {});
  }, [agentNodesLoading, refreshAgentNodes, skillOptions.length, slashState]);

  const handlePlanBlockStateChange = useCallback((planState: ComposerPlanState) => {
    if (!selectedConversationTarget) {
      return;
    }
    setComposerPlanStateForTarget(selectedConversationTarget, planState);
  }, [selectedConversationTarget, setComposerPlanStateForTarget]);

  const removeActivePlanBlock = useCallback((options?: { clearSkill?: boolean }) => {
    const chatState = useChatWorkspaceStore.getState();
    const target = chatState.selectedConversationTarget;
    if (!target) {
      if (options?.clearSkill !== false) {
        clearSelectedSkill();
      }
      return;
    }
    const planState = chatState.getComposerPlanStateForTarget(target);
    const view = composerRef.current?.getView() || null;
    if (planState) {
      const currentContent = view ? view.state.doc.toString() : chatState.getDraftForTarget(target);
      const currentSelection = view ? view.state.selection.main.head : currentContent.length;
      const removed = removeComposerPlanBlock({
        content: currentContent,
        cursor: currentSelection,
        planState,
      });
      if (view) {
        if (removed.content !== currentContent) {
          replaceComposerContent(view, removed.content, removed.selection);
        } else if (removed.selection !== currentSelection) {
          view.dispatch({
            selection: { anchor: removed.selection },
            scrollIntoView: true,
          });
          view.focus();
        }
      } else {
        setDraftForSelectedTarget(removed.content);
      }
    }
    clearComposerPlanStateForTarget(target);
    if (options?.clearSkill !== false) {
      clearSelectedSkill();
    }
  }, [
    clearComposerPlanStateForTarget,
    clearSelectedSkill,
    setDraftForSelectedTarget,
  ]);

  const activateSelectedPlanSkill = useCallback((
    option: SkillOption,
    options?: { draftOverride?: string; cursorOverride?: number },
  ) => {
    let target = useChatWorkspaceStore.getState().selectedConversationTarget;
    if (!target) {
      const pendingID = createPendingConversation();
      target = { kind: 'pending', id: pendingID };
    }

    const chatState = useChatWorkspaceStore.getState();
    const currentPlan = chatState.getComposerPlanStateForTarget(target);
    const view = composerRef.current?.getView() || null;
    const currentContent = view
      ? view.state.doc.toString()
      : chatState.getDraftForTarget(target);
    const nextContent = options?.draftOverride ?? currentContent;
    const nextCursor = options?.cursorOverride
      ?? (view ? view.state.selection.main.head : nextContent.length);
    const activated = activateComposerPlanBlock({
      content: nextContent,
      cursor: nextCursor,
      currentPlan,
    });

    if (view) {
      replaceComposerContent(view, activated.content, activated.selection);
    } else {
      setDraftForSelectedTarget(activated.content);
    }
    setComposerPlanStateForTarget(target, activated.planState);
    setSelectedSkill({
      id: option.id,
      slug: option.slug,
      name: option.name,
    });
    setDismissedSlashToken(null);
    setSlashHighlightedIndex(0);
    setSlashCursorPos(activated.selection);
    focusComposer();
  }, [
    createPendingConversation,
    focusComposer,
    setComposerPlanStateForTarget,
    setDraftForSelectedTarget,
    setSelectedSkill,
  ]);

  const applySelectedSkillOption = useCallback((
    option: SkillOption,
    options?: { draft?: string; cursor?: number },
  ) => {
    if (option.slug === 'plan') {
      activateSelectedPlanSkill(option, {
        draftOverride: options?.draft,
        cursorOverride: options?.cursor,
      });
      return;
    }

    removeActivePlanBlock({ clearSkill: false });
    setSelectedSkill({
      id: option.id,
      slug: option.slug,
      name: option.name,
    });
    if (options?.draft !== undefined) {
      setDraftForSelectedTarget(options.draft);
    }
    setDismissedSlashToken(null);
    setSlashHighlightedIndex(0);
    setSlashCursorPos(options?.cursor ?? 0);
    focusComposer();
  }, [
    activateSelectedPlanSkill,
    focusComposer,
    removeActivePlanBlock,
    setDraftForSelectedTarget,
    setSelectedSkill,
  ]);

  const handleSlashItemSelect = useCallback((item: SlashMenuItem) => {
    if (!slashState) {
      return;
    }

    const nextDraft = removeSlashTokenFromDraft(draft, slashState);
    setDraftForSelectedTarget(nextDraft);
    setDismissedSlashToken(null);
    setSlashHighlightedIndex(0);
    setSlashCursorPos(null);

    if (item.kind === 'command') {
      if (item.slug === 'compact') {
        void compactCurrentThread().catch((error) => {
          pushToast(error instanceof Error ? error.message : 'Compact failed');
        });
      }
      return;
    }

    applySelectedSkillOption(item, {
      draft: nextDraft,
      cursor: Math.min(slashState.tokenStart, nextDraft.length),
    });
  }, [
    applySelectedSkillOption,
    draft,
    setDraftForSelectedTarget,
    slashState,
  ]);

  const handleGlobalPlanSkillHotkey = useCallback((event: KeyboardEvent) => {
    if (useUiStore.getState().hasBlockingModal) {
      return;
    }
    const planShortcutAction = resolvePlanSkillShortcutAction({
      key: event.key,
      shiftKey: event.shiftKey,
      altKey: event.altKey,
      metaKey: event.metaKey,
      ctrlKey: event.ctrlKey,
      isImeComposing: isImeComposingEvent(event),
      isCommandMode,
      isQueuedReadOnly: isComposerReadOnly,
      skillOptions,
      agentNodesLoading,
    });
    if (planShortcutAction.action === 'ignore') {
      return;
    }

    event.preventDefault();
    event.stopPropagation();

    if (planShortcutAction.action === 'select') {
      applySelectedSkillOption(planShortcutAction.option);
      return;
    }
    if (planShortcutAction.action === 'loading') {
      pushToast('Plan skill is still loading');
      return;
    }
    pushToast('Plan skill is unavailable');
  }, [
    agentNodesLoading,
    applySelectedSkillOption,
    isCommandMode,
    isComposerReadOnly,
    pushToast,
    skillOptions,
  ]);

  useEffect(() => {
    if (hasBlockingModal) {
      return undefined;
    }
    window.addEventListener('keydown', handleGlobalPlanSkillHotkey, true);
    return () => window.removeEventListener('keydown', handleGlobalPlanSkillHotkey, true);
  }, [handleGlobalPlanSkillHotkey, hasBlockingModal]);

  useEffect(() => {
    if (isCommandMode || displayedSelectedSkill?.slug !== 'plan' || !selectedTargetKey || selectedPlanBlock) {
      return;
    }
    activateSelectedPlanSkill({
      id: displayedSelectedSkill.id,
      slug: displayedSelectedSkill.slug,
      name: displayedSelectedSkill.name,
      description: '',
    });
  }, [
    activateSelectedPlanSkill,
    displayedSelectedSkill?.id,
    displayedSelectedSkill?.name,
    displayedSelectedSkill?.slug,
    isCommandMode,
    selectedPlanBlock,
    selectedTargetKey,
  ]);

  const handleSend = useCallback(() => {
    const trimmedDraft = draft.trim();
    if (trimmedDraft === '/compact' && inputMode === 'chat' && selectedConversationTarget?.kind === 'thread') {
      void compactCurrentThread().then(() => {
        setDraftForSelectedTarget('');
      }).catch((error) => {
        pushToast(error instanceof Error ? error.message : 'Compact failed');
      });
      return;
    }
    const nextSlashItem = slashSendItems[slashHighlightedIndex] || slashSendItems[0];
    if (nextSlashItem) {
      handleSlashItemSelect(nextSlashItem);
      return;
    }
    if (hasPendingUserRequest) {
      return;
    }
    void submitChatTurn().catch((error) => {
      pushToast(error instanceof Error ? error.message : 'Submit chat turn failed');
    });
  }, [
    draft,
    handleSlashItemSelect,
    inputMode,
    pushToast,
    hasPendingUserRequest,
    selectedConversationTarget,
    setDraftForSelectedTarget,
    slashHighlightedIndex,
    slashSendItems,
  ]);

  const handleFollowUp = useCallback(() => {
    void queueFollowUp().catch((error) => {
      pushToast(error instanceof Error ? error.message : 'Queue follow-up failed');
    });
  }, [pushToast]);

  const handleStop = useCallback(() => {
    if (selectedChatPath && activeCommand?.filePath === selectedChatPath) {
      void stopCommand();
      return;
    }
    if (inputMode === 'command' && activeCommand) {
      void stopCommand();
      return;
    }
    void stopStream();
  }, [activeCommand, inputMode, selectedChatPath]);

  const handleQueuedPromote = useCallback((message: QueuedMessage) => {
    void promoteQueuedMessage({
      chatPath: message.chatPath,
      itemID: message.id,
    }).catch((error) => {
      pushToast(error instanceof Error ? error.message : 'Promote follow-up failed');
    });
  }, [pushToast]);

  const handleQueuedEdit = useCallback((message: QueuedMessage) => {
    void editQueuedMessage({
      chatPath: message.chatPath,
      itemID: message.id,
      queueKind: message.kind,
    }).catch((error) => {
      pushToast(error instanceof Error ? error.message : 'Edit queued message failed');
    });
  }, [pushToast]);

  const handleQueuedDelete = useCallback((message: QueuedMessage) => {
    void removeQueuedMessage({
      chatPath: message.chatPath,
      itemID: message.id,
      queueKind: message.kind,
    }).catch((error) => {
      pushToast(error instanceof Error ? error.message : 'Remove queued message failed');
    });
  }, [pushToast]);

  const hasSubmissionContent = hasConversationSubmissionContent({
    draft,
    selectedSkillSlug: displayedSelectedSkill?.slug || null,
  });
  const primaryButtonMode = useMemo<ConversationPrimaryButtonMode>(() => (
    getConversationPrimaryButtonMode({
      isCommandMode,
      isSelectedTargetInProgress: selectedTargetInProgress,
      canContinueSelectedThread,
      hasSubmissionContent,
    })
  ), [canContinueSelectedThread, hasSubmissionContent, isCommandMode, selectedTargetInProgress]);

  const handleChatTabSelect = useCallback((tabId: string, tabPath?: string, threadID?: string) => {
    const nextPath = tabPath || null;
    const normalizedThreadID = (threadID || '').trim();
    setActiveConversationTab(tabId);
    if (normalizedThreadID) {
      selectThreadConversation(normalizedThreadID, nextPath);
    } else {
      selectChatConversation(nextPath);
    }

  }, [selectChatConversation, selectThreadConversation, setActiveConversationTab]);

  const handleChatTabClose = useCallback((tabId: string, tabPath?: string) => {
    if (!tabPath) {
      closeTab(tabId);
      return;
    }
    const tab = chatTabs.find((candidate) => candidate.id === tabId || candidate.filePath === tabPath);
    const tabThreadID = (tab?.threadID || '').trim();
    const isSelected = tabThreadID
      ? selectedConversationTarget?.kind === 'thread' && selectedConversationTarget.threadID === tabThreadID
      : selectedConversationTarget?.kind === 'command' && selectedConversationTarget.path === tabPath;
    const removedKey = tabThreadID ? `thread:${tabThreadID}` : `command:${tabPath}`;
    const fallbackTarget = isSelected
      ? getFallbackConversationTarget(conversationItems, removedKey)
      : null;

    removeLiveOverlay(tabPath);
    closeTab(tabId);

    if (isSelected) {
      applyConversationTarget(fallbackTarget);
    }
  }, [applyConversationTarget, chatTabs, closeTab, conversationItems, removeLiveOverlay, selectedConversationTarget]);

  const handlePendingConversationSelect = useCallback((pendingId: string) => {
    selectPendingConversation(pendingId);
    focusComposer();
  }, [focusComposer, selectPendingConversation]);

  const handlePendingConversationClose = useCallback((pendingId: string) => {
    const isSelected = selectedConversationTarget?.kind === 'pending' && selectedConversationTarget.id === pendingId;
    const fallbackTarget = isSelected
      ? getFallbackConversationTarget(conversationItems, `pending:${pendingId}`)
      : null;

    removePendingConversation(pendingId);

    if (isSelected) {
      applyConversationTarget(fallbackTarget);
    }
  }, [applyConversationTarget, conversationItems, removePendingConversation, selectedConversationTarget]);

  const handleCreateConversation = useCallback(() => {
    clearGBrainQueryScope();
    createPendingConversation();
    focusComposer();
  }, [
    clearGBrainQueryScope,
    createPendingConversation,
    focusComposer,
  ]);

  const handleAgentSelect = useCallback((nextAgentID: string, nextAgentName: string) => {
    const targetDir = (agentSwitchTargetDir || '').trim();
    const nextTarget = {
      agentID: nextAgentID,
      agentName: nextAgentName,
      agentCwd: targetDir,
    };
    setAgentForSelectedTarget(nextTarget);
    setAgentInfo(nextAgentID, nextAgentName, targetDir);
    setAgentPickerOpen(false);
    setSubagentPickerOpen(false);
  }, [agentSwitchTargetDir, setAgentForSelectedTarget, setAgentInfo]);

  const handleSubagentRemove = useCallback(async (subagentID: string) => {
    const parentID = (effectiveAgentID || '').trim();
    if (!parentID || !subagentID) {
      return;
    }
    setRemovingSubagentID(subagentID);
    try {
      const removed = await unmountAgentSubagent(parentID, subagentID);
      if (!removed) {
        pushToast('Failed to remove subagent');
        return;
      }
      pushToast('Subagent removed');
      setSubagentAvailableOpen(true);
    } finally {
      setRemovingSubagentID(null);
    }
  }, [effectiveAgentID, pushToast, unmountAgentSubagent]);

  const handleSubagentMount = useCallback(async (subagentID: string) => {
    const parentID = (effectiveAgentID || '').trim();
    if (!parentID || !subagentID) {
      return;
    }
    setMountingSubagentID(subagentID);
    try {
      const mounted = await mountAgentSubagent(parentID, subagentID);
      if (!mounted) {
        pushToast('Failed to attach subagent');
        return;
      }
      pushToast('Subagent attached');
      setSubagentAvailableOpen(false);
    } finally {
      setMountingSubagentID(null);
    }
  }, [effectiveAgentID, mountAgentSubagent, pushToast]);

  const handleModelSelect = useCallback((modelKey: string) => {
    setRememberedModelKey(modelKey);
    setSelectedModelKey(modelKey);
    if (selectedConversationTarget?.kind === 'thread' && selectedChatPath && isThreadChatPath(selectedChatPath)) {
      void persistChatSettings(selectedChatPath, { modelKey }).catch((error) => {
        pushToast(error instanceof Error ? error.message : '保存模型设置失败');
      });
    }
    setModelPickerOpen(false);
  }, [
    persistChatSettings,
    pushToast,
    selectedChatPath,
    selectedConversationTarget,
    setSelectedModelKey,
    setRememberedModelKey,
  ]);

  const handleThinkingLevelSelect = useCallback((level: ThinkingLevel, modelKey = effectiveModelKey, model = activeModel) => {
    if (!modelKey || !model) {
      return;
    }
    const normalizedLevel = normalizeThinkingLevelForModel(model, level);
    void setModelPreference(modelKey, { thinkingLevel: normalizedLevel }).catch((error) => {
      pushToast(error instanceof Error ? error.message : '保存 thinking 失败');
    });
  }, [activeModel, effectiveModelKey, pushToast, setModelPreference]);

  const handleContextWindowSelect = useCallback((contextWindow: number, modelKey = effectiveModelKey) => {
    if (!modelKey) {
      return;
    }
    void setModelPreference(modelKey, { contextWindow }).catch((error) => {
      pushToast(error instanceof Error ? error.message : '保存 context 失败');
    });
  }, [effectiveModelKey, pushToast, setModelPreference]);

  const persistPriorityMode = useCallback((enabled: boolean, modelKey = effectiveModelKey) => {
    if (!modelKey) {
      return;
    }
    void setModelPreference(modelKey, { serviceTier: enabled ? 'priority' : null }).catch((error) => {
      pushToast(error instanceof Error ? error.message : '保存 Fast mode 失败');
    });
  }, [effectiveModelKey, pushToast, setModelPreference]);

  const handlePriorityModeToggle = useCallback((modelKey = effectiveModelKey, model = activeModel) => {
    if (!modelKey || !modelSupportsPriorityServiceTier(model)) {
      return;
    }
    setAgentPickerOpen(false);
    setSubagentPickerOpen(false);
    const preference = resolveChatModelPreference(useModelsStore.getState().config, model);
    persistPriorityMode(preference.serviceTier !== 'priority', modelKey);
  }, [activeModel, effectiveModelKey, persistPriorityMode]);

  const insertPreparedChatImages = useCallback(async (images: ChatInputImage[]) => {
    if (!canAttachImages) {
      if (inputMode !== 'chat') {
        pushToast('Command mode does not support images yet.');
      } else if (!effectiveAgentCwd) {
        pushToast('Select an agent before adding images.');
      }
      return;
    }
    if (images.length === 0) {
      return;
    }
    try {
      const persisted = await persistChatImageAssets(images, effectiveAgentCwd, writeBase64File);
      const markdown = persisted.map((asset) => asset.markdown).join('\n\n');
      const view = composerRef.current?.getView();
      if (view) {
        insertBlockMarkdown(view, markdown, { leaveCursorAfterBlock: true });
      } else {
        setDraftForSelectedTarget(draft ? `${draft}\n\n${markdown}` : markdown);
      }
      focusComposer();
    } catch (error) {
      const message = error instanceof Error ? error.message : '读取图片失败';
      pushToast(message);
    }
  }, [
    canAttachImages,
    draft,
    effectiveAgentCwd,
    focusComposer,
    inputMode,
    pushToast,
    setDraftForSelectedTarget,
    writeBase64File,
  ]);

  const insertChatImages = useCallback(async (files: Iterable<File> | ArrayLike<File>) => {
    try {
      const images = await readChatImages(files);
      await insertPreparedChatImages(images);
    } catch (error) {
      const message = error instanceof Error ? error.message : '读取图片失败';
      pushToast(message);
    }
  }, [insertPreparedChatImages, pushToast]);

  const handleImagePickerClick = useCallback(() => {
    if (!canAttachImages) {
      return;
    }
    imageInputRef.current?.click();
  }, [canAttachImages]);

  const handleImageInputChange = useCallback((event: React.ChangeEvent<HTMLInputElement>) => {
    const files = event.target.files;
    if (!files || files.length === 0) {
      return;
    }
    void insertChatImages(files);
    event.target.value = '';
  }, [insertChatImages]);

  const handleDragOver = useCallback((event: React.DragEvent) => {
    const hasText = event.dataTransfer.types.includes('text/plain');
    const hasFiles = Array.from(event.dataTransfer.items || []).some((item) => item.kind === 'file');
    if (!hasText && !hasFiles) return;
    event.preventDefault();
    event.stopPropagation();
    event.dataTransfer.dropEffect = 'copy';
    setIsDragOver(true);
  }, []);

  const handleDragLeave = useCallback((event: React.DragEvent) => {
    event.stopPropagation();
    if (event.currentTarget === event.target) {
      setIsDragOver(false);
    }
  }, []);

  const handleDrop = useCallback((event: React.DragEvent) => {
    event.preventDefault();
    event.stopPropagation();
    setIsDragOver(false);

    const droppedImages = extractDroppedImageFiles(event.dataTransfer);
    if (droppedImages.length > 0) {
      void insertChatImages(droppedImages);
      return;
    }

    const data = event.dataTransfer.getData('text/plain');
    if (!data.startsWith('openbrain-dir:') && !data.startsWith('openbrain-file:')) return;

    const isDir = data.startsWith('openbrain-dir:');
    const path = isDir
      ? data.slice('openbrain-dir:'.length)
      : data.slice('openbrain-file:'.length);
    const markdown = buildFileReferenceLink(path, isDir);
    if (!insertComposerBlockMarkdown(markdown)) {
      requestComposerBlockInsert(markdown);
    }
    focusComposer();
  }, [focusComposer, insertChatImages, insertComposerBlockMarkdown, requestComposerBlockInsert]);

  const isPendingSelected = selectedConversationTarget?.kind === 'pending';
  const selectedPendingId = selectedConversationTarget?.kind === 'pending'
    ? selectedConversationTarget.id
    : null;

  useEffect(() => {
    if (!selectedPendingId) {
      return;
    }
    focusComposer();
  }, [focusComposer, selectedPendingId]);

  const moveComposerCursorOutsideImageSource = useCallback((view: EditorView, anchorPos: number) => {
    requestAnimationFrame(() => {
      const clamped = Math.min(anchorPos, Math.max(0, view.state.doc.length));
      const line = view.state.doc.lineAt(clamped);
      const nextAnchor = line.number < view.state.doc.lines ? view.state.doc.line(line.number + 1).from : line.to;
      view.dispatch({
        selection: { anchor: nextAnchor },
        scrollIntoView: true,
      });
    });
  }, []);

  const {
    imageMenu,
    openImageMenu,
    closeImageContextMenu,
    handleImageWidthSelect,
    handleImageEditSource,
    handleImageDelete,
  } = useMarkdownImageMenu({
    getView: () => composerRef.current?.getView() || null,
    afterWidthChange: moveComposerCursorOutsideImageSource,
    afterDelete: moveComposerCursorOutsideImageSource,
    afterDeleteImage: ({ currentText, nextContent }) => {
      const parsed = parseMarkdownImage(currentText);
      const resolvedPath = composerDocumentPath && parsed
        ? resolveMarkdownPath(composerDocumentPath, parsed.url, false)
        : null;
      const assetsDir = composerDocumentPath ? `${dirnamePosix(composerDocumentPath)}/assets/` : '';
      if (!resolvedPath || !composerDocumentPath || !assetsDir || !resolvedPath.startsWith(`${assetsDir}/`)) {
        return;
      }
      if (contentReferencesImagePath(nextContent, composerDocumentPath, resolvedPath)) {
        return;
      }
      const persistedChatContent = tabs.find((tab) => tab.filePath === composerDocumentPath)?.content || '';
      if (contentReferencesImagePath(persistedChatContent, composerDocumentPath, resolvedPath)) {
        return;
      }
      void deleteEntry(resolvedPath, false).then((result) => {
        if (!result.success && result.error) {
          pushToast(result.error);
        }
      });
    },
  });

  const resolveCurrentImageTarget = useCallback(() => {
    if (!imageMenu) {
      return null;
    }
    const view = composerRef.current?.getView();
    if (!view) {
      return null;
    }
    return resolveImageMenuTarget(view, imageMenu, composerDocumentPath);
  }, [composerDocumentPath, imageMenu]);

  const handleImageCopyPath = useCallback(async () => {
    const target = resolveCurrentImageTarget();
    if (!target) {
      pushToast('Failed to copy path');
      return;
    }
    try {
      await writeClipboardText(target.path);
      pushToast('Path copied');
    } catch {
      pushToast('Failed to copy path');
    }
  }, [pushToast, resolveCurrentImageTarget]);

  const handleImageCopy = useCallback(async () => {
    const imageElement = imageMenu?.imageElement ?? null;
    if (!imageElement) {
      pushToast('Image is not loaded');
      return;
    }
    try {
      await writeClipboardImageFromElement(imageElement);
      pushToast('Image copied');
    } catch (error) {
      pushToast(error instanceof Error ? error.message : 'Failed to copy image');
    }
  }, [imageMenu, pushToast]);

  const handleComposerFocus = useCallback(() => {
    // Keep focus/typing pure; chat files are created only when a turn is sent.
  }, []);

  const handleComposerKeyDownCapture = useCallback((event: React.KeyboardEvent<HTMLDivElement>) => {
    const isImeComposing = isImeComposingEvent(event.nativeEvent);
    if (slashMenuVisible && !isImeComposing) {
      if (slashMenuHasResults && event.key === 'ArrowDown') {
        event.preventDefault();
        setSlashHighlightedIndex((current) => (
          filteredSlashItems.length === 0 ? 0 : (current + 1) % filteredSlashItems.length
        ));
        return;
      }
      if (slashMenuHasResults && event.key === 'ArrowUp') {
        event.preventDefault();
        setSlashHighlightedIndex((current) => (
          filteredSlashItems.length === 0
            ? 0
            : (current - 1 + filteredSlashItems.length) % filteredSlashItems.length
        ));
        return;
      }
      if (event.key === 'Escape') {
        event.preventDefault();
        setDismissedSlashToken(slashState?.token || null);
        return;
      }
      if (slashMenuHasResults && event.key === 'Enter' && !event.shiftKey && !event.altKey) {
        const nextItem = filteredSlashItems[slashHighlightedIndex] || filteredSlashItems[0];
        if (nextItem) {
          event.preventDefault();
          handleSlashItemSelect(nextItem);
          return;
        }
      }
    }
    if (event.key === 'Escape' && selectedTargetInProgress) {
      event.preventDefault();
      handleStop();
      return;
    }
    if (event.key === 'Enter') {
      if (isImeComposing) {
        return;
      }
      if (event.shiftKey) {
        return;
      }
      event.preventDefault();
      if (event.altKey && inputMode === 'chat' && selectedTargetInProgress) {
        handleFollowUp();
        return;
      }
      handleSend();
    }
  }, [
    filteredSlashItems,
    handleFollowUp,
    handleSend,
    handleSlashItemSelect,
    handleStop,
    inputMode,
    selectedTargetInProgress,
    slashHighlightedIndex,
    slashMenuHasResults,
    slashMenuVisible,
    slashState?.token,
  ]);

  const handleComposerPasteCapture = useCallback((event: React.ClipboardEvent<HTMLDivElement>) => {
    if (!hasClipboardImage(event.clipboardData)) {
      return;
    }
    event.preventDefault();
    if (!canAttachImages) {
      if (inputMode !== 'chat') {
        pushToast('Command mode does not support images yet.');
      } else {
        pushToast('Select an agent before adding images.');
      }
      return;
    }
    const clipboardData = event.clipboardData;
    void (async () => {
      try {
        const images = await readClipboardImages(clipboardData);
        await insertPreparedChatImages(images);
      } catch (error) {
        pushToast(error instanceof Error ? error.message : '读取剪贴板图片失败');
      }
    })();
  }, [
    canAttachImages,
    inputMode,
    insertPreparedChatImages,
    pushToast,
  ]);

  const agentLabel = useMemo(() => {
    const indexedName = (resolvedAgent?.name || '').trim();
    if (indexedName) {
      return indexedName;
    }
    const storedName = (effectiveAgentName || '').trim();
    if (storedName && storedName !== effectiveAgentID) {
      return storedName;
    }
    return (effectiveAgentID || '').trim();
  }, [effectiveAgentID, effectiveAgentName, resolvedAgent?.name]);
  const agentDisplayLabel = useMemo(
    () => formatAgentTargetDisplayLabel(agentSwitchTargetDir, agentLabel),
    [agentLabel, agentSwitchTargetDir]
  );

  const agentDisplayTitle = useMemo(
    () => formatAgentTargetDisplayTitle(agentSwitchTargetDir, agentLabel),
    [agentLabel, agentSwitchTargetDir]
  );
  const gbrainScopeLabel = useMemo(() => gbrainQueryScopeLabel(gbrainQueryScope), [gbrainQueryScope]);
  const gbrainScopePrompt = useMemo(() => buildGBrainQueryScopePrompt(gbrainQueryScope), [gbrainQueryScope]);
  useEffect(() => {
    if (!gbrainScopeLabel) {
      setGBrainScopePromptVisible(false);
    }
  }, [gbrainScopeLabel]);
  const toggleGBrainScopePrompt = useCallback(() => {
    if (!gbrainScopePrompt) {
      return;
    }
    setGBrainScopePromptVisible((visible) => !visible);
  }, [gbrainScopePrompt]);
  const handleGBrainScopeKeyDown = useCallback((event: React.KeyboardEvent<HTMLSpanElement>) => {
    if (event.target !== event.currentTarget) {
      return;
    }
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      toggleGBrainScopePrompt();
      return;
    }
    if (event.key === 'Escape') {
      event.preventDefault();
      setGBrainScopePromptVisible(false);
    }
  }, [toggleGBrainScopePrompt]);
  const handleClearGBrainQueryScope = useCallback((event: React.MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation();
    setGBrainScopePromptVisible(false);
    clearGBrainQueryScope();
  }, [clearGBrainQueryScope]);
  const slashMenuMessage = useMemo(() => {
    if (slashMenuState.status === 'loading') {
      return 'Loading slash commands...';
    }
    if (slashMenuState.status === 'no-commands') {
      return 'No slash commands available';
    }
    if (slashMenuState.status === 'no-match') {
      return 'No matching slash commands';
    }
    return '';
  }, [slashMenuState.status]);
  const slashMenuEstimatedHeight = useMemo(() => {
    if (slashMenuHasResults) {
      return Math.min(SLASH_MENU_MAX_HEIGHT, filteredSlashItems.length * SLASH_MENU_ITEM_HEIGHT + 8);
    }
    return SLASH_MENU_MESSAGE_HEIGHT;
  }, [filteredSlashItems.length, slashMenuHasResults]);

  useLayoutEffect(() => {
    if (!slashMenuVisible) {
      setSlashMenuPosition(null);
      return;
    }

    const updatePosition = () => {
      const shell = composerShellRef.current;
      if (!shell) {
        setSlashMenuPosition(null);
        return;
      }
      const rect = shell.getBoundingClientRect();
      const left = Math.min(
        Math.max(SLASH_MENU_EDGE_GAP, rect.left),
        Math.max(SLASH_MENU_EDGE_GAP, window.innerWidth - SLASH_MENU_WIDTH - SLASH_MENU_EDGE_GAP),
      );
      const spaceBelow = window.innerHeight - rect.bottom - SLASH_MENU_EDGE_GAP;
      const spaceAbove = rect.top - SLASH_MENU_EDGE_GAP;
      const preferBelow = spaceBelow >= slashMenuEstimatedHeight || spaceBelow >= spaceAbove;
      const top = preferBelow
        ? Math.min(
          window.innerHeight - slashMenuEstimatedHeight - SLASH_MENU_EDGE_GAP,
          rect.bottom + SLASH_MENU_VERTICAL_GAP,
        )
        : Math.max(
          SLASH_MENU_EDGE_GAP,
          rect.top - slashMenuEstimatedHeight - SLASH_MENU_VERTICAL_GAP,
        );

      setSlashMenuPosition({ left, top });
    };

    updatePosition();
    window.addEventListener('resize', updatePosition);
    window.addEventListener('scroll', updatePosition, true);
    return () => {
      window.removeEventListener('resize', updatePosition);
      window.removeEventListener('scroll', updatePosition, true);
    };
  }, [slashMenuEstimatedHeight, slashMenuVisible]);

  return (
    <div className={`bg-editor-bg flex h-full min-h-0 flex-col overflow-visible ${showTopBorder ? 'border-t border-border' : ''}`}>
      <div className="flex h-10 items-center px-2 shrink-0 overflow-visible">
        <div className="w-10 flex items-center justify-center shrink-0">
          <ConversationCloseButton onClick={hideComposer} />
        </div>
        <div className="ui-tabbar flex min-w-0 items-center">
          <div className="flex min-w-0 items-center gap-1 overflow-x-auto overflow-y-hidden pr-2">
            {chatTabs.map((tab) => {
              const chatPath = tab.filePath || '';
              const threadID = (tab.threadID || pathToThreadID[chatPath] || '').trim();
              const tabTarget: ConversationTarget = threadID
                ? { kind: 'thread', threadID, chatPath }
                : { kind: 'command', path: chatPath };
              const isSelected = getConversationTargetKey(selectedConversationTarget) === getConversationTargetKey(tabTarget);
              return (
                <ConversationTabItem
                  key={tab.id}
                  title={tab.title}
                  buttonTitle={tab.filePath ?? tab.title}
                  closeLabel={`Close ${tab.title}`}
                  isSelected={isSelected}
                  isOpenInEditor={tab.id === activeEditorTabId}
                  isPinned={tab.id === pinnedTabId}
                  status={getConversationRunStatus(tabTarget)}
                  awaitingUser={getAwaitingUser(chatPath)}
                  onSelect={() => handleChatTabSelect(tab.id, tab.filePath, tab.threadID)}
                  onClose={(event) => {
                    event.stopPropagation();
                    handleChatTabClose(tab.id, tab.filePath);
                  }}
                />
              );
            })}

            {openComposerTargets
              .filter((thread) => {
                const threadID = (thread.threadID || '').trim();
                return threadID && !chatTabs.some((tab) => (tab.threadID || pathToThreadID[tab.filePath || ''] || '').trim() === threadID);
              })
              .map((thread) => {
                const threadID = thread.threadID.trim();
                const tabTarget: ConversationTarget = {
                  kind: 'thread',
                  threadID,
                  ...(thread.chatPath ? { chatPath: thread.chatPath } : {}),
                };
                const title = thread.title || threadID;
                const isSelected = getConversationTargetKey(selectedConversationTarget) === getConversationTargetKey(tabTarget);
                return (
                  <ConversationTabItem
                    key={threadID}
                    title={title}
                    buttonTitle={thread.chatPath || threadID}
                    closeLabel={`Close ${title}`}
                    isSelected={isSelected}
                    status={getConversationRunStatus(tabTarget)}
                    awaitingUser={getAwaitingUser(thread.chatPath || threadID)}
                    onSelect={() => applyConversationTarget(tabTarget)}
                    onClose={(event) => {
                      event.stopPropagation();
                      closeComposerTarget(threadID);
                    }}
                  />
                );
              })}

            {pendingConversations.map((pending) => {
              const isSelected = selectedConversationTarget?.kind === 'pending' && selectedConversationTarget.id === pending.id;
              const pendingTitle = getConversationTitle();
              return (
                <ConversationTabItem
                  key={pending.id}
                  title={pendingTitle}
                  buttonTitle={pendingTitle}
                  closeLabel={`Close ${pendingTitle}`}
                  isSelected={isSelected}
                  status={getConversationRunStatus({ kind: 'pending', id: pending.id })}
                  onSelect={() => handlePendingConversationSelect(pending.id)}
                  onClose={(event) => {
                    event.stopPropagation();
                    handlePendingConversationClose(pending.id);
                  }}
                />
              );
            })}
          </div>
          <div className="ml-1 shrink-0">
            <IconButton
              variant="inline"
              size={20}
              className={`${TAB_ICON_HOVER_LIFT_CLASS} text-secondary-text`}
              onClick={handleCreateConversation}
              title={newChatTitle()}
              aria-label={newChatTitle()}
            >
              <PlusIcon className="w-3.5 h-3.5" />
            </IconButton>
          </div>
        </div>
        <div className="flex-1 min-w-0" />
      </div>

      <div className="flex-1 min-h-0 p-2 flex flex-col">
        <input
          ref={imageInputRef}
          type="file"
          accept="image/png,image/jpeg,image/gif,image/webp"
          multiple
          className="hidden"
          onChange={handleImageInputChange}
        />
        <div className="flex-1 min-h-0 flex flex-row overflow-hidden">
          <div className="w-10 flex shrink-0 flex-col items-center justify-center">
            <IconButton
              size={28}
              onClick={() => setInputMode(inputMode === 'chat' ? 'command' : 'chat')}
              title="Toggle mode (Cmd/Ctrl+Shift+;)"
              aria-label={`Mode: ${inputMode}`}
            >
              {inputMode === 'chat' ? <ChatLineIcon className="w-5 h-5" /> : <TerminalIcon className="w-5 h-5" />}
            </IconButton>
          </div>
          <div className="flex flex-1 min-w-0 min-h-0 flex-col">
            <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden">
              {pendingResources.length > 0 && (
                <div className="mb-2 flex shrink-0 flex-wrap items-start gap-2 px-2">
                  {pendingResources.map((resource) => {
                    return (
                      <span key={resource.key} className="ui-capsule-pill inline-flex max-w-full items-center gap-2">
                        <span className="truncate text-sm">{resource.skill.name}</span>
                        <CloseButton
                          onClick={() => clearSelectedSkill()}
                          disabled={selectedTargetInProgress}
                          aria-label={`Remove skill ${resource.skill.name}`}
                          title="Remove selected skill"
                          variant="inline"
                          className="shrink-0 disabled:opacity-50"
                        />
                      </span>
                    );
                  })}
                </div>
              )}
              {gbrainScopeLabel ? (
                <div className="mb-2 flex shrink-0 flex-wrap items-start gap-2 px-2">
                  <span ref={gbrainScopePopoverRef} className="relative inline-flex max-w-full">
                    <span
                      role="button"
                      tabIndex={0}
                      className="ui-capsule-pill cm-md-inline-pill inline-flex max-w-full items-center gap-2"
                      aria-expanded={gbrainScopePromptVisible}
                      aria-controls={gbrainScopePromptVisible ? 'gbrain-scope-prompt-popover' : undefined}
                      onClick={toggleGBrainScopePrompt}
                      onKeyDown={handleGBrainScopeKeyDown}
                    >
                      <span className="truncate text-sm">GBrain scope: {gbrainScopeLabel}</span>
                      <CloseButton
                        onClick={handleClearGBrainQueryScope}
                        disabled={selectedTargetInProgress}
                        aria-label="Remove GBrain query scope"
                        title="Remove GBrain query scope"
                        variant="inline"
                        className="shrink-0 disabled:opacity-50"
                      />
                    </span>
                    {gbrainScopePromptVisible && gbrainScopePrompt ? (
                      <div
                        id="gbrain-scope-prompt-popover"
                        className="absolute left-0 top-[calc(100%+6px)] z-[90] w-[min(520px,calc(100vw-48px))] rounded-lg border border-border bg-overlay-bg p-3 shadow-xl"
                      >
                        <pre className="max-h-56 overflow-auto whitespace-pre-wrap break-words text-left font-mono text-[11px] leading-4 text-secondary-text">
                          {gbrainScopePrompt}
                        </pre>
                      </div>
                    ) : null}
                  </span>
                </div>
              ) : null}
              <div ref={composerShellRef} className="relative min-w-0 overflow-visible">
                <ChatMarkdownComposer
                  ref={composerRef}
                  value={draft}
                  planBlock={!isCommandMode && displayedSelectedSkill?.slug === 'plan' ? selectedPlanBlock : null}
                  documentPath={composerDocumentPath}
                  dragOver={isDragOver}
                  placeholder={inputMode === 'chat'
                    ? (isPendingSelected
                      ? 'Type first message, then press Enter to create chat…'
                      : 'Send a message…')
                    : 'Run a command…'}
                  readOnly={isComposerReadOnly}
                  onChange={setDraftForSelectedTarget}
                  onSelectionChange={setSlashCursorPos}
                  onFocus={handleComposerFocus}
                  onImageActivate={openImageMenu}
                  onImageDelete={handleImageDelete}
                  onRemovePlanLine={() => removeActivePlanBlock()}
                  onPlanBlockStateChange={handlePlanBlockStateChange}
                  onKeyDownCapture={handleComposerKeyDownCapture}
                  onPasteCapture={handleComposerPasteCapture}
                  onDragOver={handleDragOver}
                  onDragLeave={handleDragLeave}
                  onDrop={handleDrop}
                />
                {slashMenuVisible && slashMenuPosition && (
                  <PopupMenu
                    className="fixed z-[60] p-0 overflow-hidden w-72"
                    style={{ left: slashMenuPosition.left, top: slashMenuPosition.top }}
                  >
                    {slashMenuHasResults ? (
                      <div className="max-h-56 overflow-auto py-1">
                        {filteredSlashItems.map((item, index) => (
                          <PopupMenuItem
                            key={item.kind === 'command' ? item.key : item.id}
                            highlighted={index === slashHighlightedIndex}
                            className="group items-start gap-1 px-3 py-1.5"
                            onMouseEnter={() => setSlashHighlightedIndex(index)}
                            onMouseDown={(event) => event.preventDefault()}
                            onClick={() => handleSlashItemSelect(item)}
                            title={item.description || undefined}
                          >
                            <div className="min-w-0 flex-1">
                              <div className="flex items-center gap-2 min-w-0">
                                <span className="truncate text-sm">/{item.slug}</span>
                                {item.kind === 'command' && (
                                  <span className="shrink-0 text-[10px] uppercase tracking-wide text-tertiary-text">
                                    Command
                                  </span>
                                )}
                              </div>
                              {item.description && (
                                <div className="truncate text-xs text-tertiary-text group-hover:text-secondary-text">
                                  {item.description}
                                </div>
                              )}
                            </div>
                          </PopupMenuItem>
                        ))}
                      </div>
                    ) : (
                      <div
                        className="px-3 py-2 text-sm text-secondary-text"
                        onMouseDown={(event) => event.preventDefault()}
                      >
                        {slashMenuMessage}
                      </div>
                    )}
                  </PopupMenu>
                )}
              </div>
            </div>
          </div>
          {hasQueuedMessages && (
            <aside className="conversation-queued-rail" aria-label="Queued messages">
              <div className="conversation-queued-count">Queued {flattenedQueuedMessages.length}</div>
              <div className="conversation-queued-scroll">
                <div className="conversation-queued-list">
                  {flattenedQueuedMessages.map((message) => {
                    const summary = buildQueuedMessageSummary(message);
                    const stateLabel = getQueuedMessageStateLabel(message);
                    const stateTitle = getQueuedMessageStateTitle(message);
                    const actionsDisabled = message.pending === true;
                    return (
                      <div
                        key={message.id}
                        className="conversation-queued-item"
                        title={`${stateTitle}: ${summary}`}
                      >
                        <div className="conversation-queued-chip">
                          <span className="conversation-queued-chip-kind">{stateLabel}</span>
                          <span className="conversation-queued-chip-text">{summary}</span>
                          <div className="conversation-queued-actions">
                            <IconButton
                              variant="inline"
                              className="conversation-queued-action"
                              title="Edit queued message"
                              aria-label="Edit queued message"
                              disabled={actionsDisabled}
                              onClick={() => handleQueuedEdit(message)}
                            >
                              <EditIcon className="w-3.5 h-3.5" />
                            </IconButton>
                            {message.kind === 'follow_up' && (
                              <IconButton
                                variant="inline"
                                className="conversation-queued-action"
                                title="Promote to steer next"
                                aria-label="Promote follow-up to steer next"
                                disabled={actionsDisabled}
                                onClick={() => handleQueuedPromote(message)}
                              >
                                <ArrowUpTinyIcon className="w-3.5 h-3.5" />
                              </IconButton>
                            )}
                            <IconButton
                              variant="inline"
                              className="conversation-queued-action"
                              title="Delete queued message"
                              aria-label="Delete queued message"
                              disabled={actionsDisabled}
                              onClick={() => handleQueuedDelete(message)}
                            >
                              <TrashIcon className="w-3.5 h-3.5" />
                            </IconButton>
                          </div>
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
            </aside>
          )}
        </div>

        <div className="mt-2 flex items-center shrink-0">
          <div className="w-24 flex items-center justify-start gap-1 pl-1.5 shrink-0">
            <IconButton
              size={28}
              className={primaryButtonMode === 'stop' ? 'is-active-hover' : ''}
              onClick={primaryButtonMode === 'stop' ? handleStop : handleSend}
              disabled={primaryButtonMode !== 'stop' && hasPendingUserRequest}
              title={
                primaryButtonMode === 'stop'
                  ? 'Stop'
                  : primaryButtonMode === 'continue'
                    ? 'Continue thread'
                  : primaryButtonMode === 'queue'
                    ? 'Send steering message'
                    : isCommandMode
                      ? 'Run command'
                      : hasPendingUserRequest
                        ? 'Waiting for required user input'
                        : 'Send'
              }
              aria-label={
                primaryButtonMode === 'stop'
                  ? 'Stop response'
                  : primaryButtonMode === 'continue'
                    ? 'Continue thread'
                  : primaryButtonMode === 'queue'
                    ? 'Send steering message'
                    : isCommandMode
                      ? 'Run command'
                      : hasPendingUserRequest
                        ? 'Waiting for required user input'
                        : 'Send message'
              }
            >
              {primaryButtonMode === 'stop' ? (
                <svg className="w-5 h-5" viewBox="0 0 24 24" fill="currentColor">
                  <rect x="6.5" y="6.5" width="11" height="11" />
                </svg>
              ) : (
                <SendArrowSimpleIcon className="w-5 h-5" />
              )}
            </IconButton>
            {!isCommandMode && (
              <IconButton
                size={28}
                className={!canAttachImages ? 'opacity-50' : ''}
                onClick={handleImagePickerClick}
                title="Add images"
                aria-label="Add images"
                disabled={!canAttachImages}
              >
                <svg className="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <rect x="3" y="5" width="18" height="14" rx="2" />
                  <circle cx="8.5" cy="10" r="1.5" />
                  <path d="M21 15l-4.5-4.5-5 5-2.5-2.5L3 19" />
                </svg>
              </IconButton>
            )}
          </div>
          <div className="flex-1 flex items-center gap-2 text-sm">
            {isCommandMode ? (
              <span className="text-secondary-text">Command mode</span>
            ) : (
              <>
                <div className="relative" ref={agentPickerRef}>
                  <button
                    type="button"
                    className={`inline-flex max-w-[220px] items-center gap-1 text-sm text-secondary-text hover:text-link-text-hover disabled:opacity-50 ${agentPickerOpen ? 'text-prime-text' : ''}`}
                    title={agentDisplayTitle}
                    disabled={!canSwitchAgent}
                    onClick={() => {
                        setSubagentPickerOpen(false);
                        setModelPickerOpen(false);
                        setThinkingPickerOpen(false);
                        setAgentPickerOpen((open) => !open);
                    }}
                  >
                    <span className="truncate">{agentDisplayLabel}</span>
                    <ChevronDownIcon className="w-3 h-3 flex-shrink-0" />
                  </button>
                  {agentPickerOpen && canSwitchAgent && (
                    <PopupMenu
                      className={`${CHAT_BOTTOM_PICKER_MENU_CLASS_NAME} w-[280px]`}
                      onMouseDown={(event) => event.stopPropagation()}
                    >
                      {agentPickerRefreshing && agentPickerOptions.length === 0 ? (
                        <div className="px-3 py-2 text-sm text-secondary-text">Loading...</div>
                      ) : agentPickerOptions.length === 0 ? (
                        <div className="px-3 py-2 text-sm text-secondary-text">No global agents found</div>
                      ) : (
                        <div className="max-h-56 overflow-auto py-1">
                          {agentPickerOptions.map((option) => {
                            const disabled = false;
                            return (
                              <PopupMenuItem
                                key={option.id}
                                active={option.selected}
                                disabled={disabled}
                                className="items-start gap-2 px-3 py-1.5"
                                onClick={() => handleAgentSelect(option.id, option.name)}
                              >
                                <span className="flex h-4 w-4 items-center justify-center flex-shrink-0">
                                  {option.selected ? <CheckTinyIcon className="w-3 h-3" /> : null}
                                </span>
                                <span className="min-w-0 flex-1">
                                  <span className="block truncate text-sm">{option.name}</span>
                                  <span className="block truncate text-xs text-tertiary-text">{option.path || option.id}</span>
                                </span>
                              </PopupMenuItem>
                            );
                          })}
                        </div>
                      )}
                    </PopupMenu>
                  )}
                </div>
                {effectiveAgentID && (
                  <div className="relative" ref={subagentPickerRef}>
                    <button
                      type="button"
                      className={`inline-flex max-w-[220px] items-center gap-1 text-sm text-secondary-text hover:text-link-text-hover ${subagentPickerOpen ? 'text-prime-text' : ''}`}
                      title={subagentDisplayTitle}
                      aria-label={subagentAriaLabel}
                      aria-haspopup="menu"
                      aria-expanded={subagentPickerOpen}
                      onClick={() => {
                          setAgentPickerOpen(false);
                          setModelPickerOpen(false);
                          setThinkingPickerOpen(false);
                          setSubagentPickerOpen((open) => {
                          if (open) {
                            setSubagentAvailableOpen(false);
                          }
                          return !open;
                        });
                      }}
                    >
                      {subagentNames.length === 0 ? (
                        <span className="truncate">Subagents</span>
                      ) : (
                        <span className="inline-flex min-w-0 flex-1 items-center gap-1">
                          <span className="truncate">{subagentPrimaryName}</span>
                          {subagentExtraCount > 0 ? (
                            <span className="shrink-0 text-xs text-tertiary-text">
                              +{subagentExtraCount}
                            </span>
                          ) : null}
                        </span>
                      )}
                      <ChevronDownIcon className="w-3 h-3 flex-shrink-0" />
                    </button>
                    {subagentPickerOpen && (
                      <PopupMenu
                        className={`${CHAT_BOTTOM_PICKER_MENU_CLASS_NAME} w-[320px]`}
                        onMouseDown={(event) => event.stopPropagation()}
                      >
                        <div className="max-h-72 overflow-auto py-1">
                          <div className="flex items-center gap-2 px-3 pb-1 pt-1">
                            <div className="min-w-0 flex-1 text-[11px] font-medium uppercase tracking-wide text-tertiary-text">
                              SubAgent
                            </div>
                            <IconButton
                              variant="inline"
                              size={26}
                              aria-label="Add subagent"
                              title="Add subagent"
                              onClick={(event) => {
                                event.stopPropagation();
                                setSubagentAvailableOpen((open) => !open);
                              }}
                            >
                              <PlusIcon className="h-3.5 w-3.5" />
                            </IconButton>
                          </div>
                          {mountedSubagentCount === 0 ? (
                            <div className="px-3 py-2 text-sm text-secondary-text">
                              {subagentPickerRefreshing ? 'Refreshing subagents...' : 'No subagents attached'}
                            </div>
                          ) : (
                            mountedSubagents.map((subagent) => {
                              const label = (subagent.name || subagent.id).trim();
                              const disabled = removingSubagentID === subagent.id;
                              return (
                                <PopupMenuItem
                                  key={`mounted:${subagent.id}`}
                                  disabled={disabled}
                                  className="group items-center gap-2 px-3 py-1.5"
                                  aria-label={`${disabled ? 'Removing' : 'Remove'} subagent ${label}`}
                                  onClick={() => void handleSubagentRemove(subagent.id)}
                                >
                                  <span className="min-w-0 flex-1 pr-2">
                                    <span className="block truncate text-sm">{label}</span>
                                    <span className="block truncate text-xs text-tertiary-text">{subagent.path || subagent.id}</span>
                                  </span>
                                  <span
                                    aria-hidden="true"
                                    className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded border border-transparent text-tertiary-text transition-colors group-hover:border-border group-hover:bg-overlay-bg group-hover:text-prime-text"
                                  >
                                    <TrashIcon className="h-3.5 w-3.5" />
                                  </span>
                                </PopupMenuItem>
                              );
                            })
                          )}
                          {subagentAvailableOpen && (
                            <>
                              <div className="my-1 h-px bg-border" />
                              <div className="px-3 pb-1 pt-1 text-[11px] font-medium uppercase tracking-wide text-tertiary-text">
                                Available
                              </div>
                              {mountableSubagentCount === 0 ? (
                                <div className="px-3 py-2 text-sm text-secondary-text">
                                  {subagentPickerRefreshing ? 'Refreshing subagents...' : 'No available subagents'}
                                </div>
                              ) : (
                                mountableSubagents.map((subagent) => {
                                  const label = (subagent.name || subagent.id).trim();
                                  const disabled = mountingSubagentID === subagent.id;
                                  return (
                                    <PopupMenuItem
                                      key={`available:${subagent.id}`}
                                      disabled={disabled}
                                      className="group items-center gap-2 px-3 py-1.5"
                                      aria-label={`${disabled ? 'Attaching' : 'Attach'} subagent ${label}`}
                                      onClick={() => void handleSubagentMount(subagent.id)}
                                    >
                                      <span className="min-w-0 flex-1 pr-2">
                                        <span className="block truncate text-sm">{label}</span>
                                        <span className="block truncate text-xs text-tertiary-text">{subagent.path || subagent.id}</span>
                                      </span>
                                      <span
                                        aria-hidden="true"
                                        className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded border border-transparent text-tertiary-text transition-colors group-hover:border-border group-hover:bg-overlay-bg group-hover:text-prime-text"
                                      >
                                        <PlusIcon className="h-3.5 w-3.5" />
                                      </span>
                                    </PopupMenuItem>
                                  );
                                })
                              )}
                            </>
                          )}
                        </div>
                      </PopupMenu>
                    )}
                  </div>
                )}
                <div className="relative" ref={modelPickerRef}>
                  <button
                    type="button"
                    className={`inline-flex items-center gap-1 text-sm text-secondary-text hover:text-link-text-hover ${modelPickerOpen ? 'text-prime-text' : ''}`}
                    title={effectiveModelKey ? activeModelDisplay.titleText : 'Select a model'}
                    onClick={() => {
                      setAgentPickerOpen(false);
                      setSubagentPickerOpen(false);
                      setThinkingPickerOpen(false);
                        setEditingModelKey(effectiveModelKey || null);
                      setModelPickerOpen((open) => !open);
                    }}
                  >
                    <span>{activeModelTriggerText}</span>
                    <ChevronDownIcon className="w-3 h-3" />
                  </button>
                  {modelPickerOpen && (
                    <PopupMenu
                      className={`${CHAT_BOTTOM_PICKER_MENU_CLASS_NAME} w-[560px]`}
                      onMouseDown={(event) => event.stopPropagation()}
                    >
                      {accountNeedsProviderModel ? (
                        <div className="border-b border-border px-3 py-2 text-xs text-secondary-text">
                          OpenBrain models require credits right now.
                          <button
                            type="button"
                            className="ml-1 text-link-text underline-offset-2 hover:text-link-text-hover hover:underline"
                            onClick={() => {
                              setModelPickerOpen(false);
                              openModelsTab();
                            }}
                          >
                            Open Models
                          </button>
                        </div>
                      ) : null}
                      <div className="grid grid-cols-[minmax(0,1fr)_200px] gap-1">
                        <div className="max-h-72 overflow-auto pr-1">
                          {selectableModels.map((model) => {
                            const display = getModelEntryDisplay(model);
                            const modelPreference = resolveChatModelPreference(modelsConfig, model);
                            const descriptionParts = [
                              display.secondaryText,
                              display.providerText,
                              modelPreference.thinkingLevel !== 'off' ? modelPreference.thinkingLevel : '',
                              formatContextWindowOption(modelPreference.contextWindow),
                              modelPreference.serviceTier === 'priority' ? 'Fast' : '',
                            ].filter(Boolean);
                            const descriptionText = descriptionParts.join(' · ');
                            const isActive = effectiveModelKey === model.key;
                            const isDefaultModel = defaultChatModelKey === model.key;
                            const isEditing = editingModel?.key === model.key && thinkingPickerOpen;
                            return (
                              <div key={model.key} className={`flex items-stretch gap-1 rounded ${isEditing ? 'bg-hover-bg' : ''}`}>
                                <PopupMenuItem
                                  active={isActive}
                                  className="min-w-0 flex-1 items-start gap-2 px-3 py-2"
                                  title={display.titleText}
                                  onClick={() => {
                                    handleModelSelect(model.key);
                                  }}
                                >
                                  <span className="flex h-4 w-4 flex-shrink-0 items-center justify-center">
                                    {isActive ? <CheckTinyIcon className="w-3 h-3" /> : null}
                                  </span>
                                  <span className="min-w-0 flex-1">
                                    <span className="flex min-w-0 items-center gap-2">
                                      <span className={`block min-w-0 truncate text-sm ${isActive ? 'font-semibold' : 'font-semibold text-prime-text'}`}>
                                        {display.primaryText}
                                      </span>
                                    </span>
                                    {descriptionText ? (
                                      <span className="block truncate text-xs text-tertiary-text">{descriptionText}</span>
                                    ) : null}
                                    {accountNeedsProviderModel && modelRequiresBundledTokenValue(model.key, model) ? (
                                      <span className="block truncate text-xs text-tertiary-text">Requires credits</span>
                                    ) : null}
                                  </span>
                                </PopupMenuItem>
                                {isDefaultModel ? (
                                  <span className="mx-1 self-center rounded-full border border-border bg-editor-bg px-2 py-0.5 text-[11px] font-medium leading-none text-tertiary-text shadow-sm">
                                    Default
                                  </span>
                                ) : null}
                                <IconButton
                                  variant="inline"
                                  size={28}
                                  className={`my-1 mr-1 shrink-0 ${isEditing ? 'text-prime-text' : ''}`}
                                  title={`Edit ${display.primaryText} options`}
                                  aria-label={`Edit ${display.primaryText} options`}
                                  onClick={() => {
                                    setEditingModelKey(model.key);
                                    setThinkingPickerOpen(true);
                                  }}
                                >
                                  <EditIcon className="h-3.5 w-3.5" />
                                </IconButton>
                              </div>
                            );
                          })}
                          <div className="border-t border-border mt-1 pt-1">
                            <PopupMenuItem
                              className="px-3 py-2 gap-0"
                              onClick={() => {
                                setModelPickerOpen(false);
                                openModelsTab();
                              }}
                            >
                              <span className="text-sm text-link-text">Add custom model →</span>
                            </PopupMenuItem>
                          </div>
                        </div>
                        <div ref={thinkingPickerRef} className="border-l border-border pl-2">
                          {editingModel ? (
                            <div className="space-y-1">
                              {editingThinkingOptions.length > 1 ? (
                                <div>
                                  <div className="px-2 pb-1 pt-2 text-xs font-medium text-tertiary-text">Thinking</div>
                                  <div className="flex flex-wrap gap-1 px-2">
                                    {editingThinkingOptions.map(({ level }) => {
                                      const isSelected = editingModelPreference.thinkingLevel === level;
                                      return (
                                        <button
                                          key={level}
                                          type="button"
                                          className={`rounded-full border px-2.5 py-1 text-xs font-medium transition-colors ${isSelected ? 'border-highlight bg-highlight/10 text-highlight' : 'border-border bg-editor-bg text-secondary-text hover:border-highlight hover:text-highlight'}`}
                                          onClick={() => handleThinkingLevelSelect(level, editingModel.key, editingModel)}
                                        >
                                          {getThinkingLevelLabel(level)}
                                        </button>
                                      );
                                    })}
                                  </div>
                                </div>
                              ) : null}
                              {editingModelPreference.contextWindowOptions.length > 0 ? (
                                <div>
                                  <div className="px-2 pb-1 pt-2 text-xs font-medium text-tertiary-text">Context</div>
                                  {editingModelPreference.contextWindowOptions.map((contextWindow) => (
                                    <PopupMenuItem
                                      key={contextWindow}
                                      active={editingModelPreference.contextWindow === contextWindow}
                                      className="px-2 py-1.5 gap-2"
                                      onClick={() => handleContextWindowSelect(contextWindow, editingModel.key)}
                                    >
                                      <span className="flex h-4 w-4 items-center justify-center">
                                        {editingModelPreference.contextWindow === contextWindow ? <CheckTinyIcon className="w-3 h-3" /> : null}
                                      </span>
                                      <span>{formatContextWindowOption(contextWindow)}</span>
                                    </PopupMenuItem>
                                  ))}
                                </div>
                              ) : null}
                              {modelSupportsPriorityServiceTier(editingModel) ? (
                                <button
                                  type="button"
                                  className="flex w-full items-center justify-between rounded px-2 py-1.5 text-sm text-prime-text hover:bg-hover-bg"
                                  onClick={() => handlePriorityModeToggle(editingModel.key, editingModel)}
                                >
                                  <span>Fast</span>
                                  <span
                                    className={modelOptionToggleTrackClass(editingModelPreference.serviceTier === 'priority')}
                                    aria-hidden="true"
                                  >
                                    <span className={modelOptionToggleThumbClass(editingModelPreference.serviceTier === 'priority')} />
                                  </span>
                                </button>
                              ) : null}
                            </div>
                          ) : (
                            <div className="px-2 py-2 text-xs text-secondary-text">Select a model to edit.</div>
                          )}
                        </div>
                      </div>
                    </PopupMenu>
                  )}
                </div>
                {accountNeedsProviderModel && activeModelNeedsBundledTokenValue ? (
                  <button
                    type="button"
                    className="truncate text-xs text-secondary-text hover:text-link-text-hover"
                    title="Current model requires credits. Open Models to choose or configure another provider model."
                    onClick={() => {
                      setSubagentPickerOpen(false);
                      setModelPickerOpen(false);
                      openModelsTab();
                    }}
                  >
                    Open Models
                  </button>
                ) : null}
              </>
            )}
          </div>
        </div>
      </div>
      <ImageContextMenu
        open={!!imageMenu}
        x={imageMenu?.x || 0}
        y={imageMenu?.y || 0}
        currentWidthPercent={imageMenu?.widthPercent ?? null}
        onClose={closeImageContextMenu}
        onCopy={handleImageCopy}
        onCopyPath={handleImageCopyPath}
        onEditSource={handleImageEditSource}
        onDelete={handleImageDelete}
        onSelectWidth={handleImageWidthSelect}
      />
    </div>
  );
}
