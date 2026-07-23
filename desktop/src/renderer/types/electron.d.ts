// Type definitions for Electron API exposed via preload

export type ModelAPI = 'openai-completions' | 'openai-responses' | 'anthropic-messages' | 'gemini-native';
export type ModelServiceTier = 'priority' | 'flex';
export type ModelReasoningControl = 'level' | 'toggle';

export type SshHostPayload = {
  id?: string;
  alias: string;
  hostname?: string;
  user?: string;
  port?: string;
  identityFile?: string;
  source?: string;
  authMethod?: 'agent' | 'keyFile' | 'password';
  credentialID?: string;
  hasPassword?: boolean;
  hasPassphrase?: boolean;
};

export type ManualSshHostPayload = {
  id?: string;
  alias: string;
  hostname: string;
  user: string;
  port?: string;
  identityFile?: string;
  authMethod: 'keyFile' | 'password';
  password?: string;
  passphrase?: string;
};

export type ModelEntry = {
  key: string;
  id: string;
  label?: string;
  enabled: boolean;
  provider: string;
  providerLabel?: string;
  api: ModelAPI;
  reasoning: boolean;
  reasoningControl?: ModelReasoningControl;
  reasoningLevels?: string[];
  contextWindow?: number;
  contextWindows?: number[];
  defaultContextWindow?: number;
  serviceTiers?: ModelServiceTier[];
  maxOutputTokens?: number;
  baseUrl?: string;
  apiKey?: string;
  updatedAt?: number;
};

export type ProviderModelEntry = {
  id: string;
  label?: string;
  enabled: boolean;
  api?: ModelAPI;
  baseUrl?: string;
  apiKey?: string;
  reasoning: boolean;
  reasoningControl?: ModelReasoningControl;
  reasoningLevels?: string[];
  contextWindow?: number;
  contextWindows?: number[];
  defaultContextWindow?: number;
  serviceTiers?: ModelServiceTier[];
  maxOutputTokens?: number;
  updatedAt?: number;
};

export type ProviderEntry = {
  label?: string;
  api?: ModelAPI;
  baseUrl?: string;
  apiKey?: string;
  managed?: boolean;
  models: ProviderModelEntry[];
};

export type ModelAutoStrategy = {
  defaultChatModelID?: string;
  defaultChatThinkingLevel?: string;
  defaultInlineCompletionModelID?: string;
  defaultInlineCompletionThinkingLevel?: string;
};

export type ModelStrategies = {
  auto?: ModelAutoStrategy;
};

export type ModelPreference = {
  thinkingLevel?: string;
  contextWindow?: number;
  serviceTier?: ModelServiceTier | null;
};

export type ModelsConfig = {
  version: number;
  defaultModelKey: string | null;
  providers: Record<string, ProviderEntry>;
  models: ModelEntry[];
  strategies?: ModelStrategies;
  modelPreferences?: Record<string, ModelPreference>;
  updatedAt: number;
};

export interface DashboardRuntimeConnection {
  nodeID: string;
  name: string;
  transport: string;
  daemon?: boolean;
  pid?: number;
  startedAt?: string;
  uptimeSec?: number;
  lastActiveAt?: string;
  url?: string;
}

export interface DashboardRuntimeUpdater {
  currentVersion?: string;
  targetVersion?: string;
  stagedVersion?: string;
  phase?: string;
  downloaded?: boolean;
  applying?: boolean;
  lastCheckedAt?: string;
  lastError?: string;
}

export interface DashboardHost {
  id: string;
  hostname?: string;
  env?: string;
  baseDir?: string;
  online: boolean;
  lastSeenAt?: string;
  receivedAt?: string;
  runtimeConnections: DashboardRuntimeConnection[];
  runtimeUpdater?: DashboardRuntimeUpdater;
}

export interface BillingSubscription {
  uid: string;
  planId?: string;
  planName?: string;
  effectivePlanId?: string;
  effectivePlanName?: string;
  effectivePlanSource?: string;
  stripeSubscriptionId?: string;
  status?: string;
  currentPeriodStart?: string;
  currentPeriodEnd?: string;
  cancelAtPeriodEnd?: boolean;
  quota?: {
    currency?: string;
    baseMonthlyCost?: string;
    giftedMonthlyCost?: string;
    grantedAdjustment?: string;
    effectiveCostQuota?: string;
    usedCost?: string;
    remainingCost?: string;
  };
  aiChatEligible?: boolean;
  bundledTokenEligible?: boolean;
}

export interface RuntimeBootstrapState {
  phase: 'idle' | 'checking' | 'installing' | 'updating' | 'starting' | 'ready' | 'error';
  visible: boolean;
  busy: boolean;
  ready: boolean;
  message: string;
  detail?: string;
  error?: string;
  canRetry: boolean;
  canQuit: boolean;
  installedVersion?: string;
  runningVersion?: string;
  latestVersion?: string;
  needsInstall: boolean;
  needsUpdate: boolean;
  needsStart: boolean;
  healthy: boolean;
  offline: boolean;
  isFirstInstall: boolean;
  lastUpdatedAt: number;
}

export interface DesktopUpdateState {
  phase: 'unsupported' | 'idle' | 'checking' | 'downloading' | 'ready' | 'installing' | 'error';
  currentVersion: string | null;
  targetVersion: string | null;
  error?: string;
}

export type WorkspaceSyncPolicy = {
  autoSync?: boolean;
  onOpen?: boolean;
  onLocalChange?: boolean;
  intervalSec?: number;
  conflict?: string;
  deleteMode?: string;
};

export type OpenBrainProviderMode = 'cloud' | 'local';

export type LocalGBrainSettings = {
  engine?: 'pglite' | 'postgres';
  databaseUrl?: string;
  databasePath?: string;
  remoteMcpUrl?: string;
  remoteMcpClientID?: string;
  remoteMcpClientSecret?: string;
  remoteMcpClientSecretEnvVar?: string;
  cliPath?: string;
};

export type OpenBrainUserSettings = {
  provider?: OpenBrainProviderMode;
  local?: LocalGBrainSettings;
};

export type WorkspaceGitHubAccount = {
  owner: string;
  accountType?: string;
  installationID?: string;
  connectedAt?: string;
  canCreateRepository?: boolean;
  canSyncRepository?: boolean;
  permissionState?: string;
  permissionMessage?: string;
};

export type WorkspaceStorageProviderOption = {
  provider: string;
  backend?: string;
  name?: string;
  enabled?: boolean;
  configured?: boolean;
  connectable?: boolean;
  connected?: boolean;
  connectedAs?: string;
  connectedAt?: string;
  region?: string;
  canCreateRepository?: boolean;
  canSyncRepository?: boolean;
  permissionState?: string;
  permissionMessage?: string;
  accounts?: WorkspaceGitHubAccount[];
};

export type WorkspaceStorageBinding = {
  enabled: boolean;
  backend?: string;
  provider?: string;
  region?: string;
  remoteID?: string;
  remoteName?: string;
  remoteURL?: string;
  connectedAs?: string;
  providers?: WorkspaceStorageProviderOption[];
  syncPolicy?: WorkspaceSyncPolicy;
};

export interface ElectronAPI {
  getPath: (name: string) => Promise<string>;
  getHomeDir: () => Promise<string>;
  getDefaultDir: () => Promise<string>;
  localDirectoryPicker: {
    listDirectory: (path: string) => Promise<{
      path?: string;
      entries?: Array<{
        name: string;
        isDir: boolean;
        size: number;
        modTime: number;
      }>;
      error?: string;
    }>;
    statPath: (path: string) => Promise<{
      path?: string;
      name?: string;
      size?: number;
      isDir?: boolean;
      modTime?: number;
      error?: string;
    }>;
    getSpecialDirectories: () => Promise<Array<{
      key: string;
      label: string;
      path: string;
    }>>;
    mkdir: (path: string) => Promise<{ success?: boolean; error?: string }>;
    writeFile: (path: string, content: string) => Promise<{ error?: string }>;
  };
  exportMarkdownPdfToPath: (payload: unknown) => Promise<{ canceled: boolean; filePath?: string; error?: string }>;
  getPdfExportDefaultPath: (payload: {
    sourcePath?: string;
    currentDir?: string;
    isRemote?: boolean;
  }) => Promise<{ defaultDir: string; defaultFileName: string }>;
  pdfExport: {
    getPayload: () => Promise<{
      title: string;
      content: string;
      sourcePath?: string;
      currentDir?: string;
      remoteSession?: {
        hostLabel: string;
        localPort: number;
        remotePort: number;
        wsUrl: string;
        httpUrl: string;
        remoteHome: string;
        workspaceDir: string;
        installDir: string;
      } | null;
      baseDir?: string;
      workspaceRootDir?: string;
      agentsRootDir?: string;
      instanceID?: string;
    } | null>;
    reportReady: () => void;
    reportError: (message: string) => void;
  };
  clipboard: {
    writeText: (text: string) => void;
    readImagePngBase64: () => Promise<{ base64: string } | null> | { base64: string } | null;
  };
  revealInFileManager: (path: string) => Promise<{ success: boolean; error?: string }>;
  backup: {
    save: (data: { id: string; title: string; content: string; editorId: string }) => Promise<{ success: boolean }>;
    load: () => Promise<Array<{
      id: string;
      title: string;
      content: string;
      editorId: string;
      timestamp: number;
    }>>;
    delete: (tabId: string) => Promise<{ success: boolean }>;
  };
  window: {
    updateWorkspaceTabsSession: (session: {
      version: number;
      activeTabId: string;
      tabs: Array<{
        id: string;
        label: string;
        kind: 'local' | 'remote';
        workspaceId: string;
        workspacePath?: string;
        remoteHost?: SshHostPayload;
        currentDir?: string;
        openEditorFilePaths?: string[];
        chatSession?: {
          openChats: Array<{
            threadID: string;
            path: string;
            title: string;
          }>;
          selectedThreadID?: string;
        };
      }>;
    }) => Promise<{
      version: number;
      activeTabId: string;
      tabs: Array<{
        id: string;
        label: string;
        kind: 'local' | 'remote';
        workspaceId: string;
        workspacePath?: string;
        remoteHost?: SshHostPayload;
        currentDir?: string;
        openEditorFilePaths?: string[];
        chatSession?: {
          openChats: Array<{
            threadID: string;
            path: string;
            title: string;
          }>;
          selectedThreadID?: string;
        };
      }>;
    } | null>;
    createNew: () => Promise<{ success: boolean }>;
    createLocal: (options?: { path?: string }) => Promise<{ canceled: boolean }>;
    createRemote: (host: SshHostPayload) => Promise<{ success: boolean }>;
    list: () => Promise<Array<{
      id: number;
      sessionId: string;
      label: string;
      mode: 'local' | 'remote';
      presentation: 'default' | 'newWindowLanding';
      workspaceId: string;
      workspacePath?: string;
      remoteHost?: SshHostPayload;
      active: boolean;
    }>>;
    focus: (id: number) => Promise<void>;
    close: (id: number) => Promise<void>;
    readyToClose: () => Promise<{ success: boolean }>;
    setZoomLevel: (level: number) => void;
    getBootstrap: () => Promise<{
      windowId: number;
      info: {
        id: number;
        sessionId: string;
        label: string;
        mode: 'local' | 'remote';
        presentation: 'default' | 'newWindowLanding';
        authRequired?: boolean;
        workspaceId: string;
        workspacePath?: string;
        remoteHost?: SshHostPayload;
        active: boolean;
      };
      workspaceTabsSession?: {
        version: number;
        activeTabId: string;
        tabs: Array<{
          id: string;
          label: string;
          kind: 'local' | 'remote';
          workspaceId: string;
          workspacePath?: string;
          remoteHost?: SshHostPayload;
          currentDir?: string;
          openEditorFilePaths?: string[];
          chatSession?: {
            openChats: Array<{
              threadID: string;
              path: string;
              title: string;
            }>;
            selectedThreadID?: string;
          };
        }>;
      };
      initialWorkspace?: { mode: 'local' | 'remote'; workspacePath?: string; remoteHost?: SshHostPayload };
      runtimeBootstrap?: RuntimeBootstrapState | null;
    } | null>;
    onListChanged: (handler: (windows: Array<{
      id: number;
      sessionId: string;
      label: string;
      mode: 'local' | 'remote';
      presentation: 'default' | 'newWindowLanding';
      workspaceId: string;
      workspacePath?: string;
      remoteHost?: SshHostPayload;
      active: boolean;
    }>) => void) => () => void;
    onActiveChanged: (handler: (payload: { active: boolean }) => void) => () => void;
    onPrepareClose: (handler: () => void) => () => void;
  };
  runtimeBootstrap: {
    retry: () => Promise<{ success: boolean }>;
    quit: () => Promise<{ success: boolean }>;
    onChanged: (handler: (payload: RuntimeBootstrapState) => void) => () => void;
  };
  desktopUpdate: {
    getState: () => Promise<DesktopUpdateState>;
    install: () => Promise<{ success: boolean; error?: string }>;
    onChanged: (handler: (payload: DesktopUpdateState) => void) => () => void;
  };
  ssh: {
    listHosts: () => Promise<SshHostPayload[]>;
    saveHost: (host: ManualSshHostPayload) => Promise<SshHostPayload>;
    deleteHost: (id: string) => Promise<{ success: boolean }>;
    pickIdentityFile: () => Promise<{ canceled: boolean; path?: string }>;
  };
  remote: {
    connectSsh: (host: SshHostPayload, tabId: string) => Promise<{
      hostLabel: string;
      localPort: number;
      remotePort: number;
      wsUrl: string;
      httpUrl: string;
      remoteHome: string;
      workspaceDir: string;
      installDir: string;
    }>;
    disconnect: (tabId?: string) => Promise<{ success: boolean }>;
    status: (tabId: string) => Promise<{
      hostLabel: string;
      localPort: number;
      remotePort: number;
      wsUrl: string;
      httpUrl: string;
      remoteHome: string;
      workspaceDir: string;
      installDir: string;
    } | null>;
  };
  settings: {
    get: () => Promise<any>;
    getRoot: () => Promise<string>;
    set: (patch: any) => Promise<any>;
    previewMarkdownTextOffset: (value: number) => void;
    previewMarkdownContentWidth: (value: number) => void;
    onChanged: (handler: (settings: any) => void) => () => void;
  };
  power: {
    setAgentRunning: (running: boolean) => Promise<void>;
  };
  auth: {
    get: () => Promise<{
      loggedIn: boolean;
      uid: string;
      email?: string;
      baseUrl: string;
      aiGateway?: string;
      activeOrgID?: string;
      activeOrgName?: string;
      profile?: UserProfile;
    } | null>;
    startLogin: (options?: { gateway?: string; orgSlug?: string }) => Promise<{ success: boolean; mode?: 'device_code'; loginUrl?: string }>;
    listOrgs?: () => Promise<{
      success: boolean;
      error?: string;
      activeOrgID?: string;
      orgs?: Array<{ id: string; slug?: string; name?: string }>;
      workspaceTargets?: Array<{ id: string; slug?: string; name?: string }>;
    }>;
    setActiveOrg?: (orgID?: string | null, orgName?: string | null) => Promise<{
      success: boolean;
      error?: string;
      activeOrgID?: string;
      activeOrgName?: string;
    }>;
    logout: () => Promise<{ success: boolean }>;
    onChanged: (handler: (payload: {
      loggedIn: boolean;
      uid?: string;
      email?: string;
      activeOrgID?: string;
      activeOrgName?: string;
      reason?: 'logout' | 'session_expired';
      profile?: UserProfile;
    }) => void) => () => void;
    onDeviceCode?: (handler: (payload: { userCode: string; verificationUri: string; expiresAt: number }) => void) => () => void;
    onDeviceCodeComplete?: (handler: (payload: { success: boolean; error?: string }) => void) => () => void;
  };
  models: {
    get: () => Promise<ModelsConfig>;
    set: (config: ModelsConfig) => Promise<ModelsConfig>;
    refreshFromOpenBrain: () => Promise<{ success: boolean; error?: string; config?: ModelsConfig }>;
  };
  workspace: {
    listTemplates: (input?: { orgID?: string | null; targetOrgID?: string | null }) => Promise<{
      success: boolean;
      error?: string;
      defaultID?: string;
      templates?: Array<{
        templateID: string;
        orgID: string;
        name: string;
        description?: string;
        version: number;
        defaultLocalName: string;
        backupEnabled: boolean;
        repository?: {
          enabled: boolean;
          provider?: string;
          providers?: Array<{
            provider: string;
            name: string;
            enabled: boolean;
            accounts?: WorkspaceGitHubAccount[];
          }>;
        };
        storage?: WorkspaceStorageBinding;
      }>;
    }>;
    listOpenBrains: () => Promise<{
      success: boolean;
      error?: string;
      sources?: Array<{
        sourceID: string;
        name: string;
        path?: string;
        workspaceID?: string;
        orgID?: string;
        brainID?: string;
        updatedAt?: string;
        pageCount?: number;
        federated?: boolean;
            remoteURL?: string | null;
            openable: boolean;
            locationKind?: 'local' | 'remote';
            remoteHost?: SshHostPayload;
            bindingStatus?: 'connected' | 'needs_binding';
            bindingReason?: 'unbound' | 'moved' | 'mismatch';
            lastVerifiedAt?: string;
            lastVerifyReason?: string;
          }>;
        }>;
    openStorageBackendSettings: (input?: { storageBackend?: string; provider?: string }) => Promise<{ success: boolean; error?: string }>;
    createOpenBrain: (input?: { name?: string; localPath?: string }) => Promise<{
      success: boolean;
      error?: string;
      workspace?: {
        workspaceID: string;
        orgID: string;
        templateID: string;
        templateVersion: number;
        backupEnabled: boolean;
        defaultLocalName: string;
        localName: string;
        path: string;
      };
    }>;
    createFromTemplate: (input?: { templateID?: string; storageBackend?: string; provider?: string; repositoryOwner?: string; repositoryName?: string; name?: string; orgID?: string | null; targetOrgID?: string | null; localPath?: string }) => Promise<{
      success: boolean;
      error?: string;
      workspace?: {
        workspaceID: string;
        orgID: string;
        templateID: string;
        templateVersion: number;
        backupEnabled: boolean;
        defaultLocalName: string;
        localName: string;
        path: string;
      };
    }>;
  };
  openBrain: {
    getProvider: () => Promise<{
      provider: 'cloud' | 'local';
      authRequired?: boolean;
      configured: boolean;
      githubConnected?: boolean;
      cloudReady?: boolean;
      githubCheckError?: string;
    }>;
    setProvider: (input?: { provider?: 'cloud' | 'local'; local?: LocalGBrainSettings }) => Promise<{ provider: 'cloud' | 'local'; authRequired?: boolean; configured: boolean }>;
    listSources: () => Promise<{
      success: boolean;
      code?: string;
      error?: string;
      provider?: 'cloud' | 'local';
      authRequired?: boolean;
      sources?: Array<{
        sourceID: string;
        name: string;
        path?: string;
        workspaceID?: string;
        orgID?: string;
        brainID?: string;
        updatedAt?: string;
        pageCount?: number;
        federated?: boolean;
        remoteURL?: string | null;
        openable: boolean;
        disabledQueries?: boolean;
        publicAccess?: boolean;
        effectivePermission?: 'read' | 'write' | 'admin';
        canMutateSource?: boolean;
        publicOwnerUID?: string;
        bindingMode?: 'own' | 'granted';
        locationKind?: 'local' | 'remote';
        remoteHost?: SshHostPayload;
      }>;
    }>;
    query: (input?: { brainID?: string; scope?: 'brain' | 'workspace'; workspaceID?: string; orgID?: string; publicOwnerUID?: string; query?: string; limit?: number }) => Promise<{
      success: boolean;
      code?: string;
      error?: string;
      provider?: 'cloud' | 'local';
      authRequired?: boolean;
      results?: Array<{
        chunkID: string;
        workspaceID: string;
        workspaceName: string;
        path?: string;
        relativePath: string;
        title: string;
        text: string;
        score: number;
      }>;
    }>;
    createSource: (input?: { name?: string; localPath?: string; remotePath?: string; tabId?: string; remoteHost?: SshHostPayload }) => Promise<{
      success: boolean;
      code?: string;
      error?: string;
      provider?: 'cloud' | 'local';
      authRequired?: boolean;
      workspace?: {
        sourceID: string;
        name: string;
        path?: string;
        workspaceID?: string;
        orgID?: string;
        brainID?: string;
        updatedAt?: string;
        pageCount?: number;
        federated?: boolean;
        remoteURL?: string | null;
        openable: boolean;
        disabledQueries?: boolean;
        publicAccess?: boolean;
        effectivePermission?: 'read' | 'write' | 'admin';
        canMutateSource?: boolean;
        publicOwnerUID?: string;
        bindingMode?: 'own' | 'granted';
        locationKind?: 'local' | 'remote';
        remoteHost?: SshHostPayload;
        bindingStatus?: 'connected' | 'needs_binding';
        bindingReason?: 'unbound' | 'moved' | 'mismatch';
        lastVerifiedAt?: string;
        lastVerifyReason?: string;
        localName?: string;
        templateID?: string;
        templateVersion?: number;
        backupEnabled?: boolean;
        defaultLocalName?: string;
      };
    }>;
    removeSourceFromDevice: (input?: { sourceID?: string; workspaceID?: string; orgID?: string; path?: string }) => Promise<{
      success: boolean;
      code?: string;
      error?: string;
      provider?: 'cloud' | 'local';
      authRequired?: boolean;
    }>;
    archiveSource: (input?: { sourceID?: string; workspaceID?: string; orgID?: string; path?: string }) => Promise<{
      success: boolean;
      code?: string;
      error?: string;
      provider?: 'cloud' | 'local';
      authRequired?: boolean;
    }>;
    sourceAction: (input?: {
      sourceID?: string;
      workspaceID?: string;
      orgID?: string;
      path?: string;
      disableQueries?: boolean;
      enableQueries?: boolean;
      disableSync?: boolean;
      hardDelete?: boolean;
      confirmWorkspaceID?: string;
      confirmName?: string;
    }) => Promise<{
      success: boolean;
      code?: string;
      error?: string;
      provider?: 'cloud' | 'local';
      authRequired?: boolean;
      sourceID?: string;
      workspaceID?: string;
      orgID?: string;
      disabledQueries?: boolean;
      enabledQueries?: boolean;
      disabledSync?: boolean;
      hardDeleted?: boolean;
      syncJobsRemoved?: number;
      status?: string;
    }>;
  };
  profile: {
    get: () => Promise<UserProfile | null>;
    refresh: () => Promise<{
      success: boolean;
      error?: string;
      authInvalid?: boolean;
      activeOrgID?: string;
      activeOrgName?: string;
      profile?: UserProfile;
    }>;
  };
  billing: {
    getSubscription: () => Promise<{
      success: boolean;
      error?: string;
      authInvalid?: boolean;
      subscription?: BillingSubscription | null;
    }>;
  };
  dashboard: {
    getHosts: () => Promise<DashboardHost[]>;
  };
  nodes: {
    get: () => Promise<Record<string, Record<string, unknown>>>;
    upsert: (
      hostId: string,
      nodes: Array<Record<string, unknown>>
    ) => Promise<{ success: boolean; error?: string }>;
  };
  avatar: {
    cacheNode: (
      hostId: string,
      node: Record<string, unknown>
    ) => Promise<{ success: boolean; node?: Record<string, unknown>; error?: string }>;
  };
  onConfigSyncPush: (
    handler: (payload: { files: Array<{ name: string; content: string }> }) => void
  ) => () => void;
  platform: NodeJS.Platform;
  isDev: boolean;
}

export interface UserProfile {
  uid: string;
  name: string;
  username: string;
  email?: string;
  avatar?: string;
  provider?: string;
  updatedAt?: number;
}


export interface MarketplaceListItem {
  id: string;
  kind: 'agent' | 'skill' | 'tool';
  scope?: 'public' | 'org';
  orgID?: string | null;
  orgName?: string | null;
  name: string;
  description: string;
  builtin: boolean;
  version: string | null;
  installedVersion: string | null;
  installPath: string;
  sourceUrl: string | null;
  managed: boolean;
  updateAvailable: boolean;
  inUse: boolean;
  status: 'not_installed' | 'installed' | 'update_available';
  missingFromCatalog: boolean;
}

export interface MarketplaceManagedStateItem {
  id: string;
  kind: 'agent' | 'skill' | 'tool';
  scope?: 'public' | 'org';
  orgID?: string | null;
  orgName?: string | null;
  installedVersion: string | null;
  sourceUrl: string | null;
  managed: boolean;
  builtin: boolean;
  installPath: string;
  lastCheckedAt: number;
  updateAvailable: boolean;
}

export interface MarketplaceStateFile {
  version: number;
  lastCatalogVersion: string | null;
  lastCatalogAt: string | null;
  items: MarketplaceManagedStateItem[];
}

export interface MarketplaceListResult {
  items: MarketplaceListItem[];
  catalogVersion: string | null;
  generatedAt: string | null;
  error?: string;
}

export interface MarketplaceActionResult {
  success: boolean;
  error?: string;
  item?: MarketplaceListItem;
}

export interface MarketplaceOrg {
  id: string;
  slug: string;
  name: string;
  role?: string;
}

export interface MarketplaceOrgListResult {
  orgs: MarketplaceOrg[];
  error?: string;
}

declare global {
  interface Window {
    electronAPI?: ElectronAPI;
  }
}

export {};
