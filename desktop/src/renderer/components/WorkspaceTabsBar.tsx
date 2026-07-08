import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useStore } from 'zustand';
import { CloseButton, FolderIcon, OpenBrainLogo, RemoteIcon } from './Icons';
import { IconButton } from './IconButton';
import { PopupMenu, PopupMenuItem } from './PopupMenu';
import { useDismissOnOutsideInteraction } from '../hooks/useDismissOnOutsideInteraction';
import {
  TAB_CLOSE_BUTTON_DELAYED_REVEAL_CLASS,
  TAB_ITEM_FLEX_STYLE,
  getTabCloseButtonClassName,
} from './tabLayout';
import {
  OP_SG_CAPSULE,
  OP_SG_CAPSULE_ON_TITLEBAR,
} from './staticGlassCapsule';
import { getWorkspaceStore } from '../store/appStore';
import { resolveHostLabel, type SshHost, type WorkspaceTab } from '../store/tabManagerStore';
import {
  getSortedRemoteBuckets,
  useRecentWorkspacesStore,
  type LocalRecentWorkspace,
  type RemoteRecentWorkspaceBucket,
} from '../store/recentWorkspacesStore';
import { useOpenBrainStore } from '../store/openBrainStore';

function normalizeWorkspacePath(path: string | undefined): string {
  const normalized = (path || '').trim().replace(/\\/g, '/');
  return normalized.length > 1 ? normalized.replace(/\/+$/, '') : normalized;
}

function isOpenBrainWorkspacePath(path: string | undefined, knownPaths: Set<string>): boolean {
  const normalized = normalizeWorkspacePath(path);
  if (!normalized) {
    return false;
  }
  return knownPaths.has(normalized);
}

function getPathBaseName(path: string | undefined): string {
  const normalized = normalizeWorkspacePath(path);
  if (!normalized) {
    return '';
  }
  const parts = normalized.split('/');
  return parts[parts.length - 1] || normalized;
}

function shouldRegenerateWorkspaceTabLabel(label: string | undefined): boolean {
  return !label || label === 'Untitled';
}

function useTabDisplayState(tab: WorkspaceTab): { label: string; workspacePath?: string } {
  const store = getWorkspaceStore(tab.id);
  const currentDir = useStore(store, (s) => s.currentDir);
  const remoteHostLabel = useStore(store, (s) => s.remoteSession?.hostLabel ?? null);

  if (tab.kind === 'remote') {
    return {
      label: remoteHostLabel ?? tab.label,
      workspacePath: tab.workspacePath,
    };
  }

  const workspacePath = tab.workspacePath || currentDir || undefined;
  const label = shouldRegenerateWorkspaceTabLabel(tab.label)
    ? getPathBaseName(workspacePath) || tab.label || 'Untitled'
    : tab.label;
  return { label, workspacePath };
}

function WorkspaceTabRow({
  tab,
  isActive,
  openbrainPaths,
  onSelect,
  onClose,
}: {
  tab: WorkspaceTab;
  isActive: boolean;
  openbrainPaths: Set<string>;
  onSelect: () => void;
  onClose: () => void;
}) {
  const { label, workspacePath } = useTabDisplayState(tab);
  const isOpenBrain = tab.kind === 'local' && isOpenBrainWorkspacePath(workspacePath, openbrainPaths);
  return (
    <div
      className={`workspace-tab-shell no-drag group relative flex items-center gap-1 px-2.5 text-secondary-text ${
        isActive ? `is-active ${OP_SG_CAPSULE} ${OP_SG_CAPSULE_ON_TITLEBAR}` : ''
      }`}
      style={TAB_ITEM_FLEX_STYLE}
    >
      <button
        type="button"
        className="no-drag flex h-full min-w-0 flex-1 items-center truncate"
        onClick={onSelect}
        title={label}
      >
        {isOpenBrain ? (
          <OpenBrainLogo
            className="mr-1.5 h-4 w-4 shrink-0"
            title="OpenBrain workspace"
          />
        ) : null}
        {label}
      </button>
      <CloseButton
        className={getTabCloseButtonClassName(
          'bg-secondary-bg',
          `${TAB_CLOSE_BUTTON_DELAYED_REVEAL_CLASS} no-drag`,
        )}
        onClick={(event) => {
          event.stopPropagation();
          onClose();
        }}
        title="Close tab"
        variant="inline"
      />
    </div>
  );
}

function getLocalWorkspaceLabel(entry: LocalRecentWorkspace): string {
  return getPathBaseName(entry.path);
}

type WorkspaceTabsBarProps = {
  tabs: WorkspaceTab[];
  activeTabId: string;
  onSelectTab: (tabId: string) => void;
  onCloseTab: (tabId: string) => void;
  onNewFolder: () => void;
  onNewRemote: () => void;
  onOpenRecentLocal: (path: string) => Promise<void>;
  onOpenRecentRemote: (host: SshHost, path?: string) => Promise<void>;
};

const HOVER_LEAVE_MS = 120;
const MAIN_MENU_WIDTH = 220;
const RECENT_PANEL_WIDTH = 300;
const MAIN_PLUS_RECENT_WIDTH = MAIN_MENU_WIDTH + RECENT_PANEL_WIDTH;
const RECENT_LIST_MAX_H = 320;

function RemoteRecentGroup({
  group,
  onSelect,
}: {
  group: RemoteRecentWorkspaceBucket;
  onSelect: (host: SshHost, path?: string) => void;
}) {
  return (
    <div className="border-b border-border last:border-b-0">
      <button
        type="button"
        className="flex w-full items-center gap-2 px-2 py-1.5 text-left text-prime-text hover:bg-hover-bg"
        onClick={() => onSelect(group.host)}
      >
        <RemoteIcon className="w-4 h-4 opacity-70 flex-shrink-0" />
        <div className="min-w-0 flex-1">
          <div className="text-sm truncate">{group.label || resolveHostLabel(group.host)}</div>
          <div className="text-xs text-secondary-text truncate opacity-70">{resolveHostLabel(group.host)}</div>
        </div>
      </button>
      {group.directories.map((entry) => (
        <button
          key={`${group.instanceID}-${entry.path}`}
          type="button"
          className="flex w-full items-center gap-2 pl-8 pr-2 py-1.5 text-left text-secondary-text hover:bg-hover-bg hover:text-prime-text"
          onClick={() => onSelect(group.host, entry.path)}
        >
          <div className="min-w-0 flex-1">
            <div className="text-sm truncate">{getPathBaseName(entry.path)}</div>
            <div className="text-xs truncate opacity-70">{entry.path}</div>
          </div>
        </button>
      ))}
    </div>
  );
}

export const WorkspaceTabsBar: React.FC<WorkspaceTabsBarProps> = ({
  tabs,
  activeTabId,
  onSelectTab,
  onCloseTab,
  onNewFolder,
  onNewRemote,
  onOpenRecentLocal,
  onOpenRecentRemote,
}) => {
  const { t } = useTranslation('menu');
  const [menuOpen, setMenuOpen] = useState(false);
  const [submenuType, setSubmenuType] = useState<'folder' | 'remote' | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const hoverLeaveRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const recent = useRecentWorkspacesStore((s) => s.recent);
  const loadRecent = useRecentWorkspacesStore((s) => s.load);
  const openbrainSources = useOpenBrainStore((s) => s.sources);
  const openbrainPaths = useMemo(
    () => new Set(openbrainSources.map((brain) => normalizeWorkspacePath(brain.path)).filter(Boolean)),
    [openbrainSources],
  );

  useEffect(() => {
    loadRecent();
  }, [loadRecent]);

  const recentLocal = useMemo(() => recent.local, [recent.local]);
  const recentRemoteGroups = useMemo(() => getSortedRemoteBuckets(recent), [recent]);

  useEffect(() => {
    if (!menuOpen) {
      setSubmenuType(null);
      if (hoverLeaveRef.current) {
        clearTimeout(hoverLeaveRef.current);
        hoverLeaveRef.current = null;
      }
    }
  }, [menuOpen]);

  const clearHoverTimers = useCallback(() => {
    if (hoverLeaveRef.current) {
      clearTimeout(hoverLeaveRef.current);
      hoverLeaveRef.current = null;
    }
  }, []);

  const closeMenu = useCallback(() => {
    clearHoverTimers();
    setSubmenuType(null);
    setMenuOpen(false);
  }, [clearHoverTimers]);

  useDismissOnOutsideInteraction({
    active: menuOpen,
    onDismiss: closeMenu,
    insideRefs: [menuRef],
  });

  const handleRecentLocalClick = async (path: string) => {
    await onOpenRecentLocal(path);
    setMenuOpen(false);
  };

  const handleRecentRemoteClick = async (host: SshHost, path?: string) => {
    await onOpenRecentRemote(host, path);
    setMenuOpen(false);
  };

  return (
    <div className="flex h-full w-full min-w-0 items-center">
      <div className="flex h-full min-w-0 items-center gap-1 overflow-x-auto overflow-y-visible">
        {tabs.map((tab) => (
          <WorkspaceTabRow
            key={tab.id}
            tab={tab}
            isActive={tab.id === activeTabId}
            openbrainPaths={openbrainPaths}
            onSelect={() => onSelectTab(tab.id)}
            onClose={() => onCloseTab(tab.id)}
          />
        ))}
      </div>
      <div className="workspace-tab-action relative flex no-drag" ref={menuRef}>
        <IconButton
          size={20}
          className={`no-drag ml-1 text-xl ${menuOpen ? 'bg-hover-bg text-highlight' : 'text-secondary-text'}`}
          onClick={() => setMenuOpen((open) => !open)}
          title={t('workspaceTab.newTab')}
          aria-label={t('workspaceTab.newTab')}
        >
          +
        </IconButton>
        {menuOpen && (
          <PopupMenu className="absolute left-full top-0 ml-1 flex text-secondary-text no-drag z-50">
            <div className="min-w-[180px]">
              <div
                className="relative"
                style={submenuType === 'folder' ? { width: MAIN_PLUS_RECENT_WIDTH } : undefined}
                onMouseEnter={() => {
                  clearHoverTimers();
                  setSubmenuType('folder');
                }}
                onMouseLeave={() => {
                  clearHoverTimers();
                  hoverLeaveRef.current = setTimeout(() => {
                    setSubmenuType((current) => (current === 'folder' ? null : current));
                  }, HOVER_LEAVE_MS);
                }}
              >
                <div className="w-[220px]">
                  <PopupMenuItem
                    onClick={() => {
                      onNewFolder();
                      setMenuOpen(false);
                    }}
                  >
                    <FolderIcon className="w-4 h-4 opacity-70" />
                    <span>{t('workspaceTab.openFolder')}</span>
                  </PopupMenuItem>
                </div>
                {submenuType === 'folder' && (
                  <PopupMenu
                    className="absolute top-0 z-[51]"
                    style={{ width: RECENT_PANEL_WIDTH, left: MAIN_MENU_WIDTH }}
                    onMouseEnter={clearHoverTimers}
                    onMouseLeave={() => {
                      hoverLeaveRef.current = setTimeout(() => setSubmenuType(null), HOVER_LEAVE_MS);
                    }}
                  >
                    <div className="px-2 py-1.5 border-b border-border text-xs uppercase tracking-wider text-secondary-text mb-1">
                      Recent
                    </div>
                    {recentLocal.length === 0 ? (
                      <div className="px-2 py-1.5 text-xs text-secondary-text">No recent folders</div>
                    ) : (
                      <div className="overflow-auto" style={{ maxHeight: RECENT_LIST_MAX_H }}>
                        {recentLocal.map((entry) => (
                          <PopupMenuItem
                            key={entry.path}
                            className="group"
                            onClick={() => handleRecentLocalClick(entry.path)}
                          >
                            <FolderIcon className="w-4 h-4 opacity-70 flex-shrink-0" />
                            <div className="flex flex-col min-w-0 flex-1">
                              <div className="text-sm truncate">{getLocalWorkspaceLabel(entry)}</div>
                              <div className="text-xs text-secondary-text group-hover:text-prime-text truncate opacity-70">
                                {entry.path}
                              </div>
                            </div>
                          </PopupMenuItem>
                        ))}
                      </div>
                    )}
                  </PopupMenu>
                )}
              </div>
              <div
                className="relative"
                style={submenuType === 'remote' ? { width: MAIN_PLUS_RECENT_WIDTH } : undefined}
                onMouseEnter={() => {
                  clearHoverTimers();
                  setSubmenuType('remote');
                }}
                onMouseLeave={() => {
                  clearHoverTimers();
                  hoverLeaveRef.current = setTimeout(() => {
                    setSubmenuType((current) => (current === 'remote' ? null : current));
                  }, HOVER_LEAVE_MS);
                }}
              >
                <div className="w-[220px]">
                  <PopupMenuItem
                    onClick={() => {
                      onNewRemote();
                      setMenuOpen(false);
                    }}
                  >
                    <RemoteIcon className="w-4 h-4 opacity-70" />
                    <span>{t('workspaceTab.connectRemote')}</span>
                  </PopupMenuItem>
                </div>
                {submenuType === 'remote' && (
                  <PopupMenu
                    className="absolute top-0 z-[51]"
                    style={{ width: RECENT_PANEL_WIDTH, left: MAIN_MENU_WIDTH }}
                    onMouseEnter={clearHoverTimers}
                    onMouseLeave={() => {
                      hoverLeaveRef.current = setTimeout(() => setSubmenuType(null), HOVER_LEAVE_MS);
                    }}
                  >
                    <div className="px-2 py-1.5 border-b border-border text-xs uppercase tracking-wider text-secondary-text mb-1">
                      Recent
                    </div>
                    {recentRemoteGroups.length === 0 ? (
                      <div className="px-2 py-1.5 text-xs text-secondary-text">No recent remotes</div>
                    ) : (
                      <div className="overflow-auto" style={{ maxHeight: RECENT_LIST_MAX_H }}>
                        {recentRemoteGroups.map((group) => (
                          <RemoteRecentGroup key={group.instanceID} group={group} onSelect={handleRecentRemoteClick} />
                        ))}
                      </div>
                    )}
                  </PopupMenu>
                )}
              </div>
            </div>
          </PopupMenu>
        )}
      </div>
    </div>
  );
};
