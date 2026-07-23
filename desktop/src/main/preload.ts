import { clipboard, contextBridge, ipcRenderer, webFrame } from 'electron';

type RuntimeBootstrapStatePayload = {
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
};

type SshHostPayload = {
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

type ManualSshHostPayload = {
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

// Expose protected methods that allow the renderer process to use
// the ipcRenderer without exposing the entire object
contextBridge.exposeInMainWorld('electronAPI', {
  // App paths
  getPath: (name: string) => ipcRenderer.invoke('app:getPath', name),
  getHomeDir: () => ipcRenderer.invoke('app:getHomeDir'),
  getDefaultDir: () => ipcRenderer.invoke('app:getDefaultDir'),
  localDirectoryPicker: {
    listDirectory: (targetPath: string) => ipcRenderer.invoke('app:listLocalDirectory', { path: targetPath }),
    statPath: (targetPath: string) => ipcRenderer.invoke('app:statLocalPath', { path: targetPath }),
    getSpecialDirectories: () => ipcRenderer.invoke('app:getLocalSpecialDirectories'),
    mkdir: (targetPath: string) => ipcRenderer.invoke('app:localMkdir', { path: targetPath }),
    writeFile: (targetPath: string, content: string) => ipcRenderer.invoke('app:localWriteFile', { path: targetPath, content }),
  },
  exportMarkdownPdfToPath: (payload: unknown) => ipcRenderer.invoke('app:exportMarkdownPdfToPath', payload),
  getPdfExportDefaultPath: (payload: { sourcePath?: string; currentDir?: string; isRemote?: boolean }) =>
    ipcRenderer.invoke('app:getPdfExportDefaultPath', payload),
  pdfExport: {
    getPayload: () => ipcRenderer.invoke('pdfExport:getPayload'),
    reportReady: () => ipcRenderer.send('pdfExport:reportReady'),
    reportError: (message: string) => ipcRenderer.send('pdfExport:reportError', { message }),
  },
  clipboard: {
    writeText: (text: string) => clipboard.writeText(text),
    readImagePngBase64: () => {
      const image = clipboard.readImage();
      if (image.isEmpty()) {
        return null;
      }
      return { base64: image.toPNG().toString('base64') };
    },
  },
  revealInFileManager: (targetPath: string) => ipcRenderer.invoke('app:revealInFileManager', { path: targetPath }),

  // Backup for Untitled tabs
  backup: {
    save: (data: { id: string; title: string; content: string; editorId: string }) =>
      ipcRenderer.invoke('backup:save', data),
    load: () => ipcRenderer.invoke('backup:load'),
    delete: (tabId: string) => ipcRenderer.invoke('backup:delete', tabId),
  },

  // SSH config
  ssh: {
    listHosts: () => ipcRenderer.invoke('ssh:listHosts'),
    saveHost: (host: ManualSshHostPayload) => ipcRenderer.invoke('ssh:saveHost', host),
    deleteHost: (id: string) => ipcRenderer.invoke('ssh:deleteHost', { id }),
    pickIdentityFile: () => ipcRenderer.invoke('ssh:pickIdentityFile'),
  },
  remote: {
    connectSsh: (
      host: SshHostPayload,
      tabId: string
    ) => ipcRenderer.invoke('remote:connectSsh', { host, tabId }),
    disconnect: (tabId?: string) => ipcRenderer.invoke('remote:disconnect', { tabId }),
    status: (tabId: string) => ipcRenderer.invoke('remote:status', { tabId }),
  },

  window: {
    list: () => ipcRenderer.invoke('window:list'),
    getBootstrap: () => ipcRenderer.invoke('window:getBootstrap'),
    updateWorkspaceTabsSession: (session: unknown) => ipcRenderer.invoke('window:updateWorkspaceTabsSession', session),
    createNew: () => ipcRenderer.invoke('window:createNew'),
    createLocal: (options?: { path?: string }) => ipcRenderer.invoke('window:createLocal', options),
    createRemote: (host: SshHostPayload) =>
      ipcRenderer.invoke('window:createRemote', host),
    focus: (windowId: number) => ipcRenderer.invoke('window:focus', windowId),
    close: (windowId: number) => ipcRenderer.invoke('window:close', windowId),
    readyToClose: () => ipcRenderer.invoke('window:readyToClose'),
    setZoomLevel: (level: number) => webFrame.setZoomLevel(level),
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
    }>) => void) => {
      const listener = (_: unknown, payload: Array<{
        id: number;
        sessionId: string;
        label: string;
        mode: 'local' | 'remote';
        presentation: 'default' | 'newWindowLanding';
        workspaceId: string;
        workspacePath?: string;
        remoteHost?: SshHostPayload;
        active: boolean;
      }>) => handler(payload);
      ipcRenderer.on('window:listChanged', listener);
      return () => ipcRenderer.off('window:listChanged', listener);
    },
    onActiveChanged: (handler: (payload: { active: boolean }) => void) => {
      const listener = (_: unknown, payload: { active: boolean }) => handler(payload);
      ipcRenderer.on('window:activeChanged', listener);
      return () => ipcRenderer.off('window:activeChanged', listener);
    },
    onPrepareClose: (handler: () => void) => {
      const listener = () => handler();
      ipcRenderer.on('window:prepareClose', listener);
      return () => ipcRenderer.off('window:prepareClose', listener);
    },
  },

  runtimeBootstrap: {
    retry: () => ipcRenderer.invoke('runtimeBootstrap:retry'),
    quit: () => ipcRenderer.invoke('runtimeBootstrap:quit'),
    onChanged: (handler: (payload: RuntimeBootstrapStatePayload) => void) => {
      const listener = (_: unknown, payload: RuntimeBootstrapStatePayload) => handler(payload);
      ipcRenderer.on('runtimeBootstrap:changed', listener);
      return () => ipcRenderer.off('runtimeBootstrap:changed', listener);
    },
  },

  desktopUpdate: {
    getState: () => ipcRenderer.invoke('desktopUpdate:getState'),
    install: () => ipcRenderer.invoke('desktopUpdate:install'),
    onChanged: (handler: (payload: DesktopUpdateState) => void) => {
      const listener = (_: unknown, payload: DesktopUpdateState) => handler(payload);
      ipcRenderer.on('desktopUpdate:changed', listener);
      return () => ipcRenderer.off('desktopUpdate:changed', listener);
    },
  },

  settings: {
    get: () => ipcRenderer.invoke('settings:get'),
    getRoot: () => ipcRenderer.invoke('settings:getRoot'),
    set: (patch: any) => ipcRenderer.invoke('settings:set', patch),
    previewMarkdownTextOffset: (value: number) => ipcRenderer.send('settings:previewMarkdownTextOffset', { value }),
    previewMarkdownContentWidth: (value: number) => ipcRenderer.send('settings:previewMarkdownContentWidth', { value }),
    onChanged: (handler: (settings: SettingsState) => void) => {
      const listener = (_: unknown, payload: SettingsState) => handler(payload);
      ipcRenderer.on('settings:changed', listener);
      return () => ipcRenderer.off('settings:changed', listener);
    },
  },

  power: {
    setAgentRunning: (running: boolean) => ipcRenderer.invoke('power:setAgentRunning', { running }),
  },

  // Auth API
  auth: {
    get: () => ipcRenderer.invoke('auth:get'),
    startLogin: (options?: { gateway?: string; orgSlug?: string }) => ipcRenderer.invoke('auth:startLogin', options),
    listOrgs: () => ipcRenderer.invoke('auth:listOrgs'),
    setActiveOrg: (orgID?: string | null, orgName?: string | null) => ipcRenderer.invoke('auth:setActiveOrg', { orgID, orgName }),
    logout: () => ipcRenderer.invoke('auth:logout'),
    onChanged: (handler: (payload: {
      loggedIn: boolean;
      uid?: string;
      email?: string;
      activeOrgID?: string;
      activeOrgName?: string;
      reason?: 'logout' | 'session_expired';
      profile?: {
        uid: string;
        username: string;
        email?: string;
        avatar?: string;
        provider?: string;
      };
    }) => void) => {
      const listener = (_: unknown, payload: {
        loggedIn: boolean;
        uid?: string;
        email?: string;
        activeOrgID?: string;
        activeOrgName?: string;
        reason?: 'logout' | 'session_expired';
        profile?: {
          uid: string;
          username: string;
          email?: string;
          avatar?: string;
          provider?: string;
        };
      }) => handler(payload);
      ipcRenderer.on('auth:changed', listener);
      return () => ipcRenderer.off('auth:changed', listener);
    },
    onDeviceCode: (handler: (payload: { userCode: string; verificationUri: string; expiresAt: number }) => void) => {
      const listener = (_: unknown, payload: { userCode: string; verificationUri: string; expiresAt: number }) =>
        handler(payload);
      ipcRenderer.on('auth:deviceCode', listener);
      return () => ipcRenderer.off('auth:deviceCode', listener);
    },
    onDeviceCodeComplete: (handler: (payload: { success: boolean; error?: string }) => void) => {
      const listener = (_: unknown, payload: { success: boolean; error?: string }) => handler(payload);
      ipcRenderer.on('auth:deviceCodeComplete', listener);
      return () => ipcRenderer.off('auth:deviceCodeComplete', listener);
    },
  },

  // Profile API
  profile: {
    get: () => ipcRenderer.invoke('profile:get'),
    refresh: () => ipcRenderer.invoke('profile:refresh'),
  },

  billing: {
    getSubscription: () => ipcRenderer.invoke('billing:getSubscription'),
  },

  dashboard: {
    getHosts: () => ipcRenderer.invoke('dashboard:getHosts'),
  },

  nodes: {
    get: () => ipcRenderer.invoke('nodes:get'),
    upsert: (hostId: string, nodes: Array<Record<string, unknown>>) =>
      ipcRenderer.invoke('nodes:upsert', { hostId, nodes }),
  },

  avatar: {
    cacheNode: (hostId: string, node: Record<string, unknown>) =>
      ipcRenderer.invoke('avatar:cacheNode', { hostId, node }),
  },

  onConfigSyncPush: (handler: (payload: { files: Array<{ name: string; content: string }> }) => void) => {
    const listener = (_: unknown, payload: { files: Array<{ name: string; content: string }> }) => handler(payload);
    ipcRenderer.on('configSync:push', listener);
    return () => ipcRenderer.off('configSync:push', listener);
  },

  // Models API
  models: {
    get: () => ipcRenderer.invoke('models:get'),
    set: (config: ModelsConfig) => ipcRenderer.invoke('models:set', config),
    refreshFromOpenBrain: () => ipcRenderer.invoke('models:refreshFromOpenBrain'),
  },

  workspace: {
    listTemplates: (input?: { orgID?: string | null; targetOrgID?: string | null }) => ipcRenderer.invoke('workspace:listTemplates', input),
    listOpenBrains: () => ipcRenderer.invoke('workspace:listOpenBrains'),
    openStorageBackendSettings: (input?: { storageBackend?: string; provider?: string }) =>
      ipcRenderer.invoke('workspace:openStorageBackendSettings', input),
    createOpenBrain: (input?: { name?: string; localPath?: string }) => ipcRenderer.invoke('workspace:createOpenBrain', input),
    createFromTemplate: (input?: { templateID?: string; storageBackend?: string; provider?: string; repositoryOwner?: string; repositoryName?: string; name?: string; orgID?: string | null; targetOrgID?: string | null; localPath?: string }) =>
      ipcRenderer.invoke('workspace:createFromTemplate', input),
  },

  openBrain: {
    getProvider: () => ipcRenderer.invoke('openBrain:getProvider'),
    setProvider: (input?: { provider?: 'cloud' | 'local'; local?: Record<string, unknown> }) =>
      ipcRenderer.invoke('openBrain:setProvider', input),
    listSources: () => ipcRenderer.invoke('openBrain:listSources'),
    query: (input?: { brainID?: string; scope?: 'brain' | 'workspace'; workspaceID?: string; orgID?: string; publicOwnerUID?: string; query?: string; limit?: number }) =>
      ipcRenderer.invoke('openBrain:query', input),
    createSource: (input?: { name?: string; localPath?: string; remotePath?: string; tabId?: string; remoteHost?: SshHostPayload }) =>
      ipcRenderer.invoke('openBrain:createSource', input),
    removeSourceFromDevice: (input?: { sourceID?: string; workspaceID?: string; orgID?: string; path?: string }) =>
      ipcRenderer.invoke('openBrain:removeSourceFromDevice', input),
    archiveSource: (input?: { sourceID?: string; workspaceID?: string; orgID?: string; path?: string }) =>
      ipcRenderer.invoke('openBrain:archiveSource', input),
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
    }) => ipcRenderer.invoke('openBrain:sourceAction', input),
  },

  // Platform info
  platform: process.platform,
  isDev: process.env.NODE_ENV === 'development',
});

// Type definitions for the exposed API
declare global {
  type RecentRemoteHost = import('./shared/recentWorkspaces').RecentRemoteHost;
  type RecentRemoteWorkspaceBucket = import('./shared/recentWorkspaces').RemoteRecentWorkspaceBucket;
  type RecentWorkspaces = import('./shared/recentWorkspaces').RecentWorkspaces;

  type UserSettings = {
    version: number;
    recentWorkspaces?: RecentWorkspaces;
    openBrain?: {
      provider?: 'cloud' | 'local';
      local?: {
        engine?: 'pglite' | 'postgres';
        databaseUrl?: string;
        databasePath?: string;
        remoteMcpUrl?: string;
        remoteMcpClientID?: string;
        remoteMcpClientSecret?: string;
        remoteMcpClientSecretEnvVar?: string;
        cliPath?: string;
      };
    };
  };

  type SettingsState = {
    system: any;
    user: UserSettings;
    ui: any;
    editor: any;
    terminal: any;
    profiles: any[];
    defaultProfileId: string;
    keybindings: any[];
    theme?: any;
  };

  type ModelAPI = 'openai-completions' | 'openai-responses' | 'anthropic-messages' | 'gemini-native';
  type ModelServiceTier = 'priority' | 'flex';
  type ModelReasoningControl = 'level' | 'toggle';

  type ModelEntry = {
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

  type ProviderModelEntry = {
    id: string;
    label?: string;
    enabled: boolean;
    api?: ModelAPI;
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

  type ProviderEntry = {
    label?: string;
    api?: ModelAPI;
    baseUrl?: string;
    apiKey?: string;
    managed?: boolean;
    models: ProviderModelEntry[];
  };

  type ModelAutoStrategy = {
    defaultChatModelID?: string;
    defaultChatThinkingLevel?: string;
    defaultInlineCompletionModelID?: string;
    defaultInlineCompletionThinkingLevel?: string;
  };

  type ModelStrategies = {
    auto?: ModelAutoStrategy;
  };

  type ModelPreference = {
    thinkingLevel?: string;
    contextWindow?: number;
    serviceTier?: ModelServiceTier | null;
  };

	  type ModelsConfig = {
	    version: number;
	    defaultModelKey: string | null;
	    providers: Record<string, ProviderEntry>;
	    models: ModelEntry[];
    strategies?: ModelStrategies;
    modelPreferences?: Record<string, ModelPreference>;
    updatedAt: number;
  };

  type BillingSubscription = {
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
  };

  type RuntimeBootstrapState = {
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
  };

  type DesktopUpdateState = {
    phase: 'unsupported' | 'idle' | 'checking' | 'downloading' | 'ready' | 'installing' | 'error';
    currentVersion: string | null;
    targetVersion: string | null;
    error?: string;
  };

  type DashboardRuntimeConnection = {
    nodeID: string;
    name: string;
    transport: string;
    daemon?: boolean;
    pid?: number;
    startedAt?: string;
    uptimeSec?: number;
    lastActiveAt?: string;
    url?: string;
  };

  type DashboardRuntimeUpdater = {
    currentVersion?: string;
    targetVersion?: string;
    stagedVersion?: string;
    phase?: string;
    downloaded?: boolean;
    applying?: boolean;
    lastCheckedAt?: string;
    lastError?: string;
  };

  type DashboardHost = {
    id: string;
    hostname?: string;
    env?: string;
    baseDir?: string;
    online: boolean;
    lastSeenAt?: string;
    receivedAt?: string;
    runtimeConnections: DashboardRuntimeConnection[];
    runtimeUpdater?: DashboardRuntimeUpdater;
  };

  interface Window {
    electronAPI: {
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
      ssh: {
        listHosts: () => Promise<SshHostPayload[]>;
        saveHost: (host: ManualSshHostPayload) => Promise<SshHostPayload>;
        deleteHost: (id: string) => Promise<{ success: boolean }>;
        pickIdentityFile: () => Promise<{ canceled: boolean; path?: string }>;
      };
      remote: {
        connectSsh: (
          host: SshHostPayload,
          tabId: string
        ) => Promise<{
          hostLabel: string;
          localPort: number;
          remotePort: number;
          wsUrl: string;
          httpUrl: string;
          remoteHome: string;
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
          installDir: string;
        } | null>;
      };
      settings: {
        get: () => Promise<SettingsState>;
        getRoot: () => Promise<string>;
        set: (patch: Partial<SettingsState>) => Promise<SettingsState>;
        previewMarkdownTextOffset: (value: number) => void;
        previewMarkdownContentWidth: (value: number) => void;
        onChanged: (handler: (settings: SettingsState) => void) => () => void;
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
          profile?: {
            uid: string;
            name: string;
            username: string;
            email?: string;
            avatar?: string;
            provider?: string;
            updatedAt?: number;
          };
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
          profile?: {
            uid: string;
            name: string;
            username: string;
            email?: string;
            avatar?: string;
            provider?: string;
          };
        }) => void) => () => void;
        onDeviceCode: (handler: (payload: { userCode: string; verificationUri: string; expiresAt: number }) => void) => () => void;
        onDeviceCodeComplete: (handler: (payload: { success: boolean; error?: string }) => void) => () => void;
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
                accounts?: Array<{
                  owner: string;
                  accountType?: string;
                  installationID?: string;
                  connectedAt?: string;
                  canCreateRepository?: boolean;
                  canSyncRepository?: boolean;
                  permissionState?: string;
                  permissionMessage?: string;
                }>;
              }>;
            };
            storage?: {
              enabled: boolean;
              backend?: string;
              provider?: string;
              region?: string;
              remoteID?: string;
              remoteName?: string;
              remoteURL?: string;
              connectedAs?: string;
              providers?: Array<{
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
                accounts?: Array<{
                  owner: string;
                  accountType?: string;
                  installationID?: string;
                  connectedAt?: string;
                  canCreateRepository?: boolean;
                  canSyncRepository?: boolean;
                  permissionState?: string;
                  permissionMessage?: string;
                }>;
              }>;
              syncPolicy?: {
                autoSync?: boolean;
                onOpen?: boolean;
                onLocalChange?: boolean;
                intervalSec?: number;
                conflict?: string;
                deleteMode?: string;
              };
            };
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
        setProvider: (input?: { provider?: 'cloud' | 'local'; local?: {
          engine?: 'pglite' | 'postgres';
          databaseUrl?: string;
          databasePath?: string;
          remoteMcpUrl?: string;
          remoteMcpClientID?: string;
          remoteMcpClientSecret?: string;
          remoteMcpClientSecretEnvVar?: string;
          cliPath?: string;
        } }) => Promise<{ provider: 'cloud' | 'local'; authRequired?: boolean; configured: boolean }>;
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
            bindingStatus?: 'connected' | 'needs_binding';
            bindingReason?: 'unbound' | 'moved' | 'mismatch';
            lastVerifiedAt?: string;
            lastVerifyReason?: string;
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
        get: () => Promise<{
          uid: string;
          name: string;
          username: string;
          email?: string;
          avatar?: string;
          provider?: string;
          updatedAt?: number;
        } | null>;
        refresh: () => Promise<{
          success: boolean;
          error?: string;
          authInvalid?: boolean;
          activeOrgID?: string;
          activeOrgName?: string;
          profile?: {
            uid: string;
            name: string;
            username: string;
            email?: string;
            avatar?: string;
            provider?: string;
          };
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
        list: () => Promise<Array<{
          id: number;
          label: string;
          mode: 'local' | 'remote';
          presentation: 'default' | 'newWindowLanding';
          authRequired?: boolean;
          workspaceId: string;
          workspacePath?: string;
          remoteHost?: SshHostPayload;
          active: boolean;
        }>>;
        getBootstrap: () => Promise<{
          windowId: number;
          info: {
            id: number;
            label: string;
            mode: 'local' | 'remote';
            presentation: 'default' | 'newWindowLanding';
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
          initialWorkspace: {
            mode: 'local' | 'remote';
            workspacePath?: string;
            remoteHost?: SshHostPayload;
          };
          runtimeBootstrap?: RuntimeBootstrapState | null;
        } | null>;
        createNew: () => Promise<{ success: boolean }>;
        createLocal: (options?: { path?: string }) => Promise<{ canceled: boolean }>;
        createRemote: (host: SshHostPayload) => Promise<{ success: boolean }>;
        focus: (windowId: number) => Promise<{ success: boolean }>;
        close: (windowId: number) => Promise<{ success: boolean }>;
        readyToClose: () => Promise<{ success: boolean }>;
        setZoomLevel: (level: number) => void;
        onListChanged: (handler: (windows: Array<{
          id: number;
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
      platform: NodeJS.Platform;
      isDev: boolean;
    };
  }
}
