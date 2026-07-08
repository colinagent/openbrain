import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';

import { useAppStore } from '../../store/appStore';
import { useAuthStore } from '../../store/authStore';
import { useChatWorkspaceStore, type GBrainQueryScope } from '../../store/chatWorkspaceStore';
import { canManageOpenBrainSource, useOpenBrainStore, type LocalOpenBrainWorkspace, type PublicBrainSource } from '../../store/openBrainStore';
import { showLoginRequiredDialog } from '../../store/loginRequiredStore';
import { useToastStore } from '../../store/toastStore';
import { resolveOpenBrainPublicBrainSources, type OpenBrainPublicBrainProfile, type OpenBrainSourceShare } from '../../services/openBrainService';
import type { ChatAgentTarget } from '../../utils/chatAgentTarget';
import { PopupMenu, PopupMenuItem, PopupMenuSeparator } from '../PopupMenu';
import { ChatLineIcon, RefreshIcon, TrashIcon } from '../Icons';
import { IconButton } from '../IconButton';
import { OPENBRAIN_GRAPH_CAPSULE } from '../staticGlassCapsule';
import {
  CloudSourceActionMenuItems,
  HardDeleteCloudSourceDialog,
  sourceActionSuccessMessage,
  type CloudSourceActionPayload,
} from './CloudSourceActions';
import { OpenBrainFlowGraph, type OpenBrainFlowSourceContextMenuEvent, type OpenBrainRenderedFlowNode } from './OpenBrainFlowGraph';
import { MyGBrainAddPopover } from './MyGBrainAddPopover';
import { SourceShareDialog } from './SourceShareDialog';
import {
  buildOpenBrainFlow,
  DEMO_ORBIT_NODES,
  TEAM_BRAIN_CLUSTER_PEER_IDS,
  type OpenBrainFlowNodeData,
  type OpenBrainFlowWorkspace,
} from './openBrainFlow';
import { resolveOpenBrainSourceDisplayState } from './openBrainSourceDisplay';

type OpenBrainPageProps = {
  onOpenWorkspace: (workspace: LocalOpenBrainWorkspace) => Promise<void>;
  onCreateSource: () => Promise<void>;
  onBindSource: (workspace: LocalOpenBrainWorkspace) => Promise<void>;
};

type OpenBrainInteractiveNode = OpenBrainFlowNodeData & { id: string };

type SourceContextMenuState = {
  source: LocalOpenBrainWorkspace;
  x: number;
  y: number;
};

type PeerContextMenuState = {
  node: OpenBrainInteractiveNode;
  x: number;
  y: number;
};

type PublicBrainScopeBrain = {
  ownerUID: string;
  name: string;
  username?: string;
  sources: PublicBrainSource[];
};

const SOURCE_CONTEXT_MENU_WIDTH = 288;
const SOURCE_CONTEXT_MENU_HEIGHT = 286;
const SOURCE_CONTEXT_MENU_MARGIN = 8;
const GBRAIN_AGENT_ID = 'agent-gbrain';

function sourceContextMenuPosition(event: OpenBrainFlowSourceContextMenuEvent): { x: number; y: number } {
  const pointerEvent = event as React.MouseEvent<Element> | React.PointerEvent<Element>;
  const hasPointer = Number.isFinite(pointerEvent.clientX) && Number.isFinite(pointerEvent.clientY);
  const rawX = hasPointer ? pointerEvent.clientX : SOURCE_CONTEXT_MENU_MARGIN;
  const rawY = hasPointer ? pointerEvent.clientY : SOURCE_CONTEXT_MENU_MARGIN;
  const maxX = Math.max(SOURCE_CONTEXT_MENU_MARGIN, window.innerWidth - SOURCE_CONTEXT_MENU_WIDTH - SOURCE_CONTEXT_MENU_MARGIN);
  const maxY = Math.max(SOURCE_CONTEXT_MENU_MARGIN, window.innerHeight - SOURCE_CONTEXT_MENU_HEIGHT - SOURCE_CONTEXT_MENU_MARGIN);
  return {
    x: Math.min(Math.max(SOURCE_CONTEXT_MENU_MARGIN, rawX), maxX),
    y: Math.min(Math.max(SOURCE_CONTEXT_MENU_MARGIN, rawY), maxY),
  };
}

function isSameSource(a: LocalOpenBrainWorkspace, b: LocalOpenBrainWorkspace): boolean {
  if (a.sourceID && b.sourceID && a.sourceID === b.sourceID) {
    return true;
  }
  return Boolean(a.workspaceID && b.workspaceID && a.workspaceID === b.workspaceID);
}

function toGraphWorkspace(workspace: LocalOpenBrainWorkspace): OpenBrainFlowWorkspace {
  return {
    sourceID: workspace.sourceID,
    workspaceID: workspace.workspaceID || workspace.sourceID,
    orgID: workspace.orgID,
    name: workspace.name,
    path: workspace.path,
    instanceID: workspace.instanceID,
    runtimeReachable: workspace.runtimeReachable,
    bindingStatus: workspace.bindingStatus,
    bindingReason: workspace.bindingReason,
    disabledQueries: workspace.disabledQueries === true,
    publicAccess: workspace.publicAccess === true,
    effectivePermission: workspace.effectivePermission,
    canMutateSource: workspace.canMutateSource,
    publicOwnerUID: workspace.publicOwnerUID,
    bindingMode: workspace.bindingMode,
    updatedAt: workspace.updatedAt,
  };
}

function stableOpenBrainWorkspaceID(workspace: LocalOpenBrainWorkspace): string {
  const explicit = (workspace.sourceID || workspace.workspaceID || '').trim();
  if (explicit) {
    return explicit;
  }
  if (!workspace.path) {
    return 'workspace-unknown';
  }
  let hash = 2166136261;
  const normalizedPath = workspace.path.replace(/\/+$/, '');
  for (let i = 0; i < normalizedPath.length; i += 1) {
    hash ^= normalizedPath.charCodeAt(i);
    hash = Math.imul(hash, 16777619);
  }
  return `workspace-${(hash >>> 0).toString(16).padStart(8, '0')}`;
}

function publicBrainContextLabel(name: string | undefined, fallback: string | undefined): string {
  const base = (name || fallback || 'Public Brain').trim() || 'Public Brain';
  const lower = base.toLowerCase();
  if (lower.endsWith("'s brain") || lower.endsWith(' brain')) {
    return base;
  }
  return `${base}'s Brain`;
}

function gbrainSourceScopeForWorkspace(workspace: LocalOpenBrainWorkspace): GBrainQueryScope {
  const sourceID = stableOpenBrainWorkspaceID(workspace);
  return {
    kind: 'source',
    label: workspace.name || workspace.path || sourceID,
    sourceID,
    workspaceID: workspace.workspaceID || sourceID,
    orgID: workspace.orgID,
  };
}

function publicBrainSourcesForScope(sources: PublicBrainSource[] | undefined) {
  return (sources || [])
    .map((source) => ({
      sourceID: (source.sourceID || '').trim(),
      name: (source.name || '').trim() || undefined,
      workspaceID: (source.workspaceID || source.sourceID || '').trim() || undefined,
      orgID: (source.orgID || '').trim() || undefined,
    }))
    .filter((source) => source.sourceID);
}

function publicBrainOwnerUIDForNode(node: OpenBrainInteractiveNode): string {
  return (node.ownerUID || (node.id.startsWith('public:') ? node.id.slice('public:'.length) : '')).trim();
}

function sourceLinkKeyForWorkspace(workspace: LocalOpenBrainWorkspace): string {
  return (workspace.sourceID || workspace.workspaceID || workspace.path || '').trim();
}

function publicBrainScopeForBrain(brain: PublicBrainScopeBrain, fallbackLabel?: string): GBrainQueryScope | null {
  const ownerUID = brain.ownerUID.trim();
  if (!ownerUID) {
    return null;
  }
  const sourcesForScope = publicBrainSourcesForScope(brain.sources);
  if (sourcesForScope.length === 0) {
    return null;
  }
  return {
    kind: 'publicBrain',
    label: publicBrainContextLabel(fallbackLabel || brain.name, ownerUID),
    ownerUID,
    username: (brain.username || '').trim() || undefined,
    sources: sourcesForScope,
  };
}

async function waitForWorkspaceConnection(timeoutMs = 5000): Promise<boolean> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (useAppStore.getState().connectionState === 'connected') {
      return true;
    }
    await new Promise((resolve) => window.setTimeout(resolve, 50));
  }
  return useAppStore.getState().connectionState === 'connected';
}

export const OpenBrainPage: React.FC<OpenBrainPageProps> = ({ onOpenWorkspace, onCreateSource, onBindSource }) => {
  const { t } = useTranslation('menu');
  const pushToast = useToastStore((state) => state.pushToast);
  const sources = useOpenBrainStore((state) => state.sources);
  const provider = useOpenBrainStore((state) => state.provider);
  const cloudReady = useOpenBrainStore((state) => state.cloudReady);
  const providerStatusChecked = useOpenBrainStore((state) => state.providerStatusChecked);
  const githubCheckError = useOpenBrainStore((state) => state.githubCheckError);
  const publicBrains = useOpenBrainStore((state) => state.publicBrains);
  const peerLinks = useOpenBrainStore((state) => state.peerLinks);
  const togglePeerLink = useOpenBrainStore((state) => state.togglePeerLink);
  const setSourceLinked = useOpenBrainStore((state) => state.setSourceLinked);
  const isSourceLinked = useOpenBrainStore((state) => state.isSourceLinked);
  const subscribePublicBrain = useOpenBrainStore((state) => state.subscribePublicBrain);
  const unsubscribePublicBrain = useOpenBrainStore((state) => state.unsubscribePublicBrain);
  const listPublicBrainDirectory = useOpenBrainStore((state) => state.listPublicBrainDirectory);
  const getSourceShare = useOpenBrainStore((state) => state.getSourceShare);
  const shareSourceWithUser = useOpenBrainStore((state) => state.shareSourceWithUser);
  const revokeSourceUserShare = useOpenBrainStore((state) => state.revokeSourceUserShare);
  const setSourcePublic = useOpenBrainStore((state) => state.setSourcePublic);
  const revokeSourcePublic = useOpenBrainStore((state) => state.revokeSourcePublic);
  const getPublicBrainProfile = useOpenBrainStore((state) => state.getPublicBrainProfile);
  const updatePublicBrainProfile = useOpenBrainStore((state) => state.updatePublicBrainProfile);
  const applySourceAction = useOpenBrainStore((state) => state.applySourceAction);
  const loading = useOpenBrainStore((state) => state.loading);
  const refreshing = useOpenBrainStore((state) => state.refreshing);
  const error = useOpenBrainStore((state) => state.error);
  const hydrateCachedOpenBrains = useOpenBrainStore((state) => state.hydrateCachedSources);
  const refreshOpenBrainsInBackground = useOpenBrainStore((state) => state.refreshInBackground);
  const refreshProviderStatus = useOpenBrainStore((state) => state.refreshProviderStatus);
  const loggedIn = useAuthStore((state) => state.loggedIn);
  const authInitialized = useAuthStore((state) => state.initialized);
  const authRevision = useAuthStore((state) => state.authRevision);
  const startLogin = useAuthStore((state) => state.startLogin);
  const [onboardingBusy, setOnboardingBusy] = useState(false);
  const [addPopoverOpen, setAddPopoverOpen] = useState(false);
  const [demoSourceLinks] = useState<Record<string, boolean>>(() => (
    Object.fromEntries(
      DEMO_ORBIT_NODES
        .filter((node) => node.kind === 'source')
        .map((node) => [`workspace:${node.id}`, node.defaultLinked]),
    )
  ));
  const [teamBrainClusterVisible, setTeamBrainClusterVisible] = useState(true);
  const [sourceContextMenu, setSourceContextMenu] = useState<SourceContextMenuState | null>(null);
  const [peerContextMenu, setPeerContextMenu] = useState<PeerContextMenuState | null>(null);
  const [mutatingSourceID, setMutatingSourceID] = useState<string | null>(null);
  const [hardDeleteDialogSource, setHardDeleteDialogSource] = useState<LocalOpenBrainWorkspace | null>(null);
  const [shareDialogSource, setShareDialogSource] = useState<LocalOpenBrainWorkspace | null>(null);
  const [shareDialogView, setShareDialogView] = useState<OpenBrainSourceShare | null>(null);
  const [shareDialogProfile, setShareDialogProfile] = useState<OpenBrainPublicBrainProfile | null>(null);
  const [shareDialogBusy, setShareDialogBusy] = useState(false);
  const [shareDialogError, setShareDialogError] = useState<string | null>(null);
  const [sourceActionError, setSourceActionError] = useState<string | null>(null);
  const viewportRef = useRef<HTMLDivElement | null>(null);
  const coreRef = useRef<HTMLButtonElement | null>(null);
  const providerStatusRefreshInFlightRef = useRef(false);
  const openingPublicBrainOwnerUIDRef = useRef<string | null>(null);
  const lastAuthRevisionRef = useRef(authRevision);

  useEffect(() => {
    const authChanged = lastAuthRevisionRef.current !== authRevision;
    lastAuthRevisionRef.current = authRevision;
    // Only hydrate from the on-disk snapshot when the store is empty; otherwise the
    // in-memory Zustand state is already authoritative and a fresh background
    // refresh (TTL-gated) is enough.
    if (useOpenBrainStore.getState().sources.length === 0) {
      void hydrateCachedOpenBrains().catch(() => {});
    }
    void refreshOpenBrainsInBackground(undefined, { force: authChanged }).catch(() => {});
  }, [authRevision, hydrateCachedOpenBrains, refreshOpenBrainsInBackground]);

  useEffect(() => {
    if (!loggedIn || provider !== 'cloud' || (providerStatusChecked && cloudReady) || loading) {
      return;
    }
    const onFocus = () => {
      void refreshOpenBrainsInBackground(undefined, { force: true }).catch(() => {});
    };
    window.addEventListener('focus', onFocus);
    return () => window.removeEventListener('focus', onFocus);
  }, [cloudReady, loading, loggedIn, provider, providerStatusChecked, refreshOpenBrainsInBackground]);

  useEffect(() => {
    if (!loggedIn || provider !== 'cloud' || (providerStatusChecked && cloudReady)) {
      return;
    }

    let canceled = false;
    const refreshCloudReadiness = async () => {
      if (providerStatusRefreshInFlightRef.current) {
        return;
      }
      providerStatusRefreshInFlightRef.current = true;
      try {
        const status = await refreshProviderStatus();
        if (!canceled && status.cloudReady === true) {
          void refreshOpenBrainsInBackground(undefined, { force: true }).catch(() => {});
        }
      } catch {
        // Surface provider check failures through githubCheckError in the store.
      } finally {
        providerStatusRefreshInFlightRef.current = false;
      }
    };

    void refreshCloudReadiness();
    const intervalID = window.setInterval(() => {
      void refreshCloudReadiness();
    }, 2500);

    return () => {
      canceled = true;
      window.clearInterval(intervalID);
    };
  }, [cloudReady, loggedIn, provider, providerStatusChecked, refreshOpenBrainsInBackground, refreshProviderStatus]);

  useEffect(() => {
    if (!sourceContextMenu && !peerContextMenu) {
      return;
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setSourceContextMenu(null);
        setPeerContextMenu(null);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [peerContextMenu, sourceContextMenu]);

  useEffect(() => {
    if (!sourceContextMenu) {
      return;
    }
    if (!sources.some((source) => isSameSource(source, sourceContextMenu.source))) {
      setSourceContextMenu(null);
    }
  }, [sourceContextMenu, sources]);

  const handleRefreshOpenBrainGraph = useCallback(() => {
    void refreshOpenBrainsInBackground(undefined, { force: true }).catch((err) => {
      pushToast(err instanceof Error ? err.message : 'Failed to refresh OpenBrain graph.');
    });
  }, [pushToast, refreshOpenBrainsInBackground]);

  const readinessKnown = authInitialized && (provider !== 'cloud' || providerStatusChecked);
  const needsGitHubConnection = readinessKnown && loggedIn && provider === 'cloud' && !cloudReady;
  const showDemoGraph = authInitialized && (!loggedIn || needsGitHubConnection);
  const showOnboardingOverlay = readinessKnown && (!loggedIn || needsGitHubConnection || sources.length === 0);
  const graphFlowKey = `openbrain-flow-${showOnboardingOverlay ? 'onboarding' : 'ready'}`;
  const onboardingStep = !loggedIn
    ? 'login'
    : needsGitHubConnection
      ? 'connect_github'
      : 'add_source';
  const graphPublicBrains = useMemo(
    () => publicBrains
      .filter((brain) => brain.activeSourceCount > 0)
      .map((brain) => ({
        ownerUID: brain.ownerUID,
        name: brain.name,
        username: brain.username,
        ownerInitial: brain.ownerInitial,
        avatar: brain.avatar,
        activeSourceCount: brain.activeSourceCount,
        colorKey: brain.colorKey,
        sources: brain.sources,
      })),
    [publicBrains],
  );
  const flow = useMemo(
    () => (showDemoGraph
      ? buildOpenBrainFlow([], peerLinks, { demoMode: true, teamBrainClusterVisible })
      : buildOpenBrainFlow(sources.map(toGraphWorkspace), peerLinks, { publicBrains: graphPublicBrains })),
    [graphPublicBrains, peerLinks, showDemoGraph, sources, teamBrainClusterVisible],
  );
  const workspaceByPath = useMemo(
    () => new Map(sources.filter((workspace) => workspace.path).map((workspace) => [workspace.path as string, workspace])),
    [sources],
  );
  const workspaceByID = useMemo(
    () => new Map(sources.flatMap((workspace) => {
      const entries: Array<[string, LocalOpenBrainWorkspace]> = [];
      if (workspace.sourceID) {
        entries.push([workspace.sourceID, workspace]);
        if (workspace.instanceID) {
          entries.push([`${workspace.instanceID}:${workspace.sourceID}`, workspace]);
        }
      }
      if (workspace.workspaceID && workspace.workspaceID !== workspace.sourceID) {
        entries.push([workspace.workspaceID, workspace]);
        if (workspace.instanceID) {
          entries.push([`${workspace.instanceID}:${workspace.workspaceID}`, workspace]);
        }
      }
      return entries;
    })),
    [sources],
  );

  const prepareWorkspaceRuntimeForChat = async (workspace: LocalOpenBrainWorkspace): Promise<string | null> => {
    const normalizedPath = (workspace.path || '').trim();
    if (!normalizedPath) {
      return null;
    }
    const activeState = useAppStore.getState();
    const activeInstanceID = (activeState.instanceID || '').trim();
    const workspaceInstanceID = (workspace.instanceID || '').trim();
    const sameRuntime = workspaceInstanceID
      ? activeInstanceID === workspaceInstanceID || (!activeInstanceID && workspaceInstanceID === 'local:default')
      : true;
    if (!sameRuntime || activeState.currentDir !== normalizedPath) {
      await onOpenWorkspace(workspace);
    }
    await waitForWorkspaceConnection();
    const appState = useAppStore.getState();
    await appState.ensureDirectory(normalizedPath);
    await appState.fetchDirAgentsInfo(normalizedPath);
    return normalizedPath;
  };

  const preparePathForChat = async (workspace: LocalOpenBrainWorkspace): Promise<ChatAgentTarget | null> => {
    const normalizedPath = await prepareWorkspaceRuntimeForChat(workspace);
    if (!normalizedPath) {
      return null;
    }
    const appState = useAppStore.getState();
    const boundAgent = appState.getChatAgentForCwd(normalizedPath);
    if (boundAgent) {
      return boundAgent;
    }
    await appState.refreshAgentNodes({ force: true });
    return appState.getDefaultOpenBrainForCwd(normalizedPath);
  };

  const resolveGBrainAgentForCwd = async (agentCwd: string): Promise<ChatAgentTarget | null> => {
    const normalizedCwd = agentCwd.trim();
    let appState = useAppStore.getState();
    if (!appState.getAgentOpCode(GBRAIN_AGENT_ID)) {
      await appState.refreshAgentNodes({ force: true });
      appState = useAppStore.getState();
    }
    if (!appState.getAgentOpCode(GBRAIN_AGENT_ID)) {
      return null;
    }
    const resolved = appState.resolveAgentByID(GBRAIN_AGENT_ID);
    return {
      agentID: GBRAIN_AGENT_ID,
      agentName: resolved?.name?.trim() || 'gbrain',
      agentCwd: normalizedCwd,
    };
  };

  const prepareWorkspaceForChat = async (workspace: LocalOpenBrainWorkspace): Promise<ChatAgentTarget | null> => {
    if (!workspace.path) {
      return prepareBrainForChat();
    }
    return preparePathForChat(workspace);
  };

  const prepareBrainForChat = async (): Promise<ChatAgentTarget | null> => {
    await waitForWorkspaceConnection();
    return resolveGBrainAgentForCwd('');
  };

  const startOpenBrainChat = async (
    workspace: LocalOpenBrainWorkspace | null,
    options?: { scope?: GBrainQueryScope },
  ) => {
    const agentInfo: ChatAgentTarget | null = workspace
      ? await prepareWorkspaceForChat(workspace)
      : await prepareBrainForChat();
    if (!agentInfo) {
      pushToast(workspace ? 'Default OpenBrain agent is not available yet.' : 'GBrain agent is not available yet.');
      return;
    }

    const chatState = useChatWorkspaceStore.getState();
    chatState.setInputMode('chat');
    chatState.setAgentInfo(agentInfo.agentID, agentInfo.agentName ?? null, agentInfo.agentCwd);
    chatState.setGBrainQueryScope(options?.scope || null);
    chatState.showComposer();
    chatState.createPendingConversation();
    chatState.setAgentForSelectedTarget(agentInfo);
    chatState.requestComposerFocus();
  };

  const disconnectPublicBrain = useCallback((node: OpenBrainInteractiveNode) => {
    if (showDemoGraph) {
      togglePeerLink(node.id);
      return;
    }
    const ownerUID = node.id.startsWith('public:')
      ? node.id.slice('public:'.length).trim()
      : '';
    if (!ownerUID) {
      togglePeerLink(node.id);
      return;
    }
    void unsubscribePublicBrain(ownerUID)
      .then(() => pushToast('Public brain disconnected.'))
      .catch((error) => pushToast(error instanceof Error ? error.message : 'Failed to disconnect public brain.'));
  }, [pushToast, showDemoGraph, togglePeerLink, unsubscribePublicBrain]);

  const workspaceForNode = useCallback((node: OpenBrainInteractiveNode): LocalOpenBrainWorkspace | null => (
    (node.instanceID && node.sourceID ? workspaceByID.get(`${node.instanceID}:${node.sourceID}`) : null)
    || (node.instanceID && node.workspaceID ? workspaceByID.get(`${node.instanceID}:${node.workspaceID}`) : null)
    || (node.sourceID ? workspaceByID.get(node.sourceID) : null)
    || (node.workspaceID ? workspaceByID.get(node.workspaceID) : null)
    || (node.path ? workspaceByPath.get(node.path) : null)
    || null
  ), [workspaceByID, workspaceByPath]);

  const startSourceChat = useCallback((workspace: LocalOpenBrainWorkspace | null) => {
    if (!workspace) {
      pushToast('This source is not available on this device.');
      return;
    }
    void startOpenBrainChat(workspace, { scope: gbrainSourceScopeForWorkspace(workspace) });
  }, [pushToast, startOpenBrainChat]);

  const resolvePublicBrainScopeForNode = useCallback(async (node: OpenBrainInteractiveNode): Promise<GBrainQueryScope | null> => {
    const ownerUID = publicBrainOwnerUIDForNode(node);
    if (!ownerUID) {
      return null;
    }
    const sources = await resolveOpenBrainPublicBrainSources(ownerUID);
    return publicBrainScopeForBrain({
      ownerUID,
      name: node.label || ownerUID,
      username: node.username,
      sources,
    }, node.label);
  }, []);

  const startPublicBrainChat = useCallback((node: OpenBrainInteractiveNode) => {
    const ownerUID = publicBrainOwnerUIDForNode(node);
    if (!ownerUID) {
      pushToast('This public brain is not available.');
      return;
    }
    if (openingPublicBrainOwnerUIDRef.current === ownerUID) {
      return;
    }
    openingPublicBrainOwnerUIDRef.current = ownerUID;
    void (async () => {
      try {
        const scope = await resolvePublicBrainScopeForNode(node);
        if (!scope) {
          pushToast('This public brain does not have available public sources yet.');
          return;
        }
        await startOpenBrainChat(null, { scope });
      } catch (error) {
        pushToast(error instanceof Error ? error.message : 'Failed to open public brain chat.');
      } finally {
        if (openingPublicBrainOwnerUIDRef.current === ownerUID) {
          openingPublicBrainOwnerUIDRef.current = null;
        }
      }
    })();
  }, [pushToast, resolvePublicBrainScopeForNode, startOpenBrainChat]);

  const loadSourceShareDialog = useCallback(async (workspace: LocalOpenBrainWorkspace) => {
    const [share, profile] = await Promise.all([
      getSourceShare(workspace),
      getPublicBrainProfile(),
    ]);
    setShareDialogView(share);
    setShareDialogProfile(profile);
  }, [getPublicBrainProfile, getSourceShare]);

  const handleShareSource = useCallback((workspace: LocalOpenBrainWorkspace) => {
    setSourceContextMenu(null);
    if (!canManageOpenBrainSource(workspace)) {
      pushToast('This source is read-only on this brain.');
      return;
    }
    if (!workspace.orgID || workspace.orgID === 'local') {
      pushToast('Public sharing is available for OpenBrain Cloud sources only.');
      return;
    }
    setShareDialogSource(workspace);
    setShareDialogView(null);
    setShareDialogProfile(null);
    setShareDialogError(null);
    setShareDialogBusy(true);
    void loadSourceShareDialog(workspace)
      .catch((error) => setShareDialogError((error as Error).message || 'Failed to load source sharing.'))
      .finally(() => setShareDialogBusy(false));
  }, [loadSourceShareDialog, pushToast]);

  const refreshSourceShareDialog = useCallback(async () => {
    if (!shareDialogSource) {
      return;
    }
    await loadSourceShareDialog(shareDialogSource);
  }, [loadSourceShareDialog, shareDialogSource]);

  const runSourceShareDialogAction = useCallback(async (action: () => Promise<void>, successMessage: string) => {
    if (!shareDialogSource || shareDialogBusy) {
      return;
    }
    setShareDialogBusy(true);
    setShareDialogError(null);
    try {
      await action();
      await refreshSourceShareDialog();
      pushToast(successMessage);
    } catch (error) {
      setShareDialogError((error as Error).message || 'Failed to update source sharing.');
    } finally {
      setShareDialogBusy(false);
    }
  }, [pushToast, refreshSourceShareDialog, shareDialogBusy, shareDialogSource]);

  const handleApplySourceAction = useCallback(async (workspace: LocalOpenBrainWorkspace, action: CloudSourceActionPayload) => {
    if (mutatingSourceID) {
      return;
    }
    if (!canManageOpenBrainSource(workspace)) {
      pushToast('This source is read-only on this brain.');
      return;
    }
    setSourceContextMenu(null);
    setSourceActionError(null);
    setMutatingSourceID(workspace.sourceID);
    try {
      const result = await applySourceAction(workspace, action);
      pushToast(sourceActionSuccessMessage(action, result));
      setHardDeleteDialogSource(null);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to update OpenBrain Cloud source.';
      if (action.hardDelete) {
        setSourceActionError(message);
      } else {
        pushToast(message);
      }
    } finally {
      setMutatingSourceID(null);
    }
  }, [applySourceAction, mutatingSourceID, pushToast]);

  const handleOpenHardDeleteDialog = useCallback((workspace: LocalOpenBrainWorkspace) => {
    if (mutatingSourceID) {
      return;
    }
    if (!canManageOpenBrainSource(workspace)) {
      pushToast('This source is read-only on this brain.');
      return;
    }
    setSourceContextMenu(null);
    setSourceActionError(null);
    setHardDeleteDialogSource(workspace);
  }, [mutatingSourceID]);

  const handleSourceContextMenu = useCallback((node: OpenBrainInteractiveNode, event: OpenBrainFlowSourceContextMenuEvent) => {
    if (node.kind !== 'source') {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    if (showDemoGraph) {
      return;
    }
    const workspace = workspaceForNode(node);
    if (!workspace) {
      pushToast('This source is not available on this device.');
      return;
    }
    setSourceActionError(null);
    setPeerContextMenu(null);
    setSourceContextMenu({
      source: workspace,
      ...sourceContextMenuPosition(event),
    });
  }, [pushToast, showDemoGraph, workspaceForNode]);

  const handlePeerContextMenu = useCallback((node: OpenBrainInteractiveNode, event: React.MouseEvent<Element>) => {
    if (node.kind !== 'peer' && node.kind !== 'companyRoot' && node.kind !== 'department' && node.kind !== 'member') {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    setSourceContextMenu(null);
    setPeerContextMenu({
      node,
      ...sourceContextMenuPosition(event),
    });
  }, []);

  const handleNodeClick = (node: OpenBrainInteractiveNode, event: React.MouseEvent<Element>) => {
    event.stopPropagation();
    setSourceContextMenu(null);
    setPeerContextMenu(null);
    if (node.kind === 'user') {
      void startOpenBrainChat(null);
      return;
    }
    if (node.kind === 'source') {
      const workspace = workspaceForNode(node);
      startSourceChat(workspace);
      return;
    }
    if (node.kind === 'peer') {
      startPublicBrainChat(node);
      return;
    }
    if (node.kind === 'companyRoot' || node.kind === 'department' || node.kind === 'member') {
      void startOpenBrainChat(null);
      return;
    }
  };

  const handleNodeContextMenu = (node: OpenBrainInteractiveNode, event: React.MouseEvent<Element>) => {
    event.preventDefault();
    event.stopPropagation();
    if (node.kind === 'user') {
      setSourceContextMenu(null);
      setPeerContextMenu(null);
      setAddPopoverOpen(true);
      return;
    }
    handlePeerContextMenu(node, event);
  };

  const handleRemoveTeamBrainCluster = (event: React.MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation();
    setTeamBrainClusterVisible(false);
  };

  const renderedNodes: OpenBrainRenderedFlowNode[] = flow.nodes.map((node) => {
    const interactiveNode: OpenBrainInteractiveNode = { ...node.data, id: node.id };
    const sourceKey = node.data.sourceID || node.data.workspaceID || node.data.path || node.id.replace(/^workspace:/, '');
    const sourceWorkspace = node.data.kind === 'source' ? workspaceForNode(interactiveNode) : null;
    const sourceDisplay = sourceWorkspace
      ? resolveOpenBrainSourceDisplayState(sourceWorkspace, {
        provider,
        uiLinked: isSourceLinked(sourceKey),
      })
      : null;
    const sourceContextMenuEnabled = Boolean(
      !showDemoGraph
      && sourceWorkspace
      && sourceDisplay
    );
    const sourceLinked = showDemoGraph
      ? demoSourceLinks[node.id] !== false
      : sourceDisplay?.arcLinked ?? isSourceLinked(sourceKey);
    const peerLinked = node.data.kind === 'user'
      ? true
      : Boolean(peerLinks[node.id] ?? node.data.defaultLinked);
    const linked = node.data.kind === 'source' ? sourceLinked : peerLinked;
    const subtitle = node.data.kind === 'source'
      ? (sourceDisplay?.statusText ?? node.data.subtitle)
      : node.data.subtitle;
    return {
      ...node,
      data: {
        ...node.data,
        linked,
        subtitle,
        teamMembers: node.data.teamMembers?.map((member) => ({
          ...member,
          defaultLinked: Boolean(peerLinks[member.id] ?? member.defaultLinked),
        })),
        coreRef: node.id === 'user-root' ? coreRef : undefined,
        onNodeAction: handleNodeClick,
        onNodeContextMenu: handleNodeContextMenu,
        onSourceContextMenu: handleSourceContextMenu,
        sourceContextMenuEnabled,
        sourceContextMenuDisabled: Boolean(sourceWorkspace && mutatingSourceID === sourceWorkspace.sourceID),
        sourceContextMenuOpen: Boolean(
          sourceWorkspace
          && sourceContextMenu
          && isSameSource(sourceWorkspace, sourceContextMenu.source),
        ),
        onRemoveTeamBrainCluster: handleRemoveTeamBrainCluster,
        onRestoreTeamBrainCluster: () => setTeamBrainClusterVisible(true),
      },
      hidden: Boolean(node.hidden || node.data.hidden),
    };
  });
  const renderedNodeByID = new Map(renderedNodes.map((node) => [node.id, node]));
  const renderedEdges = flow.edges.map((edge) => {
    const target = renderedNodeByID.get(edge.target);
    const targetLinked = Boolean(target?.data.linked);
    const isUnlinkedSourceEdge = edge.data?.kind === 'source'
      && target?.data.kind === 'source'
      && !targetLinked;
    const isTeamPeerEdge = edge.data?.kind === 'peer' && TEAM_BRAIN_CLUSTER_PEER_IDS.has(edge.target);
    const hidden = Boolean(edge.hidden)
      || isUnlinkedSourceEdge
      || (isTeamPeerEdge && (!targetLinked || !teamBrainClusterVisible))
      || (edge.data?.kind === 'hierarchy' && !teamBrainClusterVisible);
    return {
      ...edge,
      hidden,
    };
  });
  const graphSignature = renderedNodes
    .map((node) => `${node.id}:${node.hidden ? 'h' : 'v'}:${node.data.linked ? '1' : '0'}:${node.position.x},${node.position.y}`)
    .join('|');

  const handleOnboardingAction = async () => {
    if (onboardingBusy) {
      return;
    }
    if (onboardingStep === 'login') {
      showLoginRequiredDialog('chat');
      return;
    }
    setOnboardingBusy(true);
    try {
      if (onboardingStep === 'connect_github') {
        const result = await window.electronAPI?.workspace?.openStorageBackendSettings?.({ storageBackend: 'github' });
        if (!result?.success) {
          throw new Error(result?.error || t('openBrainOnboarding.connectGitHubFailed'));
        }
        return;
      }
      await onCreateSource();
    } catch (err) {
      pushToast(err instanceof Error ? err.message : t('openBrainOnboarding.setupFailed'));
    } finally {
      setOnboardingBusy(false);
    }
  };

  const onboardingTitle = onboardingStep === 'login'
    ? t('openBrainOnboarding.loginTitle')
    : onboardingStep === 'connect_github'
      ? t('openBrainOnboarding.connectGitHubTitle')
      : t('openBrainOnboarding.addSourceTitle');
  const onboardingSubtitle = onboardingStep === 'connect_github'
    ? t('openBrainOnboarding.connectGitHubSubtitle')
    : null;
  const onboardingButtonLabel = onboardingBusy || (loading && onboardingStep === 'add_source')
    ? t('openBrainOnboarding.opening')
    : onboardingStep === 'login'
      ? t('openBrainOnboarding.loginAction')
      : onboardingStep === 'connect_github'
        ? t('openBrainOnboarding.connectGitHubAction')
        : t('openBrainOnboarding.addSourceAction');
  const onboardingInlineError = onboardingStep === 'connect_github' && githubCheckError
    ? githubCheckError
    : error;

  return (
    <div className="openbrain-graph-shell no-drag flex h-full flex-col overflow-hidden bg-editor-bg text-prime-text">
      <style>{`
        .openbrain-stage {
          border: 1px solid color-mix(in srgb, var(--color-prime-text) 8%, transparent);
          border-radius: 36px;
          background: transparent;
          overflow: hidden;
          contain: paint;
          isolation: isolate;
        }
        .openbrain-stage,
        .openbrain-stage * {
          box-sizing: border-box;
        }
        .openbrain-stage::before,
        .openbrain-stage::after {
          content: '';
          position: absolute;
          inset: 0;
          pointer-events: none;
        }
        .openbrain-stage::before {
          opacity: 0.18;
          background-image: radial-gradient(color-mix(in srgb, var(--color-prime-text) 12%, transparent) 0.7px, transparent 0.7px);
          background-size: 18px 18px;
        }
        .openbrain-stage::after {
          opacity: 0.12;
          background-image:
            linear-gradient(color-mix(in srgb, var(--color-highlight) 12%, transparent) 1px, transparent 1px),
            linear-gradient(90deg, color-mix(in srgb, var(--color-highlight) 10%, transparent) 1px, transparent 1px);
          background-size: 76px 76px;
        }
        .openbrain-flow {
          position: absolute;
          inset: 0;
          z-index: 2;
        }
        .openbrain-flow-locked,
        .openbrain-flow-locked * {
          pointer-events: none !important;
        }
        .openbrain-flow .react-flow__renderer,
        .openbrain-flow .react-flow__pane,
        .openbrain-flow .react-flow__viewport {
          outline: none;
        }
        .openbrain-flow .react-flow__node {
          background: transparent;
          border: 0;
          box-shadow: none;
        }
        .openbrain-flow .react-flow__edge {
          pointer-events: none;
        }
        .openbrain-flow-background {
          opacity: 0.65;
        }
        .openbrain-flow-handle {
          width: 10px;
          height: 10px;
          border: 0;
          background: transparent;
          opacity: 0;
          pointer-events: none;
        }
        .openbrain-flow-node {
          position: relative;
          width: 100%;
          height: 100%;
          pointer-events: auto;
        }
        .openbrain-flow-node-button {
          width: 100%;
          height: 100%;
          pointer-events: auto;
        }
        .openbrain-flow-source {
          width: 100%;
          height: 100%;
        }
        .openbrain-flow-source .openbrain-flow-node-button {
          width: 100%;
          height: 100%;
        }
        .openbrain-flow-edge-pulse {
          fill: none;
          stroke-linecap: round;
          stroke-width: 2.4;
          stroke-dasharray: 7 18;
          animation: openbrain-dash 10s linear infinite;
        }
        @keyframes openbrain-dash {
          to { stroke-dashoffset: -160; }
        }
        .openbrain-flow-user {
          display: grid;
          place-items: center;
        }
        .openbrain-personal-orbit {
          position: absolute;
          inset: 0;
          border-radius: 999px;
          pointer-events: none;
        }
        .openbrain-personal-ring {
          position: absolute;
          border-radius: 999px;
          border: 1.5px solid rgba(47, 138, 122, 0.18);
          background: transparent;
          box-shadow: 0 0 26px rgba(47, 138, 122, 0.1);
        }
        .openbrain-personal-ring--outer {
          inset: 0;
          border-color: rgba(47, 138, 122, 0.14);
          animation: openbrain-ring-breathe-outer 5.6s ease-in-out infinite;
        }
        .openbrain-personal-ring--inner {
          inset: 28px;
          border-color: rgba(47, 138, 122, 0.44);
          animation: openbrain-ring-breathe-inner 5.6s ease-in-out infinite;
          animation-delay: -1.6s;
        }
        @keyframes openbrain-ring-breathe-outer {
          0%, 100% {
            opacity: 0.24;
            border-color: rgba(47, 138, 122, 0.12);
            box-shadow: 0 0 14px rgba(47, 138, 122, 0.06);
          }
          50% {
            opacity: 0.48;
            border-color: rgba(47, 138, 122, 0.24);
            box-shadow: 0 0 34px rgba(47, 138, 122, 0.18);
          }
        }
        @keyframes openbrain-ring-breathe-inner {
          0%, 100% {
            opacity: 0.48;
            border-color: rgba(47, 138, 122, 0.26);
            box-shadow: 0 0 18px rgba(47, 138, 122, 0.08);
          }
          50% {
            opacity: 1;
            border-color: rgba(47, 138, 122, 0.72);
            box-shadow: 0 0 44px rgba(47, 138, 122, 0.3);
          }
        }
        .openbrain-core-node:focus-visible {
          outline: none;
        }
        .openbrain-core-node {
          position: absolute;
          left: 50%;
          top: 50%;
          z-index: 2;
          width: 72px;
          height: 72px;
          border-radius: 999px;
          transform: translate(-50%, -50%);
        }

        .openbrain-source-node:hover {
          border-color: color-mix(in srgb, #2f8f6b 34%, var(--op-sg-capsule-border)) !important;
        }
        .openbrain-source-node.openbrain-source-unlinked {
          opacity: 0.56;
        }
        .openbrain-source-copy {
          max-width: 96px;
        }
        .openbrain-source-node {
          min-height: 36px;
        }
        .openbrain-source-node:focus-visible,
        .openbrain-source-node.is-menu-open {
          outline: 2px solid #2f8f6b;
          outline-offset: 2px;
          border-radius: 999px;
        }
        .openbrain-source-node-anchor-right {
          flex-direction: row-reverse;
        }
        .openbrain-source-node-anchor-right .openbrain-source-copy {
          text-align: right;
        }
        .openbrain-core-node .openbrain-avatar-wrap {
          position: relative;
          display: grid;
          place-items: center;
        }
        .openbrain-peer-brain-node {
          border-radius: 50%;
          background: transparent;
          box-shadow: none;
        }
        .openbrain-peer-brain-node:hover {
          box-shadow: none;
        }
        .openbrain-peer-brain-node.openbrain-peer-linked {
          box-shadow: 0 0 0 4px rgba(195, 106, 115, 0.14);
        }
        .openbrain-peer-brain-dot {
          width: 100%;
          height: 100%;
          border-radius: 50%;
          display: grid;
          place-items: center;
          overflow: hidden;
          color: white;
          font-size: 18px;
          font-weight: 800;
          line-height: 1;
          box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.42);
        }
        .openbrain-peer-brain-img {
          width: 100%;
          height: 100%;
          border-radius: inherit;
          object-fit: cover;
          display: block;
        }
        .openbrain-peer-brain-initial {
          display: grid;
          width: 100%;
          height: 100%;
          place-items: center;
        }
        .openbrain-peer-brain-copy {
          position: absolute;
          left: 50%;
          top: calc(100% + 8px);
          transform: translateX(-50%);
          width: max-content;
          text-align: center;
          pointer-events: none;
        }
        .openbrain-peer-brain-copy strong {
          display: block;
          font-size: 12px;
          font-weight: 700;
          line-height: 1.2;
        }
        .openbrain-source-dot {
          width: 26px;
          height: 26px;
          border-radius: 10px;
          display: flex;
          flex: none;
          align-items: center;
          justify-content: center;
          color: white;
          font-size: 12px;
          font-weight: 800;
          line-height: 1;
          padding: 0;
          box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.35), 0 6px 14px rgba(38, 35, 31, 0.1);
        }
        .openbrain-flow-company-root {
          position: relative;
          width: 100%;
          height: 100%;
          display: grid;
          place-items: center;
        }
        .openbrain-cluster-enterprise {
          width: 100%;
          height: 100%;
          display: grid;
          place-items: center;
          border-radius: 999px;
        }
        .openbrain-cluster-enterprise-panel {
          position: absolute;
          left: 50%;
          bottom: calc(100% - 14px);
          transform: translateX(-50%);
          opacity: 0;
          visibility: hidden;
          pointer-events: none;
          transition: opacity 180ms ease, visibility 180ms ease;
          z-index: 8;
        }
        .openbrain-flow-company-root:hover .openbrain-cluster-enterprise-panel,
        .openbrain-flow-company-root:focus-within .openbrain-cluster-enterprise-panel {
          opacity: 1;
          visibility: visible;
          pointer-events: auto;
        }
        .openbrain-cluster-enterprise-copy {
          position: absolute;
          z-index: 10;
          left: 50%;
          bottom: calc(100% + 10px);
          width: 230px;
          transform: translateX(-50%);
          text-align: center;
          pointer-events: none;
        }
        .openbrain-cluster-restore-btn,
        .openbrain-cluster-team,
        .openbrain-cluster-person-wrap {
          z-index: 10;
        }
        .openbrain-cluster-restore-btn {
          display: grid;
          place-items: center;
          width: 100%;
          height: 100%;
          font-size: 20px;
          font-weight: 800;
        }
        .openbrain-cluster-team {
          display: flex;
          flex-direction: column;
          align-items: center;
          justify-content: center;
          width: 100%;
          height: 100%;
          padding: 0 18px;
          text-align: center;
        }
        .openbrain-cluster-team.openbrain-peer-linked {
          border-color: color-mix(in srgb, #2f8f6b 32%, var(--op-sg-capsule-border)) !important;
          box-shadow: 0 0 0 3px rgba(47, 143, 107, 0.13), var(--op-sg-shadow);
        }
        .openbrain-cluster-team strong {
          display: block;
          overflow: hidden;
          text-overflow: ellipsis;
          white-space: nowrap;
          font-size: 13px;
          letter-spacing: -0.02em;
        }
        .openbrain-cluster-team-meta {
          display: block;
          margin-top: 3px;
          font-size: 11px;
          font-weight: 600;
          line-height: 1.35;
        }
        .openbrain-cluster-person-wrap {
          position: relative;
          width: 100%;
          height: 100%;
        }
        .openbrain-cluster-person-tip {
          position: absolute;
          bottom: calc(100% + 8px);
          left: 50%;
          transform: translateX(-50%);
          opacity: 0;
          pointer-events: none;
          white-space: nowrap;
          transition: opacity 160ms ease;
          z-index: 6;
        }
        .openbrain-cluster-person-wrap:hover .openbrain-cluster-person-tip,
        .openbrain-cluster-person-wrap:focus-within .openbrain-cluster-person-tip {
          opacity: 1;
        }
        .openbrain-cluster-person {
          border-radius: 999px;
          border: 2px solid white;
          display: grid;
          place-items: center;
          color: white;
          font-size: 13px;
          font-weight: 700;
          letter-spacing: -0.03em;
          transition: box-shadow 160ms ease, opacity 160ms ease;
        }
        .openbrain-cluster-person.openbrain-peer-linked {
          box-shadow: 0 0 0 3px rgba(47, 143, 107, 0.22), 0 10px 24px rgba(47, 143, 107, 0.24);
        }
        .openbrain-cluster-person:focus-visible {
          outline: 2px solid #2f8f6b;
          outline-offset: 3px;
        }
        @media (prefers-reduced-motion: reduce) {
          .openbrain-flow-edge-pulse,
          .openbrain-personal-ring {
            animation: none;
          }
        }
      `}</style>
      <div ref={viewportRef} className="min-h-0 flex-1 overflow-hidden px-7 py-7">
        <div className="openbrain-stage relative h-full min-h-[560px]">
          <OpenBrainFlowGraph
            key={graphFlowKey}
            nodes={renderedNodes}
            edges={renderedEdges}
            graphSignature={graphSignature}
            interactive={!showOnboardingOverlay}
          />

          <IconButton
            className="no-drag absolute right-4 top-4 z-50"
            size={28}
            title="Refresh OpenBrain graph"
            aria-label="Refresh OpenBrain graph"
            disabled={loading || refreshing}
            onClick={handleRefreshOpenBrainGraph}
          >
            <RefreshIcon className={`h-4 w-4${refreshing ? ' animate-spin' : ''}`} />
          </IconButton>

          {addPopoverOpen ? createPortal(
            <MyGBrainAddPopover
              anchorRef={coreRef}
              open={addPopoverOpen}
              loggedIn={loggedIn}
              busy={onboardingBusy}
              onClose={() => setAddPopoverOpen(false)}
              onCreateSource={onCreateSource}
              onSubscribePublicBrain={subscribePublicBrain}
              onUnsubscribePublicBrain={unsubscribePublicBrain}
              listPublicBrainDirectory={listPublicBrainDirectory}
              onLogin={async () => {
                const result = await startLogin();
                if (!result?.success) {
                  throw new Error('Failed to start OpenBrain sign in.');
                }
              }}
            />,
            document.body,
          ) : null}

          {showOnboardingOverlay ? (
            <>
              <div className="openbrain-onboarding-scrim absolute inset-0 z-[30]" aria-hidden="true" />
              <div className="openbrain-onboarding-overlay absolute inset-0 z-40 flex items-center justify-center px-6 text-center">
                <div
                  className="openbrain-onboarding-copy pointer-events-auto max-w-[420px] px-4 py-2"
                  onPointerDown={(event) => event.stopPropagation()}
                  onClick={(event) => event.stopPropagation()}
                >
                  <div className="openbrain-onboarding-title text-lg font-bold leading-6 tracking-[-0.045em]">{onboardingTitle}</div>
                  {onboardingSubtitle ? (
                    <p className="mt-2 text-sm leading-5 text-secondary-text">{onboardingSubtitle}</p>
                  ) : null}
                  {onboardingInlineError ? (
                    <p className="mt-3 text-xs leading-5" style={{ color: '#b16161' }}>{onboardingInlineError}</p>
                  ) : null}
                  <button
                    type="button"
                    className={`openbrain-onboarding-action no-drag ${OPENBRAIN_GRAPH_CAPSULE} mt-5 px-5 py-2.5 text-sm font-bold transition-[color] disabled:cursor-wait disabled:opacity-60`}
                    onClick={() => void handleOnboardingAction()}
                    disabled={onboardingBusy || loading}
                  >
                    {onboardingButtonLabel}
                  </button>
                </div>
              </div>
            </>
          ) : null}

          {error && !showOnboardingOverlay ? (
            <div className="absolute left-0 right-0 top-[220px] z-30 text-center text-xs" style={{ color: '#b16161' }}>
              {error}
            </div>
          ) : null}
        </div>
      </div>
      {sourceContextMenu ? createPortal(
        <>
          <button
            type="button"
            className="no-drag fixed inset-0 z-[60] cursor-default border-0 bg-transparent p-0"
            aria-label="Close source menu"
            onMouseDown={() => setSourceContextMenu(null)}
          />
          <PopupMenu
            className="no-drag fixed z-[70] w-72"
            style={{ left: sourceContextMenu.x, top: sourceContextMenu.y }}
            onContextMenu={(event) => event.preventDefault()}
          >
            <CloudSourceActionMenuItems
              source={sourceContextMenu.source}
              disabled={mutatingSourceID === sourceContextMenu.source.sourceID}
              sourceLinked={isSourceLinked(sourceLinkKeyForWorkspace(sourceContextMenu.source))}
              onChatWithSource={(source) => {
                setSourceContextMenu(null);
                startSourceChat(source);
              }}
              onToggleSourceLink={(source) => {
                setSourceContextMenu(null);
                const sourceKey = sourceLinkKeyForWorkspace(source);
                if (!sourceKey) {
                  pushToast('This source is not available on this device.');
                  return;
                }
                setSourceLinked(sourceKey, !isSourceLinked(sourceKey));
              }}
              onShareSource={(source) => void handleShareSource(source)}
              onBindSource={(source) => void onBindSource(source)}
              onApplyAction={(source, action) => void handleApplySourceAction(source, action)}
              onOpenHardDelete={handleOpenHardDeleteDialog}
              showCloudManagement={provider === 'cloud'}
            />
          </PopupMenu>
        </>,
        document.body,
      ) : null}
      {peerContextMenu ? createPortal(
        <>
          <button
            type="button"
            className="no-drag fixed inset-0 z-[60] cursor-default border-0 bg-transparent p-0"
            aria-label="Close public brain menu"
            onMouseDown={() => setPeerContextMenu(null)}
          />
          <PopupMenu
            className="no-drag fixed z-[70] w-72"
            style={{ left: peerContextMenu.x, top: peerContextMenu.y }}
            onContextMenu={(event) => event.preventDefault()}
          >
            <PopupMenuItem
              onClick={() => {
                const { node } = peerContextMenu;
                setPeerContextMenu(null);
                if (node.kind === 'peer') {
                  startPublicBrainChat(node);
                } else {
                  void startOpenBrainChat(null);
                }
              }}
            >
              <ChatLineIcon className="h-4 w-4 opacity-70" />
              <span>Open GBrain chat</span>
            </PopupMenuItem>
            <PopupMenuSeparator />
            <PopupMenuItem
              onClick={() => {
                const { node } = peerContextMenu;
                setPeerContextMenu(null);
                disconnectPublicBrain(node);
              }}
            >
              <TrashIcon className="h-4 w-4 opacity-70" />
              <span>{peerContextMenu.node.kind === 'peer' ? 'Disconnect public brain' : 'Hide from graph'}</span>
            </PopupMenuItem>
          </PopupMenu>
        </>,
        document.body,
      ) : null}
      <HardDeleteCloudSourceDialog
        open={Boolean(hardDeleteDialogSource)}
        source={hardDeleteDialogSource}
        busy={Boolean(hardDeleteDialogSource && mutatingSourceID === hardDeleteDialogSource.sourceID)}
        error={sourceActionError}
        onCancel={() => {
          if (hardDeleteDialogSource && mutatingSourceID === hardDeleteDialogSource.sourceID) {
            return;
          }
          setHardDeleteDialogSource(null);
          setSourceActionError(null);
        }}
        onSubmit={(action) => {
          if (!hardDeleteDialogSource) {
            return;
          }
          void handleApplySourceAction(hardDeleteDialogSource, action);
        }}
      />
      <SourceShareDialog
        open={Boolean(shareDialogSource)}
        source={shareDialogSource}
        share={shareDialogView}
        publicProfile={shareDialogProfile}
        busy={shareDialogBusy}
        error={shareDialogError}
        onCancel={() => {
          if (shareDialogBusy) {
            return;
          }
          setShareDialogSource(null);
          setShareDialogView(null);
          setShareDialogProfile(null);
          setShareDialogError(null);
        }}
        onShareEmail={(email) => runSourceShareDialogAction(async () => {
          if (!shareDialogSource) {
            return;
          }
          await shareSourceWithUser(shareDialogSource, email);
        }, 'Read-only source share added.')}
        onRevokeUser={(uid) => runSourceShareDialogAction(async () => {
          if (!shareDialogSource) {
            return;
          }
          await revokeSourceUserShare(shareDialogSource, uid);
        }, 'Read-only share removed.')}
        onMakePublic={() => runSourceShareDialogAction(async () => {
          if (!shareDialogSource) {
            return;
          }
          await setSourcePublic(shareDialogSource);
        }, 'Source is public.')}
        onMakePrivate={() => runSourceShareDialogAction(async () => {
          if (!shareDialogSource) {
            return;
          }
          await revokeSourcePublic(shareDialogSource);
        }, 'Source is private.')}
        onUpdatePublicProfile={(description) => runSourceShareDialogAction(async () => {
          await updatePublicBrainProfile(description);
        }, 'Public brain description updated.')}
      />
    </div>
  );
};
