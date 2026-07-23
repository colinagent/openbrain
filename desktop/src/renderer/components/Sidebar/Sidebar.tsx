import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useAppStore } from '../../store/appStore';
import { useRecentWorkspacesStore, type LocalRecentWorkspace } from '../../store/recentWorkspacesStore';
import { useToastStore } from '../../store/toastStore';
import { useUiStore } from '../../store/uiStore';
import { formatMessengerPendingBadgeCount, selectMessengerPendingRequestTotal, useMessengerStore } from '../../store/messengerStore';
import type { LocalOpenBrainWorkspace } from '../../store/openBrainStore';
import type { DesktopUpdateState } from '../../types/electron';
import {
  AgentBotIcon,
  ClockIcon,
  ChevronDownIcon,
  MarketplaceIcon,
  MessengerIcon,
  OpenBrainLogo,
  PlusIcon,
  RefreshIcon,
  SearchIcon,
  SkillPuzzleIcon,
  SwitchDirectoryIcon,
  ToolsIcon,
  WorkspaceIcon,
} from '../Icons';
import { IconButton } from '../IconButton';
import { AddAgentPopup, type AddAgentPopupAnchor } from '../Agent/AddAgentPopup';
import { useFileTreeSelectionStore } from '../FileExplorer/fileTreeSelectionStore';
import { PopupMenu, PopupMenuItem, PopupMenuSeparator } from '../PopupMenu';
import { AgentsSidebar } from './AgentsSidebar';
import { MessengerSidebar } from './MessengerSidebar';
import { OpenBrainSidebar } from './OpenBrainSidebar';
import { SearchSidebar } from './SearchSidebar';
import { SkillsSidebar } from './SkillsSidebar';
import { CronSidebar } from './CronSidebar';
import { ToolsSidebar } from './ToolsSidebar';
import { WorkspaceSidebar } from './WorkspaceSidebar';
import {
  getMainSidebarRailItems,
  isMainSidebarRailItemActive,
  isSidebarMoreRailActive,
  type SidebarRailItemKey,
} from './sidebarTabs';
import { SidebarRailAlertBadge } from './SidebarRailAlertBadge';
import { sidebarRailAlertCount } from './sidebarRailAlerts';
import { useSidebarRailAlerts } from './useSidebarRailAlerts';

type SidebarProps = {
  onSwitchLocal: (path: string) => Promise<void>;
  onOpenLocalNewTab: (path: string) => Promise<void>;
  onOpenRemoteNewTab: (host: NonNullable<LocalOpenBrainWorkspace['remoteHost']>, path?: string) => Promise<void>;
  onOpenLocalSwitchDirectory: () => void | Promise<void>;
  onOpenRemoteSwitchDirectory: () => void;
  onCreateOpenBrainSource: () => Promise<void>;
  onBindOpenBrainSource: (workspace: LocalOpenBrainWorkspace) => Promise<void>;
};

function getWorkspaceLabel(entry: LocalRecentWorkspace) {
  const parts = entry.path.split('/');
  return parts[parts.length - 1] || entry.path;
}

function getPathBaseName(path: string | null) {
  if (path === '/') {
    return '/';
  }
  const normalized = (path || '').replace(/\/+$/, '');
  if (!normalized) {
    return 'workspace';
  }
  const parts = normalized.split('/');
  return parts[parts.length - 1] || normalized;
}

const FOLDER_MENU_WIDTH = 260;
const ROOT_ADD_MENU_WIDTH = 220;
const MENU_ITEM_HEIGHT = 34;
const MENU_SEPARATOR_HEIGHT = 9;
const MENU_VERTICAL_PADDING = 8;
const RECENT_HEADER_HEIGHT = 28;
const RECENT_EMPTY_HEIGHT = 30;
const RECENT_LIST_MAX_HEIGHT = 256;
const RECENT_MENU_ITEM_HEIGHT = 44;

function getSwitchMenuHeight(recentCount: number) {
  const recentSectionHeight = recentCount > 0
    ? Math.min(RECENT_LIST_MAX_HEIGHT, recentCount * RECENT_MENU_ITEM_HEIGHT)
    : RECENT_EMPTY_HEIGHT;
  return MENU_VERTICAL_PADDING + MENU_ITEM_HEIGHT + MENU_SEPARATOR_HEIGHT + RECENT_HEADER_HEIGHT + recentSectionHeight;
}

function getAddMenuHeight(hasRootAgent: boolean) {
  if (hasRootAgent) {
    return MENU_VERTICAL_PADDING + MENU_ITEM_HEIGHT * 2;
  }
  return MENU_VERTICAL_PADDING + MENU_ITEM_HEIGHT * 4 + MENU_SEPARATOR_HEIGHT;
}

type SidebarOverflowView = 'tools' | 'marketplace';

const OVERFLOW_RAIL_ITEMS: { key: SidebarOverflowView; label: string }[] = [
  { key: 'tools', label: 'Tools' },
  { key: 'marketplace', label: 'Marketplace' },
];

function SidebarRailIconSlot({
  itemKey,
  children,
}: {
  itemKey: string;
  children: React.ReactNode;
}) {
  return (
    <span className="sidebar-rail-icon-slot" data-rail-icon={itemKey}>
      {children}
    </span>
  );
}

function SidebarOverflowRailIcon({ itemKey }: { itemKey: SidebarOverflowView }) {
  switch (itemKey) {
    case 'tools':
      return (
        <SidebarRailIconSlot itemKey={itemKey}>
          <ToolsIcon className="w-5 h-5" />
        </SidebarRailIconSlot>
      );
    case 'marketplace':
      return (
        <SidebarRailIconSlot itemKey={itemKey}>
          <MarketplaceIcon className="w-5 h-5" />
        </SidebarRailIconSlot>
      );
  }
}

function SidebarRailIcon({ itemKey }: { itemKey: SidebarRailItemKey }) {
  switch (itemKey) {
    case 'workspace':
      return (
        <SidebarRailIconSlot itemKey={itemKey}>
          <WorkspaceIcon className="w-5 h-5" />
        </SidebarRailIconSlot>
      );
    case 'agents':
      return (
        <SidebarRailIconSlot itemKey={itemKey}>
          <AgentBotIcon className="w-5 h-5" />
        </SidebarRailIconSlot>
      );
    case 'skills':
      return (
        <SidebarRailIconSlot itemKey={itemKey}>
          <SkillPuzzleIcon className="w-5 h-5" />
        </SidebarRailIconSlot>
      );
    case 'openbrain':
      return (
        <SidebarRailIconSlot itemKey={itemKey}>
          <OpenBrainLogo className="h-5 w-5" monochrome />
        </SidebarRailIconSlot>
      );
    case 'messenger':
      return (
        <SidebarRailIconSlot itemKey={itemKey}>
          <MessengerIcon className="w-5 h-5" />
        </SidebarRailIconSlot>
      );
    case 'cron':
      return (
        <SidebarRailIconSlot itemKey={itemKey}>
          <ClockIcon className="w-5 h-5" />
        </SidebarRailIconSlot>
      );
  }
}

export function Sidebar({
  onSwitchLocal,
  onOpenLocalNewTab,
  onOpenRemoteNewTab,
  onOpenLocalSwitchDirectory,
  onOpenRemoteSwitchDirectory,
  onCreateOpenBrainSource,
  onBindOpenBrainSource,
}: SidebarProps) {
  const [menuKind, setMenuKind] = useState<'switch' | 'add' | null>(null);
  const [moreMenuOpen, setMoreMenuOpen] = useState(false);
  const [desktopUpdate, setDesktopUpdate] = useState<DesktopUpdateState | null>(null);
  const [addAgentPopupAnchor, setAddAgentPopupAnchor] = useState<AddAgentPopupAnchor | null>(null);
  const [addAgentPopupTargetDir, setAddAgentPopupTargetDir] = useState<string | null>(null);
  const pendingRequestCount = useMessengerStore((state) => selectMessengerPendingRequestTotal(state));
  const railAlerts = useSidebarRailAlerts();
  /** When set, menu was opened by right-click; position is used for fixed placement. */
  const [contextMenuPosition, setContextMenuPosition] = useState<{ x: number; y: number } | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const switchTriggerRef = useRef<HTMLDivElement | null>(null);
  const addTriggerRef = useRef<HTMLDivElement | null>(null);

  /** Compute fixed position for a popup anchored to a trigger element's right edge. */
  const getFixedMenuPosition = (
    triggerRef: React.RefObject<HTMLDivElement | null>,
    menuWidth: number,
    menuHeight: number,
  ): { left: number; top: number } | null => {
    const rect = triggerRef.current?.getBoundingClientRect();
    if (!rect) return null;
    const left = Math.min(rect.right + 4, window.innerWidth - menuWidth - 8);
    const top = Math.min(Math.max(8, rect.top), window.innerHeight - menuHeight - 8);
    return { left, top };
  };

  const currentDir = useAppStore((state) => state.currentDir);
  const remoteSession = useAppStore((state) => state.remoteSession);
  const agentBindingByCwd = useAppStore((state) => state.agentBindingByCwd);
  const setRequestRootAction = useAppStore((state) => state.setRequestRootAction);
  const addAgentReference = useAppStore((state) => state.addAgentReference);
  const addCustomAgent = useAppStore((state) => state.addCustomAgent);
  const openMarketplaceTab = useAppStore((state) => state.openMarketplaceTab);
  const invalidateAgentScanCache = useAppStore((state) => state.invalidateAgentScanCache);
  const fetchDirAgentsInfo = useAppStore((state) => state.fetchDirAgentsInfo);
  const refreshVisibleWorkspaceTree = useAppStore((state) => state.refreshVisibleWorkspaceTree);
  const recent = useRecentWorkspacesStore((state) => state.recent.local);
  const loadRecent = useRecentWorkspacesStore((state) => state.load);
  const recordLocalRecent = useRecentWorkspacesStore((state) => state.recordLocal);
  const pushToast = useToastStore((state) => state.pushToast);
  const view = useUiStore((state) => state.sidebarView);
  const setView = useUiStore((state) => state.setSidebarView);
  const isRemoteWorkspace = !!remoteSession;
  const currentDirLabel = getPathBaseName(currentDir);
  const rootHasAgent = useMemo(
    () => (currentDir ? agentBindingByCwd.has(currentDir) : false),
    [agentBindingByCwd, currentDir],
  );
  const canCreateAtRoot = Boolean(currentDir);

  useEffect(() => {
    loadRecent();
  }, [loadRecent]);

  useEffect(() => {
    const api = window.electronAPI?.desktopUpdate;
    if (!api) {
      return;
    }
    let active = true;
    void api.getState().then((snapshot) => {
      if (active) {
        setDesktopUpdate(snapshot);
      }
    }).catch(() => {});
    const dispose = api.onChanged((snapshot) => {
      if (active) {
        setDesktopUpdate(snapshot);
      }
    });
    return () => {
      active = false;
      dispose?.();
    };
  }, []);

  // Merge current directory into recent list if not already present
  const displayRecent = useMemo(() => {
    if (!currentDir) {
      return recent;
    }
    const currentInRecent = recent.some((entry) => entry.path === currentDir);
    if (currentInRecent) {
      return recent;
    }
    const currentEntry: LocalRecentWorkspace = {
      path: currentDir,
      lastOpenedAt: Date.now(),
    };
    return [currentEntry, ...recent];
  }, [recent, currentDir]);

  const mainRailItems = useMemo(() => getMainSidebarRailItems(), []);
  const showDesktopUpdatePill = desktopUpdate?.phase === 'ready' || desktopUpdate?.phase === 'installing';
  const desktopUpdateLabel = desktopUpdate?.phase === 'installing' ? 'Updating…' : 'Update';
  const desktopUpdateTitle = desktopUpdate?.targetVersion
    ? `${desktopUpdateLabel} OpenBrain ${desktopUpdate.targetVersion}`
    : desktopUpdateLabel;

  const closeMenu = () => {
    setMenuKind(null);
    setContextMenuPosition(null);
  };

  const openSwitchMenu = () => {
    if (isRemoteWorkspace) {
      onOpenRemoteSwitchDirectory();
      closeMenu();
      return;
    }
    setContextMenuPosition(null);
    setMenuKind((prev) => (prev === 'switch' ? null : 'switch'));
  };

  const openAddMenu = () => {
    if (!currentDir) {
      return;
    }
    setContextMenuPosition(null);
    setMenuKind((prev) => (prev === 'add' ? null : 'add'));
  };

  const closeAddAgentPopup = () => {
    setAddAgentPopupAnchor(null);
    setAddAgentPopupTargetDir(null);
  };

  const getAddTriggerAnchor = (): AddAgentPopupAnchor | null => {
    const rect = addTriggerRef.current?.getBoundingClientRect();
    if (!rect) {
      return null;
    }
    return {
      kind: 'rect',
      rect: {
        left: rect.left,
        top: rect.top,
        right: rect.right,
        bottom: rect.bottom,
      },
    };
  };

  const openRootAgentPopup = (anchor: AddAgentPopupAnchor) => {
    if (!currentDir || rootHasAgent) {
      return;
    }
    setAddAgentPopupTargetDir(currentDir);
    setAddAgentPopupAnchor(anchor);
  };

  const handleSelectRecent = async (entry: LocalRecentWorkspace) => {
    await onSwitchLocal(entry.path);
    await recordLocalRecent({ path: entry.path });
    closeMenu();
  };

  const handleSwitchDirectory = async () => {
    if (isRemoteWorkspace) {
      onOpenRemoteSwitchDirectory();
      closeMenu();
      return;
    }
    await onOpenLocalSwitchDirectory();
    closeMenu();
  };

  const handleWorkspaceClick = () => {
    setView('workspace');
    closeMenu();
    invalidateAgentScanCache();
    if (currentDir) {
      void fetchDirAgentsInfo(currentDir);
    }
  };

  const handleOpenBrainWorkspace = async (workspace: LocalOpenBrainWorkspace) => {
    if (workspace.locationKind === 'remote' && workspace.remoteHost) {
      await onOpenRemoteNewTab(workspace.remoteHost, workspace.path);
    } else if (workspace.path) {
      await onOpenLocalNewTab(workspace.path);
    }
    setView('workspace');
    closeMenu();
  };

  const handleBuiltinViewClick = (nextView: SidebarRailItemKey) => {
    if (nextView === 'workspace') {
      handleWorkspaceClick();
      return;
    }
    setView(nextView);
    closeMenu();
  };

  const handleOverflowRailItemClick = (itemKey: SidebarOverflowView) => {
    if (itemKey === 'marketplace') {
      setView('workspace');
      openMarketplaceTab();
      closeMenu();
      return;
    }
    setView(itemKey);
    closeMenu();
  };

  const handleDesktopUpdateInstall = async (event: React.MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    event.stopPropagation();
    if (desktopUpdate?.phase === 'installing') {
      return;
    }
    try {
      const result = await window.electronAPI?.desktopUpdate?.install?.();
      if (result && !result.success) {
        pushToast(result.error || '安装更新失败');
      }
    } catch (error) {
      pushToast(error instanceof Error ? error.message : '安装更新失败');
    }
  };

  const handleContextMenu = (e: React.MouseEvent) => {
    e.preventDefault();
    if (isRemoteWorkspace) {
      onOpenRemoteSwitchDirectory();
      closeMenu();
      return;
    }
    const menuHeight = getSwitchMenuHeight(displayRecent.length);
    // 光标菜单：从触发点右侧偏移少量像素后展开（弹窗设计 §3）
    const x = Math.min(
      Math.max(8, e.clientX + 8),
      window.innerWidth - FOLDER_MENU_WIDTH - 8
    );
    const y = Math.min(
      Math.max(8, e.clientY),
      window.innerHeight - menuHeight - 8
    );
    setContextMenuPosition({ x, y });
    setMenuKind('switch');
  };

  const handleRootAddContextMenu = (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (!currentDir) {
      return;
    }
    const menuHeight = getAddMenuHeight(rootHasAgent);
    const x = Math.min(
      Math.max(8, e.clientX + 8),
      window.innerWidth - ROOT_ADD_MENU_WIDTH - 8
    );
    const y = Math.min(
      Math.max(8, e.clientY),
      window.innerHeight - menuHeight - 8
    );
    setContextMenuPosition({ x, y });
    setMenuKind('add');
  };

  // Close menu when clicking outside (Switch panel or right-click menu)
  useEffect(() => {
    if (!menuKind) return;
    const onMouseDown = (e: MouseEvent) => {
      if (menuRef.current?.contains(e.target as Node)) return;
      closeMenu();
    };
    window.addEventListener('mousedown', onMouseDown, true);
    return () => window.removeEventListener('mousedown', onMouseDown, true);
  }, [menuKind]);

  const dropdownContent = (
    <PopupMenu
      className="w-full min-w-0"
      onContextMenu={(e) => e.preventDefault()}
    >
      <PopupMenuItem onClick={handleSwitchDirectory}>
        <SwitchDirectoryIcon className="w-4 h-4 opacity-70" />
        <span>Switch Directory…</span>
      </PopupMenuItem>
      {isRemoteWorkspace ? null : (
        <>
          <PopupMenuSeparator />
          <div className="px-2 py-1 text-xs uppercase tracking-wider text-secondary-text">
            Recent
          </div>
          {displayRecent.length === 0 ? (
            <div className="px-2 py-1.5 text-xs text-secondary-text">No recent workspaces</div>
          ) : (
            <div className="max-h-64 overflow-auto">
              {displayRecent.map((entry) => (
                <PopupMenuItem
                  key={entry.path}
                  onClick={() => handleSelectRecent(entry)}
                >
                  <div className="flex flex-col items-stretch gap-0.5 min-w-0">
                    <span className="text-sm truncate">{getWorkspaceLabel(entry)}</span>
                    <span className="text-xs text-tertiary-text truncate">{entry.path}</span>
                  </div>
                </PopupMenuItem>
              ))}
            </div>
          )}
        </>
      )}
    </PopupMenu>
  );

  const addMenuContent = (
    <PopupMenu className="w-[220px]" onContextMenu={(e) => e.preventDefault()}>
      <PopupMenuItem
        disabled={!canCreateAtRoot}
        onClick={() => {
          if (!currentDir) {
            return;
          }
          closeMenu();
          setRequestRootAction('new-file');
        }}
      >
        New File
      </PopupMenuItem>
      <PopupMenuItem
        disabled={!canCreateAtRoot}
        onClick={() => {
          if (!currentDir) {
            return;
          }
          closeMenu();
          setRequestRootAction('new-folder');
        }}
      >
        New Folder
      </PopupMenuItem>
      {!rootHasAgent ? (
        <>
          <PopupMenuSeparator />
          <PopupMenuItem
            onClick={(event) => {
              if (!currentDir) {
                return;
              }
              const anchor: AddAgentPopupAnchor = contextMenuPosition
                ? { kind: 'point', x: contextMenuPosition.x, y: contextMenuPosition.y }
                : getAddTriggerAnchor() ?? { kind: 'point', x: event.clientX, y: event.clientY };
              closeMenu();
              openRootAgentPopup(anchor);
            }}
          >
            Add Agent
          </PopupMenuItem>
          <PopupMenuItem
            onClick={() => {
              if (!currentDir) {
                return;
              }
              closeMenu();
              void addCustomAgent(currentDir);
            }}
          >
            Add Custom Agent
          </PopupMenuItem>
        </>
      ) : null}
    </PopupMenu>
  );

  const workspaceToolbarActions = (
    <>
      {showDesktopUpdatePill ? (
        <button
          type="button"
          className="sidebar-update-pill"
          disabled={desktopUpdate?.phase === 'installing'}
          onClick={handleDesktopUpdateInstall}
          title={desktopUpdateTitle}
        >
          {desktopUpdateLabel}
        </button>
      ) : null}
      <IconButton
        onClick={(event) => {
          event.stopPropagation();
          void refreshVisibleWorkspaceTree();
        }}
        title="Refresh Directory"
      >
        <RefreshIcon className="w-3.5 h-3.5" />
      </IconButton>
      <div
        ref={switchTriggerRef}
        className="relative"
        onClick={(event) => event.stopPropagation()}
      >
        <IconButton
          onClick={(event) => {
            event.stopPropagation();
            openSwitchMenu();
          }}
          title="Switch Directory"
        >
          <SwitchDirectoryIcon className="w-3.5 h-3.5" />
        </IconButton>
        {menuKind === 'switch' && !contextMenuPosition && !isRemoteWorkspace && (() => {
          const pos = getFixedMenuPosition(switchTriggerRef, FOLDER_MENU_WIDTH, getSwitchMenuHeight(displayRecent.length));
          if (!pos) return null;
          return createPortal(
            <div
              ref={menuRef}
              style={{ position: 'fixed', left: pos.left, top: pos.top, width: FOLDER_MENU_WIDTH, zIndex: 9999 }}
              onClick={(event) => event.stopPropagation()}
            >
              {dropdownContent}
            </div>,
            document.body
          );
        })()}
      </div>
      <div ref={addTriggerRef} className="relative" onClick={(event) => event.stopPropagation()}>
        <IconButton
          onClick={openAddMenu}
          title="Add"
        >
          <PlusIcon className="w-3.5 h-3.5" />
        </IconButton>
        {menuKind === 'add' && !contextMenuPosition ? (() => {
          const pos = getFixedMenuPosition(addTriggerRef, ROOT_ADD_MENU_WIDTH, getAddMenuHeight(rootHasAgent));
          if (!pos) return null;
          return createPortal(
            <div
              ref={menuRef}
              style={{ position: 'fixed', left: pos.left, top: pos.top, width: ROOT_ADD_MENU_WIDTH, zIndex: 9999 }}
              onClick={(event) => event.stopPropagation()}
            >
              {addMenuContent}
            </div>,
            document.body
          );
        })() : null}
      </div>
      <IconButton
        onClick={(event) => {
          event.stopPropagation();
          setView('search');
          closeMenu();
        }}
        title="Search"
      >
        <SearchIcon className="w-3.5 h-3.5" />
      </IconButton>
    </>
  );

  return (
    <div
      className="sidebar-hover-area flex h-full min-w-0"
      onMouseLeave={() => {
        useFileTreeSelectionStore.getState().clearAllSelections();
      }}
    >
      <div className="sidebar-activity-rail flex w-[52px] shrink-0 flex-col items-center">
        <div className="sidebar-activity-rail-items flex flex-col items-center gap-1">
          {mainRailItems.map((item) => {
            const active = isMainSidebarRailItemActive(view, item.key);
            return (
              <IconButton
                key={item.key}
                size={34}
                className={`sidebar-activity-button${active ? ' is-active' : ''}`}
                data-sidebar-rail-item={item.key}
                aria-label={item.label}
                aria-current={active ? 'page' : undefined}
                title={item.label}
                onClick={() => handleBuiltinViewClick(item.key)}
              >
                <SidebarRailIcon itemKey={item.key} />
                <SidebarRailAlertBadge count={sidebarRailAlertCount(railAlerts, item.key)} />
                {item.key === 'messenger' && pendingRequestCount > 0 ? (
                  <span className="sidebar-messenger-badge">{formatMessengerPendingBadgeCount(pendingRequestCount)}</span>
                ) : null}
              </IconButton>
            );
          })}
          <IconButton
            size={34}
            className={`sidebar-activity-button sidebar-more-toggle${isSidebarMoreRailActive(view, moreMenuOpen) ? ' is-active' : ''}${moreMenuOpen ? ' is-expanded' : ''}`}
            data-sidebar-rail-more
            aria-expanded={moreMenuOpen}
            onClick={(event) => {
              event.stopPropagation();
              setMoreMenuOpen((open) => !open);
            }}
            title={moreMenuOpen ? 'Collapse' : 'Expand'}
            aria-label={moreMenuOpen ? 'Collapse' : 'Expand'}
          >
            <ChevronDownIcon className="w-5 h-5 sidebar-more-chevron" />
          </IconButton>
          {moreMenuOpen ? (
            <>
              {OVERFLOW_RAIL_ITEMS.map((item) => {
                const active = view === item.key;
                return (
                  <IconButton
                    key={item.key}
                    size={34}
                    className={`sidebar-activity-button${active ? ' is-active' : ''}`}
                    data-sidebar-rail-item={item.key}
                    aria-label={item.label}
                    aria-current={active ? 'page' : undefined}
                    title={item.label}
                    onClick={() => handleOverflowRailItemClick(item.key)}
                  >
                    <SidebarOverflowRailIcon itemKey={item.key} />
                  </IconButton>
                );
              })}
            </>
          ) : null}
        </div>
      </div>

      <div className="flex min-w-0 flex-1 flex-col">
        {view === 'workspace' && (
          <div
            className="ui-tabbar sidebar-root-header flex shrink-0 items-center gap-1 overflow-hidden px-2 text-secondary-text"
            title={currentDir || undefined}
            onContextMenu={handleRootAddContextMenu}
          >
            <div className="flex min-w-0 flex-1 items-center">
              <span className="ui-chrome-row-label truncate">{currentDirLabel}</span>
            </div>
            <div className="sidebar-root-header-actions ml-auto flex shrink-0 items-center gap-0.5">
              {workspaceToolbarActions}
            </div>
          </div>
        )}

        {menuKind === 'switch' && contextMenuPosition && !isRemoteWorkspace && createPortal(
          <div
            ref={menuRef}
            className="w-[260px] z-50"
            style={{
              position: 'fixed',
              left: contextMenuPosition.x,
              top: contextMenuPosition.y,
              width: FOLDER_MENU_WIDTH,
              zIndex: 9999,
            }}
          >
            {dropdownContent}
          </div>,
          document.body
        )}

        {menuKind === 'add' && contextMenuPosition && createPortal(
          <div
            ref={menuRef}
            className="z-50"
            style={{
              position: 'fixed',
              left: contextMenuPosition.x,
              top: contextMenuPosition.y,
              width: ROOT_ADD_MENU_WIDTH,
              zIndex: 9999,
            }}
          >
            {addMenuContent}
          </div>,
          document.body
        )}

        <AddAgentPopup
          open={!!addAgentPopupAnchor}
          anchor={addAgentPopupAnchor}
          onClose={closeAddAgentPopup}
          onSelect={async (agentID) => {
            const targetDir = addAgentPopupTargetDir;
            closeAddAgentPopup();
            if (!targetDir) {
              return;
            }
            await addAgentReference(targetDir, agentID);
          }}
          onCustomAgent={addAgentPopupTargetDir ? () => {
            void addCustomAgent(addAgentPopupTargetDir);
          } : undefined}
        />

        <div className="flex-1 overflow-hidden">
          {view === 'workspace'
            ? <WorkspaceSidebar />
            : view === 'agents'
              ? <AgentsSidebar />
              : view === 'skills'
                ? <SkillsSidebar />
                : view === 'openbrain'
                  ? <OpenBrainSidebar onOpenWorkspace={handleOpenBrainWorkspace} onCreateSource={onCreateOpenBrainSource} onBindSource={onBindOpenBrainSource} />
                  : view === 'messenger'
                    ? <MessengerSidebar />
                    : view === 'search'
                      ? <SearchSidebar />
                      : view === 'cron'
                        ? <CronSidebar />
                        : <ToolsSidebar />}
        </div>
      </div>
    </div>
  );
}
