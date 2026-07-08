import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

import { useAppStore } from '../../store/appStore';
import { useChatWorkspaceStore } from '../../store/chatWorkspaceStore';
import { ChevronDownIcon, ChevronRightIcon, MoreHorizontalIcon } from '../Icons';
import {
  formatMessengerPendingBadgeCount,
  getMessengerChannelPendingRequestCount,
  selectMessengerAgentSummaries,
  selectMessengerChannelsForAgent,
  useMessengerStore,
  type MessengerAgentSummary,
} from '../../store/messengerStore';
import { useToastStore } from '../../store/toastStore';
import { PopupMenu, PopupMenuItem } from '../PopupMenu';

const MESSENGER_THREAD_PREVIEW_LIMIT = 10;
const MESSENGER_THREAD_MAX_VISIBLE = 99;
const MESSENGER_AGENT_MENU_WIDTH = 240;
const MESSENGER_AGENT_MENU_HEIGHT = 88;
const MESSENGER_AGENT_MENU_MARGIN = 8;

type MessengerAgentContextMenu = {
  agentID: string;
  pendingRequestCount: number;
  x: number;
  y: number;
};

type MessengerThreadSummary = {
  channelID: string;
  threadID: string;
  agentID: string;
  title: string;
  lastUpdatedAt: number;
  pendingRequestCount: number;
};

function formatSidebarTime(value: number) {
  if (!value) {
    return '';
  }
  const date = new Date(value);
  const today = new Date();
  if (date.toDateString() === today.toDateString()) {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }
  return date.toLocaleDateString([], { month: 'short', day: 'numeric' });
}

function parseMessengerTime(value: string | undefined, fallback = 0) {
  const parsed = Date.parse(value || '');
  return Number.isFinite(parsed) ? parsed : fallback;
}

function agentInitial(label: string) {
  return (label.trim().charAt(0) || 'A').toUpperCase();
}

function resolveMessengerAgentName(
  agentID: string,
  fallbackTitle: string,
  resolveAgentByID: (agentID: string) => { name?: string | null } | null,
) {
  const resolved = resolveAgentByID(agentID);
  return (resolved?.name || '').trim() || fallbackTitle;
}

function compactTitle(value: string) {
  const normalized = value.trim().replace(/\s+/g, ' ');
  if (!normalized) {
    return '';
  }
  return normalized.length > 72 ? `${normalized.slice(0, 69)}...` : normalized;
}

function messengerAgentMenuPosition(menu: MessengerAgentContextMenu): { left: number; top: number } {
  const maxLeft = window.innerWidth - MESSENGER_AGENT_MENU_WIDTH - MESSENGER_AGENT_MENU_MARGIN;
  const maxTop = window.innerHeight - MESSENGER_AGENT_MENU_HEIGHT - MESSENGER_AGENT_MENU_MARGIN;
  return {
    left: Math.min(Math.max(MESSENGER_AGENT_MENU_MARGIN, menu.x), Math.max(MESSENGER_AGENT_MENU_MARGIN, maxLeft)),
    top: Math.min(Math.max(MESSENGER_AGENT_MENU_MARGIN, menu.y), Math.max(MESSENGER_AGENT_MENU_MARGIN, maxTop)),
  };
}

function buildMessengerThreadSummariesByAgent({
  channels,
  recordsByID,
  agents,
}: {
  channels: ReturnType<typeof useMessengerStore.getState>['channels'];
  recordsByID: ReturnType<typeof useMessengerStore.getState>['recordsByID'];
  agents: MessengerAgentSummary[];
}): Record<string, MessengerThreadSummary[]> {
  const next: Record<string, MessengerThreadSummary[]> = {};
  const records = Object.values(recordsByID);

  for (const agent of agents) {
    const agentID = agent.agentID;
    const agentChannels = selectMessengerChannelsForAgent({ channels }, agentID);
    next[agentID] = agentChannels
      .map((channel) => {
        const channelID = (channel.channelID || '').trim();
        const threadID = (channel.threadID || '').trim();
        if (!channelID || !threadID) {
          return null;
        }
        const title = compactTitle(channel.title || channel.lastMessage?.title || threadID) || 'Thread';
        const lastUpdatedAt = parseMessengerTime(channel.updatedAt || channel.lastMessage?.updatedAt || channel.lastMessage?.createdAt, 0);
        const pendingRequestCount = getMessengerChannelPendingRequestCount(
          channel,
          records.filter((record) => (record.channelID || '').trim() === channelID),
        );
        return {
          channelID,
          threadID,
          agentID,
          title,
          lastUpdatedAt,
          pendingRequestCount,
        };
      })
      .filter((thread): thread is MessengerThreadSummary => Boolean(thread))
      .sort((a, b) => b.lastUpdatedAt - a.lastUpdatedAt);
  }

  return next;
}

export function MessengerSidebar() {
  const refreshMessenger = useAppStore((state) => state.refreshMessenger);
  const loadMessengerChannel = useAppStore((state) => state.loadMessengerChannel);
  const archiveMessengerAgentPendingRequests = useAppStore((state) => state.archiveMessengerAgentPendingRequests);
  const archiveMessengerAgentMessages = useAppStore((state) => state.archiveMessengerAgentMessages);
  const resolveAgentByID = useAppStore((state) => state.resolveAgentByID);
  const openThreadConversation = useChatWorkspaceStore((state) => state.openThreadConversation);
  const selectChatConversation = useChatWorkspaceStore((state) => state.selectChatConversation);
  const hideComposer = useChatWorkspaceStore((state) => state.hideComposer);
  const selectedConversationTarget = useChatWorkspaceStore((state) => state.selectedConversationTarget);
  const pushToast = useToastStore((state) => state.pushToast);
  const selectedAgentID = useMessengerStore((state) => state.selectedAgentID);
  const selectedChannelID = useMessengerStore((state) => state.selectedChannelID);
  const selectAgent = useMessengerStore((state) => state.selectAgent);
  const selectChannel = useMessengerStore((state) => state.selectChannel);
  const channels = useMessengerStore((state) => state.channels);
  const recordsByID = useMessengerStore((state) => state.recordsByID);
  const messages = useMessengerStore((state) => state.messages);
  const [expandedAgentIDs, setExpandedAgentIDs] = useState<Record<string, boolean>>({});
  const [showAllThreadAgentIDs, setShowAllThreadAgentIDs] = useState<Record<string, boolean>>({});
  const [agentContextMenu, setAgentContextMenu] = useState<MessengerAgentContextMenu | null>(null);
  const [clearingAgentID, setClearingAgentID] = useState<string | null>(null);
  const agentMenuRef = useRef<HTMLDivElement | null>(null);
  const summaries = useMemo(
    () => selectMessengerAgentSummaries({ channels, recordsByID, messages }),
    [channels, recordsByID, messages],
  );
  const visibleSummaries = useMemo(() => summaries, [summaries]);
  const threadSummariesByAgent = useMemo(
    () => buildMessengerThreadSummariesByAgent({ channels, recordsByID, agents: visibleSummaries }),
    [channels, recordsByID, visibleSummaries],
  );

  useEffect(() => {
    void refreshMessenger();
  }, [refreshMessenger]);

  useEffect(() => {
    selectChatConversation(null);
    hideComposer();
  }, [hideComposer, selectChatConversation]);

  const openThreadSummary = useCallback((thread: MessengerThreadSummary): boolean => {
    const normalizedAgentID = thread.agentID.trim();
    const normalizedChannelID = thread.channelID.trim();
    const normalizedThreadID = thread.threadID.trim();
    if (!normalizedAgentID || !normalizedChannelID || !normalizedThreadID) {
      return false;
    }
    selectAgent(normalizedAgentID);
    selectChannel(normalizedChannelID);
    openThreadConversation(normalizedThreadID, {
      title: thread.title || undefined,
      agentID: normalizedAgentID,
    });
    return true;
  }, [openThreadConversation, selectAgent, selectChannel]);

  const handleSelect = (summary: MessengerAgentSummary) => {
    const agentID = summary.agentID;
    const wasExpanded = Boolean(expandedAgentIDs[agentID]);
    selectAgent(agentID);
    setExpandedAgentIDs((current) => ({
      ...current,
      [agentID]: !current[agentID],
    }));
    if (wasExpanded) {
      setShowAllThreadAgentIDs((current) => ({
        ...current,
        [agentID]: false,
      }));
    }
  };

  const handleSelectThread = (thread: MessengerThreadSummary) => {
    if (!openThreadSummary(thread)) {
      return;
    }
    void loadMessengerChannel(thread.channelID)
      .catch((error) => {
        pushToast(error instanceof Error ? error.message : 'Failed to load messages');
      });
  };

  const closeAgentContextMenu = useCallback(() => {
    setAgentContextMenu(null);
  }, []);

  useEffect(() => {
    if (!agentContextMenu) {
      return;
    }
    const handleAgentMenuMouseDown = (event: MouseEvent) => {
      if (agentMenuRef.current?.contains(event.target as Node)) {
        return;
      }
      closeAgentContextMenu();
    };
    const handleAgentMenuKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        closeAgentContextMenu();
      }
    };
    window.addEventListener('mousedown', handleAgentMenuMouseDown, true);
    window.addEventListener('keydown', handleAgentMenuKeyDown, true);
    return () => {
      window.removeEventListener('mousedown', handleAgentMenuMouseDown, true);
      window.removeEventListener('keydown', handleAgentMenuKeyDown, true);
    };
  }, [agentContextMenu, closeAgentContextMenu]);

  const handleAgentContextMenu = (
    event: React.MouseEvent,
    summary: MessengerAgentSummary,
  ) => {
    event.preventDefault();
    event.stopPropagation();
    if (clearingAgentID === summary.agentID) {
      pushToast('Clearing messages...', { durationMs: 1600 });
      closeAgentContextMenu();
      return;
    }
    setAgentContextMenu({
      agentID: summary.agentID,
      pendingRequestCount: summary.pendingRequestCount,
      x: event.clientX,
      y: event.clientY,
    });
  };

  const handleClearAgentPendingRequests = useCallback(async () => {
    const menu = agentContextMenu;
    if (!menu || clearingAgentID) {
      if (clearingAgentID) {
        pushToast('Clearing messages...', { durationMs: 1600 });
        closeAgentContextMenu();
      }
      return;
    }
    setClearingAgentID(menu.agentID);
    closeAgentContextMenu();
    try {
      const archived = await archiveMessengerAgentPendingRequests(menu.agentID);
      if (archived > 0) {
        pushToast(`Cleared ${archived} pending request${archived === 1 ? '' : 's'}`);
      } else {
        pushToast('No pending requests to clear');
      }
    } catch (error) {
      pushToast(error instanceof Error ? error.message : 'Failed to clear pending requests');
    } finally {
      setClearingAgentID(null);
    }
  }, [agentContextMenu, archiveMessengerAgentPendingRequests, clearingAgentID, closeAgentContextMenu, pushToast]);

  const handleClearAgentAllMessages = useCallback(async () => {
    const menu = agentContextMenu;
    if (!menu || clearingAgentID) {
      if (clearingAgentID) {
        pushToast('Clearing messages...', { durationMs: 1600 });
        closeAgentContextMenu();
      }
      return;
    }
    setClearingAgentID(menu.agentID);
    closeAgentContextMenu();
    try {
      const archived = await archiveMessengerAgentMessages(menu.agentID);
      if (archived > 0) {
        pushToast(`Cleared ${archived} message${archived === 1 ? '' : 's'}`);
      } else {
        pushToast('No messages to clear');
      }
    } catch (error) {
      pushToast(error instanceof Error ? error.message : 'Failed to clear messages');
    } finally {
      setClearingAgentID(null);
    }
  }, [agentContextMenu, archiveMessengerAgentMessages, clearingAgentID, closeAgentContextMenu, pushToast]);

  const agentMenuPosition = agentContextMenu ? messengerAgentMenuPosition(agentContextMenu) : null;

  return (
    <div className="flex h-full min-h-0 flex-col bg-sidebar-bg">
      <div className="ui-tabbar sidebar-root-header flex shrink-0 items-center border-b border-border px-3">
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium text-prime-text">Messenger</div>
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto py-1">
        {visibleSummaries.length === 0 ? (
          <div className="px-3 py-6 text-sm text-secondary-text">No messages</div>
        ) : (
          visibleSummaries.map((summary) => {
            const active = summary.agentID === selectedAgentID;
            const agentName = resolveMessengerAgentName(summary.agentID, summary.title, resolveAgentByID);
            const threadSummaries = threadSummariesByAgent[summary.agentID] || [];
            const expanded = Boolean(expandedAgentIDs[summary.agentID]);
            const showAllThreads = Boolean(showAllThreadAgentIDs[summary.agentID]);
            const visibleThreadSummaries = showAllThreads
              ? threadSummaries.slice(0, MESSENGER_THREAD_MAX_VISIBLE)
              : threadSummaries.slice(0, MESSENGER_THREAD_PREVIEW_LIMIT);
            const hiddenThreadCount = Math.max(
              0,
              Math.min(threadSummaries.length, MESSENGER_THREAD_MAX_VISIBLE) - visibleThreadSummaries.length,
            );
            return (
              <React.Fragment key={summary.agentID}>
                <button
                  type="button"
                  className={`flex w-full min-w-0 items-center gap-2 px-2 py-2 text-left transition-colors ${
                    active ? 'bg-hover-bg text-prime-text' : 'text-secondary-text hover:bg-hover-bg hover:text-prime-text'
                  }`}
                  onClick={() => handleSelect(summary)}
                  onContextMenu={(event) => handleAgentContextMenu(event, summary)}
                  title={summary.agentID}
                >
                  <span className="flex h-5 w-4 shrink-0 items-center justify-center text-tertiary-text">
                    {expanded ? <ChevronDownIcon className="h-3.5 w-3.5" /> : <ChevronRightIcon className="h-3.5 w-3.5" />}
                  </span>
                  <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-md border text-sm font-medium ${
                    active
                      ? 'border-active-border bg-secondary-bg text-highlight'
                      : 'border-border bg-secondary-bg text-secondary-text'
                  }`}>
                    {agentInitial(agentName)}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex min-w-0 items-center gap-2">
                      <span className="truncate text-sm font-medium">{agentName}</span>
                      {summary.channelCount > 1 ? (
                        <span className="shrink-0 rounded border border-border px-1.5 py-0.5 text-[10px] font-medium text-tertiary-text">
                          {summary.channelCount}
                        </span>
                      ) : null}
                    </span>
                    <span className="mt-0.5 block truncate text-xs text-tertiary-text">
                      {summary.agentID}
                    </span>
                  </span>
                  <span className="flex shrink-0 flex-col items-end gap-1">
                    <span className="text-[11px] text-tertiary-text">{formatSidebarTime(summary.lastUpdatedAt)}</span>
                    {summary.pendingRequestCount > 0 ? (
                      <span className="messenger-pending-badge min-w-4 rounded-full px-1.5 text-center text-[10px] font-semibold leading-4">
                        {formatMessengerPendingBadgeCount(summary.pendingRequestCount)}
                      </span>
                    ) : null}
                  </span>
                </button>
                {expanded && threadSummaries.length > 0 ? (
                  <div className="py-0.5">
                    {visibleThreadSummaries.map((thread) => {
                      const threadActive = selectedChannelID === thread.channelID
                        || (selectedConversationTarget?.kind === 'thread' && selectedConversationTarget.threadID === thread.threadID);
                      return (
                        <button
                          key={thread.channelID}
                          type="button"
                          className={`flex w-full min-w-0 items-center gap-2 px-2 py-1.5 pl-12 text-left transition-colors ${
                            threadActive ? 'bg-secondary-bg text-prime-text' : 'text-secondary-text hover:bg-hover-bg hover:text-prime-text'
                          }`}
                          onClick={() => handleSelectThread(thread)}
                          title={thread.title}
                        >
                          <span className={`mt-1 h-1.5 w-1.5 shrink-0 rounded-full ${
                            thread.pendingRequestCount > 0
                              ? 'messenger-pending-dot'
                              : threadActive
                                ? 'bg-highlight'
                                : 'bg-tertiary-text/60'
                          }`} />
                          <span className="min-w-0 flex-1 truncate text-xs font-medium">
                            {thread.title}
                          </span>
                        </button>
                      );
                    })}
                    {!showAllThreads && hiddenThreadCount > 0 ? (
                      <button
                        type="button"
                        className="flex w-full min-w-0 items-center gap-2 px-2 py-1.5 pl-12 text-left text-tertiary-text transition-colors hover:bg-hover-bg hover:text-prime-text"
                        onClick={() => setShowAllThreadAgentIDs((current) => ({
                          ...current,
                          [summary.agentID]: true,
                        }))}
                        title={`Show up to ${MESSENGER_THREAD_MAX_VISIBLE} threads`}
                        aria-label={`Show up to ${MESSENGER_THREAD_MAX_VISIBLE} threads`}
                      >
                        <MoreHorizontalIcon className="h-4 w-4 shrink-0" />
                        <span className="truncate text-[11px] font-medium">{hiddenThreadCount} more</span>
                      </button>
                    ) : null}
                  </div>
                ) : null}
              </React.Fragment>
            );
          })
        )}
      </div>
      {agentContextMenu && agentMenuPosition ? createPortal(
        <PopupMenu
          ref={agentMenuRef}
          className="no-drag fixed z-[70]"
          style={{ left: agentMenuPosition.left, top: agentMenuPosition.top, width: MESSENGER_AGENT_MENU_WIDTH }}
          onMouseDown={(event) => event.stopPropagation()}
          onContextMenu={(event) => event.preventDefault()}
        >
          <PopupMenuItem
            disabled={Boolean(clearingAgentID) || agentContextMenu.pendingRequestCount <= 0}
            onClick={() => void handleClearAgentPendingRequests()}
          >
            Clear pending requests
            <span className="ml-auto text-xs text-tertiary-text">
              {formatMessengerPendingBadgeCount(agentContextMenu.pendingRequestCount)}
            </span>
          </PopupMenuItem>
          <PopupMenuItem
            disabled={Boolean(clearingAgentID)}
            onClick={() => void handleClearAgentAllMessages()}
          >
            Clear all messages
          </PopupMenuItem>
        </PopupMenu>,
        document.body,
      ) : null}
    </div>
  );
}
