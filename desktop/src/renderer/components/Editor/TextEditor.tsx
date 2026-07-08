import React, { useEffect, useRef, useCallback, useState } from 'react';
import { createTextEditor, TextEditorInstance } from './codemirror/textSetup';
import { EditorSearchOverlay, type EditorSearchOverlayHandle } from './search/EditorSearchOverlay';
import type { ReviewOverlay, ReviewOverlayDecision } from './codemirror/reviewOverlay';
import { revealTargetToPos } from './revealTarget';
import { SelectionAddToChatHint } from './SelectionAddToChatHint';
import { useAppStore } from '../../store/appStore';
import { useChatWorkspaceStore } from '../../store/chatWorkspaceStore';
import { useUiStore } from '../../store/uiStore';
import { useToastStore } from '../../store/toastStore';
import { languageRegistry } from '../../services/languageRegistry';
import {
  buildSelectionReferenceLink,
} from '../../utils/chatReferenceLinks';
import { findPendingReviewOverlayForFile } from '../../utils/reviewOverlay';
import { formatReviewActionError } from '../../utils/reviewMessages';
import { getAddToChatShortcutLabel, resolveSelectionHintPosition } from './selectionHintPosition';
import { cancelInlineCompletion, isInlineCompletionEnabledForPath, requestInlineCompletion } from './resolveInlineCompletion';
import { PinIcon } from '../Icons';

type TextEditorProps = {
  tabId?: string | null;
  autoFocus?: boolean;
  pinEnabled?: boolean;
  pinned?: boolean;
  onPinToggle?: () => void;
};

export const TextEditor: React.FC<TextEditorProps> = ({
  tabId = null,
  autoFocus = false,
  pinEnabled = false,
  pinned = false,
  onPinToggle,
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<TextEditorInstance | null>(null);
  const searchOverlayRef = useRef<EditorSearchOverlayHandle | null>(null);
  const suppressedSelectionKeyRef = useRef<string | null>(null);
  const editorHasFocusRef = useRef(false);
  const [selectionHintPosition, setSelectionHintPosition] = useState<{ left: number; top: number } | null>(null);
  const showLineNumbers = useUiStore((state) => state.showLineNumbers);
  const appState = useAppStore();
  const {
    documents,
    editorFocusRequest,
    setFileContent,
    setTabContent,
    saveFile,
    saveTabByPath,
    setPendingRevealTarget,
    setEditorFocused,
    listThreadReviews,
    resolveThreadReview,
    reloadOpenTabsByPaths,
    setCurrentReviewOverlay,
  } = appState;
  const openDocuments = documents;
  const boundTab = tabId ? openDocuments.find((tab) => tab.id === tabId) || null : null;
  const isBoundToTab = Boolean(tabId);
  const boundTabId = boundTab?.id || null;
  const currentFileURI = isBoundToTab ? (boundTab?.uri || null) : appState.currentFileURI;
  const currentFilePath = isBoundToTab ? (boundTab?.filePath || null) : appState.currentFilePath;
  const fileContent = isBoundToTab ? (boundTab?.content || '') : appState.fileContent;
  const isDirty = isBoundToTab ? Boolean(boundTab?.isDirty) : appState.isDirty;
  const pendingRevealTarget = isBoundToTab ? (boundTab?.pendingRevealTarget || null) : appState.pendingRevealTarget;
  const currentReviewOverlay = appState.currentReviewOverlay;
  const addToChatShortcutLabel = getAddToChatShortcutLabel();
  const pushToast = useToastStore((state) => state.pushToast);
  const setReviews = useChatWorkspaceStore((state) => state.setReviews);
  const selectedThreadReviews = useChatWorkspaceStore((state) => state.getReviews(
    state.getTargetChatPath(state.selectedConversationTarget)
  ));
  const reviewActionBusyRef = useRef(false);
  const reviewDecisionHandlerRef = useRef<((decision: ReviewOverlayDecision, overlay: ReviewOverlay) => void) | null>(null);

  const autoReviewOverlay = findPendingReviewOverlayForFile(selectedThreadReviews, currentFilePath);
  const reviewOverlay: ReviewOverlay | null = currentReviewOverlay?.filePath === currentFilePath
    ? currentReviewOverlay
    : autoReviewOverlay;

  const fileContentRef = useRef(fileContent);
  const pendingRevealTargetRef = useRef(pendingRevealTarget);

  useEffect(() => {
    fileContentRef.current = fileContent;
  }, [fileContent]);

  useEffect(() => {
    pendingRevealTargetRef.current = pendingRevealTarget;
  }, [pendingRevealTarget]);

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

  const updateSelectionHint = useCallback(() => {
    const editor = editorRef.current;
    if (!editor || !editorHasFocusRef.current) {
      setSelectionHintPosition(null);
      return;
    }
    const selection = editor.getSelectionSnapshot();
    if (!selection) {
      suppressedSelectionKeyRef.current = null;
      setSelectionHintPosition(null);
      return;
    }
    const selectionKey = `${(currentFilePath || '').trim()}:${selection.from}:${selection.to}`;
    if (suppressedSelectionKeyRef.current === selectionKey) {
      setSelectionHintPosition(null);
      return;
    }
    if (suppressedSelectionKeyRef.current && suppressedSelectionKeyRef.current !== selectionKey) {
      suppressedSelectionKeyRef.current = null;
    }
    setSelectionHintPosition(resolveSelectionHintPosition(editor.getView()));
  }, [currentFilePath]);

  const focusEditorIfRequested = useCallback((editor: TextEditorInstance | null): boolean => {
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

  useEffect(() => {
    if (!containerRef.current) return;

    let cancelled = false;
    const openingIdentity = currentFileURI || currentFilePath;

    if (editorRef.current) {
      editorRef.current.destroy();
    }

    const resolveLanguage = async () => {
      const settings = await window.electronAPI?.settings.get();
      const editorSettings = settings?.editor;
      const languageId = currentFilePath
        ? languageRegistry.resolveLanguageId(
          currentFilePath,
          editorSettings?.filesAssociations || {},
          editorSettings?.defaultLanguage || 'plaintext'
        )
        : (editorSettings?.defaultLanguage || 'plaintext');
      const languageExtensions = await languageRegistry.getExtensions(languageId);
      return { languageExtensions, languageId };
    };

    resolveLanguage().then(({ languageExtensions, languageId }) => {
      if (cancelled) return;
      if (!containerRef.current) return;
      if (openingIdentity !== (currentFileURI || currentFilePath)) return;

      const completion = {
        editorKind: 'text',
        languageId,
        documentPath: currentFilePath,
        enabled: () => isInlineCompletionEnabledForPath(currentFilePath),
        cancel: cancelInlineCompletion,
        request: (payload: Parameters<typeof requestInlineCompletion>[1]) =>
          requestInlineCompletion(currentFilePath, payload),
      };

      editorRef.current = createTextEditor(containerRef.current, {
        initialContent: fileContentRef.current,
        onContentChange: (content) => {
          if (isBoundToTab && boundTabId) {
            setTabContent(boundTabId, content);
            return;
          }
          setFileContent(content);
        },
        onSelectionChange: () => {
          requestAnimationFrame(() => {
            updateSelectionHint();
          });
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
        languageExtensions,
        showLineNumbers,
        reviewOverlay,
        reviewActions: {
          onDecision: (decision, overlay) => reviewDecisionHandlerRef.current?.(decision, overlay),
        },
        completion,
        onOpenSearchPanel: ({ replace }) => {
          searchOverlayRef.current?.open({ replace });
        },
      });

      const reveal = pendingRevealTargetRef.current;
      if (reveal) {
        editorRef.current.scrollToPos(revealTargetToPos(fileContentRef.current, reveal));
        if (!isBoundToTab) {
          useAppStore.getState().setPendingRevealTarget(null);
        }
      }

      if (focusEditorIfRequested(editorRef.current)) {
        // Focus request consumed.
      } else if (autoFocus) {
        editorRef.current.focus();
      } else if (!isBoundToTab) {
        setEditorFocused(false);
      }
    });

    return () => {
      cancelled = true;
      if (editorRef.current) {
        editorRef.current.destroy();
        editorRef.current = null;
      }
      if (!isBoundToTab) {
        setEditorFocused(false);
      }
      editorHasFocusRef.current = false;
      setSelectionHintPosition(null);
    };
  }, [autoFocus, boundTabId, currentFileURI, currentFilePath, focusEditorIfRequested, isBoundToTab, setEditorFocused, setFileContent, setTabContent, updateSelectionHint]);

  useEffect(() => {
    if (!editorFocusRequest) {
      return;
    }
    focusEditorIfRequested(editorRef.current);
  }, [editorFocusRequest, focusEditorIfRequested]);

  useEffect(() => {
    editorRef.current?.setLineNumbersVisible(showLineNumbers);
  }, [showLineNumbers]);

  useEffect(() => {
    if (editorRef.current) {
      const currentContent = editorRef.current.getContent();
      if (currentContent !== fileContent) {
        editorRef.current.setContent(fileContent);
      }
    }
  }, [fileContent]);

  useEffect(() => {
    editorRef.current?.setReviewOverlay(reviewOverlay);
  }, [reviewOverlay]);

  useEffect(() => {
    if (isBoundToTab || !editorRef.current || !pendingRevealTarget) {
      return;
    }
    editorRef.current.scrollToPos(revealTargetToPos(fileContent, pendingRevealTarget));
    setPendingRevealTarget(null);
  }, [fileContent, isBoundToTab, pendingRevealTarget, setPendingRevealTarget]);

  useEffect(() => {
    const view = editorRef.current?.getView();
    if (!view) {
      return;
    }
    const hideHint = () => {
      setSelectionHintPosition(null);
    };
    view.scrollDOM.addEventListener('scroll', hideHint, { passive: true });
    return () => {
      view.scrollDOM.removeEventListener('scroll', hideHint);
    };
  }, [currentFilePath]);

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

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
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

  return (
    <div className="op-text-editor flex-1 overflow-hidden relative">
      <div
        ref={containerRef}
        className="flex-1 h-full overflow-hidden"
        onKeyDown={handleKeyDown}
      />
      {pinEnabled && (
        <button
          type="button"
          className={`op-text-editor-pin icon-gutter-btn-sm icon-button-inline ${pinned ? 'is-pinned' : ''}`}
          onClick={(event) => {
            event.stopPropagation();
            onPinToggle?.();
          }}
          title={pinned ? 'Unpin file' : 'Pin file to right'}
          aria-label={pinned ? 'Unpin file' : 'Pin file to right'}
          aria-pressed={pinned}
        >
          <PinIcon className="h-3.5 w-3.5" />
        </button>
      )}
      <EditorSearchOverlay
        getView={() => editorRef.current?.getView() || null}
        registerHandle={(handle) => {
          searchOverlayRef.current = handle;
        }}
      />
      <SelectionAddToChatHint
        position={selectionHintPosition}
        shortcutLabel={addToChatShortcutLabel}
        onClick={() => {
          appendSelectionToConversationDraft();
        }}
      />
    </div>
  );
};
