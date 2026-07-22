import React, { useEffect, useRef, useCallback, useState } from 'react';
import type { Text } from '@codemirror/state';
import type { EditorView } from '@codemirror/view';
import { createMarkdownEditor, MarkdownEditorInstance, type MarkdownEditorViewState } from './codemirror/setup';
import { EditorSearchOverlay, type EditorSearchOverlayHandle } from './search/EditorSearchOverlay';
import { useAppStore } from '../../store/appStore';
import { useChatWorkspaceStore } from '../../store/chatWorkspaceStore';
import { EditorContextMenu } from './EditorContextMenu';
import { DocumentOutline } from './DocumentOutline';
import { ImageContextMenu } from './ImageContextMenu';
import { SelectionAddToChatHint } from './SelectionAddToChatHint';
import { getImageMenuStateFromElement, resolveImageMenuTarget, useMarkdownImageMenu } from './useMarkdownImageMenu';
import { ResizeDivider } from '../ResizeDivider';
import { IconButton } from '../IconButton';
import { ArrowUpTinyIcon } from '../Icons';
import { normalizePosixPath } from '../../utils/markdownMedia';
import { useToastStore } from '../../store/toastStore';
import { writeClipboardImageFromElement, writeClipboardText } from '../../services/clipboardService';
import type { ReviewOverlay, ReviewOverlayDecision } from './codemirror/reviewOverlay';
import { revealTargetToPos } from './revealTarget';
import {
  DEFAULT_MARKDOWN_CONTENT_WIDTH,
  DEFAULT_MARKDOWN_TEXT_OFFSET,
  MARKDOWN_LINE_PADDING_X,
  MARKDOWN_DOCUMENT_COLUMN_OFFSET,
  clampMarkdownContentWidthForEditor,
  clampMarkdownTextOffsetForEditor,
  normalizeMarkdownContentWidth,
  normalizeMarkdownTextOffset,
  setMarkdownContentWidthDragLocked,
  setMarkdownTextOffsetDragLocked,
} from '../../utils/markdownTextOffset';
import {
  buildSelectionReferenceLink,
} from '../../utils/chatReferenceLinks';
import {
  isChatMarkdownScrollNearBottom,
  shouldFollowChatMarkdownUpdate,
} from '../../utils/chatMarkdownScroll';
import { findPendingReviewOverlayForFile } from '../../utils/reviewOverlay';
import { formatReviewActionError } from '../../utils/reviewMessages';
import { getAddToChatShortcutLabel, resolveSelectionHintPosition } from './selectionHintPosition';
import { cancelInlineCompletion, isInlineCompletionEnabledForPath, requestInlineCompletion } from './resolveInlineCompletion';
import { getMarkdownDocumentTitle } from '../../utils/documentHeader';
import type { DocumentHeaderOptions } from '../../utils/documentHeaderState';

const VIEW_STATE_CACHE_LIMIT = 200;
const TEXT_OFFSET_HOVER_DELAY_MS = 1000;
const SCROLL_JUMP_CONTROL_VISIBLE_MS = 1500;
const SCROLL_JUMP_CONTROL_SCREEN_THRESHOLD = 2;
const MARKDOWN_OUTLINE_MIN_WIDTH = 180;
const MARKDOWN_OUTLINE_MAX_WIDTH = 480;
const DEFAULT_MARKDOWN_OUTLINE_WIDTH = 260;
const MARKDOWN_EDITOR_MIN_WIDTH = 320;

type MarkdownEditorProps = {
  tabId?: string | null;
  autoFocus?: boolean;
  outlinePinEnabled?: boolean;
  outlinePinned?: boolean;
  onOutlinePinToggle?: () => void;
  outlineToggleEnabled?: boolean;
  textOffsetEnabled?: boolean;
  compact?: boolean;
};

type MarkdownUiSettingsSnapshot = {
  ui?: {
    markdownContentWidth?: unknown;
    markdownTextOffset?: unknown;
    markdownOutlineWidth?: unknown;
  };
};

function isDotMdFile(path: string | null | undefined): boolean {
  if (!path) {
    return false;
  }
  const normalized = path.trim().toLowerCase();
  return normalized.endsWith('.md') || normalized.endsWith('.markdown');
}

function isTextEntryFocusedElement(element: Element | null): boolean {
  if (!element) {
    return false;
  }
  const tagName = typeof element.tagName === 'string'
    ? element.tagName.toUpperCase()
    : typeof element.nodeName === 'string'
      ? element.nodeName.toUpperCase()
      : '';
  if (tagName === 'INPUT' || tagName === 'TEXTAREA') {
    return true;
  }
  if ('isContentEditable' in element && (element as HTMLElement).isContentEditable) {
    return true;
  }
  return Boolean(element.closest?.('.cm-content'));
}

function isDocumentTextEntryFocused(): boolean {
  if (typeof document === 'undefined') {
    return false;
  }
  return isTextEntryFocusedElement(document.activeElement);
}

function blurDocumentTextEntryFocus(): void {
  if (typeof document === 'undefined') {
    return;
  }
  const active = document.activeElement;
  if (!isTextEntryFocusedElement(active) || !(active instanceof HTMLElement)) {
    return;
  }
  active.blur();
}

function isMarkdownMarkerLine(line: string): boolean {
  const trimmed = line.trim();
  if (!trimmed) {
    return true;
  }
  if (/^(#{1,6}\s+|>\s?)/.test(trimmed)) {
    return true;
  }
  if (/^([-+*]|\d+\.)\s+/.test(trimmed)) {
    return true;
  }
  if (/^(```+|~~~+)/.test(trimmed)) {
    return true;
  }
  if (/^(\*{3,}|-{3,}|_{3,})$/.test(trimmed.replace(/\s+/g, ''))) {
    return true;
  }
  if (/^\|.*\|$/.test(trimmed)) {
    return true;
  }
  if (/^\[.+\]:\s+\S+/.test(trimmed)) {
    return true;
  }
  return false;
}

function findFirstPlainTextPos(content: string): number | null {
  const normalized = content.replace(/\r\n/g, '\n');
  const lines = normalized.split('\n');
  const hasFrontmatter = lines.length > 1
    && lines[0].trim() === '---'
    && lines.slice(1).some((entry) => {
      const token = entry.trim();
      return token === '---' || token === '...';
    });
  let inFrontmatter = hasFrontmatter;
  let checkedFrontmatter = hasFrontmatter;
  let inFence = false;
  let fenceChar = '';
  let fenceWidth = 0;
  let offset = 0;

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index];
    const trimmed = line.trim();
    if (!checkedFrontmatter) {
      checkedFrontmatter = true;
      if (trimmed === '---') {
        inFrontmatter = true;
        offset += line.length + 1;
        continue;
      }
    }

    if (inFrontmatter) {
      if (offset > 0 && (trimmed === '---' || trimmed === '...')) {
        inFrontmatter = false;
      }
      offset += line.length + 1;
      continue;
    }

    const fenceMatch = line.match(/^\s*(```+|~~~+)/);
    if (fenceMatch) {
      const nextFenceChar = fenceMatch[1][0];
      const nextFenceWidth = fenceMatch[1].length;
      if (!inFence) {
        inFence = true;
        fenceChar = nextFenceChar;
        fenceWidth = nextFenceWidth;
      } else if (fenceChar === nextFenceChar && nextFenceWidth >= fenceWidth) {
        inFence = false;
        fenceChar = '';
        fenceWidth = 0;
      }
      offset += line.length + 1;
      continue;
    }

    if (inFence) {
      offset += line.length + 1;
      continue;
    }

    if (!isMarkdownMarkerLine(line)) {
      const firstTextIndex = line.search(/\S/);
      return offset + (firstTextIndex >= 0 ? firstTextIndex : 0);
    }

    offset += line.length + 1;
  }

  return null;
}

function clampMarkdownOutlineWidth(width: number): number {
  return Math.min(
    MARKDOWN_OUTLINE_MAX_WIDTH,
    Math.max(MARKDOWN_OUTLINE_MIN_WIDTH, width)
  );
}

function resolveMarkdownOutlineWidth(value: unknown): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return DEFAULT_MARKDOWN_OUTLINE_WIDTH;
  }
  return clampMarkdownOutlineWidth(value);
}

function clampMarkdownOutlineWidthForContainer(
  width: number,
  containerWidth: number | null | undefined
): number {
  const clamped = clampMarkdownOutlineWidth(width);
  if (typeof containerWidth !== 'number' || !Number.isFinite(containerWidth) || containerWidth <= 0) {
    return clamped;
  }
  const dynamicMax = Math.max(
    MARKDOWN_OUTLINE_MIN_WIDTH,
    Math.min(MARKDOWN_OUTLINE_MAX_WIDTH, Math.floor(containerWidth - MARKDOWN_EDITOR_MIN_WIDTH))
  );
  return Math.min(clamped, dynamicMax);
}

function getLineAnchor(doc: Text, preferredLineNumber: number): number {
  const clampedPreferred = Math.min(Math.max(preferredLineNumber, 1), doc.lines);
  return doc.line(clampedPreferred).from;
}

function MarkdownPinIcon() {
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 16 16"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
    >
      <path
        d="M5.3 2.8h5.4L9.6 6.4l2.1 2.1v1.4H8.6L8 14H6.9l-.6-4.1H3.2V8.5l2.1-2.1-1-3.6Z"
        stroke="currentColor"
        strokeWidth="1.35"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export const MarkdownEditor: React.FC<MarkdownEditorProps> = ({
  tabId = null,
  autoFocus = false,
  outlinePinEnabled = false,
  outlinePinned = false,
  onOutlinePinToggle,
  outlineToggleEnabled = true,
  textOffsetEnabled = true,
  compact = false,
}) => {
  const layoutRef = useRef<HTMLDivElement>(null);
  const mainAreaRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<MarkdownEditorInstance | null>(null);
  const searchOverlayRef = useRef<EditorSearchOverlayHandle | null>(null);
  const textOffsetValueRef = useRef(DEFAULT_MARKDOWN_TEXT_OFFSET);
  const contentWidthValueRef = useRef(DEFAULT_MARKDOWN_CONTENT_WIDTH);
  const textOffsetPreviewBroadcastRef = useRef<number | null>(null);
  const contentWidthPreviewBroadcastRef = useRef<number | null>(null);
  const textOffsetHoverDeadlineRef = useRef(0);
  const contentWidthHoverDeadlineRef = useRef(0);
  const textOffsetHoveredStateRef = useRef(false);
  const contentWidthHoveredStateRef = useRef(false);
  const textOffsetDraggingStateRef = useRef(false);
  const contentWidthDraggingStateRef = useRef(false);
  const textOffsetHoverTimerRef = useRef<number | null>(null);
  const contentWidthHoverTimerRef = useRef<number | null>(null);
  const textOffsetResizeRef = useRef<{
    startX: number;
    startOffset: number;
    currentOffset: number;
  } | null>(null);
  const contentWidthResizeRef = useRef<{
    startX: number;
    startWidth: number;
    currentWidth: number;
  } | null>(null);
  const outlineResizeRef = useRef<{ startX: number; startWidth: number; currentWidth: number } | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const [menuX, setMenuX] = useState(0);
  const [menuY, setMenuY] = useState(0);
  const [textOffset, setTextOffset] = useState(DEFAULT_MARKDOWN_TEXT_OFFSET);
  const [contentWidth, setContentWidth] = useState(DEFAULT_MARKDOWN_CONTENT_WIDTH);
  const [textOffsetHovered, setTextOffsetHovered] = useState(false);
  const [contentWidthHovered, setContentWidthHovered] = useState(false);
  const [textOffsetDragging, setTextOffsetDragging] = useState(false);
  const [contentWidthDragging, setContentWidthDragging] = useState(false);
  const [outlineExpanded, setOutlineExpanded] = useState(false);
  const [outlineWidth, setOutlineWidth] = useState(DEFAULT_MARKDOWN_OUTLINE_WIDTH);
  const [outlineHovered, setOutlineHovered] = useState(false);
  const [outlineDragging, setOutlineDragging] = useState(false);
  const [hasOutlineEntries, setHasOutlineEntries] = useState(false);
  const [selectionHintPosition, setSelectionHintPosition] = useState<{ left: number; top: number } | null>(null);
  const [scrollJumpControlsVisible, setScrollJumpControlsVisible] = useState(false);
  const scrollJumpControlsTimerRef = useRef<number | null>(null);
  const scrollJumpSessionRef = useRef<{ baselineTop: number; lastScrollAt: number } | null>(null);
  const viewStateCacheRef = useRef<Map<string, MarkdownEditorViewState>>(new Map());
  const chatScrollDetachedRef = useRef(false);
  const suppressedSelectionKeyRef = useRef<string | null>(null);
  const editorHasFocusRef = useRef(false);
  const appState = useAppStore();
  const {
    connectionState,
    currentDir,
    remoteSession,
    baseDir,
    workspaceRootDir,
    agentsRootDir,
    instanceID,
    documents,
    activeTabId,
    editorBlurRequestSeq,
    editorFocusRequest,
    setFileContent,
    setTabContent,
    setEditorFocused,
    saveFile,
    saveTabByPath,
    setPendingScrollHeading,
    setPendingRevealTarget,
    listThreadReviews,
    resolveThreadReview,
    reloadOpenTabsByPaths,
    setCurrentReviewOverlay,
    requestEditorRandomID,
    statPath,
  } = appState;
  const openDocuments = documents;
  const boundTab = tabId ? openDocuments.find((tab) => tab.id === tabId) || null : null;
  const isBoundToTab = Boolean(tabId);
  const currentFileURI = isBoundToTab ? (boundTab?.uri || null) : appState.currentFileURI;
  const currentFilePath = isBoundToTab ? (boundTab?.filePath || null) : appState.currentFilePath;
  const fileContent = isBoundToTab ? (boundTab?.content || '') : appState.fileContent;
  const isDirty = isBoundToTab ? Boolean(boundTab?.isDirty) : appState.isDirty;
  const pendingScrollHeading = isBoundToTab ? (boundTab?.pendingScrollHeading || null) : appState.pendingScrollHeading;
  const pendingRevealTarget = isBoundToTab ? (boundTab?.pendingRevealTarget || null) : appState.pendingRevealTarget;
  const currentReviewOverlay = appState.currentReviewOverlay;
  const pushToast = useToastStore((state) => state.pushToast);
  const setReviews = useChatWorkspaceStore((state) => state.setReviews);
  const selectedThreadReviews = useChatWorkspaceStore((state) => state.getReviews(
    state.getTargetChatPath(state.selectedConversationTarget)
  ));
  const reviewActionBusyRef = useRef(false);
  const reviewDecisionHandlerRef = useRef<((decision: ReviewOverlayDecision, overlay: ReviewOverlay) => void) | null>(null);
  const lastBlurSeqRef = useRef(editorBlurRequestSeq);
  const isDotMd = isDotMdFile(currentFilePath);
  const activeTab = isBoundToTab ? boundTab : openDocuments.find((tab) => tab.id === activeTabId);
  const activeTabTitle = activeTab?.title || '';
  const isConversationMarkdown = activeTab?.documentRole === 'conversation';
  const showDocumentHeader = isDotMd && !compact && !isConversationMarkdown;
  const [fileModTime, setFileModTime] = useState<number | null>(null);
  const documentHeaderOptions: DocumentHeaderOptions = {
    enabled: showDocumentHeader,
    title: showDocumentHeader && currentFilePath ? getMarkdownDocumentTitle(currentFilePath) : '',
    modTime: fileModTime,
  };
  const autoReviewOverlay = findPendingReviewOverlayForFile(selectedThreadReviews, currentFilePath);
  const reviewOverlay: ReviewOverlay | null = currentReviewOverlay?.filePath === currentFilePath
    ? currentReviewOverlay
    : autoReviewOverlay;
  const activePath = normalizePosixPath((currentFilePath || '').trim());
  const activeChatPath = isConversationMarkdown ? activePath : '';
  const shouldFollowStreaming = useChatWorkspaceStore((state) => (
    activeChatPath && state.getTargetChatPath(state.selectedConversationTarget) === activeChatPath
      ? state.isTargetInProgress(state.selectedConversationTarget)
      : false
  ));
  const shouldScrollChatToBottom = useChatWorkspaceStore((state) => (
    activeChatPath ? state.shouldScrollChatToBottom(activeChatPath) : false
  ));
  const consumeChatScrollToBottom = useChatWorkspaceStore((state) => state.consumeChatScrollToBottom);
  const addToChatShortcutLabel = getAddToChatShortcutLabel();

  const refreshFileModTime = useCallback(async (path: string | null | undefined) => {
    const normalizedPath = (path || '').trim();
    if (!normalizedPath) {
      setFileModTime(null);
      return;
    }
    try {
      const stat = await statPath(normalizedPath);
      if (stat.error || typeof stat.modTime !== 'number' || !Number.isFinite(stat.modTime)) {
        setFileModTime(null);
        return;
      }
      setFileModTime(stat.modTime);
    } catch {
      setFileModTime(null);
    }
  }, [statPath]);

  useEffect(() => {
    if (!showDocumentHeader || !currentFilePath) {
      setFileModTime(null);
      return;
    }
    void refreshFileModTime(currentFilePath);
  }, [currentFilePath, refreshFileModTime, showDocumentHeader]);

  useEffect(() => {
    if (!showDocumentHeader || !currentFilePath || isDirty) {
      return;
    }
    void refreshFileModTime(currentFilePath);
  }, [currentFilePath, isDirty, refreshFileModTime, showDocumentHeader]);

  const handleReviewOverlayDecision = useCallback(async (decision: ReviewOverlayDecision, overlay: ReviewOverlay) => {
    if (reviewActionBusyRef.current) {
      return;
    }
    if (isDirty) {
      pushToast('Save or discard local edits before reviewing this file.');
      return;
    }
    const threadID = (overlay.threadID || '').trim();
    const turnID = (overlay.turnID || '').trim();
    const chatPath = (overlay.chatPath || '').trim();
    const filePath = (overlay.filePath || '').trim();
    if (!threadID || !turnID || !chatPath || !filePath) {
      pushToast('Review metadata is incomplete.');
      return;
    }
    reviewActionBusyRef.current = true;
    try {
      await resolveThreadReview({
        threadID,
        turnID,
        decision: decision === 'keepFile' ? 'approve' : 'reject',
        path: filePath,
      });
      const nextReviews = await listThreadReviews(threadID);
      setReviews(chatPath, nextReviews);
      setCurrentReviewOverlay(null);
      await reloadOpenTabsByPaths([filePath], { skipDirty: true });
    } catch (error) {
      pushToast(formatReviewActionError(error));
    } finally {
      reviewActionBusyRef.current = false;
    }
  }, [isDirty, listThreadReviews, pushToast, reloadOpenTabsByPaths, resolveThreadReview, setCurrentReviewOverlay, setReviews]);

  reviewDecisionHandlerRef.current = handleReviewOverlayDecision;

  const getLayoutWidth = useCallback(() => layoutRef.current?.clientWidth ?? null, []);
  const getEditorMainWidth = useCallback(() => mainAreaRef.current?.clientWidth ?? null, []);
  const getTextColumnStart = useCallback(() => {
    return textOffset
      + (isDotMd ? MARKDOWN_DOCUMENT_COLUMN_OFFSET : 0)
      + MARKDOWN_LINE_PADDING_X;
  }, [isDotMd, textOffset]);

  const clampOutlineWidthForLayout = useCallback((width: number) => {
    return clampMarkdownOutlineWidthForContainer(width, getLayoutWidth());
  }, [getLayoutWidth]);

  const applyTextOffsetForLayout = useCallback((nextValue: number) => {
    const rendered = clampMarkdownTextOffsetForEditor(nextValue, getEditorMainWidth());
    setTextOffset((prev) => (prev === rendered ? prev : rendered));
    return rendered;
  }, [getEditorMainWidth]);

  const applyContentWidthForLayout = useCallback((nextValue: number) => {
    const rendered = clampMarkdownContentWidthForEditor(
      nextValue,
      getEditorMainWidth(),
      getTextColumnStart()
    );
    setContentWidth((prev) => (prev === rendered ? prev : rendered));
    return rendered;
  }, [getEditorMainWidth, getTextColumnStart]);

  const updateSelectionHint = useCallback(() => {
    const editor = editorRef.current;
    if (!editor || !editorHasFocusRef.current) {
      setSelectionHintPosition(null);
      return;
    }
    const snapshot = editor.getSelectionSnapshot();
    if (!snapshot) {
      suppressedSelectionKeyRef.current = null;
      setSelectionHintPosition(null);
      return;
    }
    const selectionKey = `${(currentFilePath || '').trim()}:${snapshot.from}:${snapshot.to}`;
    if (suppressedSelectionKeyRef.current === selectionKey) {
      setSelectionHintPosition(null);
      return;
    }
    if (suppressedSelectionKeyRef.current && suppressedSelectionKeyRef.current !== selectionKey) {
      suppressedSelectionKeyRef.current = null;
    }
    setSelectionHintPosition(resolveSelectionHintPosition(editor.getView()));
  }, [currentFilePath]);

  const focusEditorIfRequested = useCallback((editor: MarkdownEditorInstance | null): boolean => {
    if (isBoundToTab || !editor) {
      return false;
    }
    const state = useAppStore.getState();
    const tabId = (state.activeTabId || '').trim();
    if (!tabId || state.editorFocusRequest?.tabId !== tabId) {
      return false;
    }
    editor.focus();
    state.consumeEditorFocusRequest(tabId);
    return true;
  }, [isBoundToTab]);

  const broadcastMarkdownTextOffsetPreview = useCallback((nextValue: number) => {
    if (textOffsetPreviewBroadcastRef.current === nextValue) {
      return;
    }
    textOffsetPreviewBroadcastRef.current = nextValue;
    window.electronAPI?.settings?.previewMarkdownTextOffset?.(nextValue);
  }, []);

  const broadcastMarkdownContentWidthPreview = useCallback((nextValue: number) => {
    if (contentWidthPreviewBroadcastRef.current === nextValue) {
      return;
    }
    contentWidthPreviewBroadcastRef.current = nextValue;
    window.electronAPI?.settings?.previewMarkdownContentWidth?.(nextValue);
  }, []);

  useEffect(() => {
    textOffsetHoveredStateRef.current = textOffsetHovered;
  }, [textOffsetHovered]);

  useEffect(() => {
    contentWidthHoveredStateRef.current = contentWidthHovered;
  }, [contentWidthHovered]);

  useEffect(() => {
    textOffsetDraggingStateRef.current = textOffsetDragging;
  }, [textOffsetDragging]);

  useEffect(() => {
    contentWidthDraggingStateRef.current = contentWidthDragging;
  }, [contentWidthDragging]);

  const clearTextOffsetHoverTimer = useCallback(() => {
    if (textOffsetHoverTimerRef.current === null) {
      return;
    }
    window.clearTimeout(textOffsetHoverTimerRef.current);
    textOffsetHoverTimerRef.current = null;
  }, []);

  const clearContentWidthHoverTimer = useCallback(() => {
    if (contentWidthHoverTimerRef.current === null) {
      return;
    }
    window.clearTimeout(contentWidthHoverTimerRef.current);
    contentWidthHoverTimerRef.current = null;
  }, []);

  const hideTextOffsetHover = useCallback(() => {
    clearTextOffsetHoverTimer();
    textOffsetHoveredStateRef.current = false;
    if (!textOffsetDraggingStateRef.current) {
      setTextOffsetHovered(false);
    }
  }, [clearTextOffsetHoverTimer]);

  const hideContentWidthHover = useCallback(() => {
    clearContentWidthHoverTimer();
    contentWidthHoveredStateRef.current = false;
    if (!contentWidthDraggingStateRef.current) {
      setContentWidthHovered(false);
    }
  }, [clearContentWidthHoverTimer]);

  const scheduleScrollJumpControlsHide = useCallback(() => {
    if (scrollJumpControlsTimerRef.current !== null) {
      window.clearTimeout(scrollJumpControlsTimerRef.current);
    }
    scrollJumpControlsTimerRef.current = window.setTimeout(() => {
      scrollJumpControlsTimerRef.current = null;
      scrollJumpSessionRef.current = null;
      setScrollJumpControlsVisible(false);
    }, SCROLL_JUMP_CONTROL_VISIBLE_MS);
  }, []);

  const handleScrollJumpControls = useCallback((scrollEl: HTMLElement) => {
    const now = performance.now();
    const currentTop = scrollEl.scrollTop;
    const threshold = Math.max(1, scrollEl.clientHeight * SCROLL_JUMP_CONTROL_SCREEN_THRESHOLD);
    const session = scrollJumpSessionRef.current;
    const sessionExpired = !session || now - session.lastScrollAt > SCROLL_JUMP_CONTROL_VISIBLE_MS;

    if (sessionExpired) {
      scrollJumpSessionRef.current = {
        baselineTop: currentTop,
        lastScrollAt: now,
      };
      scheduleScrollJumpControlsHide();
      return;
    }

    session.lastScrollAt = now;
    if (!scrollJumpControlsVisible && Math.abs(currentTop - session.baselineTop) < threshold) {
      scheduleScrollJumpControlsHide();
      return;
    }

    setScrollJumpControlsVisible(true);
    scheduleScrollJumpControlsHide();
  }, [scheduleScrollJumpControlsHide, scrollJumpControlsVisible]);

  const ensureTextOffsetHoverTimer = useCallback(() => {
    if (textOffsetHoverTimerRef.current !== null) {
      return;
    }
    const tick = () => {
      textOffsetHoverTimerRef.current = null;
      if (textOffsetDraggingStateRef.current || textOffsetHoveredStateRef.current) {
        return;
      }
      const remaining = textOffsetHoverDeadlineRef.current - performance.now();
      if (remaining <= 0) {
        if (isDocumentTextEntryFocused()) {
          textOffsetHoveredStateRef.current = false;
          setTextOffsetHovered(false);
          return;
        }
        textOffsetHoveredStateRef.current = true;
        setTextOffsetHovered(true);
        return;
      }
      textOffsetHoverTimerRef.current = window.setTimeout(tick, remaining);
    };
    const remaining = Math.max(0, textOffsetHoverDeadlineRef.current - performance.now());
    textOffsetHoverTimerRef.current = window.setTimeout(tick, remaining);
  }, []);

  const ensureContentWidthHoverTimer = useCallback(() => {
    if (contentWidthHoverTimerRef.current !== null) {
      return;
    }
    const tick = () => {
      contentWidthHoverTimerRef.current = null;
      if (contentWidthDraggingStateRef.current || contentWidthHoveredStateRef.current) {
        return;
      }
      const remaining = contentWidthHoverDeadlineRef.current - performance.now();
      if (remaining <= 0) {
        if (isDocumentTextEntryFocused()) {
          contentWidthHoveredStateRef.current = false;
          setContentWidthHovered(false);
          return;
        }
        contentWidthHoveredStateRef.current = true;
        setContentWidthHovered(true);
        return;
      }
      contentWidthHoverTimerRef.current = window.setTimeout(tick, remaining);
    };
    const remaining = Math.max(0, contentWidthHoverDeadlineRef.current - performance.now());
    contentWidthHoverTimerRef.current = window.setTimeout(tick, remaining);
  }, []);

  const touchTextOffsetHoverReveal = useCallback(() => {
    if (textOffsetDraggingStateRef.current || textOffsetHoveredStateRef.current) {
      return;
    }
    if (isDocumentTextEntryFocused()) {
      hideTextOffsetHover();
      return;
    }
    textOffsetHoverDeadlineRef.current = performance.now() + TEXT_OFFSET_HOVER_DELAY_MS;
    ensureTextOffsetHoverTimer();
  }, [ensureTextOffsetHoverTimer, hideTextOffsetHover]);

  const touchContentWidthHoverReveal = useCallback(() => {
    if (contentWidthDraggingStateRef.current || contentWidthHoveredStateRef.current) {
      return;
    }
    if (isDocumentTextEntryFocused()) {
      hideContentWidthHover();
      return;
    }
    contentWidthHoverDeadlineRef.current = performance.now() + TEXT_OFFSET_HOVER_DELAY_MS;
    ensureContentWidthHoverTimer();
  }, [ensureContentWidthHoverTimer, hideContentWidthHover]);

  const armTextOffsetHoverReveal = useCallback(() => {
    if (textOffsetDraggingStateRef.current || textOffsetHoveredStateRef.current) {
      return;
    }
    blurDocumentTextEntryFocus();
    textOffsetHoverDeadlineRef.current = performance.now() + TEXT_OFFSET_HOVER_DELAY_MS;
    ensureTextOffsetHoverTimer();
  }, [ensureTextOffsetHoverTimer]);

  const armContentWidthHoverReveal = useCallback(() => {
    if (contentWidthDraggingStateRef.current || contentWidthHoveredStateRef.current) {
      return;
    }
    blurDocumentTextEntryFocus();
    contentWidthHoverDeadlineRef.current = performance.now() + TEXT_OFFSET_HOVER_DELAY_MS;
    ensureContentWidthHoverTimer();
  }, [ensureContentWidthHoverTimer]);

  useEffect(() => {
    if (!textOffsetEnabled || typeof document === 'undefined') {
      return;
    }
    const hideIfTextEntryFocused = () => {
      if (isDocumentTextEntryFocused()) {
        hideTextOffsetHover();
        hideContentWidthHover();
      }
    };
    document.addEventListener('focusin', hideIfTextEntryFocused, true);
    document.addEventListener('keydown', hideIfTextEntryFocused, true);
    document.addEventListener('input', hideIfTextEntryFocused, true);
    return () => {
      document.removeEventListener('focusin', hideIfTextEntryFocused, true);
      document.removeEventListener('keydown', hideIfTextEntryFocused, true);
      document.removeEventListener('input', hideIfTextEntryFocused, true);
    };
  }, [hideContentWidthHover, hideTextOffsetHover, textOffsetEnabled]);

  const moveCursorOutsideImageSource = useCallback((view: EditorView, anchorPos: number) => {
    requestAnimationFrame(() => {
      const line = view.state.doc.lineAt(Math.min(anchorPos, Math.max(0, view.state.doc.length)));
      const nextAnchor = getLineAnchor(
        view.state.doc,
        line.number < view.state.doc.lines ? line.number + 1 : line.number
      );

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
    getView: () => editorRef.current?.getView() || null,
    afterWidthChange: moveCursorOutsideImageSource,
    afterDelete: moveCursorOutsideImageSource,
  });

  const resolveCurrentImageTarget = useCallback(() => {
    if (!imageMenu) {
      return null;
    }
    const view = editorRef.current?.getView();
    if (!view) {
      return null;
    }
    return resolveImageMenuTarget(view, imageMenu, currentFilePath || null);
  }, [currentFilePath, imageMenu]);

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

  const saveViewState = useCallback((path: string, state: MarkdownEditorViewState) => {
    const cache = viewStateCacheRef.current;
    if (cache.has(path)) {
      cache.delete(path);
    }
    cache.set(path, state);
    if (cache.size > VIEW_STATE_CACHE_LIMIT) {
      const oldestPath = cache.keys().next().value;
      if (oldestPath) {
        cache.delete(oldestPath);
      }
    }
  }, []);

  const loadViewState = useCallback((path: string): MarkdownEditorViewState | null => {
    const cache = viewStateCacheRef.current;
    const state = cache.get(path);
    if (!state) {
      return null;
    }
    cache.delete(path);
    cache.set(path, state);
    return state;
  }, []);

  const activeTabViewStateKey = activeTab?.id ? `tab:${activeTab.id}` : '';

  const syncChatScrollAttachment = useCallback(() => {
    const path = activeChatPath;
    if (!path) {
      chatScrollDetachedRef.current = false;
      return;
    }
    const scrollEl = editorRef.current?.getView().scrollDOM;
    chatScrollDetachedRef.current = scrollEl
      ? !isChatMarkdownScrollNearBottom(scrollEl)
      : false;
  }, [activeChatPath]);

  const shouldFollowActiveChatBottom = useCallback(() => {
    if (!activeChatPath || !isConversationMarkdown) {
      return false;
    }
    const scrollEl = editorRef.current?.getView().scrollDOM;
    return shouldFollowChatMarkdownUpdate({
      isConversation: true,
      userDetached: chatScrollDetachedRef.current,
      wasNearBottom: scrollEl ? isChatMarkdownScrollNearBottom(scrollEl) : true,
    });
  }, [activeChatPath, isConversationMarkdown]);

  // Create editor instance
  useEffect(() => {
    if (!containerRef.current) return;

    // Clean up previous editor
    if (editorRef.current) {
      editorRef.current.destroy();
    }

    // Create new editor
    const completion = {
      editorKind: 'markdown',
      languageId: 'markdown',
      documentPath: currentFilePath,
      enabled: () => isInlineCompletionEnabledForPath(currentFilePath),
      cancel: cancelInlineCompletion,
      request: (payload: Parameters<typeof requestInlineCompletion>[1]) =>
        requestInlineCompletion(currentFilePath, payload),
    };

    editorRef.current = createMarkdownEditor(containerRef.current, {
      initialContent: fileContent,
      documentPath: currentFilePath || null,
      onContentChange: (content) => {
        if (isBoundToTab && boundTab) {
          setTabContent(boundTab.id, content);
          return;
        }
        setFileContent(content);
      },
      onFocusChange: (focused) => {
        if (!isBoundToTab) {
          setEditorFocused(focused);
        }
        editorHasFocusRef.current = focused;
        if (!focused) {
          setSelectionHintPosition(null);
          return;
        }
        requestAnimationFrame(() => {
          updateSelectionHint();
        });
      },
      onSelectionChange: () => {
        requestAnimationFrame(() => {
          updateSelectionHint();
        });
      },
      livePreview: true,
      readOnly: shouldFollowStreaming,
      onImageActivate: (image) => {
        setMenuOpen(false);
        openImageMenu(image);
      },
      reviewOverlay,
      reviewActions: {
        onDecision: (decision, overlay) => reviewDecisionHandlerRef.current?.(decision, overlay),
      },
      completion,
      onOpenSearchPanel: ({ replace }) => {
        searchOverlayRef.current?.open({ replace });
      },
      documentHeader: documentHeaderOptions,
    });
    const activeIdentity = (currentFileURI || currentFilePath || '').trim();
    if (activeIdentity) {
      const cached = loadViewState(activeIdentity)
        || (activeTabViewStateKey ? loadViewState(activeTabViewStateKey) : null);
      if (cached) {
        editorRef.current.restoreViewState(cached);
      } else if (!pendingScrollHeading) {
        const firstPlainTextPos = findFirstPlainTextPos(fileContent);
        if (firstPlainTextPos !== null) {
          editorRef.current.restoreViewState({
            selectionAnchor: firstPlainTextPos,
            selectionHead: firstPlainTextPos,
            scrollTop: 0,
            scrollLeft: 0,
          });
        }
      }
    }
    requestAnimationFrame(syncChatScrollAttachment);

    const shouldAutoFocus = autoFocus && Boolean(currentFilePath);
    if (focusEditorIfRequested(editorRef.current)) {
      // Focus request consumed.
    } else if (shouldAutoFocus) {
      editorRef.current.focus();
    } else {
      if (!isBoundToTab) {
        setEditorFocused(false);
      }
    }

    return () => {
      const editor = editorRef.current;
      const activePathOnCleanup = (currentFileURI || currentFilePath || '').trim();
      const shouldCacheViewState = editor && (activePathOnCleanup || activeTabViewStateKey);
      if (shouldCacheViewState) {
        const nextViewState = editor.getViewState();
        if (activePathOnCleanup) {
          saveViewState(activePathOnCleanup, nextViewState);
        }
        if (activeTabViewStateKey) {
          saveViewState(activeTabViewStateKey, nextViewState);
        }
      }
      if (editor) {
        editor.destroy();
        editorRef.current = null;
      }
      if (!isBoundToTab) {
        setEditorFocused(false);
      }
      editorHasFocusRef.current = false;
      setSelectionHintPosition(null);
    };
  }, [activeTabViewStateKey, autoFocus, boundTab?.id, currentFileURI, currentFilePath, focusEditorIfRequested, isBoundToTab, loadViewState, openImageMenu, saveViewState, setEditorFocused, setFileContent, setTabContent, syncChatScrollAttachment, updateSelectionHint]); // Recreate editor only when file identity changes; chat classification changes must not reset view state

  useEffect(() => {
    if (!editorFocusRequest) {
      return;
    }
    focusEditorIfRequested(editorRef.current);
  }, [editorFocusRequest, focusEditorIfRequested]);

  useEffect(() => {
    editorRef.current?.setReadOnly(shouldFollowStreaming);
  }, [shouldFollowStreaming]);

  useEffect(() => {
    editorRef.current?.setDocumentHeader(documentHeaderOptions);
  }, [documentHeaderOptions.enabled, documentHeaderOptions.modTime, documentHeaderOptions.title]);

  useEffect(() => {
    if (!activeChatPath || !shouldScrollChatToBottom || !editorRef.current) {
      return;
    }
    if (shouldFollowActiveChatBottom()) {
      chatScrollDetachedRef.current = false;
      editorRef.current.scrollToBottom();
    }
    consumeChatScrollToBottom(activeChatPath);
  }, [activeChatPath, consumeChatScrollToBottom, shouldFollowActiveChatBottom, shouldScrollChatToBottom]);

  useEffect(() => {
    if (editorBlurRequestSeq === lastBlurSeqRef.current) {
      return;
    }
    lastBlurSeqRef.current = editorBlurRequestSeq;
    editorRef.current?.blur();
    setEditorFocused(false);
    editorHasFocusRef.current = false;
    setSelectionHintPosition(null);
  }, [editorBlurRequestSeq, setEditorFocused]);

  useEffect(() => {
    const view = editorRef.current?.getView();
    if (!view) {
      return;
    }
    const hideHint = () => {
      setSelectionHintPosition(null);
    };
    view.scrollDOM.addEventListener('scroll', hideHint, { passive: true });
    const onScrollJumpControls = () => {
      handleScrollJumpControls(view.scrollDOM);
    };
    const onChatScrollAttachment = () => {
      syncChatScrollAttachment();
    };
    view.scrollDOM.addEventListener('scroll', onScrollJumpControls, { passive: true });
    view.scrollDOM.addEventListener('scroll', onChatScrollAttachment, { passive: true });
    requestAnimationFrame(onChatScrollAttachment);
    return () => {
      view.scrollDOM.removeEventListener('scroll', hideHint);
      view.scrollDOM.removeEventListener('scroll', onScrollJumpControls);
      view.scrollDOM.removeEventListener('scroll', onChatScrollAttachment);
    };
  }, [currentFilePath, handleScrollJumpControls, syncChatScrollAttachment]);

  // Sync content when file content changes externally.
  // Skip when editor is focused — the editor's doc is authoritative during editing;
  // external updates during a dirty edit would cause cursor/scroll jumps.
  // If the tab is clean, focused editors should still accept file reloads/stream updates.
  useEffect(() => {
    if (!editorRef.current) return;
    const currentContent = editorRef.current.getContent();
    const cmContent = containerRef.current?.querySelector('.cm-content');
    if (cmContent && cmContent === document.activeElement && isDirty) return;

    if (currentContent !== fileContent) {
      const shouldFollowBottom = shouldFollowActiveChatBottom();
      editorRef.current.setContent(fileContent, shouldFollowBottom
        ? { preserveScroll: false, scrollToBottom: true }
        : { preserveScroll: true });
      if (activeChatPath) {
        if (shouldScrollChatToBottom) {
          consumeChatScrollToBottom(activeChatPath);
        }
        requestAnimationFrame(syncChatScrollAttachment);
      }
    }
  }, [
    activeChatPath,
    consumeChatScrollToBottom,
    currentFilePath,
    fileContent,
    isDirty,
    shouldFollowActiveChatBottom,
    shouldScrollChatToBottom,
    syncChatScrollAttachment,
  ]);

  useEffect(() => {
    if (!hasOutlineEntries && outlineExpanded) {
      setOutlineExpanded(false);
      setOutlineHovered(false);
    }
  }, [hasOutlineEntries, outlineExpanded]);

  useEffect(() => {
    return () => {
      clearTextOffsetHoverTimer();
      clearContentWidthHoverTimer();
      if (scrollJumpControlsTimerRef.current !== null) {
        window.clearTimeout(scrollJumpControlsTimerRef.current);
        scrollJumpControlsTimerRef.current = null;
      }
      scrollJumpSessionRef.current = null;
      setMarkdownTextOffsetDragLocked(false);
      setMarkdownContentWidthDragLocked(false);
    };
  }, [clearContentWidthHoverTimer, clearTextOffsetHoverTimer]);

  useEffect(() => {
    const settingsApi = window.electronAPI?.settings;
    if (!settingsApi?.get) {
      textOffsetValueRef.current = DEFAULT_MARKDOWN_TEXT_OFFSET;
      applyTextOffsetForLayout(DEFAULT_MARKDOWN_TEXT_OFFSET);
      return;
    }

    let disposed = false;
    const applyTextOffsetSetting = (settings?: MarkdownUiSettingsSnapshot) => {
      if (disposed || textOffsetResizeRef.current) {
        return;
      }
      const resolved = normalizeMarkdownTextOffset(settings?.ui?.markdownTextOffset);
      textOffsetValueRef.current = resolved;
      applyTextOffsetForLayout(resolved);
    };

    settingsApi.get()
      .then((settings) => {
        applyTextOffsetSetting(settings as MarkdownUiSettingsSnapshot);
      })
      .catch(() => {
        textOffsetValueRef.current = DEFAULT_MARKDOWN_TEXT_OFFSET;
        applyTextOffsetForLayout(DEFAULT_MARKDOWN_TEXT_OFFSET);
      });

    const disposeSettingsChanged = settingsApi.onChanged?.((settings) => {
      applyTextOffsetSetting(settings as MarkdownUiSettingsSnapshot);
    });

    return () => {
      disposed = true;
      disposeSettingsChanged?.();
    };
  }, [applyTextOffsetForLayout]);

  useEffect(() => {
    const settingsApi = window.electronAPI?.settings;
    if (!settingsApi?.get) {
      contentWidthValueRef.current = DEFAULT_MARKDOWN_CONTENT_WIDTH;
      applyContentWidthForLayout(DEFAULT_MARKDOWN_CONTENT_WIDTH);
      return;
    }

    let disposed = false;
    const applyContentWidthSetting = (settings?: MarkdownUiSettingsSnapshot) => {
      if (disposed || contentWidthResizeRef.current) {
        return;
      }
      const resolved = normalizeMarkdownContentWidth(settings?.ui?.markdownContentWidth);
      contentWidthValueRef.current = resolved;
      applyContentWidthForLayout(resolved);
    };

    settingsApi.get()
      .then((settings) => {
        applyContentWidthSetting(settings as MarkdownUiSettingsSnapshot);
      })
      .catch(() => {
        contentWidthValueRef.current = DEFAULT_MARKDOWN_CONTENT_WIDTH;
        applyContentWidthForLayout(DEFAULT_MARKDOWN_CONTENT_WIDTH);
      });

    const disposeSettingsChanged = settingsApi.onChanged?.((settings) => {
      applyContentWidthSetting(settings as MarkdownUiSettingsSnapshot);
    });

    return () => {
      disposed = true;
      disposeSettingsChanged?.();
    };
  }, [applyContentWidthForLayout]);

  useEffect(() => {
    const settingsApi = window.electronAPI?.settings;
    if (!settingsApi?.get) {
      return;
    }

    let disposed = false;
    const applyOutlineWidthSetting = (settings?: MarkdownUiSettingsSnapshot) => {
      if (disposed || outlineResizeRef.current) {
        return;
      }
      const nextWidth = clampOutlineWidthForLayout(
        resolveMarkdownOutlineWidth(settings?.ui?.markdownOutlineWidth)
      );
      setOutlineWidth((prev) => (prev === nextWidth ? prev : nextWidth));
    };

    settingsApi.get()
      .then((settings) => {
        applyOutlineWidthSetting(settings as MarkdownUiSettingsSnapshot);
      })
      .catch(() => {
        // Keep the in-memory default when settings cannot be loaded.
      });

    const disposeSettingsChanged = settingsApi.onChanged?.((settings) => {
      applyOutlineWidthSetting(settings as MarkdownUiSettingsSnapshot);
    });

    return () => {
      disposed = true;
      disposeSettingsChanged?.();
    };
  }, [clampOutlineWidthForLayout]);

  useEffect(() => {
    const mainAreaEl = mainAreaRef.current;
    if (!mainAreaEl || typeof ResizeObserver === 'undefined') {
      return;
    }

    const observer = new ResizeObserver(() => {
      if (!textOffsetResizeRef.current) {
        applyTextOffsetForLayout(textOffsetValueRef.current);
      }
      if (!contentWidthResizeRef.current) {
        applyContentWidthForLayout(contentWidthValueRef.current);
      }
    });

    observer.observe(mainAreaEl);
    return () => {
      observer.disconnect();
    };
  }, [applyContentWidthForLayout, applyTextOffsetForLayout]);

  useEffect(() => {
    if (contentWidthResizeRef.current) {
      return;
    }
    applyContentWidthForLayout(contentWidthValueRef.current);
  }, [applyContentWidthForLayout]);

  useEffect(() => {
    const layoutEl = layoutRef.current;
    if (!layoutEl || typeof ResizeObserver === 'undefined') {
      return;
    }

    const observer = new ResizeObserver((entries) => {
      if (outlineResizeRef.current) {
        return;
      }
      const nextContainerWidth = entries[0]?.contentRect.width ?? layoutEl.clientWidth;
      setOutlineWidth((prev) => {
        const next = clampMarkdownOutlineWidthForContainer(prev, nextContainerWidth);
        return next === prev ? prev : next;
      });
    });

    observer.observe(layoutEl);
    return () => {
      observer.disconnect();
    };
  }, []);

  // Scroll to heading if requested after file open
  useEffect(() => {
    if (isBoundToTab) {
      return;
    }
    if (editorRef.current && pendingScrollHeading) {
      editorRef.current.scrollToHeading(pendingScrollHeading);
      setPendingScrollHeading(null);
    }
  }, [fileContent, isBoundToTab, pendingScrollHeading, setPendingScrollHeading]);

  useEffect(() => {
    if (isBoundToTab) {
      return;
    }
    if (editorRef.current && pendingRevealTarget) {
      editorRef.current.scrollToPos(revealTargetToPos(fileContent, pendingRevealTarget));
      setPendingRevealTarget(null);
    }
  }, [fileContent, isBoundToTab, pendingRevealTarget, setPendingRevealTarget]);

  useEffect(() => {
    editorRef.current?.setReviewOverlay(reviewOverlay);
  }, [reviewOverlay]);

  const appendSelectionToConversationDraft = useCallback((): boolean => {
    const editor = editorRef.current;
    if (!editor) {
      return false;
    }
    const selection = editor.getSelectionSnapshot();
    if (!selection) {
      return false;
    }
    const chatStore = useChatWorkspaceStore.getState();
    const markdown = buildSelectionReferenceLink(selection, currentFilePath || null);
    suppressedSelectionKeyRef.current = `${(currentFilePath || '').trim()}:${selection.from}:${selection.to}`;
    setSelectionHintPosition(null);
    chatStore.showComposer();
    chatStore.setInputMode('chat');
    chatStore.requestComposerBlockInsert(markdown);
    chatStore.requestComposerFocus();
    return true;
  }, [currentFilePath]);

  // Handle keyboard shortcuts
  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    // Save: Cmd+S / Ctrl+S
    const key = e.key.toLowerCase();
    if ((e.metaKey || e.ctrlKey) && key === 's') {
      e.preventDefault();
      if (isBoundToTab && currentFilePath) {
        void saveTabByPath(currentFilePath);
      } else {
        saveFile();
      }
      return;
    }
    if ((e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey && key === 'l') {
      if (e.nativeEvent.isComposing || e.nativeEvent.keyCode === 229) {
        return;
      }
      if (!appendSelectionToConversationDraft()) {
        return;
      }
      e.preventDefault();
      e.stopPropagation();
    }
  }, [appendSelectionToConversationDraft, currentFilePath, isBoundToTab, saveFile, saveTabByPath]);

  const handleContextMenuCapture = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    e.preventDefault();

    const target = e.target instanceof Element ? e.target : null;
    const imageEl = target?.closest('[data-md-image-source-from][data-md-image-source-to]') as HTMLElement | null;
    if (imageEl) {
      const nextMenu = getImageMenuStateFromElement(imageEl, e.clientX, e.clientY);
      if (nextMenu) {
        openImageMenu(nextMenu);
        return;
      }
    }

    closeImageContextMenu();
    setMenuOpen(true);
    setMenuX(e.clientX);
    setMenuY(e.clientY);
  }, [closeImageContextMenu, getImageMenuStateFromElement, openImageMenu]);

  const closeContextMenu = useCallback(() => {
    setMenuOpen(false);
  }, []);

  const handleInsertRandomID = useCallback(async () => {
    const randomID = await requestEditorRandomID();
    if (!randomID) {
      pushToast('Failed to generate random ID');
      return;
    }
    editorRef.current?.insertAtSelection(randomID);
  }, [pushToast, requestEditorRandomID]);

  const handleScrollToTop = useCallback(() => {
    const view = editorRef.current?.getView();
    if (!view) {
      return;
    }
    view.scrollDOM.scrollTop = 0;
    if (activeChatPath) {
      chatScrollDetachedRef.current = true;
    }
    setScrollJumpControlsVisible(true);
    scheduleScrollJumpControlsHide();
  }, [activeChatPath, scheduleScrollJumpControlsHide]);

  const handleScrollToBottom = useCallback(() => {
    editorRef.current?.scrollToBottom();
    if (activeChatPath) {
      chatScrollDetachedRef.current = false;
      consumeChatScrollToBottom(activeChatPath);
    }
    setScrollJumpControlsVisible(true);
    scheduleScrollJumpControlsHide();
  }, [activeChatPath, consumeChatScrollToBottom, scheduleScrollJumpControlsHide]);

  const handleExportPdf = useCallback(async () => {
    const api = window.electronAPI;
    if (!api?.exportMarkdownPdfToPath || !api?.getPdfExportDefaultPath) {
      pushToast('PDF export is not available');
      return;
    }

    const fallbackTitle = currentFilePath
      ? currentFilePath.split('/').pop() || 'Untitled'
      : 'Untitled';

    // Step 1: get default path info from main process
    const defaults = await api.getPdfExportDefaultPath({
      sourcePath: currentFilePath || undefined,
      currentDir: currentDir || undefined,
      isRemote: !!remoteSession,
    });

    // Step 2: show unified save dialog
    const { requestSaveFileDialog } = await import('../FileDialog/saveFileDialogBridge');
    const chosenPath = await requestSaveFileDialog({
      defaultDir: defaults.defaultDir,
      defaultFileName: defaults.defaultFileName,
      filters: [{ name: 'PDF', extensions: ['pdf'] }],
    });
    if (!chosenPath) return;

    // Step 3: export to the chosen path
    const result = await api.exportMarkdownPdfToPath({
      outputPath: chosenPath,
      title: activeTabTitle || fallbackTitle,
      content: fileContent,
      sourcePath: currentFilePath || undefined,
      currentDir: currentDir || undefined,
      remoteSession,
      baseDir: baseDir || undefined,
      workspaceRootDir: workspaceRootDir || undefined,
      agentsRootDir: agentsRootDir || undefined,
      instanceID: instanceID || undefined,
    });

    if (result.error) {
      pushToast(result.error);
      return;
    }
    pushToast('PDF exported');
  }, [
    activeTabTitle,
    agentsRootDir,
    baseDir,
    currentDir,
    currentFilePath,
    fileContent,
    instanceID,
    pushToast,
    remoteSession,
    workspaceRootDir,
  ]);

  const handleTextOffsetResizeStart = useCallback((event: React.PointerEvent<HTMLDivElement>) => {
    if (!textOffsetHoveredStateRef.current && !textOffsetDraggingStateRef.current) {
      return;
    }
    event.preventDefault();
    if (textOffsetResizeRef.current) {
      return;
    }

    clearTextOffsetHoverTimer();
    setMarkdownTextOffsetDragLocked(true);
    textOffsetDraggingStateRef.current = true;
    textOffsetPreviewBroadcastRef.current = textOffsetValueRef.current;
    textOffsetResizeRef.current = {
      startX: event.clientX,
      startOffset: textOffset,
      currentOffset: textOffset,
    };
    setTextOffsetDragging(true);

    const handlePointerMove = (moveEvent: PointerEvent) => {
      const drag = textOffsetResizeRef.current;
      if (!drag) {
        return;
      }
      const deltaX = moveEvent.clientX - drag.startX;
      const nextOffset = clampMarkdownTextOffsetForEditor(
        drag.startOffset + deltaX,
        getEditorMainWidth()
      );
      drag.currentOffset = nextOffset;
      setTextOffset((prev) => (prev === nextOffset ? prev : nextOffset));
      broadcastMarkdownTextOffsetPreview(nextOffset);
    };

    const cleanup = () => {
      const drag = textOffsetResizeRef.current;
      textOffsetResizeRef.current = null;
      textOffsetHoveredStateRef.current = false;
      textOffsetDraggingStateRef.current = false;
      textOffsetPreviewBroadcastRef.current = null;
      setTextOffsetDragging(false);
      setTextOffsetHovered(false);
      setMarkdownTextOffsetDragLocked(false);
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', cleanup);
      window.removeEventListener('pointercancel', cleanup);

      if (!drag) {
        return;
      }

      if (drag.currentOffset === drag.startOffset) {
        applyTextOffsetForLayout(textOffsetValueRef.current);
        return;
      }

      const finalPersistedOffset = normalizeMarkdownTextOffset(drag.currentOffset);
      textOffsetValueRef.current = finalPersistedOffset;
      applyTextOffsetForLayout(finalPersistedOffset);

      const persistPromise = window.electronAPI?.settings?.set?.({
        ui: {
          markdownTextOffset: finalPersistedOffset,
        },
      });
      if (!persistPromise) {
        return;
      }
      void persistPromise.catch((error) => {
        console.warn('[markdownTextOffset] failed to persist:', error);
      });
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', cleanup);
    window.addEventListener('pointercancel', cleanup);
  }, [applyTextOffsetForLayout, broadcastMarkdownTextOffsetPreview, clearTextOffsetHoverTimer, getEditorMainWidth, textOffset]);

  const handleContentWidthResizeStart = useCallback((event: React.PointerEvent<HTMLDivElement>) => {
    if (!contentWidthHoveredStateRef.current && !contentWidthDraggingStateRef.current) {
      return;
    }
    event.preventDefault();
    if (contentWidthResizeRef.current) {
      return;
    }

    clearContentWidthHoverTimer();
    setMarkdownContentWidthDragLocked(true);
    contentWidthDraggingStateRef.current = true;
    contentWidthPreviewBroadcastRef.current = contentWidthValueRef.current;
    contentWidthResizeRef.current = {
      startX: event.clientX,
      startWidth: contentWidth,
      currentWidth: contentWidth,
    };
    setContentWidthDragging(true);

    const handlePointerMove = (moveEvent: PointerEvent) => {
      const drag = contentWidthResizeRef.current;
      if (!drag) {
        return;
      }
      const deltaX = moveEvent.clientX - drag.startX;
      const nextWidth = clampMarkdownContentWidthForEditor(
        drag.startWidth + deltaX,
        getEditorMainWidth(),
        getTextColumnStart()
      );
      drag.currentWidth = nextWidth;
      setContentWidth((prev) => (prev === nextWidth ? prev : nextWidth));
      broadcastMarkdownContentWidthPreview(nextWidth);
    };

    const cleanup = () => {
      const drag = contentWidthResizeRef.current;
      contentWidthResizeRef.current = null;
      contentWidthHoveredStateRef.current = false;
      contentWidthDraggingStateRef.current = false;
      contentWidthPreviewBroadcastRef.current = null;
      setContentWidthDragging(false);
      setContentWidthHovered(false);
      setMarkdownContentWidthDragLocked(false);
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', cleanup);
      window.removeEventListener('pointercancel', cleanup);

      if (!drag) {
        return;
      }

      if (drag.currentWidth === drag.startWidth) {
        applyContentWidthForLayout(contentWidthValueRef.current);
        return;
      }

      const finalPersistedWidth = normalizeMarkdownContentWidth(drag.currentWidth);
      contentWidthValueRef.current = finalPersistedWidth;
      applyContentWidthForLayout(finalPersistedWidth);

      const persistPromise = window.electronAPI?.settings?.set?.({
        ui: {
          markdownContentWidth: finalPersistedWidth,
        },
      });
      if (!persistPromise) {
        return;
      }
      void persistPromise.catch((error) => {
        console.warn('[markdownContentWidth] failed to persist:', error);
      });
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', cleanup);
    window.addEventListener('pointercancel', cleanup);
  }, [
    applyContentWidthForLayout,
    broadcastMarkdownContentWidthPreview,
    clearContentWidthHoverTimer,
    contentWidth,
    getEditorMainWidth,
    getTextColumnStart,
  ]);

  const handleOutlineExpandedChange = useCallback((expanded: boolean) => {
    if (expanded && !hasOutlineEntries) {
      return;
    }
    setOutlineExpanded(expanded);
    if (!expanded) {
      setOutlineHovered(false);
    }
  }, [hasOutlineEntries]);

  const handleOutlineEntriesChange = useCallback((hasEntries: boolean) => {
    setHasOutlineEntries((prev) => (prev === hasEntries ? prev : hasEntries));
  }, []);

  const handleOutlineResizeStart = useCallback((event: React.PointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    if (!outlineExpanded || outlineResizeRef.current) {
      return;
    }

    outlineResizeRef.current = {
      startX: event.clientX,
      startWidth: outlineWidth,
      currentWidth: outlineWidth,
    };
    setOutlineDragging(true);

    const handlePointerMove = (moveEvent: PointerEvent) => {
      const drag = outlineResizeRef.current;
      if (!drag) {
        return;
      }
      const deltaX = moveEvent.clientX - drag.startX;
      const nextWidth = clampOutlineWidthForLayout(drag.startWidth - deltaX);
      drag.currentWidth = nextWidth;
      setOutlineWidth(nextWidth);
    };

    const cleanup = () => {
      const drag = outlineResizeRef.current;
      outlineResizeRef.current = null;
      setOutlineDragging(false);
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', cleanup);
      window.removeEventListener('pointercancel', cleanup);

      if (!drag) {
        return;
      }

      const finalWidth = clampOutlineWidthForLayout(drag.currentWidth);
      if (finalWidth === drag.startWidth) {
        return;
      }

      const persistPromise = window.electronAPI?.settings?.set?.({
        ui: {
          markdownOutlineWidth: finalWidth,
        },
      });
      if (!persistPromise) {
        return;
      }
      void persistPromise.catch((error) => {
        console.warn('[markdownOutlineWidth] failed to persist:', error);
      });
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', cleanup);
    window.addEventListener('pointercancel', cleanup);
  }, [clampOutlineWidthForLayout, outlineExpanded, outlineWidth]);

  const textOffsetHotzoneBaseWidth = Math.max(
    0,
    textOffset + (isDotMd ? MARKDOWN_DOCUMENT_COLUMN_OFFSET : 0)
  );
  const contentWidthLineX = Math.max(0, getTextColumnStart() + contentWidth);
  const markdownEditorStyle = {
    '--op-md-effective-column-offset': `${isDotMd ? MARKDOWN_DOCUMENT_COLUMN_OFFSET : 0}px`,
    '--op-md-content-padding-left': `${Math.max(0, textOffset)}px`,
    '--op-md-content-max-width': `${Math.max(0, contentWidth)}px`,
  } as React.CSSProperties;
  const outlineElements: React.ReactNode[] = [];
  if (outlineExpanded && hasOutlineEntries) {
    outlineElements.push(
      <div
        key="outline-panel"
        className="absolute top-2 right-4 bottom-2 z-[20] flex"
        onPointerEnter={() => {
          setOutlineHovered(true);
        }}
        onPointerLeave={() => {
          setOutlineHovered(false);
        }}
      >
        <ResizeDivider
          direction="vertical"
          onResizeStart={handleOutlineResizeStart}
          ariaLabel="Resize markdown outline"
          highlighted={outlineDragging}
          visible={outlineHovered || outlineDragging}
          activeColor="var(--color-highlight)"
        />
        <div
          className="op-md-outline-shell is-expanded"
          style={{ width: `${outlineWidth}px` }}
        >
          <DocumentOutline
            content={fileContent}
            editorRef={editorRef}
            expanded={outlineExpanded}
            onExpandedChange={handleOutlineExpandedChange}
            onEntriesChange={handleOutlineEntriesChange}
            pinEnabled={outlinePinEnabled}
            pinned={outlinePinned}
            onPinToggle={onOutlinePinToggle}
            outlineToggleEnabled={outlineToggleEnabled}
          />
        </div>
      </div>
    );
  } else {
    outlineElements.push(
      <div
        key="outline-shell"
        className="op-md-outline-shell is-collapsed"
      >
        <DocumentOutline
          content={fileContent}
          editorRef={editorRef}
          expanded={outlineExpanded}
          onExpandedChange={handleOutlineExpandedChange}
          onEntriesChange={handleOutlineEntriesChange}
          pinEnabled={outlinePinEnabled}
          pinned={outlinePinned}
          onPinToggle={onOutlinePinToggle}
          outlineToggleEnabled={outlineToggleEnabled}
        />
      </div>
    );
  }

  return (
    <>
      <div
        ref={layoutRef}
        className={`op-markdown-editor relative flex flex-1 overflow-hidden ${isDotMd ? 'is-dot-md' : ''} ${compact ? 'is-compact' : ''} ${shouldFollowStreaming ? 'is-stream-follow' : ''} ${outlineExpanded && hasOutlineEntries ? 'is-outline-expanded' : 'is-outline-collapsed'}`}
        style={markdownEditorStyle}
      >
        <div
          ref={mainAreaRef}
          className="op-markdown-editor-main relative flex-1 min-w-0 overflow-hidden"
        >
          {textOffsetEnabled && (
            <div
              className="op-md-text-offset-zone"
              style={{ width: `calc(${textOffsetHotzoneBaseWidth}px + var(--op-md-line-padding-x))` }}
              onPointerEnter={() => {
                touchTextOffsetHoverReveal();
              }}
              onPointerDown={(event) => {
                if (event.button !== 0) {
                  return;
                }
                armTextOffsetHoverReveal();
              }}
              onPointerMove={() => {
                if (!textOffsetHovered && !textOffsetDragging) {
                  touchTextOffsetHoverReveal();
                }
              }}
              onPointerLeave={() => {
                clearTextOffsetHoverTimer();
                textOffsetHoveredStateRef.current = false;
                if (!textOffsetDragging) {
                  setTextOffsetHovered(false);
                }
              }}
            >
              <div className="absolute inset-y-0 right-0 flex">
                <ResizeDivider
                  direction="vertical"
                  onResizeStart={handleTextOffsetResizeStart}
                  ariaLabel="Resize markdown text offset"
                  highlighted={textOffsetDragging}
                  visible={textOffsetHovered || textOffsetDragging}
                  hitTargetEnabled={textOffsetHovered || textOffsetDragging}
                  activeColor="var(--color-highlight)"
                />
              </div>
            </div>
          )}
          {textOffsetEnabled && (
            <div
              className="op-md-content-width-zone"
              style={{ left: `${contentWidthLineX}px` }}
              onPointerEnter={() => {
                touchContentWidthHoverReveal();
              }}
              onPointerDown={(event) => {
                if (event.button !== 0) {
                  return;
                }
                armContentWidthHoverReveal();
              }}
              onPointerMove={() => {
                if (!contentWidthHovered && !contentWidthDragging) {
                  touchContentWidthHoverReveal();
                }
              }}
              onPointerLeave={() => {
                clearContentWidthHoverTimer();
                contentWidthHoveredStateRef.current = false;
                if (!contentWidthDragging) {
                  setContentWidthHovered(false);
                }
              }}
            >
              <div className="absolute inset-y-0 left-0 flex">
                <ResizeDivider
                  direction="vertical"
                  onResizeStart={handleContentWidthResizeStart}
                  ariaLabel="Resize markdown content width"
                  highlighted={contentWidthDragging}
                  visible={contentWidthHovered || contentWidthDragging}
                  hitTargetEnabled={contentWidthHovered || contentWidthDragging}
                  activeColor="var(--color-highlight)"
                />
              </div>
            </div>
          )}
          <div
            ref={containerRef}
            className="relative z-0 h-full"
            onKeyDown={handleKeyDown}
            onContextMenuCapture={handleContextMenuCapture}
          />
          <EditorSearchOverlay
            getView={() => editorRef.current?.getView() || null}
            registerHandle={(handle) => {
              searchOverlayRef.current = handle;
            }}
            enableReplace={!shouldFollowStreaming}
          />
        </div>
        {outlineElements}
        {scrollJumpControlsVisible && (
          <div className="op-md-scroll-jump-controls">
            <IconButton
              variant="inline"
              size={24}
              className="op-md-scroll-jump-btn"
              onClick={handleScrollToTop}
              title="Scroll to top"
              aria-label="Scroll to top"
            >
              <ArrowUpTinyIcon className="w-3.5 h-3.5" />
            </IconButton>
            <IconButton
              variant="inline"
              size={24}
              className="op-md-scroll-jump-btn"
              onClick={handleScrollToBottom}
              title="Scroll to bottom"
              aria-label="Scroll to bottom"
            >
              <ArrowUpTinyIcon className="w-3.5 h-3.5 rotate-180" />
            </IconButton>
          </div>
        )}
      </div>
      <SelectionAddToChatHint
        position={selectionHintPosition}
        shortcutLabel={addToChatShortcutLabel}
        onClick={() => {
          appendSelectionToConversationDraft();
        }}
      />
      <EditorContextMenu
        open={menuOpen}
        x={menuX}
        y={menuY}
        onClose={closeContextMenu}
        editorApi={editorRef.current}
        onExportPdf={handleExportPdf}
        onInsertRandomID={handleInsertRandomID}
        canInsertRandomID={connectionState === 'connected'}
      />
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
    </>
  );
};
