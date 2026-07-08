import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Sidebar } from './components/Sidebar/Sidebar';
import { MarkdownEditor } from './components/Editor/MarkdownEditor';
import { TextEditor } from './components/Editor/TextEditor';
import { ImageEditor } from './components/Editor/ImageEditor';
import { BookReaderEditor } from './components/Editor/BookReaderEditor';
import { EditorTabBar } from './components/Editor/EditorTabBar';
import { WelcomeEditor } from './components/Editor/WelcomeEditor';
import { DashboardEditor } from './components/Editor/DashboardEditor';
import { MarketplaceEditor } from './components/Editor/MarketplaceEditor';
import { CronTaskEditor } from './components/Editor/CronTaskEditor';
import { OpenBrainPage } from './components/OpenBrain/OpenBrainPage';
import { TitlebarLogoMenu } from './components/TitlebarLogoMenu';
import { WorkspaceTabsBar } from './components/WorkspaceTabsBar';
import {
  getConnectionStateText,
  getDisplayConnectionState,
  getWorkspaceStore,
  removeWorkspaceStore,
  setWorkspaceActive,
  useAppStore,
} from './store/appStore';
import { RemoteConnectModal } from './components/RemoteConnect/RemoteConnectModal';
import { DirectoryPickerDialog } from './components/FileDialog/DirectoryPickerDialog';
import {
  resolveSaveFileDialog,
  useSaveFileDialogRequest,
} from './components/FileDialog/saveFileDialogBridge';
import {
  createLocalDirectoryPickerProvider,
  createRemoteDirectoryPickerProvider,
} from './components/FileDialog/directoryPickerProviders';
import { ModelsEditor } from './components/Settings/ModelsEditor';
import { OpenBrainSettingsEditor } from './components/Settings/OpenBrainSettingsEditor';
import { DesktopSettingsEditor } from './components/Settings/DesktopSettingsEditor';
import { usePreventSleepWhileAgentRunning } from './hooks/usePreventSleepWhileAgentRunning';
import { getChatWorkspaceStore, removeChatWorkspaceStore, useChatWorkspaceStore } from './store/chatWorkspaceStore';
import { useUiStore } from './store/uiStore';
import { ActivityPanel } from './components/Chat/ActivityPanel';
import { ConversationComposerDock } from './components/Chat/ConversationComposerDock';
import { useSelectedThreadSnapshotSync } from './components/Chat/useSelectedThreadSnapshotSync';
import { disposeChatWorkspaceRuntime } from './services/chatService';
import { getResolvedThreadMeta, primeLocalThreadMeta } from './services/threadService';
import { DeviceCodeDialog } from './components/Auth/DeviceCodeDialog';
import { LoginRequiredDialog } from './components/Auth/LoginRequiredDialog';
import { editorRegistry } from './services/editorRegistry';
import {
  useTabManagerStore,
  resolveHostLabel,
  type SshHost,
  type WorkspaceChatSession,
  type WorkspaceTabSession,
  type WorkspaceTabsSessionState,
} from './store/tabManagerStore';
import { getRemoteBucketByInstanceID, useRecentWorkspacesStore } from './store/recentWorkspacesStore';
import { useAuthStore } from './store/authStore';
import { ResizeDivider } from './components/ResizeDivider';
import { NewWindowLanding } from './components/Window/NewWindowLanding';
import { ToastViewport } from './components/ToastViewport';
import { BranchStatusControl } from './components/BranchStatusControl';
import { WorkspaceSyncStatusControl } from './components/WorkspaceSyncStatusControl';
import { WindowZoomStatusControl } from './components/WindowZoomStatusControl';
import { EditorTextZoomStatusControl } from './components/EditorTextZoomStatusControl';
import { UnsavedTabCloseDialog } from './components/UnsavedTabCloseDialog';
import { BillingReminderDialog } from './components/Billing/BillingReminderDialog';
import { LocalRuntimeBootstrapOverlay } from './components/Runtime/LocalRuntimeBootstrapOverlay';
import { WorkspaceAgentOnboarding } from './components/Onboarding/WorkspaceAgentOnboarding';
import { shouldSyncConversationSelectionWithActiveChat } from './utils/chatSelectionSync';
import { formatStatusBarPathDisplay } from './utils/statusBarPath';
import { normalizePosixPath } from './utils/markdownMedia';
import {
  closeWorkspaceTabWithDefaultFallback,
  resolveDefaultLocalWorkspacePath,
} from './utils/workspaceClose';
import { useBillingReminderStore } from './store/billingReminderStore';
import { showLoginRequiredDialog, useLoginRequiredStore } from './store/loginRequiredStore';
import { openBrainRuntimeConnectionForWorkspace, useOpenBrainStore, type LocalOpenBrainWorkspace } from './store/openBrainStore';
import { useToastStore } from './store/toastStore';
import type { RuntimeBootstrapState } from './types/electron';

// Keep the sidebar wide enough so the tab bar never overflows into the resize divider hit area.
const SIDEBAR_MIN_WIDTH = 200;
const SIDEBAR_MAX_WIDTH = 800;
const DEFAULT_SIDEBAR_WIDTH = 300;
const RESIZE_DIVIDER_HOVER_DELAY_MS = 80;
const CONVERSATION_COMPOSER_DOCK_MIN_HEIGHT = 120;
const CONVERSATION_COMPOSER_DOCK_LAYOUT_SAFETY_GAP = 220;
const DEFAULT_CONVERSATION_COMPOSER_DOCK_HEIGHT = 160;
const PINNED_FILE_PANEL_MIN_WIDTH = 320;
const PINNED_FILE_PANEL_MAX_WIDTH = 1200;
const DEFAULT_PINNED_FILE_PANEL_WIDTH = 420;
const PINNED_FILE_PANEL_PRIMARY_MIN_WIDTH = 260;
const ACTIVITY_PANEL_MIN_WIDTH = 320;
const ACTIVITY_PANEL_MAX_WIDTH = 4000;
const ACTIVITY_PANEL_MIN_HEIGHT = 80;
const ACTIVITY_PANEL_MAX_HEIGHT = 2000;
const ACTIVITY_PANEL_LAYOUT_SAFETY_GAP = 12;
const ACTIVITY_PANEL_HORIZONTAL_EDGE_GAP = 12;
const ACTIVITY_PANEL_HORIZONTAL_DRAG_THRESHOLD = 6;
const DEFAULT_ACTIVITY_PANEL_WIDTH_RATIO = 0.85;
const DEFAULT_ACTIVITY_PANEL_MAX_HEIGHT = 400;
// Collapsed ActivityPanel behaves like a floating status pill, so keep the
// latest markdown output visually clear of the header. Expanded panels are
// allowed to overlay editor content and therefore should not reserve space.
const EDITOR_CONTENT_BOTTOM_CLEARANCE_PX = Math.ceil(14 * 1.6 * 2);
const ACTIVITY_PANEL_EDITOR_TEXT_CLEARANCE = EDITOR_CONTENT_BOTTOM_CLEARANCE_PX;
// Host padding-top (14px) + collapsed header + outer pb-3 (12px) — fallback when measure is stale/0.
const ACTIVITY_PANEL_COLLAPSED_STACK_MIN_HEIGHT = 62;

type DirectoryPickerRequest =
  | {
      source: 'switch-local-workspace';
      currentPath: string | null;
      defaultPath: string | null;
    }
  | {
      source: 'switch-remote-workspace';
      tabId: string;
      currentPath: string | null;
    }
  | {
      source: 'landing-open-folder';
      defaultPath: string | null;
    }
  | {
      source: 'new-local-workspace-tab';
      defaultPath: string | null;
    }
  | {
      source: 'create-openbrain-source';
      provider: 'cloud' | 'local';
      locationKind: 'local' | 'remote';
      sourceWorkspace?: LocalOpenBrainWorkspace;
      tabId?: string;
      currentPath?: string | null;
      defaultPath: string | null;
    };

function clampSidebarWidth(width: number) {
  return Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, width));
}

function resolveSidebarWidth(value: unknown) {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return DEFAULT_SIDEBAR_WIDTH;
  }
  return clampSidebarWidth(value);
}

function resolveActivityPanelMaxHeight(value: unknown) {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return DEFAULT_ACTIVITY_PANEL_MAX_HEIGHT;
  }
  return Math.min(ACTIVITY_PANEL_MAX_HEIGHT, Math.max(ACTIVITY_PANEL_MIN_HEIGHT, value));
}

function resolveActivityPanelWidth(value: unknown) {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return null;
  }
  return Math.min(ACTIVITY_PANEL_MAX_WIDTH, Math.max(ACTIVITY_PANEL_MIN_WIDTH, value));
}

function resolveConversationComposerDockHeight(value: unknown, maxHeightLimit: number) {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return Math.min(DEFAULT_CONVERSATION_COMPOSER_DOCK_HEIGHT, maxHeightLimit);
  }
  return Math.min(maxHeightLimit, Math.max(CONVERSATION_COMPOSER_DOCK_MIN_HEIGHT, value));
}

function normalizeWorkspacePathKey(value: string | null | undefined) {
  const normalized = (value || '').trim().replace(/\\/g, '/');
  if (!normalized) {
    return '';
  }
  return normalized.length > 1 ? normalized.replace(/\/+$/, '') : normalized;
}

function getLocalPathBaseName(value: string | null | undefined) {
  const normalized = normalizeWorkspacePathKey(value);
  if (!normalized || normalized === '/') {
    return 'workspace';
  }
  const parts = normalized.split('/').filter(Boolean);
  return parts[parts.length - 1] || 'workspace';
}

function normalizePinnedFilePanelWidth(value: unknown) {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return DEFAULT_PINNED_FILE_PANEL_WIDTH;
  }
  return Math.min(
    PINNED_FILE_PANEL_MAX_WIDTH,
    Math.max(PINNED_FILE_PANEL_MIN_WIDTH, value)
  );
}

type UiSettingsSnapshot = {
  ui?: {
    sidebarWidth?: unknown;
    activityPanelWidth?: unknown;
    activityPanelMaxHeight?: unknown;
    conversationComposerDockHeight?: unknown;
    pinnedFilePanelWidth?: unknown;
  };
};

function MessengerConversationSurface({
  selected,
  loading,
}: {
  selected: boolean;
  loading: boolean;
}) {
  const title = selected
    ? (loading ? 'Loading conversation...' : 'No activity yet')
    : 'No message selected';
  const subtitle = selected
    ? 'Activity will appear here.'
    : 'Agent messages will appear here.';

  return (
    <div className="absolute inset-0 flex items-center justify-center bg-editor-bg text-center">
      <div className="max-w-xs px-6">
        <div className="text-sm font-medium text-prime-text">{title}</div>
        <div className="mt-1 text-xs text-tertiary-text">{subtitle}</div>
      </div>
    </div>
  );
}

async function runWithConcurrency(tasks: Array<() => Promise<void>>, limit: number) {
  let index = 0;
  const workers = Array.from({ length: Math.max(1, limit) }, async () => {
    while (index < tasks.length) {
      const taskIndex = index;
      index += 1;
      await tasks[taskIndex]();
    }
  });
  await Promise.all(workers);
}

function getChatTabSignature(
  tabs: Array<{ filePath?: string; title: string }>,
): string {
  return tabs
    .map((tab) => `${tab.filePath}::${tab.title}`)
    .join('|');
}

function isPlanDocumentPath(path: string | null | undefined): boolean {
  return normalizePosixPath((path || '').trim()).includes('/.agent/context/');
}

function normalizeChatSessionPath(path: string | null | undefined): string {
  return (path || '').trim();
}

function getWorkspaceChatSessionSnapshot(tabId: string): WorkspaceChatSession | undefined {
  const workspaceState = getWorkspaceStore(tabId).getState();
  const chatState = getChatWorkspaceStore(tabId).getState();
  const openChats = workspaceState.documents
    .filter((tab): tab is typeof tab & { filePath: string } => Boolean(
      tab.filePath && tab.documentRole === 'conversation'
    ))
    .map((tab) => {
      const meta = chatState.getThreadMeta(tab.filePath);
      const threadID = (meta?.threadID || '').trim();
      if (!threadID) {
        return null;
      }
      return {
        threadID,
        path: tab.filePath,
        title: tab.title,
      };
    })
    .filter((entry): entry is NonNullable<typeof entry> => Boolean(entry));

  if (openChats.length === 0) {
    return undefined;
  }

  const selectedTarget = getChatWorkspaceStore(tabId).getState().selectedConversationTarget;
  const activeEditorChatPath = normalizeChatSessionPath(workspaceState.currentFilePath);
  const selectedChatPath = (
    activeEditorChatPath
    && openChats.some((entry) => entry.path === activeEditorChatPath)
  )
    ? activeEditorChatPath
    : selectedTarget?.kind === 'thread'
    && openChats.some((entry) => entry.threadID === selectedTarget.threadID)
      ? openChats.find((entry) => entry.threadID === selectedTarget.threadID)?.path || ''
      : '';

  const selectedThreadID = (selectedChatPath
    ? chatState.getThreadMeta(selectedChatPath)?.threadID || ''
    : '').trim();

  return {
    openChats,
    ...(selectedThreadID ? { selectedThreadID } : {}),
  };
}

function getWorkspaceOpenEditorFilePaths(tabId: string): string[] {
  const workspaceState = getWorkspaceStore(tabId).getState();
  return Array.from(new Set(
    workspaceState.documents
      .map((tab) => normalizePosixPath((tab.filePath || '').trim()))
      .filter((path) => Boolean(
        path && isPlanDocumentPath(path)
      ))
  ));
}

function isPinnableFileTab(tab: { filePath?: string } | null | undefined): boolean {
  return Boolean((tab?.filePath || '').trim());
}

async function waitForWorkspaceConnection(tabId: string, timeoutMs = 15000): Promise<boolean> {
  const store = getWorkspaceStore(tabId);
  if (store.getState().connectionState === 'connected') {
    return true;
  }

  const startedAt = Date.now();
  while (Date.now() - startedAt < timeoutMs) {
    await new Promise((resolve) => window.setTimeout(resolve, 50));
    if (store.getState().connectionState === 'connected') {
      return true;
    }
  }

  return store.getState().connectionState === 'connected';
}

let builtInEditorsRegistered = false;

function registerBuiltInEditors() {
  if (builtInEditorsRegistered) {
    return;
  }
  builtInEditorsRegistered = true;

  editorRegistry.register({
    id: 'markdown',
    displayName: 'Markdown Editor',
    component: MarkdownEditor,
  });
  editorRegistry.register({
    id: 'text',
    displayName: 'Text Editor',
    component: TextEditor,
  });
  editorRegistry.register({
    id: 'image',
    displayName: 'Image Preview',
    component: ImageEditor,
  });
  editorRegistry.register({
    id: 'pdf',
    displayName: 'Book Reader',
    component: BookReaderEditor,
  });
  editorRegistry.register({
    id: 'book',
    displayName: 'Book Reader',
    component: BookReaderEditor,
  });
  editorRegistry.register({
    id: 'welcome',
    displayName: 'Welcome',
    component: WelcomeEditor,
  });
  editorRegistry.register({
    id: 'dashboard',
    displayName: 'Dashboard',
    component: DashboardEditor,
  });
  editorRegistry.register({
    id: 'marketplace',
    displayName: 'Marketplace',
    component: MarketplaceEditor,
  });
  editorRegistry.register({
    id: 'cron-task',
    displayName: 'Cron Task',
    component: CronTaskEditor,
  });
  editorRegistry.register({
    id: 'models',
    displayName: 'Models',
    component: ModelsEditor,
  });
  editorRegistry.register({
    id: 'openbrain-settings',
    displayName: 'OpenBrain',
    component: OpenBrainSettingsEditor,
  });
  editorRegistry.register({
    id: 'desktop-settings',
    displayName: 'Desktop',
    component: DesktopSettingsEditor,
  });
}

export default function App() {
  const { t } = useTranslation(['menu', 'shell', 'common', 'error']);
  const { 
    currentDir,
    setCurrentDir,
    ensureDirectory,
    connectionState,
    connect,
    setRemoteSession,
    remoteSession,
    remoteError,
    baseDir,
    agentsRootDir,
    openWelcomeTab,
  } = useAppStore();
  usePreventSleepWhileAgentRunning();
  const displayConnectionState = useAppStore(getDisplayConnectionState);
  const pendingDirtyTabClose = useAppStore((state) => state.pendingDirtyTabClose);
  const dismissPendingDirtyTabClose = useAppStore((state) => state.dismissPendingDirtyTabClose);
  const confirmPendingDirtyTabClose = useAppStore((state) => state.confirmPendingDirtyTabClose);
  const activeTabId = useTabManagerStore((state) => state.activeTabId);
  const workspaceTabs = useTabManagerStore((state) => state.tabs);
  const setActiveWorkspaceTab = useTabManagerStore((state) => state.setActiveTab);
  const createWorkspaceTab = useTabManagerStore((state) => state.createTab);
  const closeWorkspaceTab = useTabManagerStore((state) => state.closeTab);
  const updateTabWorkspace = useTabManagerStore((state) => state.updateTabWorkspace);
  const updateActiveTabWorkspace = useTabManagerStore((state) => state.updateActiveTabWorkspace);
  const sidebarView = useUiStore((state) => state.sidebarView);
  const composerVisible = useChatWorkspaceStore((state) => state.composerVisible);
  const selectedConversationTarget = useChatWorkspaceStore((state) => state.selectedConversationTarget);
  const selectedSkill = useChatWorkspaceStore((state) => state.selectedSkill);
  const chatAgentID = useChatWorkspaceStore((state) => state.agentID);
  const chatAgentName = useChatWorkspaceStore((state) => state.agentName);
  const chatAgentCwd = useChatWorkspaceStore((state) => state.agentCwd);
  const selectedThreadMeta = useChatWorkspaceStore((state) => state.getThreadMeta(
    state.getTargetChatPath(state.selectedConversationTarget)
  ));
  const liveOverlay = useChatWorkspaceStore((state) => state.getLiveOverlayForTarget(state.selectedConversationTarget));
  const threadSnapshot = useChatWorkspaceStore((state) => state.getThreadSnapshotForTarget(state.selectedConversationTarget));
  const selectedAwaitingUser = useChatWorkspaceStore((state) => (
    state.selectedConversationTarget?.kind === 'thread'
      ? state.getAwaitingUser(state.getTargetChatPath(state.selectedConversationTarget))
      : null
  ));
  const currentThreadMeta = useChatWorkspaceStore((state) => state.getThreadMeta(
    state.getTargetChatPath(state.selectedConversationTarget)
  ));
  const workspaceDocuments = useAppStore((state) => state.documents);
  const workspaceConversationDocs = useMemo(
    () => workspaceDocuments.filter((tab): tab is typeof tab & { filePath: string } => (
      tab.documentRole === 'conversation' && Boolean((tab.filePath || '').trim())
    )),
    [workspaceDocuments],
  );
  const currentFilePath = useAppStore((state) => state.currentFilePath);
  const editorId = useAppStore((state) => state.editorId);
  const activeEditorTabId = useAppStore((state) => state.activeTabId);
  const pinnedTabId = useAppStore((state) => state.pinnedTabId);
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const activateLastNonConversationTab = useAppStore((state) => state.activateLastNonConversationTab);
  const togglePinnedTab = useAppStore((state) => state.togglePinnedTab);
  const clearPinnedTab = useAppStore((state) => state.clearPinnedTab);
  const activeDocument = useMemo(
    () => workspaceDocuments.find((tab) => tab.id === activeEditorTabId) || null,
    [activeEditorTabId, workspaceDocuments],
  );
  const activeStatusFilePath = (activeDocument?.filePath || currentFilePath || '').trim();
  const statusBarPathDisplay = useMemo(
    () => formatStatusBarPathDisplay(currentDir, activeStatusFilePath),
    [activeStatusFilePath, currentDir],
  );
  const pinnedTab = pinnedTabId
    ? workspaceDocuments.find((tab) => tab.id === pinnedTabId) || null
    : null;
  const pendingConversationSelected = selectedConversationTarget?.kind === 'pending';
  const showPendingConversationPlaceholder = Boolean(
    pendingConversationSelected
    && activeDocument?.documentRole === 'conversation'
    && (!pinnedTab || activeDocument.id !== pinnedTab.id)
  );
  const primaryEditorTab = showPendingConversationPlaceholder
    ? null
    : pinnedTab
      && activeDocument
      && activeDocument.id === pinnedTab.id
      ? null
      : activeDocument;
  const showPinnedFilePane = Boolean(pinnedTab?.filePath);
  const recentWorkspaces = useRecentWorkspacesStore((state) => state.recent);
  const loadRecentWorkspaces = useRecentWorkspacesStore((state) => state.load);
  const [windowActive, setWindowActive] = useState(true);
  const [currentWindowId, setCurrentWindowId] = useState<number | null>(null);
  const [showRemoteModal, setShowRemoteModal] = useState(false);
  const [directoryPickerRequest, setDirectoryPickerRequest] = useState<DirectoryPickerRequest | null>(null);
  const [runtimeBootstrapState, setRuntimeBootstrapState] = useState<RuntimeBootstrapState | null>(null);
  const [remoteModalTarget, setRemoteModalTarget] = useState<'current' | 'newTab' | 'newWindow'>('current');
  const [isNewWindowLanding, setIsNewWindowLanding] = useState(false);
  const [loginGatedStartup, setLoginGatedStartup] = useState(false);
  const [openingLandingFolder, setOpeningLandingFolder] = useState(false);
  const [appInitialized, setAppInitialized] = useState(false);
  const authLoggedIn = useAuthStore((state) => state.loggedIn);
  const authInit = useAuthStore((state) => state.init);
  const deviceCode = useAuthStore((state) => state.deviceCode);
  const clearDeviceCode = useAuthStore((state) => state.clearDeviceCode);
  const billingReminderOpen = useBillingReminderStore((state) => state.open);
  const billingReminderKind = useBillingReminderStore((state) => state.kind);
  const hideBillingReminder = useBillingReminderStore((state) => state.hide);
  const loginRequiredOpen = useLoginRequiredStore((state) => state.open);
  const loginRequiredReason = useLoginRequiredStore((state) => state.reason);
  const hideLoginRequired = useLoginRequiredStore((state) => state.hide);
  const editorAreaRef = useRef<HTMLDivElement | null>(null);
  const editorContentRowRef = useRef<HTMLDivElement | null>(null);
  const primaryEditorPaneRef = useRef<HTMLDivElement | null>(null);
  const openBrainPagePaneRef = useRef<HTMLDivElement | null>(null);
  const messengerConversationPaneRef = useRef<HTMLDivElement | null>(null);
  const pinnedEditorPaneRef = useRef<HTMLDivElement | null>(null);
  const activityPanelHostRef = useRef<HTMLDivElement | null>(null);
  const activityPanelStackRef = useRef<HTMLDivElement | null>(null);
  const prevActiveTabRef = useRef<string | null>(null);
  const sessionSyncTimerRef = useRef<number | null>(null);
  const sessionSyncReadyRef = useRef(false);
  const activeWorkspaceTab = workspaceTabs.find((tab) => tab.id === activeTabId) || null;
  const sidebarResizeRef = useRef<{ startX: number; startWidth: number; currentWidth: number } | null>(null);
  const conversationComposerDockResizeRef = useRef<{ startY: number; startHeight: number; currentHeight: number } | null>(null);
  const pinnedFilePanelResizeRef = useRef<{ startX: number; startWidth: number; currentWidth: number } | null>(null);
  const conversationComposerDockPreferredHeightRef = useRef(DEFAULT_CONVERSATION_COMPOSER_DOCK_HEIGHT);
  const activityPanelResizeRef = useRef<{
    edge: 'left' | 'right';
    startX: number;
    startY: number;
    startLeft: number;
    startWidth: number;
    startHeight: number;
    startPreferredWidth: number | null;
    startPreferredHeight: number;
    currentLeft: number;
    currentWidth: number;
    currentHeight: number;
  } | null>(null);
  const activityPanelHorizontalDragRef = useRef<{
    startX: number;
    startLeft: number;
    currentLeft: number;
    dragged: boolean;
  } | null>(null);
  const activityPanelSuppressClickRef = useRef(false);
  const isMac = window.electronAPI?.platform === 'darwin';
  const [sidebarWidth, setSidebarWidth] = useState(DEFAULT_SIDEBAR_WIDTH);
  const [conversationComposerDockHeight, setConversationComposerDockHeight] = useState(DEFAULT_CONVERSATION_COMPOSER_DOCK_HEIGHT);
  const [pinnedFilePanelWidth, setPinnedFilePanelWidth] = useState(DEFAULT_PINNED_FILE_PANEL_WIDTH);
  const [activityPanelMaxHeight, setActivityPanelMaxHeight] = useState(DEFAULT_ACTIVITY_PANEL_MAX_HEIGHT);
  const [activityPanelLeft, setActivityPanelLeft] = useState<number | null>(null);
  const [activityPanelWidth, setActivityPanelWidth] = useState<number | null>(null);
  const [uiSettingsHydrated, setUiSettingsHydrated] = useState(false);
  const [activityPanelMeasuredWidth, setActivityPanelMeasuredWidth] = useState(0);
  const [activityPanelCoveredBottom, setActivityPanelCoveredBottom] = useState(0);
  const [activityPanelHorizontalDragging, setActivityPanelHorizontalDragging] = useState(false);
  const [activityPanelResizeEdge, setActivityPanelResizeEdge] = useState<'left' | 'right' | null>(null);
  const hasContextUsage = Boolean(
    liveOverlay.loopUsage.contextWindow
    && (
      liveOverlay.loopUsage.contextKnown === false
      || (liveOverlay.loopUsage.contextTokens || 0) > 0
      || typeof liveOverlay.loopUsage.contextPercent === 'number'
    )
    || threadSnapshot?.contextUsage?.known
    || (threadSnapshot?.contextUsage?.tokens || 0) > 0
  );
  const hasActivityContent = Boolean(
    (threadSnapshot?.entries?.length || 0) > 0
    || (threadSnapshot?.messageRecords?.length || 0) > 0
    || liveOverlay.streamingText
    || liveOverlay.streamingSegments.length > 0
    || liveOverlay.steps.length > 0
    || liveOverlay.errorMessage
    || selectedAwaitingUser
    || hasContextUsage
  );
  const hasPersistentPlanActivity = Boolean(
    currentThreadMeta?.executionPlanPath
    || currentThreadMeta?.planPath
  );
  const chatInProgress = useChatWorkspaceStore((state) => state.isTargetInProgress(state.selectedConversationTarget));
  const showOpenBrainPage = sidebarView === 'openbrain';
  const showMessengerView = sidebarView === 'messenger';
  const showSpecialPage = showOpenBrainPage || showMessengerView;
  const showComposerDock = composerVisible;
  const threadConversationSelected = selectedConversationTarget?.kind === 'thread'
    && (composerVisible || showSpecialPage);
  const activityVisible = Boolean(selectedConversationTarget)
    && (hasActivityContent || chatInProgress || hasPersistentPlanActivity || threadConversationSelected);
  const showActivityPanel = activityVisible;
  const messengerSurfaceLoading = Boolean(
    showMessengerView
    && selectedConversationTarget
    && !threadSnapshot
    && !hasActivityContent
    && !chatInProgress
  );

  useSelectedThreadSnapshotSync();

  const handleSelectWorkspaceTab = (tabId: string) => {
    if (showSpecialPage) {
      useUiStore.getState().setSidebarView('workspace');
    }
    setActiveWorkspaceTab(tabId);
  };

  const pushWorkspaceTabsSession = async () => {
    if (!sessionSyncReadyRef.current || !window.electronAPI?.window?.updateWorkspaceTabsSession) {
      return;
    }
    const baseSnapshot = useTabManagerStore.getState().getSessionSnapshot((tabId) => {
      return getWorkspaceStore(tabId).getState().currentDir;
    });
    const snapshot = {
      ...baseSnapshot,
      tabs: baseSnapshot.tabs.map((tab) => {
        const chatSession = getWorkspaceChatSessionSnapshot(tab.id);
        const openEditorFilePaths = getWorkspaceOpenEditorFilePaths(tab.id);
        return {
          ...tab,
          ...(chatSession ? { chatSession } : {}),
          ...(openEditorFilePaths.length > 0 ? { openEditorFilePaths } : {}),
        };
      }),
    };
    try {
      await window.electronAPI.window.updateWorkspaceTabsSession(snapshot);
    } catch (error) {
      console.warn('[workspaceTabsSession] failed to sync:', error);
    }
  };

  const scheduleWorkspaceTabsSessionSync = () => {
    if (!sessionSyncReadyRef.current) {
      return;
    }
    if (sessionSyncTimerRef.current) {
      window.clearTimeout(sessionSyncTimerRef.current);
    }
    sessionSyncTimerRef.current = window.setTimeout(() => {
      sessionSyncTimerRef.current = null;
      void pushWorkspaceTabsSession();
    }, 150);
  };

  const restoreChatSessionForWorkspaceTab = async (tabId: string, chatSession?: WorkspaceChatSession) => {
    const chatStore = getChatWorkspaceStore(tabId).getState();
    const openChats = chatSession?.openChats || [];
    if (openChats.length === 0) {
      chatStore.selectChatConversation(null);
      chatStore.setComposerVisible(false);
      return;
    }

    const connected = await waitForWorkspaceConnection(tabId);
    if (!connected) {
      chatStore.selectChatConversation(null);
      chatStore.setComposerVisible(false);
      return;
    }

    const resolvedOpenChats: WorkspaceChatSession['openChats'] = [];
    for (const entry of openChats) {
      const threadID = (entry.threadID || '').trim();
      if (!threadID || resolvedOpenChats.some((item) => item.threadID === threadID)) {
        continue;
      }
      try {
        const meta = await getResolvedThreadMeta({ threadID }, tabId);
        resolvedOpenChats.push({
          threadID,
          path: (meta.chatPath || '').trim() || entry.path,
          title: (meta.title || '').trim() || entry.title,
        });
      } catch {
        if ((entry.path || '').trim()) {
          resolvedOpenChats.push({
            threadID,
            path: entry.path,
            title: entry.title,
          });
        }
      }
    }

    const selectedPath = (() => {
      const selectedThreadID = (chatSession?.selectedThreadID || '').trim();
      if (!selectedThreadID) {
        return null;
      }
      return resolvedOpenChats.find((entry) => entry.threadID === selectedThreadID)?.path || null;
    })();

    const result = await getWorkspaceStore(tabId).getState().restoreChatTabsSession(
      resolvedOpenChats,
      selectedPath,
    );

    if (!result.selectedPath || result.restoredPaths.length === 0) {
      chatStore.selectChatConversation(null);
      chatStore.setComposerVisible(false);
      return;
    }

    chatStore.selectChatConversation(result.selectedPath);
    chatStore.setComposerVisible(true);
  };

  const restoreLocalWorkspaceTab = async (tab: WorkspaceTabSession, active: boolean) => {
    const store = getWorkspaceStore(tab.id);
    const state = store.getState();
    const nextDir = tab.currentDir || tab.workspacePath;
    const shouldConnectForChatRestore = Boolean(tab.chatSession?.openChats.length);
    if (nextDir) {
      useTabManagerStore.getState().updateTabWorkspace(tab.id, {
        kind: 'local',
        workspacePath: nextDir,
      });
    }
    if (nextDir) {
      state.setCurrentDir(nextDir);
      if (active || shouldConnectForChatRestore) {
        state.connect();
        await state.ensureDirectory(nextDir);
      }
    } else if (active || shouldConnectForChatRestore) {
      state.connect();
    }
  };

  const restoreRemoteWorkspaceTab = async (tab: WorkspaceTabSession) => {
    if (!tab.remoteHost) {
      return;
    }
    const store = getWorkspaceStore(tab.id);
    store.getState().setServerUrl('');
    store.getState().disconnect();
    await connectRemoteForTab(tab.id, tab.remoteHost);
    const nextDir = tab.currentDir || tab.workspacePath;
    useTabManagerStore.getState().updateTabWorkspace(tab.id, {
      kind: 'remote',
      remoteHost: tab.remoteHost,
      workspacePath: nextDir || tab.workspacePath,
      label: resolveHostLabel(tab.remoteHost),
    });
    if (nextDir) {
      store.getState().setCurrentDir(nextDir);
      await store.getState().ensureDirectory(nextDir);
    }
  };

  const restoreWorkspaceTabsSession = async (session: WorkspaceTabsSessionState) => {
    useTabManagerStore.getState().replaceSession(session);
    const restored = useTabManagerStore.getState();
    const tabsById = new Map(session.tabs.map((tab) => [tab.id, tab]));
    const remoteTasks: Array<() => Promise<void>> = [];

    for (const tab of restored.tabs) {
      const restoredTab = tabsById.get(tab.id);
      if (!restoredTab) {
        continue;
      }
      const isActiveTab = tab.id === restored.activeTabId;
      if (restoredTab.kind === 'remote') {
        const task = async () => {
          await restoreRemoteWorkspaceTab(restoredTab);
          await restoreChatSessionForWorkspaceTab(restoredTab.id, restoredTab.chatSession);
        };
        if (isActiveTab) {
          remoteTasks.unshift(task);
        } else {
          remoteTasks.push(task);
        }
        continue;
      }
      await restoreLocalWorkspaceTab(restoredTab, isActiveTab);
      await restoreChatSessionForWorkspaceTab(restoredTab.id, restoredTab.chatSession);
    }

    await runWithConcurrency(remoteTasks, 2);
  };

  const openSettingsWorkspace = async () => {
    try {
      const settingsDir = (await window.electronAPI?.settings?.getRoot?.() || '').trim();
      if (!settingsDir) {
        return;
      }

      const targetKey = normalizeWorkspacePathKey(settingsDir);
      const tabManager = useTabManagerStore.getState();
      const existingTab = tabManager.tabs.find((tab) => {
        if (tab.kind !== 'local') {
          return false;
        }
        const storeState = getWorkspaceStore(tab.id).getState();
        return normalizeWorkspacePathKey(tab.workspacePath) === targetKey
          || normalizeWorkspacePathKey(storeState.currentDir) === targetKey;
      });

      if (existingTab) {
        updateTabWorkspace(existingTab.id, {
          kind: 'local',
          workspacePath: settingsDir,
          label: t('shell:tab.settings'),
        });
        setActiveWorkspaceTab(existingTab.id);
        const storeState = getWorkspaceStore(existingTab.id).getState();
        if (storeState.connectionState !== 'connected') {
          storeState.connect();
        }
        if (normalizeWorkspacePathKey(storeState.currentDir) !== targetKey) {
          storeState.setCurrentDir(settingsDir);
        }
        await storeState.ensureDirectory(settingsDir);
        if (storeState.documents.length === 0) {
          storeState.openWelcomeTab();
        }
        return;
      }

      const tab = createWorkspaceTab({
        kind: 'local',
        workspacePath: settingsDir,
        label: t('shell:tab.settings'),
      });
      const storeState = getWorkspaceStore(tab.id).getState();
      storeState.connect();
      storeState.setCurrentDir(settingsDir);
      await storeState.ensureDirectory(settingsDir);
      storeState.openWelcomeTab();
    } catch (error) {
      console.error('Failed to open settings workspace:', error);
    }
  };

  async function connectRemoteForTab(tabId: string, host: SshHost) {
    const store = getWorkspaceStore(tabId);
    if (!window.electronAPI?.remote) {
      store.getState().setRemoteError('Remote API not available');
      return;
    }
    store.getState().setRemoteConnecting(true);
    store.getState().setRemoteError(null);
    try {
      const session = await window.electronAPI.remote.connectSsh(host, tabId);
      store.getState().setRemoteSession(session);
      store.getState().connect();
      useTabManagerStore.getState().updateTabWorkspace(tabId, {
        kind: 'remote',
        remoteHost: host,
        label: session.hostLabel,
      });
    } catch (err) {
      store.getState().setRemoteError((err as Error).message || 'Remote connect failed');
    } finally {
      store.getState().setRemoteConnecting(false);
    }
  }

  async function disconnectRemoteForTab(tabId: string) {
    if (!window.electronAPI?.remote) {
      return;
    }
    await window.electronAPI.remote.disconnect(tabId);
    const store = getWorkspaceStore(tabId);
    store.getState().setRemoteSession(null);
    store.getState().connect();
    const defaultDir = await resolveDefaultLocalWorkspacePath();
    useTabManagerStore.getState().updateTabWorkspace(tabId, {
      kind: 'local',
      workspacePath: defaultDir,
    });
    store.getState().setCurrentDir(defaultDir);
  }

  const handleNewFolderTab = async () => {
    const defaultDir = currentDir && !remoteSession
      ? currentDir
      : await resolveDefaultLocalWorkspacePath();
    setDirectoryPickerRequest({
      source: 'new-local-workspace-tab',
      defaultPath: defaultDir,
    });
  };

  const handleNewRemoteTab = () => {
    setRemoteModalTarget('newTab');
    setShowRemoteModal(true);
  };

  const handleCreateOpenBrainSource = async () => {
    const openBrainState = useOpenBrainStore.getState();
    const provider = openBrainState.provider;
    if (provider === 'cloud' && !openBrainState.cloudReady) {
      const result = await window.electronAPI?.workspace?.openStorageBackendSettings?.({ storageBackend: 'github' });
      if (!result?.success) {
        useToastStore.getState().pushToast(
          result?.error || t('menu:openBrainOnboarding.connectGitHubFailed'),
          { durationMs: 7000, anchor: null },
        );
      }
      return;
    }
    if (remoteSession) {
      const store = getWorkspaceStore(activeTabId);
      const defaultDir = store.getState().currentDir || remoteSession.workspaceDir || '/';
      setDirectoryPickerRequest({
        source: 'create-openbrain-source',
        provider,
        locationKind: 'remote',
        tabId: activeTabId,
        currentPath: defaultDir,
        defaultPath: defaultDir,
      });
      return;
    }
    const defaultDir = currentDir || await resolveDefaultLocalWorkspacePath();
    setDirectoryPickerRequest({
      source: 'create-openbrain-source',
      provider,
      locationKind: 'local',
      defaultPath: defaultDir,
    });
  };

  const handleBindOpenBrainSource = async (workspace: LocalOpenBrainWorkspace) => {
    const provider = useOpenBrainStore.getState().provider;
    const targetConnection = openBrainRuntimeConnectionForWorkspace(workspace);
    const targetTab = targetConnection?.tab || (!workspace.instanceID
      ? useTabManagerStore.getState().tabs.find((tab) => tab.id === activeTabId) || null
      : null);
    if ((workspace.runtimeReachable === false || !targetTab) && workspace.instanceID) {
      useToastStore.getState().pushToast('Connect this runtime before binding the source.', {
        durationMs: 6000,
        anchor: null,
      });
      return;
    }
    if (targetTab?.kind === 'remote') {
      const store = getWorkspaceStore(targetTab.id);
      const session = store.getState().remoteSession;
      if (!session) {
        useToastStore.getState().pushToast('Connect this remote runtime before binding the source.', {
          durationMs: 6000,
          anchor: null,
        });
        return;
      }
      const defaultDir = workspace.path || store.getState().currentDir || session.workspaceDir || '/';
      setDirectoryPickerRequest({
        source: 'create-openbrain-source',
        provider,
        locationKind: 'remote',
        sourceWorkspace: workspace,
        tabId: targetTab.id,
        currentPath: defaultDir,
        defaultPath: defaultDir,
      });
      return;
    }
    const targetCurrentDir = targetTab ? getWorkspaceStore(targetTab.id).getState().currentDir : currentDir;
    const defaultDir = workspace.path || targetCurrentDir || await resolveDefaultLocalWorkspacePath();
    setDirectoryPickerRequest({
      source: 'create-openbrain-source',
      provider,
      locationKind: 'local',
      sourceWorkspace: workspace,
      tabId: targetTab?.id,
      defaultPath: defaultDir,
    });
  };

  const isOpenBrainBindingError = (error: unknown) => {
    const code = typeof (error as { code?: unknown })?.code === 'string' ? (error as { code: string }).code : '';
    return code === 'workspace_unbound'
      || code === 'workspace_path_missing'
      || code === 'workspace_repo_mismatch';
  };

  const collectOpenBrainRecoveryCandidatePaths = async (workspace: LocalOpenBrainWorkspace): Promise<string[]> => {
    await useRecentWorkspacesStore.getState().load().catch(() => {});
    const paths: string[] = [];
    const seen = new Set<string>();
    const addPath = (value?: string | null) => {
      const candidate = (value || '').trim();
      if (!candidate || seen.has(candidate)) {
        return;
      }
      seen.add(candidate);
      paths.push(candidate);
    };

    const targetConnection = openBrainRuntimeConnectionForWorkspace(workspace);
    const targetTab = targetConnection?.tab || (!workspace.instanceID
      ? useTabManagerStore.getState().tabs.find((tab) => tab.id === activeTabId) || null
      : null);
    if (targetTab?.kind === 'remote') {
      const store = getWorkspaceStore(targetTab.id).getState();
      addPath(store.currentDir);
      addPath(store.remoteSession?.workspaceDir);
      if (workspace.instanceID) {
        const bucket = getRemoteBucketByInstanceID(useRecentWorkspacesStore.getState().recent, workspace.instanceID);
        for (const entry of bucket?.directories || []) {
          addPath(entry.path);
        }
      }
      return paths;
    }

    const targetStore = targetTab ? getWorkspaceStore(targetTab.id).getState() : null;
    addPath(targetStore?.currentDir);
    addPath(targetTab?.workspacePath);
    addPath(currentDir);
    addPath(await resolveDefaultLocalWorkspacePath());
    for (const entry of useRecentWorkspacesStore.getState().recent.local || []) {
      addPath(entry.path);
    }
    return paths;
  };

  const tryRecoverOpenBrainBinding = async (workspace: LocalOpenBrainWorkspace): Promise<LocalOpenBrainWorkspace | null> => {
    const candidatePaths = await collectOpenBrainRecoveryCandidatePaths(workspace);
    if (candidatePaths.length === 0) {
      return null;
    }
    let candidates: Array<{ path: string; name?: string }> = [];
    try {
      candidates = await useOpenBrainStore.getState().listRecoveryCandidates(workspace, candidatePaths);
    } catch {
      return null;
    }
    if (candidates.length !== 1) {
      if (candidates.length > 1) {
        useToastStore.getState().pushToast('Multiple matching folders found. Choose one to bind this source.', {
          durationMs: 6000,
          anchor: null,
        });
      }
      return null;
    }
    const candidate = candidates[0];
    if (!window.confirm(`Bind ${workspace.name} to ${candidate.path} on this runtime?`)) {
      return null;
    }
    const targetConnection = openBrainRuntimeConnectionForWorkspace(workspace);
    const targetTab = targetConnection?.tab || (!workspace.instanceID
      ? useTabManagerStore.getState().tabs.find((tab) => tab.id === activeTabId) || null
      : null);
    let bound: LocalOpenBrainWorkspace;
    try {
      bound = targetTab?.kind === 'remote'
        ? await useOpenBrainStore.getState().createOpenBrain({
          name: workspace.name,
          remotePath: candidate.path,
          tabId: targetTab.id,
          remoteHost: targetTab.remoteHost,
          source: workspace,
        })
        : await useOpenBrainStore.getState().createOpenBrain({
          name: workspace.name,
          localPath: candidate.path,
          tabId: targetTab?.id,
          source: workspace,
        });
    } catch (error) {
      useToastStore.getState().pushToast(error instanceof Error ? error.message : 'Failed to bind OpenBrain source.', {
        durationMs: 7000,
        anchor: null,
      });
      return null;
    }
    useToastStore.getState().pushToast(`${workspace.name} was bound on this runtime.`, {
      durationMs: 4200,
      anchor: null,
    });
    return bound;
  };

  const createOpenBrainSourceAtPath = async (path: string, request: Extract<DirectoryPickerRequest, { source: 'create-openbrain-source' }>) => {
    const name = request.sourceWorkspace?.name || getLocalPathBaseName(path);
    const activeTab = useTabManagerStore.getState().tabs.find((tab) => tab.id === (request.tabId || activeTabId)) || null;
    if (request.locationKind === 'remote' && !activeTab?.remoteHost) {
      throw new Error('Remote host information is required.');
    }
    const workspace = request.locationKind === 'remote'
      ? await useOpenBrainStore.getState().createOpenBrain({
        name,
        remotePath: path,
        tabId: request.tabId || activeTabId,
        remoteHost: activeTab?.remoteHost,
        source: request.sourceWorkspace,
      })
      : await useOpenBrainStore.getState().createOpenBrain({ name, localPath: path, tabId: request.tabId, source: request.sourceWorkspace });
    if (!workspace.path) {
      throw new Error('Failed to create OpenBrain source.');
    }
    if (request.locationKind === 'remote') {
      if (activeTab?.remoteHost) {
        await handleSwitchCurrentRemoteDirectory(request.tabId || activeTab.id, workspace.path);
      }
      return;
    }
    await handleNewTabWithLocal(workspace.path);
  };

  const handleLandingOpenFolder = async () => {
    if (openingLandingFolder) {
      return;
    }
    setOpeningLandingFolder(true);
    const defaultDir = await resolveDefaultLocalWorkspacePath();
    setDirectoryPickerRequest({
      source: 'landing-open-folder',
      defaultPath: defaultDir,
    });
  };

  const handleLandingConnectRemote = () => {
    setRemoteModalTarget('current');
    setShowRemoteModal(true);
  };

  const waitForRemoteInstanceID = async (tabId: string) => {
    const deadline = Date.now() + 5000;
    while (Date.now() < deadline) {
      const store = getWorkspaceStore(tabId);
      const state = store.getState();
      const instanceID = (state.instanceID || '').trim();
      if (instanceID) {
        return instanceID;
      }
      if (state.connectionState === 'connected') {
        const derived = await state.ensureDerivedDirs();
        const nextInstanceID = (derived?.instanceID || '').trim();
        if (nextInstanceID) {
          return nextInstanceID;
        }
      }
      await new Promise((resolve) => window.setTimeout(resolve, 100));
    }
    return null;
  };

  const recordRemoteRecentForTab = async (tabId: string, host: SshHost, path?: string) => {
    try {
      const instanceID = await waitForRemoteInstanceID(tabId);
      if (!instanceID) {
        return;
      }
      const store = getWorkspaceStore(tabId);
      await useRecentWorkspacesStore.getState().recordRemote({
        instanceID,
        host,
        path: path || store.getState().currentDir || undefined,
        label: host.alias,
        lastOpenedAt: Date.now(),
      });
    } catch {
      return;
    }
  };

  const handleSwitchLocalWorkspace = async (path: string) => {
    if (remoteSession && window.electronAPI?.remote) {
      try {
        await window.electronAPI.remote.disconnect(activeTabId);
      } catch {
        // ignore
      }
      const store = getWorkspaceStore(activeTabId);
      store.getState().setRemoteSession(null);
      store.getState().connect();
    }
    updateActiveTabWorkspace({ kind: 'local', workspacePath: path });
    setCurrentDir(path);
    await ensureDirectory(path);
  };

  const handleSwitchRemoteWorkspace = async (host: SshHost, path?: string) => {
    await connectRemoteForTab(activeTabId, host);
    if (path) {
      const store = getWorkspaceStore(activeTabId);
      updateActiveTabWorkspace({ kind: 'remote', remoteHost: host, workspacePath: path, label: resolveHostLabel(host) });
      store.getState().setCurrentDir(path);
      await store.getState().ensureDirectory(path);
    }
    void recordRemoteRecentForTab(activeTabId, host, path);
  };

  const activateExistingLocalWorkspaceTab = async (path: string): Promise<boolean> => {
    const targetKey = normalizeWorkspacePathKey(path);
    if (!targetKey) {
      return false;
    }
    const existingTab = useTabManagerStore.getState().tabs.find((tab) => {
      if (tab.kind !== 'local') {
        return false;
      }
      const storeState = getWorkspaceStore(tab.id).getState();
      return normalizeWorkspacePathKey(tab.workspacePath) === targetKey
        || normalizeWorkspacePathKey(storeState.currentDir) === targetKey;
    });
    if (!existingTab) {
      return false;
    }

    updateTabWorkspace(existingTab.id, {
      kind: 'local',
      workspacePath: path,
    });
    setActiveWorkspaceTab(existingTab.id);
    const storeState = getWorkspaceStore(existingTab.id).getState();
    if (storeState.connectionState !== 'connected') {
      storeState.connect();
    }
    if (normalizeWorkspacePathKey(storeState.currentDir) !== targetKey) {
      storeState.setCurrentDir(path);
    }
    await storeState.ensureDirectory(path);
    if (storeState.documents.length === 0) {
      storeState.openWelcomeTab();
    }
    return true;
  };

  /** Open this local workspace path, reusing an existing tab for the same path. */
  const handleNewTabWithLocal = async (path: string) => {
    if (await activateExistingLocalWorkspaceTab(path)) {
      await useRecentWorkspacesStore.getState().recordLocal({ path });
      return;
    }
    const tab = createWorkspaceTab({ kind: 'local', workspacePath: path });
    const store = getWorkspaceStore(tab.id);
    store.getState().setCurrentDir(path);
    store.getState().ensureDirectory(path);
    store.getState().openWelcomeTab();
    await useRecentWorkspacesStore.getState().recordLocal({ path });
  };

  const handleOpenBrainWorkspace = async (workspace: LocalOpenBrainWorkspace) => {
    if (workspace.bindingStatus === 'needs_binding' || !workspace.path) {
      const recovered = await tryRecoverOpenBrainBinding(workspace);
      if (recovered?.path) {
        if (recovered.locationKind === 'remote' && recovered.remoteHost) {
          await handleNewTabWithRemote(recovered.remoteHost, recovered.path);
        } else {
          await handleNewTabWithLocal(recovered.path);
        }
        return;
      }
      await handleBindOpenBrainSource(workspace);
      return;
    }
    let verified = workspace;
    try {
      verified = await useOpenBrainStore.getState().verifyOpenBrain(workspace);
    } catch (error) {
      useToastStore.getState().pushToast(error instanceof Error ? error.message : 'OpenBrain source needs to be rebound.', {
        durationMs: 7000,
        anchor: null,
      });
      if (isOpenBrainBindingError(error)) {
        const recovered = await tryRecoverOpenBrainBinding(workspace);
        if (recovered?.path) {
          if (recovered.locationKind === 'remote' && recovered.remoteHost) {
            await handleNewTabWithRemote(recovered.remoteHost, recovered.path);
          } else {
            await handleNewTabWithLocal(recovered.path);
          }
          return;
        }
        await handleBindOpenBrainSource(workspace);
      }
      return;
    }
    if (verified.locationKind === 'remote' && verified.remoteHost) {
      await handleNewTabWithRemote(verified.remoteHost, verified.path);
      return;
    }
    if (verified.path) {
      await handleNewTabWithLocal(verified.path);
    }
  };

  /** Title bar "+" recent: open a new tab and connect to this host (do not change current tab). */
  const handleNewTabWithRemote = async (host: SshHost, path?: string) => {
    const tab = createWorkspaceTab({ kind: 'remote', remoteHost: host });
    const store = getWorkspaceStore(tab.id);
    store.getState().setServerUrl('');
    store.getState().disconnect();
    store.getState().openWelcomeTab();
    await connectRemoteForTab(tab.id, host);
    if (path) {
      useTabManagerStore.getState().updateTabWorkspace(tab.id, {
        kind: 'remote',
        remoteHost: host,
        workspacePath: path,
        label: resolveHostLabel(host),
      });
      store.getState().setCurrentDir(path);
      await store.getState().ensureDirectory(path);
    }
    void recordRemoteRecentForTab(tab.id, host, path);
  };

  const handleOpenLocalSwitchDirectory = async () => {
    const defaultDir = currentDir && !remoteSession
      ? currentDir
      : await resolveDefaultLocalWorkspacePath();
    setDirectoryPickerRequest({
      source: 'switch-local-workspace',
      currentPath: currentDir,
      defaultPath: defaultDir,
    });
  };

  const handleOpenRemoteSwitchDirectory = () => {
    const store = getWorkspaceStore(activeTabId);
    setDirectoryPickerRequest({
      source: 'switch-remote-workspace',
      tabId: activeTabId,
      currentPath: store.getState().currentDir || remoteSession?.workspaceDir || '/',
    });
  };

  const handleSwitchCurrentRemoteDirectory = async (tabId: string, path: string) => {
    const activeTab = useTabManagerStore.getState().tabs.find((tab) => tab.id === tabId) || null;
    if (!activeTab || activeTab.kind !== 'remote') {
      return;
    }

    const store = getWorkspaceStore(tabId);

    if (activeTab.remoteHost) {
      updateTabWorkspace(tabId, {
        kind: 'remote',
        remoteHost: activeTab.remoteHost,
        workspacePath: path,
        label: resolveHostLabel(activeTab.remoteHost),
      });
    } else {
      updateTabWorkspace(tabId, {
        kind: 'remote',
        workspacePath: path,
        label: activeTab.label,
      });
    }

    store.getState().setCurrentDir(path);
    await store.getState().ensureDirectory(path);

    if (activeTab.remoteHost) {
      void recordRemoteRecentForTab(tabId, activeTab.remoteHost, path);
    }
  };

  const handleCloseWorkspaceTab = async (tabId: string) => {
    if (showSpecialPage) {
      useUiStore.getState().setSidebarView('workspace');
    }
    await closeWorkspaceTabWithDefaultFallback(tabId, {
      workspaceTabs: useTabManagerStore.getState().tabs,
      createWorkspaceTab: (init) => createWorkspaceTab(init),
      getWorkspaceStore,
      disconnectRemote: async (targetTabId) => {
        await window.electronAPI?.remote?.disconnect(targetTabId);
      },
      setWorkspaceActive,
      disposeChatWorkspaceRuntime,
      removeWorkspaceStore,
      removeChatWorkspaceStore,
      closeWorkspaceTab,
      resolveDefaultLocalWorkspacePath,
    });
  };

  const closeDirectoryPicker = () => {
    if (directoryPickerRequest?.source === 'landing-open-folder') {
      setOpeningLandingFolder(false);
    }
    setDirectoryPickerRequest(null);
  };

  const handleDirectoryPickerSelect = async (path: string) => {
    if (!directoryPickerRequest) {
      return;
    }

    if (directoryPickerRequest.source === 'switch-local-workspace') {
      await handleSwitchLocalWorkspace(path);
      await useRecentWorkspacesStore.getState().recordLocal({ path });
      return;
    }

    if (directoryPickerRequest.source === 'new-local-workspace-tab') {
      const tab = createWorkspaceTab({ kind: 'local', workspacePath: path });
      const store = getWorkspaceStore(tab.id);
      store.getState().setCurrentDir(path);
      store.getState().ensureDirectory(path);
      store.getState().openWelcomeTab();
      await useRecentWorkspacesStore.getState().recordLocal({ path });
      return;
    }

    if (directoryPickerRequest.source === 'landing-open-folder') {
      connect();
      await handleSwitchLocalWorkspace(path);
      await useRecentWorkspacesStore.getState().recordLocal({ path });
      setIsNewWindowLanding(false);
      return;
    }

    if (directoryPickerRequest.source === 'create-openbrain-source') {
      const request = directoryPickerRequest;
      const name = getLocalPathBaseName(path);
      const pendingID = useOpenBrainStore.getState().beginPendingOpenBrainSource({
        name,
        path,
        locationKind: request.locationKind === 'remote' ? 'remote' : 'local',
        rebinding: Boolean(request.sourceWorkspace),
      });
      useUiStore.getState().setSidebarView('openbrain');
      void (async () => {
        try {
          await createOpenBrainSourceAtPath(path, request);
          useOpenBrainStore.getState().completePendingOpenBrainSource(pendingID);
          if (request.locationKind === 'remote') {
            const tab = useTabManagerStore.getState().tabs.find((item) => item.id === (request.tabId || activeTabId)) || null;
            if (tab?.remoteHost) {
              void recordRemoteRecentForTab(tab.id, tab.remoteHost, path);
            }
          } else {
            await useRecentWorkspacesStore.getState().recordLocal({ path });
          }
        } catch (error) {
          console.error('Failed to create OpenBrain source:', error);
          void useOpenBrainStore.getState().refresh().catch(() => {});
          useOpenBrainStore.getState().failPendingOpenBrainSource(
            pendingID,
            error instanceof Error ? error.message : 'Failed to create OpenBrain source.',
          );
        }
      })();
      return;
    }

    await handleSwitchCurrentRemoteDirectory(directoryPickerRequest.tabId, path);
  };

  const saveFileRequest = useSaveFileDialogRequest();
  const saveFileProvider = useMemo(
    () => (saveFileRequest ? createLocalDirectoryPickerProvider() : null),
    [saveFileRequest],
  );

  const directoryPickerConfig = useMemo(() => {
    if (!directoryPickerRequest) {
      return null;
    }

    if (directoryPickerRequest.source === 'switch-remote-workspace') {
      const store = getWorkspaceStore(directoryPickerRequest.tabId).getState();
      const remoteBucket = store.instanceID
        ? getRemoteBucketByInstanceID(recentWorkspaces, store.instanceID)
        : null;
      return {
        provider: createRemoteDirectoryPickerProvider({
          remoteSession: store.remoteSession,
          listDirectory: store.listDirectory,
          statPath: store.statPath,
        }),
        title: 'Switch Directory',
        subtitle: store.remoteSession ? `Remote: ${store.remoteSession.hostLabel}` : 'Remote Workspace',
        defaultPath: directoryPickerRequest.currentPath || store.remoteSession?.workspaceDir || '/',
        currentPath: directoryPickerRequest.currentPath,
        recentPaths: (remoteBucket?.directories || []).map((entry) => entry.path),
        submitLabel: 'Switch',
      };
    }

    const localProvider = createLocalDirectoryPickerProvider();
    const localRecentPaths = recentWorkspaces.local.map((entry) => entry.path);

    if (directoryPickerRequest.source === 'landing-open-folder') {
      return {
        provider: localProvider,
        title: 'Open Folder',
        subtitle: 'Local Workspace',
        defaultPath: directoryPickerRequest.defaultPath,
        currentPath: null,
        recentPaths: localRecentPaths,
        submitLabel: 'Open',
      };
    }

    if (directoryPickerRequest.source === 'new-local-workspace-tab') {
      return {
        provider: localProvider,
        title: 'Open Folder',
        subtitle: 'New Local Workspace Tab',
        defaultPath: directoryPickerRequest.defaultPath,
        currentPath: null,
        recentPaths: localRecentPaths,
        submitLabel: 'Open',
      };
    }

    if (directoryPickerRequest.source === 'create-openbrain-source') {
      if (directoryPickerRequest.locationKind === 'remote') {
        const tabId = directoryPickerRequest.tabId || activeTabId;
        const store = getWorkspaceStore(tabId).getState();
        const remoteBucket = store.instanceID
          ? getRemoteBucketByInstanceID(recentWorkspaces, store.instanceID)
          : null;
        return {
          provider: createRemoteDirectoryPickerProvider({
            remoteSession: store.remoteSession,
            listDirectory: store.listDirectory,
            statPath: store.statPath,
          }),
          title: 'Add Folder to OpenBrain Cloud',
          subtitle: store.remoteSession ? `Remote: ${store.remoteSession.hostLabel}` : 'Remote Workspace',
          defaultPath: directoryPickerRequest.defaultPath,
          currentPath: directoryPickerRequest.currentPath,
          recentPaths: (remoteBucket?.directories || []).map((entry) => entry.path),
          submitLabel: 'Add to Cloud',
        };
      }
      const label = directoryPickerRequest.provider === 'local'
        ? 'OpenBrain Source'
        : 'OpenBrain Cloud';
      return {
        provider: localProvider,
        title: directoryPickerRequest.provider === 'cloud' ? 'Add Folder to OpenBrain Cloud' : `Select ${label} Folder`,
        subtitle: 'The selected folder will become an OpenBrain workspace.',
        defaultPath: directoryPickerRequest.defaultPath,
        currentPath: null,
        recentPaths: localRecentPaths,
        submitLabel: 'Use Folder',
      };
    }

    return {
      provider: localProvider,
      title: 'Switch Directory',
      subtitle: 'Local Workspace',
      defaultPath: directoryPickerRequest.defaultPath,
      currentPath: directoryPickerRequest.currentPath,
      recentPaths: localRecentPaths,
      submitLabel: 'Switch',
    };
  }, [directoryPickerRequest, recentWorkspaces]);

  useEffect(() => {
    void loadRecentWorkspaces();
  }, [loadRecentWorkspaces]);

  useEffect(() => {
    const activeChatPath = normalizeChatSessionPath(currentFilePath);
    if (!activeChatPath || !workspaceConversationDocs.some((tab) => tab.filePath === activeChatPath)) {
      return;
    }
    const selectedTarget = getChatWorkspaceStore(activeTabId).getState().selectedConversationTarget;
    if (!shouldSyncConversationSelectionWithActiveChat(activeChatPath, selectedTarget)) {
      return;
    }
    getChatWorkspaceStore(activeTabId).getState().selectChatConversation(activeChatPath);
  }, [activeTabId, currentFilePath, workspaceConversationDocs]);

  useEffect(() => {
    if (!pendingConversationSelected) {
      return;
    }
    if (activeDocument?.documentRole !== 'conversation') {
      return;
    }
    if (pinnedTab && activeDocument.id === pinnedTab.id) {
      return;
    }
    activateLastNonConversationTab();
  }, [
    activateLastNonConversationTab,
    activeDocument,
    pendingConversationSelected,
    pinnedTab,
  ]);

  useEffect(() => {
    if (!pinnedTabId) {
      return;
    }
    if (pinnedTab) {
      return;
    }
    clearPinnedTab();
  }, [clearPinnedTab, pinnedTab, pinnedTabId]);

  const handlePinnedFileToggle = (tabId: string | null | undefined) => {
    const normalizedId = (tabId || '').trim();
    if (!normalizedId) {
      return;
    }
    togglePinnedTab(normalizedId);
  };

  const handleReturnPinnedFileToEditor = () => {
    if (!pinnedTab) {
      return;
    }
    clearPinnedTab();
    setActiveTab(pinnedTab.id);
  };

  const getActivityPanelAreaElement = () => {
    if (showOpenBrainPage) {
      return openBrainPagePaneRef.current;
    }
    if (showMessengerView) {
      return messengerConversationPaneRef.current;
    }
    return primaryEditorPaneRef.current;
  };

  const getActivityPanelHeightLimit = () => {
    const sourceHeight = getActivityPanelAreaElement()?.clientHeight
      || editorAreaRef.current?.clientHeight
      || window.innerHeight;
    return Math.max(
      ACTIVITY_PANEL_MIN_HEIGHT,
      sourceHeight - ACTIVITY_PANEL_LAYOUT_SAFETY_GAP
    );
  };

  const clampActivityPanelHeight = (height: number) => {
    const maxHeight = getActivityPanelHeightLimit();
    return Math.min(maxHeight, Math.max(ACTIVITY_PANEL_MIN_HEIGHT, height));
  };

  const getActivityPanelAreaWidth = () => {
    return getActivityPanelAreaElement()?.clientWidth
      || editorAreaRef.current?.clientWidth
      || window.innerWidth;
  };

  const getActivityPanelWidthLimit = () => {
    const sourceWidth = getActivityPanelAreaWidth();
    return Math.max(
      ACTIVITY_PANEL_MIN_WIDTH,
      sourceWidth - ACTIVITY_PANEL_HORIZONTAL_EDGE_GAP * 2
    );
  };

  const clampActivityPanelWidth = (width: number) => {
    const maxWidth = getActivityPanelWidthLimit();
    return Math.min(maxWidth, Math.max(ACTIVITY_PANEL_MIN_WIDTH, width));
  };

  const resolveDefaultActivityPanelWidth = () => {
    return clampActivityPanelWidth(
      Math.round(getActivityPanelAreaWidth() * DEFAULT_ACTIVITY_PANEL_WIDTH_RATIO)
    );
  };

  const getConversationComposerDockHeightLimit = () => {
    const sourceHeight = editorAreaRef.current?.clientHeight || window.innerHeight;
    return Math.max(
      CONVERSATION_COMPOSER_DOCK_MIN_HEIGHT,
      sourceHeight - CONVERSATION_COMPOSER_DOCK_LAYOUT_SAFETY_GAP
    );
  };

  const clampConversationComposerDockHeight = (height: number) => {
    const maxHeight = getConversationComposerDockHeightLimit();
    return Math.min(maxHeight, Math.max(CONVERSATION_COMPOSER_DOCK_MIN_HEIGHT, height));
  };

  const getPinnedFilePanelWidthLimit = () => {
    const sourceWidth = editorContentRowRef.current?.clientWidth
      || editorAreaRef.current?.clientWidth
      || window.innerWidth;
    return Math.max(
      PINNED_FILE_PANEL_MIN_WIDTH,
      Math.min(
        PINNED_FILE_PANEL_MAX_WIDTH,
        sourceWidth - PINNED_FILE_PANEL_PRIMARY_MIN_WIDTH
      )
    );
  };

  const clampPinnedFilePanelWidth = (width: number) => {
    const maxWidth = getPinnedFilePanelWidthLimit();
    return Math.min(maxWidth, Math.max(PINNED_FILE_PANEL_MIN_WIDTH, width));
  };

  const getActivityPanelMeasuredWidth = () => {
    return activityPanelHostRef.current?.getBoundingClientRect().width
      || activityPanelMeasuredWidth
      || renderedActivityPanelWidth
      || 0;
  };

  const clampActivityPanelLeft = (left: number, panelWidthOverride?: number) => {
    const sourceWidth = getActivityPanelAreaWidth();
    const panelWidth = panelWidthOverride ?? getActivityPanelMeasuredWidth();
    if (!(panelWidth > 0) || sourceWidth <= 0) {
      return ACTIVITY_PANEL_HORIZONTAL_EDGE_GAP;
    }
    const minLeft = ACTIVITY_PANEL_HORIZONTAL_EDGE_GAP;
    const maxLeft = Math.max(
      minLeft,
      sourceWidth - panelWidth - ACTIVITY_PANEL_HORIZONTAL_EDGE_GAP
    );
    return Math.min(maxLeft, Math.max(minLeft, left));
  };

  const resolveCenteredActivityPanelLeft = (panelWidthOverride?: number) => {
    const panelWidth = panelWidthOverride ?? getActivityPanelMeasuredWidth();
    if (!(panelWidth > 0)) {
      return ACTIVITY_PANEL_HORIZONTAL_EDGE_GAP;
    }
    return clampActivityPanelLeft(
      (getActivityPanelAreaWidth() - panelWidth) / 2,
      panelWidth
    );
  };

  const getResolvedActivityPanelLeft = (panelWidthOverride?: number) => {
    const panelWidth = panelWidthOverride ?? getActivityPanelMeasuredWidth();
    if (activityPanelLeft == null) {
      return resolveCenteredActivityPanelLeft(panelWidth);
    }
    return clampActivityPanelLeft(activityPanelLeft, panelWidth);
  };

  const defaultActivityPanelWidth = activityPanelWidth == null
    ? resolveDefaultActivityPanelWidth()
    : null;
  const renderedActivityPanelWidth = activityPanelWidth == null
    ? defaultActivityPanelWidth
    : clampActivityPanelWidth(activityPanelWidth);
  const renderedActivityPanelMaxHeight = clampActivityPanelHeight(activityPanelMaxHeight);
  const renderedPinnedFilePanelWidth = clampPinnedFilePanelWidth(pinnedFilePanelWidth);
  const renderedActivityPanelLeft = getResolvedActivityPanelLeft(
    renderedActivityPanelWidth ?? (activityPanelMeasuredWidth > 0 ? activityPanelMeasuredWidth : undefined)
  );
  const shouldReserveActivityPanelEditorSafeArea = showActivityPanel && !showSpecialPage && !liveOverlay.expanded;
  const activityPanelBottomSafeArea = shouldReserveActivityPanelEditorSafeArea
    ? Math.max(activityPanelCoveredBottom, ACTIVITY_PANEL_COLLAPSED_STACK_MIN_HEIGHT)
      + ACTIVITY_PANEL_EDITOR_TEXT_CLEARANCE
    : 0;
  const primaryEditorViewportStyle = {
    '--op-editor-bottom-safe-area': `${activityPanelBottomSafeArea}px`,
  } as React.CSSProperties;
  const activityPanelHostStyle = {
    left: `${renderedActivityPanelLeft}px`,
    width: renderedActivityPanelWidth != null ? `${renderedActivityPanelWidth}px` : undefined,
  } as React.CSSProperties;

  const renderActivityPanel = () => (
    showActivityPanel && (
      <div
        ref={activityPanelStackRef}
        className={`absolute bottom-0 left-0 right-0 z-[50] overflow-visible pb-3 ${showOpenBrainPage ? 'pointer-events-auto' : 'pointer-events-none'}`}
      >
        {/* Keep ActivityPanel above editor-local overlays like markdown outline. */}
        <div
          ref={activityPanelHostRef}
          className={`op-activity-panel-host no-drag${showOpenBrainPage ? ' is-busy-surface' : ''}`}
          style={activityPanelHostStyle}
        >
          <ActivityPanel
            bodyMaxHeight={renderedActivityPanelMaxHeight}
            forceVisible={threadConversationSelected}
            onTopLeftResizeStart={(event) => handleActivityPanelCornerResizeStart('left', event)}
            onTopRightResizeStart={(event) => handleActivityPanelCornerResizeStart('right', event)}
            onHeaderPointerDown={handleActivityPanelHeaderPointerDown}
            shouldToggleOnHeaderClick={shouldToggleActivityPanelFromHeaderClick}
            horizontalDragging={activityPanelHorizontalDragging}
            activeResizeCorner={activityPanelResizeEdge}
          />
        </div>
      </div>
    )
  );

  const renderEditorForTab = (
    tab: typeof workspaceDocuments[number] | null,
    options?: {
      pinned?: boolean;
      suppressOutlineToggle?: boolean;
      compactMarkdown?: boolean;
      textOffsetEnabled?: boolean;
      autoFocus?: boolean;
    },
  ) => {
    if (!tab) {
      return null;
    }
    const isPinned = Boolean(options?.pinned);
    const isMarkdown = tab.editorId === 'markdown';
    const isPinnable = isPinnableFileTab(tab);
    const isPinnedTab = pinnedTabId === tab.id;
    if (tab.editorId === 'marketplace' || tab.editorId.startsWith('marketplace:')) {
      return <MarketplaceEditor />;
    }
    if (tab.editorId.startsWith('cron-task:')) {
      return <CronTaskEditor />;
    }
    const editorDef = editorRegistry.get(tab.editorId);
    const EditorComponent = editorDef?.component || TextEditor;
    if (isMarkdown) {
      return (
        <MarkdownEditor
          tabId={isPinned ? tab.id : undefined}
          autoFocus={options?.autoFocus ?? false}
          outlinePinEnabled={isPinnable}
          outlinePinned={isPinnedTab}
          onOutlinePinToggle={() => handlePinnedFileToggle(tab.id)}
          outlineToggleEnabled={options?.suppressOutlineToggle ? false : true}
          textOffsetEnabled={options?.textOffsetEnabled ?? true}
          compact={options?.compactMarkdown ?? false}
        />
      );
    }
    if (tab.editorId === 'text') {
      return (
        <TextEditor
          tabId={isPinned ? tab.id : undefined}
          autoFocus={options?.autoFocus ?? false}
          pinEnabled={isPinnable}
          pinned={isPinnedTab}
          onPinToggle={() => handlePinnedFileToggle(tab.id)}
        />
      );
    }
    if (tab.editorId === 'image') {
      return <ImageEditor tabId={isPinned ? tab.id : undefined} />;
    }
    if (tab.editorId === 'book' || tab.editorId === 'pdf') {
      return <BookReaderEditor tabId={isPinned ? tab.id : undefined} />;
    }
    if (tab.editorId === 'welcome') {
      return (
        <WelcomeEditor
          chatPanelBottomInset={conversationComposerDockHeight + 1}
          chatPanelOpen={composerVisible}
        />
      );
    }
    return <EditorComponent />;
  };

  const handleSidebarResizeStart = (event: React.PointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    if (sidebarResizeRef.current) {
      return;
    }
    sidebarResizeRef.current = {
      startX: event.clientX,
      startWidth: sidebarWidth,
      currentWidth: sidebarWidth,
    };

    const handlePointerMove = (moveEvent: PointerEvent) => {
      const drag = sidebarResizeRef.current;
      if (!drag) {
        return;
      }
      const deltaX = moveEvent.clientX - drag.startX;
      const nextWidth = clampSidebarWidth(drag.startWidth + deltaX);
      drag.currentWidth = nextWidth;
      setSidebarWidth(nextWidth);
    };

    const cleanup = () => {
      const drag = sidebarResizeRef.current;
      sidebarResizeRef.current = null;
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', cleanup);
      window.removeEventListener('pointercancel', cleanup);

      if (!drag) {
        return;
      }
      const finalWidth = clampSidebarWidth(drag.currentWidth);
      if (finalWidth === drag.startWidth) {
        return;
      }
      const persistPromise = window.electronAPI?.settings?.set?.({
        ui: {
          sidebarWidth: finalWidth,
        },
      });
      if (!persistPromise) {
        return;
      }
      void persistPromise.catch((error) => {
        console.warn('[sidebarWidth] failed to persist:', error);
      });
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', cleanup);
    window.addEventListener('pointercancel', cleanup);
  };

  const handleConversationComposerDockResizeStart = (event: React.PointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    if (conversationComposerDockResizeRef.current) {
      return;
    }
    conversationComposerDockResizeRef.current = {
      startY: event.clientY,
      startHeight: conversationComposerDockHeight,
      currentHeight: conversationComposerDockHeight,
    };

    const handlePointerMove = (moveEvent: PointerEvent) => {
      const drag = conversationComposerDockResizeRef.current;
      if (!drag) {
        return;
      }
      const deltaY = drag.startY - moveEvent.clientY;
      const nextHeight = clampConversationComposerDockHeight(drag.startHeight + deltaY);
      drag.currentHeight = nextHeight;
      setConversationComposerDockHeight(nextHeight);
    };

    const cleanup = () => {
      const drag = conversationComposerDockResizeRef.current;
      conversationComposerDockResizeRef.current = null;
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', cleanup);
      window.removeEventListener('pointercancel', cleanup);

      if (!drag) {
        return;
      }
      const finalHeight = clampConversationComposerDockHeight(drag.currentHeight);
      if (finalHeight === drag.startHeight) {
        return;
      }
      conversationComposerDockPreferredHeightRef.current = finalHeight;
      setConversationComposerDockHeight(finalHeight);
      const persistPromise = window.electronAPI?.settings?.set?.({
        ui: {
          conversationComposerDockHeight: finalHeight,
        },
      });
      if (!persistPromise) {
        return;
      }
      void persistPromise.catch((error) => {
        console.warn('[conversationComposerDockHeight] failed to persist:', error);
      });
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', cleanup);
    window.addEventListener('pointercancel', cleanup);
  };

  const handlePinnedFilePanelResizeStart = (event: React.PointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    if (pinnedFilePanelResizeRef.current) {
      return;
    }
    pinnedFilePanelResizeRef.current = {
      startX: event.clientX,
      startWidth: renderedPinnedFilePanelWidth,
      currentWidth: renderedPinnedFilePanelWidth,
    };

    const handlePointerMove = (moveEvent: PointerEvent) => {
      const drag = pinnedFilePanelResizeRef.current;
      if (!drag) {
        return;
      }
      const deltaX = drag.startX - moveEvent.clientX;
      const nextWidth = clampPinnedFilePanelWidth(drag.startWidth + deltaX);
      drag.currentWidth = nextWidth;
      setPinnedFilePanelWidth(nextWidth);
    };

    const cleanup = () => {
      const drag = pinnedFilePanelResizeRef.current;
      pinnedFilePanelResizeRef.current = null;
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', cleanup);
      window.removeEventListener('pointercancel', cleanup);

      if (!drag) {
        return;
      }
      const finalWidth = normalizePinnedFilePanelWidth(drag.currentWidth);
      if (finalWidth === drag.startWidth) {
        setPinnedFilePanelWidth(finalWidth);
        return;
      }
      setPinnedFilePanelWidth(finalWidth);
      const persistPromise = window.electronAPI?.settings?.set?.({
        ui: {
          pinnedFilePanelWidth: finalWidth,
        },
      });
      if (!persistPromise) {
        return;
      }
      void persistPromise.catch((error) => {
        console.warn('[pinnedFilePanelWidth] failed to persist:', error);
      });
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', cleanup);
    window.addEventListener('pointercancel', cleanup);
  };

  const handleActivityPanelCornerResizeStart = (
    edge: 'left' | 'right',
    event: React.PointerEvent<HTMLDivElement>
  ) => {
    event.preventDefault();
    event.stopPropagation();
    if (
      !event.isPrimary
      || event.button !== 0
      || activityPanelResizeRef.current
      || activityPanelHorizontalDragRef.current
    ) {
      return;
    }

    const startWidth = getActivityPanelMeasuredWidth();
    if (!(startWidth > 0)) {
      return;
    }
    const startHeight = renderedActivityPanelMaxHeight;
    const startLeft = getResolvedActivityPanelLeft(startWidth);
    activityPanelResizeRef.current = {
      edge,
      startX: event.clientX,
      startY: event.clientY,
      startLeft,
      startWidth,
      startHeight,
      startPreferredWidth: activityPanelWidth ?? defaultActivityPanelWidth,
      startPreferredHeight: activityPanelMaxHeight,
      currentLeft: startLeft,
      currentWidth: startWidth,
      currentHeight: startHeight,
    };
    activityPanelSuppressClickRef.current = true;
    setActivityPanelResizeEdge(edge);

    const handlePointerMove = (moveEvent: PointerEvent) => {
      const drag = activityPanelResizeRef.current;
      if (!drag) {
        return;
      }
      const deltaX = moveEvent.clientX - drag.startX;
      const deltaY = drag.startY - moveEvent.clientY;
      const nextHeight = clampActivityPanelHeight(drag.startHeight + deltaY);
      const areaWidth = getActivityPanelAreaWidth();
      const fixedRight = drag.startLeft + drag.startWidth;
      const fixedLeft = drag.startLeft;
      let nextWidth: number;
      let nextLeft: number;
      if (drag.edge === 'left') {
        const maxWidth = Math.max(
          ACTIVITY_PANEL_MIN_WIDTH,
          Math.min(getActivityPanelWidthLimit(), fixedRight - ACTIVITY_PANEL_HORIZONTAL_EDGE_GAP)
        );
        nextWidth = Math.min(maxWidth, Math.max(ACTIVITY_PANEL_MIN_WIDTH, drag.startWidth - deltaX));
        nextLeft = fixedRight - nextWidth;
      } else {
        const maxWidth = Math.max(
          ACTIVITY_PANEL_MIN_WIDTH,
          Math.min(getActivityPanelWidthLimit(), areaWidth - ACTIVITY_PANEL_HORIZONTAL_EDGE_GAP - fixedLeft)
        );
        nextWidth = Math.min(maxWidth, Math.max(ACTIVITY_PANEL_MIN_WIDTH, drag.startWidth + deltaX));
        nextLeft = fixedLeft;
      }
      drag.currentWidth = nextWidth;
      drag.currentHeight = nextHeight;
      drag.currentLeft = nextLeft;
      setActivityPanelLeft(nextLeft);
      setActivityPanelWidth(nextWidth);
      setActivityPanelMaxHeight(nextHeight);
    };

    const cleanup = () => {
      const drag = activityPanelResizeRef.current;
      activityPanelResizeRef.current = null;
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', cleanup);
      window.removeEventListener('pointercancel', cleanup);
      setActivityPanelResizeEdge(null);

      if (!drag) {
        return;
      }
      const finalWidth = clampActivityPanelWidth(drag.currentWidth);
      const finalHeight = clampActivityPanelHeight(drag.currentHeight);
      const finalLeft = clampActivityPanelLeft(drag.currentLeft, finalWidth);
      const widthChanged = Math.abs(finalWidth - drag.startWidth) >= 0.5;
      const heightChanged = Math.abs(finalHeight - drag.startHeight) >= 0.5;
      const leftChanged = Math.abs(finalLeft - drag.startLeft) >= 0.5;
      if (widthChanged || heightChanged || leftChanged) {
        setActivityPanelLeft(finalLeft);
      }
      setActivityPanelWidth(widthChanged ? finalWidth : drag.startPreferredWidth);
      setActivityPanelMaxHeight(heightChanged ? finalHeight : drag.startPreferredHeight);

      if (widthChanged || heightChanged) {
        const persistPromise = window.electronAPI?.settings?.set?.({
          ui: {
            ...(widthChanged ? { activityPanelWidth: finalWidth } : {}),
            ...(heightChanged ? { activityPanelMaxHeight: finalHeight } : {}),
          },
        });
        if (persistPromise) {
          void persistPromise.catch((error) => {
            console.warn('[activityPanel] failed to persist:', error);
          });
        }
      }

      window.setTimeout(() => {
        activityPanelSuppressClickRef.current = false;
      }, 0);
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', cleanup);
    window.addEventListener('pointercancel', cleanup);
  };

  const shouldToggleActivityPanelFromHeaderClick = () => {
    if (activityPanelSuppressClickRef.current) {
      activityPanelSuppressClickRef.current = false;
      return false;
    }
    return true;
  };

  const handleActivityPanelHeaderPointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    if (
      !event.isPrimary
      || event.button !== 0
      || activityPanelHorizontalDragRef.current
      || activityPanelResizeRef.current
    ) {
      return;
    }

    const startLeft = getResolvedActivityPanelLeft();
    activityPanelHorizontalDragRef.current = {
      startX: event.clientX,
      startLeft,
      currentLeft: startLeft,
      dragged: false,
    };

    const handlePointerMove = (moveEvent: PointerEvent) => {
      const drag = activityPanelHorizontalDragRef.current;
      if (!drag) {
        return;
      }
      const deltaX = moveEvent.clientX - drag.startX;
      if (!drag.dragged && Math.abs(deltaX) < ACTIVITY_PANEL_HORIZONTAL_DRAG_THRESHOLD) {
        return;
      }
      if (!drag.dragged) {
        drag.dragged = true;
        activityPanelSuppressClickRef.current = true;
        setActivityPanelHorizontalDragging(true);
      }
      const nextLeft = clampActivityPanelLeft(
        drag.startLeft + deltaX,
        getActivityPanelMeasuredWidth()
      );
      drag.currentLeft = nextLeft;
      setActivityPanelLeft(nextLeft);
    };

    const cleanup = () => {
      const drag = activityPanelHorizontalDragRef.current;
      activityPanelHorizontalDragRef.current = null;
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', cleanup);
      window.removeEventListener('pointercancel', cleanup);

      if (!drag) {
        return;
      }
      if (drag.dragged) {
        const finalLeft = clampActivityPanelLeft(
          drag.currentLeft,
          getActivityPanelMeasuredWidth()
        );
        setActivityPanelLeft(finalLeft);
        setActivityPanelHorizontalDragging(false);
        window.setTimeout(() => {
          activityPanelSuppressClickRef.current = false;
        }, 0);
      }
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', cleanup);
    window.addEventListener('pointercancel', cleanup);
  };

  useEffect(() => {
    if (!activityPanelHorizontalDragging && !activityPanelResizeEdge) {
      return;
    }

    const body = document.body;
    const dragCursorClass = activityPanelHorizontalDragging
      ? 'op-global-cursor-grabbing'
      : activityPanelResizeEdge === 'left'
          ? 'op-global-cursor-nwse-resize'
          : 'op-global-cursor-nesw-resize';

    body.classList.add('select-none', dragCursorClass);

    return () => {
      body.classList.remove('select-none', dragCursorClass);
    };
  }, [activityPanelHorizontalDragging, activityPanelResizeEdge]);

  useEffect(() => {
    if (showActivityPanel) {
      return;
    }
    activityPanelHorizontalDragRef.current = null;
    activityPanelResizeRef.current = null;
    setActivityPanelHorizontalDragging(false);
    setActivityPanelResizeEdge(null);
    activityPanelSuppressClickRef.current = false;
    setActivityPanelLeft(null);
    setActivityPanelMeasuredWidth(0);
    setActivityPanelCoveredBottom(0);
  }, [showActivityPanel, showOpenBrainPage]);

  useLayoutEffect(() => {
    if (!showActivityPanel) {
      return;
    }
    const editorArea = getActivityPanelAreaElement();
    const stack = activityPanelStackRef.current;
    if (!editorArea || !stack) {
      return;
    }

    let frame = 0;
    const syncBounds = () => {
      frame = 0;
      const host = activityPanelHostRef.current;
      const stackRect = stack.getBoundingClientRect();
      const editorAreaRect = editorArea.getBoundingClientRect();
      const nextWidth = host?.getBoundingClientRect().width ?? stackRect.width;
      const nextCoveredBottom = Math.max(0, editorAreaRect.bottom - stackRect.top);
      setActivityPanelMeasuredWidth((prev) => (Math.abs(prev - nextWidth) < 0.5 ? prev : nextWidth));
      setActivityPanelCoveredBottom((prev) => (
        Math.abs(prev - nextCoveredBottom) < 0.5 ? prev : nextCoveredBottom
      ));
    };
    const scheduleSync = () => {
      if (frame) {
        cancelAnimationFrame(frame);
      }
      frame = requestAnimationFrame(syncBounds);
    };

    syncBounds();

    if (typeof ResizeObserver === 'undefined') {
      return () => {
        if (frame) {
          cancelAnimationFrame(frame);
        }
      };
    }

    const resizeObserver = new ResizeObserver(() => {
      scheduleSync();
    });
    resizeObserver.observe(editorArea);
    resizeObserver.observe(stack);
    if (activityPanelHostRef.current) {
      resizeObserver.observe(activityPanelHostRef.current);
    }
    window.addEventListener('resize', scheduleSync);

    return () => {
      if (frame) {
        cancelAnimationFrame(frame);
      }
      resizeObserver.disconnect();
      window.removeEventListener('resize', scheduleSync);
    };
  }, [
    liveOverlay.expanded,
    renderedActivityPanelMaxHeight,
    showActivityPanel,
    showOpenBrainPage,
  ]);

  useEffect(() => {
    if (selectedConversationTarget?.kind !== 'thread') {
      return;
    }
    const chatPath = getChatWorkspaceStore(activeTabId).getState().getTargetChatPath(selectedConversationTarget) || '';
    if (!chatPath || !workspaceConversationDocs.some((tab) => tab.filePath === chatPath)) {
      return;
    }
    void (async () => {
      primeLocalThreadMeta(chatPath, activeTabId);
      await getResolvedThreadMeta({ chatPath }, activeTabId).catch(() => null);
    })();
  }, [activeTabId, currentDir, selectedConversationTarget, workspaceConversationDocs]);

  useEffect(() => {
    const settingsApi = window.electronAPI?.settings;
    if (!settingsApi?.get) {
      setUiSettingsHydrated(true);
      return;
    }

    let disposed = false;
    const applyUiSettings = (settings?: UiSettingsSnapshot) => {
      if (
        disposed
        || sidebarResizeRef.current
        || conversationComposerDockResizeRef.current
        || pinnedFilePanelResizeRef.current
        || activityPanelHorizontalDragRef.current
        || activityPanelResizeRef.current
      ) {
        return;
      }
      setSidebarWidth(resolveSidebarWidth(settings?.ui?.sidebarWidth));
      const nextConversationComposerDockHeight = resolveConversationComposerDockHeight(
        settings?.ui?.conversationComposerDockHeight,
        getConversationComposerDockHeightLimit()
      );
      conversationComposerDockPreferredHeightRef.current = resolveConversationComposerDockHeight(
        settings?.ui?.conversationComposerDockHeight,
        Number.POSITIVE_INFINITY
      );
      setConversationComposerDockHeight(nextConversationComposerDockHeight);
      setPinnedFilePanelWidth(normalizePinnedFilePanelWidth(settings?.ui?.pinnedFilePanelWidth));
      setActivityPanelWidth(resolveActivityPanelWidth(settings?.ui?.activityPanelWidth));
      setActivityPanelMaxHeight(resolveActivityPanelMaxHeight(settings?.ui?.activityPanelMaxHeight));
    };

    settingsApi.get()
      .then((settings) => {
        applyUiSettings(settings as UiSettingsSnapshot);
      })
      .catch(() => {
        // Keep the in-memory default if settings cannot be loaded.
      })
      .finally(() => {
        if (!disposed) {
          setUiSettingsHydrated(true);
        }
      });

    const disposeSettingsChanged = settingsApi.onChanged?.((settings) => {
      applyUiSettings(settings as UiSettingsSnapshot);
    });

    return () => {
      disposed = true;
      disposeSettingsChanged?.();
    };
  }, []);

  useEffect(() => {
    if (
      !uiSettingsHydrated
      || !showActivityPanel
      || activityPanelWidth != null
      || defaultActivityPanelWidth == null
    ) {
      return;
    }

    setActivityPanelWidth(defaultActivityPanelWidth);
    const persistPromise = window.electronAPI?.settings?.set?.({
      ui: {
        activityPanelWidth: defaultActivityPanelWidth,
      },
    });
    if (!persistPromise) {
      return;
    }
    void persistPromise.catch((error) => {
      console.warn('[activityPanel] failed to persist default width:', error);
    });
  }, [
    activityPanelWidth,
    defaultActivityPanelWidth,
    showActivityPanel,
    uiSettingsHydrated,
  ]);

  useEffect(() => {
    const editorArea = editorAreaRef.current;
    if (!editorArea) {
      return;
    }

    let frame = 0;
    const scheduleClamp = () => {
      if (frame) {
        cancelAnimationFrame(frame);
      }
      frame = requestAnimationFrame(() => {
        frame = 0;
        if (conversationComposerDockResizeRef.current) {
          return;
        }
        setConversationComposerDockHeight((currentHeight) => {
          const nextHeight = resolveConversationComposerDockHeight(
            conversationComposerDockPreferredHeightRef.current,
            getConversationComposerDockHeightLimit()
          );
          return nextHeight === currentHeight ? currentHeight : nextHeight;
        });
      });
    };

    const resizeObserver = new ResizeObserver(() => {
      scheduleClamp();
    });
    resizeObserver.observe(editorArea);
    window.addEventListener('resize', scheduleClamp);

    return () => {
      if (frame) {
        cancelAnimationFrame(frame);
      }
      resizeObserver.disconnect();
      window.removeEventListener('resize', scheduleClamp);
    };
  }, []);

  useEffect(() => {
    return window.electronAPI?.runtimeBootstrap?.onChanged?.((payload) => {
      setRuntimeBootstrapState(payload);
    });
  }, []);

  // Initialize connection and default directory
  useEffect(() => {
    registerBuiltInEditors();

    const init = async () => {
      try {
        await authInit();
        const bootstrap = await window.electronAPI?.window?.getBootstrap();
        if (bootstrap) {
          setCurrentWindowId(bootstrap.windowId);
          setRuntimeBootstrapState(bootstrap.runtimeBootstrap || null);
        }
        if (bootstrap?.info) {
          setWindowActive(document.hasFocus() || bootstrap.info.active !== false);
        }

        const bootstrapPresentation = bootstrap?.info?.presentation ?? 'default';
        const shouldShowNewWindowLanding = bootstrapPresentation === 'newWindowLanding';
        const shouldRequireLogin = bootstrap?.info?.authRequired === true;
        setIsNewWindowLanding(shouldShowNewWindowLanding);
        setAppInitialized(true);

        if (shouldRequireLogin) {
          setLoginGatedStartup(true);
          showLoginRequiredDialog('chat');
          sessionSyncReadyRef.current = false;
          return;
        }

        const restoredSession = bootstrap?.workspaceTabsSession;
        const initialWorkspace = bootstrap?.initialWorkspace;
        if (!shouldShowNewWindowLanding) {
          if (restoredSession?.tabs?.length) {
            await restoreWorkspaceTabsSession(restoredSession);
          } else {
            const currentActiveTabId = useTabManagerStore.getState().activeTabId;
            const store = getWorkspaceStore(currentActiveTabId);
            const state = store.getState();
            if (initialWorkspace?.mode === 'remote' && initialWorkspace.remoteHost) {
              await connectRemoteForTab(currentActiveTabId, initialWorkspace.remoteHost);
            } else {
              state.connect();
              if (initialWorkspace?.workspacePath) {
                useTabManagerStore.getState().updateActiveTabWorkspace({ kind: 'local', workspacePath: initialWorkspace.workspacePath });
                state.setCurrentDir(initialWorkspace.workspacePath);
              } else if (!state.currentDir) {
                const defaultDir = await resolveDefaultLocalWorkspacePath();
                useTabManagerStore.getState().updateActiveTabWorkspace({ kind: 'local', workspacePath: defaultDir });
                state.setCurrentDir(defaultDir);
              }
            }
          }
        } else {
          const landingTabId = useTabManagerStore.getState().activeTabId;
          getWorkspaceStore(landingTabId).getState().connect();
        }

        if (!shouldShowNewWindowLanding && !restoredSession?.tabs?.length && window.electronAPI?.remote) {
          const currentActiveTabId = useTabManagerStore.getState().activeTabId;
          const currentActiveTab = useTabManagerStore.getState().tabs.find((tab) => tab.id === currentActiveTabId) || null;
          const currentStore = getWorkspaceStore(currentActiveTabId);
          if (!currentStore.getState().remoteSession) {
            const status = await window.electronAPI.remote.status(currentActiveTabId);
            if (status) {
              const restoredPath = bootstrap?.info?.workspacePath || currentActiveTab?.workspacePath || null;
              currentStore.getState().setRemoteSession(status);
              if (restoredPath) {
                currentStore.getState().setCurrentDir(restoredPath);
              }
              useTabManagerStore.getState().updateActiveTabWorkspace({
                kind: 'remote',
                remoteHost: bootstrap?.info?.remoteHost,
                workspacePath: restoredPath || undefined,
                label: status.hostLabel,
              });
              currentStore.getState().connect();
            }
          }
        }

        sessionSyncReadyRef.current = true;
        scheduleWorkspaceTabsSessionSync();

        if (!shouldShowNewWindowLanding) {
          const currentActiveTabId = useTabManagerStore.getState().activeTabId;
          getWorkspaceStore(currentActiveTabId).getState().openWelcomeTab();
        }

        const currentActiveTabId = useTabManagerStore.getState().activeTabId;
        if (bootstrap && bootstrap.info && bootstrap.info.active === false) {
          setWorkspaceActive(currentActiveTabId, false);
        } else {
          setWorkspaceActive(currentActiveTabId, true);
        }
      } catch (error) {
        console.error('Failed to initialize app:', error);
        setAppInitialized(true);
      }
    };

    void init();
    return () => {
      sessionSyncReadyRef.current = false;
      if (sessionSyncTimerRef.current) {
        window.clearTimeout(sessionSyncTimerRef.current);
        sessionSyncTimerRef.current = null;
      }
    };
  }, []);

  // Auth state initialization
  useEffect(() => {
    authInit();
  }, [authInit]);

  useEffect(() => {
    if (!loginGatedStartup || !authLoggedIn) {
      return;
    }
    hideLoginRequired();
    setLoginGatedStartup(false);
    const currentActiveTabId = useTabManagerStore.getState().activeTabId;
    const store = getWorkspaceStore(currentActiveTabId);
    store.getState().connect();
    store.getState().openWelcomeTab();
    setWorkspaceActive(currentActiveTabId, true);
    sessionSyncReadyRef.current = true;
    scheduleWorkspaceTabsSessionSync();
  }, [authLoggedIn, hideLoginRequired, loginGatedStartup]);

  const handleRuntimeBootstrapRetry = () => {
    void window.electronAPI?.runtimeBootstrap?.retry();
  };

  const handleRuntimeBootstrapQuit = () => {
    void window.electronAPI?.runtimeBootstrap?.quit();
  };

  useEffect(() => {
    if (!window.electronAPI?.window?.onActiveChanged) {
      return;
    }
    return window.electronAPI.window.onActiveChanged(({ active }) => {
      setWindowActive(active);
      if (active) {
        setWorkspaceActive(activeTabId, true);
      } else {
        const { tabs } = useTabManagerStore.getState();
        tabs.forEach((tab) => setWorkspaceActive(tab.id, false));
      }
    });
  }, [activeTabId, currentWindowId]);

  useEffect(() => {
    const previousTabId = prevActiveTabRef.current;
    if (previousTabId && previousTabId !== activeTabId) {
      setWorkspaceActive(previousTabId, false);
    }
    if (windowActive) {
      setWorkspaceActive(activeTabId, true);
    }
    prevActiveTabRef.current = activeTabId;
  }, [activeTabId, windowActive]);

  useEffect(() => {
    const unsubscribeTabs = useTabManagerStore.subscribe(() => {
      scheduleWorkspaceTabsSessionSync();
    });
    return () => {
      unsubscribeTabs();
    };
  }, []);

  useEffect(() => {
    const unsubscribers = workspaceTabs.map((tab) => {
      const workspaceStore = getWorkspaceStore(tab.id);
      const chatStore = getChatWorkspaceStore(tab.id);
      const unsubscribeWorkspace = workspaceStore.subscribe((state, previous) => {
        if (
          state.currentDir !== previous.currentDir
          || getChatTabSignature(
            state.documents.filter((tab) => tab.documentRole === 'conversation')
          ) !== getChatTabSignature(
            previous.documents.filter((tab) => tab.documentRole === 'conversation')
          )
        ) {
          scheduleWorkspaceTabsSessionSync();
        }
      });
      const unsubscribeChat = chatStore.subscribe((state, previous) => {
        const currentSelectedPath = state.selectedConversationTarget?.kind === 'thread'
          ? state.selectedConversationTarget.threadID
          : '';
        const previousSelectedPath = previous.selectedConversationTarget?.kind === 'thread'
          ? previous.selectedConversationTarget.threadID
          : '';
        if (currentSelectedPath !== previousSelectedPath) {
          scheduleWorkspaceTabsSessionSync();
        }
      });
      return () => {
        unsubscribeWorkspace();
        unsubscribeChat();
      };
    });
    scheduleWorkspaceTabsSessionSync();
    return () => {
      unsubscribers.forEach((unsubscribe) => unsubscribe());
    };
  }, [workspaceTabs.map((tab) => tab.id).join('|')]);

  const handleRemoteSelect = (host: SshHost) => {
    setShowRemoteModal(false);
    if (remoteModalTarget === 'newWindow') {
      window.electronAPI?.window?.createRemote(host);
      return;
    }
    if (remoteModalTarget === 'newTab') {
      const tab = createWorkspaceTab({ kind: 'remote', remoteHost: host });
      const store = getWorkspaceStore(tab.id);
      store.getState().setServerUrl('');
      store.getState().disconnect();
      store.getState().openWelcomeTab();
      void (async () => {
        await connectRemoteForTab(tab.id, host);
        void recordRemoteRecentForTab(tab.id, host);
      })();
      return;
    }
    if (isNewWindowLanding) {
      setIsNewWindowLanding(false);
    }
    void handleSwitchRemoteWorkspace(host);
  };

  if (!appInitialized) {
    return <div className="h-screen w-screen bg-editor-bg text-editor-fg" />;
  }

  if (isNewWindowLanding) {
    return (
      <div className="h-screen w-screen bg-editor-bg text-editor-fg">
        <NewWindowLanding
          openingFolder={openingLandingFolder}
          onOpenFolder={() => {
            void handleLandingOpenFolder();
          }}
          onConnectRemote={handleLandingConnectRemote}
        />
        <RemoteConnectModal
          open={showRemoteModal}
          onClose={() => setShowRemoteModal(false)}
          onSelect={handleRemoteSelect}
        />
        <DeviceCodeDialog
          open={!!deviceCode}
          userCode={deviceCode?.userCode || ''}
          verificationUri={deviceCode?.verificationUri || ''}
          expiresAt={deviceCode?.expiresAt || 0}
          onClose={clearDeviceCode}
        />
        <LoginRequiredDialog
          open={loginRequiredOpen}
          reason={loginRequiredReason}
          onCancel={hideLoginRequired}
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col h-screen bg-editor-bg text-editor-fg">
      {/* Title bar (drag region) */}
      <div className="h-[35px] flex items-center draggable ui-titlebar relative z-50">
          <div className="flex items-stretch gap-2 flex-1 min-w-0 h-full" style={{ paddingLeft: isMac ? 72 : undefined }}>
          <TitlebarLogoMenu
            currentWindowId={currentWindowId}
            windowActive={windowActive}
            onOpenSettings={openSettingsWorkspace}
          />

          <div className="flex-1 min-w-0">
            <WorkspaceTabsBar
              tabs={workspaceTabs}
              activeTabId={activeTabId}
              onSelectTab={handleSelectWorkspaceTab}
              onCloseTab={handleCloseWorkspaceTab}
              onNewFolder={handleNewFolderTab}
              onNewRemote={handleNewRemoteTab}
              onOpenRecentLocal={handleNewTabWithLocal}
              onOpenRecentRemote={handleNewTabWithRemote}
            />
          </div>
        </div>
      </div>

      {/* Main content */}
      <div className="flex flex-1 overflow-hidden">
        {/* Sidebar */}
        <div
          className="flex flex-col ui-sidebar shrink-0 overflow-hidden"
          style={{ width: `${sidebarWidth}px` }}
        >
          <Sidebar
            onSwitchLocal={handleSwitchLocalWorkspace}
            onOpenLocalNewTab={handleNewTabWithLocal}
            onOpenRemoteNewTab={handleNewTabWithRemote}
            onOpenLocalSwitchDirectory={handleOpenLocalSwitchDirectory}
            onOpenRemoteSwitchDirectory={handleOpenRemoteSwitchDirectory}
            onCreateOpenBrainSource={handleCreateOpenBrainSource}
            onBindOpenBrainSource={handleBindOpenBrainSource}
          />
        </div>
        <ResizeDivider
          direction="vertical"
          onResizeStart={handleSidebarResizeStart}
          activeColor="var(--color-highlight)"
          restingColor="var(--op-sidebar-resize-divider)"
          hoverDelayMs={RESIZE_DIVIDER_HOVER_DELAY_MS}
        />

        {/* Editor area */}
        <div className="relative flex-1 flex flex-col overflow-hidden" ref={editorAreaRef}>
          {!showSpecialPage && <EditorTabBar />}
          <div className="flex-1 min-h-0 flex overflow-hidden" ref={editorContentRowRef}>
            {showOpenBrainPage ? (
              <div className="relative min-w-0 flex-1" ref={openBrainPagePaneRef}>
                <OpenBrainPage onOpenWorkspace={handleOpenBrainWorkspace} onCreateSource={handleCreateOpenBrainSource} onBindSource={handleBindOpenBrainSource} />
                {renderActivityPanel()}
              </div>
            ) : showMessengerView ? (
              <div className="relative min-w-0 flex-1" ref={messengerConversationPaneRef}>
                {!selectedConversationTarget && (
                  <MessengerConversationSurface
                    selected={false}
                    loading={messengerSurfaceLoading}
                  />
                )}
                {renderActivityPanel()}
              </div>
            ) : (
              <>
            <div className="relative min-w-0 flex-1" ref={primaryEditorPaneRef}>
              <div
                className="absolute inset-0 flex flex-col overflow-hidden"
                style={primaryEditorViewportStyle}
              >
                {showPendingConversationPlaceholder ? (
                  <WelcomeEditor
                    chatPanelBottomInset={conversationComposerDockHeight + 1}
                    chatPanelOpen={composerVisible}
                  />
                ) : primaryEditorTab ? (
                  renderEditorForTab(primaryEditorTab)
                ) : pinnedTab ? (
                  <button
                    type="button"
                    className="flex h-full w-full items-center justify-center bg-transparent text-secondary-text transition-colors hover:text-prime-text"
                    onClick={handleReturnPinnedFileToEditor}
                    title={`Return ${pinnedTab.title} to editor`}
                    aria-label={`Return ${pinnedTab.title} to editor`}
                  >
                    <div className="text-center">
                      <div className="mx-auto mb-3 inline-flex h-10 w-10 items-center justify-center rounded-md border border-border bg-hover-bg/50">
                        <svg
                          width="18"
                          height="18"
                          viewBox="0 0 16 16"
                          fill="none"
                          xmlns="http://www.w3.org/2000/svg"
                          aria-hidden="true"
                          className="text-secondary-text"
                        >
                          <path
                            d="M5.3 2.8h5.4L9.6 6.4l2.1 2.1v1.4H8.6L8 14H6.9l-.6-4.1H3.2V8.5l2.1-2.1-1-3.6Z"
                            stroke="currentColor"
                            strokeWidth="1.35"
                            strokeLinecap="round"
                            strokeLinejoin="round"
                          />
                        </svg>
                      </div>
                      <p className="text-base text-prime-text">Pinned file</p>
                      <p className="mt-1 text-sm">{pinnedTab.title}</p>
                    </div>
                  </button>
                ) : (
                  <WelcomeEditor
                    chatPanelBottomInset={conversationComposerDockHeight + 1}
                    chatPanelOpen={composerVisible}
                  />
                )}
              </div>
              {renderActivityPanel()}
            </div>
            {showPinnedFilePane && pinnedTab && (
              <>
                <ResizeDivider
                  direction="vertical"
                  onResizeStart={handlePinnedFilePanelResizeStart}
                  activeColor="var(--color-highlight)"
                  hoverDelayMs={RESIZE_DIVIDER_HOVER_DELAY_MS}
                />
                <div
                  className="op-pinned-conversation-file op-md-outline-shell is-expanded relative shrink-0 min-w-0 h-full min-h-0 flex flex-col overflow-hidden"
                  ref={pinnedEditorPaneRef}
                  style={{
                    width: `${renderedPinnedFilePanelWidth}px`,
                  }}
                >
                  {renderEditorForTab(pinnedTab, {
                    pinned: true,
                    autoFocus: false,
                    suppressOutlineToggle: true,
                    compactMarkdown: true,
                    textOffsetEnabled: false,
                  })}
                </div>
              </>
            )}
              </>
            )}
          </div>
          {showComposerDock && (
            <>
              <ResizeDivider
                direction="horizontal"
                onResizeStart={handleConversationComposerDockResizeStart}
                activeColor="var(--color-highlight)"
                hoverDelayMs={RESIZE_DIVIDER_HOVER_DELAY_MS}
              />
              {/* Keep upward-opening conversation menus above the resize divider and editor-local overlays. */}
              <div
                className="relative z-[40] shrink-0 min-h-0 overflow-visible"
                style={{ height: `${conversationComposerDockHeight}px` }}
              >
                <ConversationComposerDock showTopBorder={false} />
              </div>
            </>
          )}
        </div>
      </div>

      <div className="ui-statusbar flex h-7 items-center gap-3 border-t border-border bg-editor-bg px-3 text-secondary-text">
        <div className="flex min-w-0 flex-1 items-center gap-2 overflow-hidden">
          <span className={displayConnectionState === 'connected' ? 'text-health-text' : 'text-accent'}>
            {getConnectionStateText(displayConnectionState)}
          </span>
          <span
            className="truncate max-w-[40vw]"
            title={statusBarPathDisplay.fullPath || statusBarPathDisplay.primary}
          >
            {statusBarPathDisplay.primary}
          </span>
          {statusBarPathDisplay.suffix ? (
            <>
              <span className="shrink-0 text-tertiary-text">/</span>
              <span
                className="truncate max-w-[34vw]"
                title={statusBarPathDisplay.fullPath || statusBarPathDisplay.suffix}
              >
                {statusBarPathDisplay.suffix}
              </span>
            </>
          ) : null}
          {remoteSession && (
            <span className="truncate max-w-[24vw]">Remote: {remoteSession.hostLabel}</span>
          )}
          {remoteError && <span className="truncate text-accent">{remoteError}</span>}
        </div>
        <div className="ml-4 flex shrink-0 items-center gap-2">
          {editorId === 'markdown' ? <EditorTextZoomStatusControl /> : null}
          <WindowZoomStatusControl />
          <WorkspaceSyncStatusControl />
          <BranchStatusControl />
        </div>
      </div>

      <LocalRuntimeBootstrapOverlay
        state={runtimeBootstrapState}
        onRetry={handleRuntimeBootstrapRetry}
        onQuit={handleRuntimeBootstrapQuit}
      />

      <RemoteConnectModal
        open={showRemoteModal}
        onClose={() => setShowRemoteModal(false)}
        onSelect={handleRemoteSelect}
      />

      <DirectoryPickerDialog
        open={!!directoryPickerRequest}
        title={directoryPickerConfig?.title || 'Select Directory'}
        subtitle={directoryPickerConfig?.subtitle || null}
        defaultPath={directoryPickerConfig?.defaultPath || null}
        currentPath={directoryPickerConfig?.currentPath || null}
        recentPaths={directoryPickerConfig?.recentPaths || []}
        submitLabel={directoryPickerConfig?.submitLabel || 'OK'}
        allowCreate
        asyncSelect={directoryPickerRequest?.source === 'create-openbrain-source'}
        provider={directoryPickerConfig?.provider || null}
        onClose={closeDirectoryPicker}
        onSelect={handleDirectoryPickerSelect}
      />

      <DirectoryPickerDialog
        open={!!saveFileRequest}
        mode="saveFile"
        title="Save File"
        defaultPath={saveFileRequest?.defaultDir || currentDir || null}
        defaultFileName={saveFileRequest?.defaultFileName || ''}
        filters={saveFileRequest?.filters}
        submitLabel="Save"
        allowCreate
        provider={saveFileProvider}
        onClose={() => resolveSaveFileDialog(null)}
        onSelect={async (path) => { resolveSaveFileDialog(path); }}
      />

      <DeviceCodeDialog
        open={!!deviceCode}
        userCode={deviceCode?.userCode || ''}
        verificationUri={deviceCode?.verificationUri || ''}
        expiresAt={deviceCode?.expiresAt || 0}
        onClose={clearDeviceCode}
      />

      <UnsavedTabCloseDialog
        open={!!pendingDirtyTabClose}
        tabTitle={pendingDirtyTabClose?.title || null}
        onCancel={dismissPendingDirtyTabClose}
        onConfirm={confirmPendingDirtyTabClose}
      />

      <BillingReminderDialog
        open={billingReminderOpen}
        kind={billingReminderKind}
        onCancel={hideBillingReminder}
      />

      <LoginRequiredDialog
        open={loginRequiredOpen}
        reason={loginRequiredReason}
        onCancel={hideLoginRequired}
      />

      {!showSpecialPage ? <WorkspaceAgentOnboarding /> : null}

      <ToastViewport />
    </div>
  );
}
