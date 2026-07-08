import React, { useCallback, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { useAppStore } from '../../store/appStore';
import { useAuthStore } from '../../store/authStore';
import { canManageOpenBrainSource, useOpenBrainStore, type LocalOpenBrainWorkspace, type PendingOpenBrainSource } from '../../store/openBrainStore';
import { useToastStore } from '../../store/toastStore';
import { useUiStore } from '../../store/uiStore';
import { MoreHorizontalIcon, PlusIcon, RefreshIcon, SettingsIcon } from '../Icons';
import { IconButton } from '../IconButton';
import {
  CloudSourceActionMenuItems,
  HardDeleteCloudSourceDialog,
  sourceActionSuccessMessage,
  type CloudSourceActionPayload,
} from '../OpenBrain/CloudSourceActions';
import { resolveOpenBrainSourceDisplayState } from '../OpenBrain/openBrainSourceDisplay';
import { PopupMenu } from '../PopupMenu';

type OpenBrainSidebarProps = {
  onOpenWorkspace: (workspace: LocalOpenBrainWorkspace) => Promise<void>;
  onCreateSource: () => Promise<void>;
  onBindSource: (workspace: LocalOpenBrainWorkspace) => Promise<void>;
};

const SOURCE_ACTION_MENU_WIDTH = 288;
const SOURCE_ACTION_MENU_HEIGHT = 242;
const SOURCE_ACTION_MENU_MARGIN = 8;

function sourceActionMenuPosition(trigger: HTMLButtonElement): { left: number; top: number } {
  const rect = trigger.getBoundingClientRect();
  const maxLeft = window.innerWidth - SOURCE_ACTION_MENU_WIDTH - SOURCE_ACTION_MENU_MARGIN;
  const maxTop = window.innerHeight - SOURCE_ACTION_MENU_HEIGHT - SOURCE_ACTION_MENU_MARGIN;
  // Pop out to the right of the trigger (into the main content area),
  // matching the AgentsSidebar/BaseDirResourceSidebar root menu placement.
  return {
    left: Math.min(rect.right + 4, maxLeft),
    top: Math.min(Math.max(SOURCE_ACTION_MENU_MARGIN, rect.top), maxTop),
  };
}

function isSameOpenBrainSource(a: LocalOpenBrainWorkspace, b: LocalOpenBrainWorkspace): boolean {
  const aIDs = [a.sourceID, a.workspaceID].map((value) => (value || '').trim()).filter(Boolean);
  const bIDs = [b.sourceID, b.workspaceID].map((value) => (value || '').trim()).filter(Boolean);
  if (aIDs.some((id) => bIDs.includes(id))) {
    return true;
  }
  const aRemote = normalizeOpenBrainRemote(a.remoteURL);
  const bRemote = normalizeOpenBrainRemote(b.remoteURL);
  if (aRemote && bRemote && aRemote === bRemote) {
    return true;
  }
  return Boolean(a.path && b.path && normalizeOpenBrainPath(a.path) === normalizeOpenBrainPath(b.path));
}

function normalizeOpenBrainRemote(value: string | null | undefined): string {
  return (value || '').trim().replace(/\/+$/, '').replace(/\.git$/i, '').toLowerCase();
}

function normalizeOpenBrainPath(value: string | null | undefined): string {
  return (value || '').trim().replace(/\/+$/, '');
}

export function OpenBrainSidebar({ onOpenWorkspace, onCreateSource, onBindSource }: OpenBrainSidebarProps) {
  const { t } = useTranslation(['sidebar']);
  const openOpenBrainSettingsTab = useAppStore((state) => state.openOpenBrainSettingsTab);
  const loggedIn = useAuthStore((state) => state.loggedIn);
  const authRevision = useAuthStore((state) => state.authRevision);
  const deviceCodeError = useAuthStore((state) => state.deviceCodeError);
  const startLogin = useAuthStore((state) => state.startLogin);
  const pushToast = useToastStore((state) => state.pushToast);
  const brains = useOpenBrainStore((state) => state.sources);
  const pendingSources = useOpenBrainStore((state) => state.pendingSources);
  const provider = useOpenBrainStore((state) => state.provider);
  const authRequired = useOpenBrainStore((state) => state.authRequired);
  const loading = useOpenBrainStore((state) => state.loading);
  const error = useOpenBrainStore((state) => state.error);
  const refresh = useOpenBrainStore((state) => state.refresh);
  const hydrateCachedSources = useOpenBrainStore((state) => state.hydrateCachedSources);
  const applySourceAction = useOpenBrainStore((state) => state.applySourceAction);
  const dismissPendingOpenBrainSource = useOpenBrainStore((state) => state.dismissPendingOpenBrainSource);
  const isSourceLinked = useOpenBrainStore((state) => state.isSourceLinked);
  const [creating, setCreating] = useState(false);
  const [signingIn, setSigningIn] = useState(false);
  const [menuSourceID, setMenuSourceID] = useState<string | null>(null);
  const [menuPosition, setMenuPosition] = useState<{ left: number; top: number } | null>(null);
  const [mutatingSourceID, setMutatingSourceID] = useState<string | null>(null);
  const [hardDeleteDialogSource, setHardDeleteDialogSource] = useState<LocalOpenBrainWorkspace | null>(null);
  const [sourceActionError, setSourceActionError] = useState<string | null>(null);
  const postLoginRefreshRevisionRef = useRef<number | null>(null);
  const lastDeviceCodeErrorRef = useRef<string | null>(null);

  const handleOpenSettings = () => {
    useUiStore.getState().setSidebarView('workspace');
    openOpenBrainSettingsTab();
  };

  const handleSignIn = async () => {
    if (signingIn) {
      return;
    }
    setSigningIn(true);
    try {
      const result = await startLogin();
      if (!result?.success) {
        throw new Error('Failed to start OpenBrain sign in.');
      }
    } catch (err) {
      pushToast(err instanceof Error ? err.message : 'Failed to start OpenBrain sign in.');
    } finally {
      setSigningIn(false);
    }
  };

  useEffect(() => {
    if (!authRequired || provider !== 'cloud') {
      postLoginRefreshRevisionRef.current = null;
      return;
    }
    if (!loggedIn || loading || postLoginRefreshRevisionRef.current === authRevision) {
      return;
    }
    postLoginRefreshRevisionRef.current = authRevision;
    void refresh().catch(() => {});
  }, [authRequired, authRevision, loading, loggedIn, provider, refresh]);

  useEffect(() => {
    if (!deviceCodeError || lastDeviceCodeErrorRef.current === deviceCodeError) {
      return;
    }
    lastDeviceCodeErrorRef.current = deviceCodeError;
    pushToast(deviceCodeError);
  }, [deviceCodeError, pushToast]);

  const closeSourceActionMenu = useCallback(() => {
    setMenuSourceID(null);
    setMenuPosition(null);
  }, []);

  useEffect(() => {
    if (!menuSourceID) {
      return;
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        closeSourceActionMenu();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [closeSourceActionMenu, menuSourceID]);

  useEffect(() => {
    if (!menuSourceID) {
      return;
    }
    if (!brains.some((brain) => brain.sourceID === menuSourceID)) {
      closeSourceActionMenu();
    }
  }, [brains, closeSourceActionMenu, menuSourceID]);

  const handleCreate = async () => {
    if (creating) {
      return;
    }
    setCreating(true);
    try {
      await onCreateSource();
    } catch (err) {
      pushToast(err instanceof Error ? err.message : 'Failed to create OpenBrain source.');
    } finally {
      setCreating(false);
    }
  };

  const handleSelect = async (brain: LocalOpenBrainWorkspace) => {
    try {
      let source = brain;
      if (!source.path) {
        const cachedSources = await hydrateCachedSources().catch(() => []);
        source = cachedSources.find((candidate) => isSameOpenBrainSource(candidate, brain))
          || useOpenBrainStore.getState().sources.find((candidate) => isSameOpenBrainSource(candidate, brain))
          || brain;
      }
      if (source.bindingStatus === 'needs_binding' || !source.path) {
        await onBindSource(source);
        return;
      }
      await onOpenWorkspace(source);
    } catch (err) {
      pushToast(err instanceof Error ? err.message : 'Failed to open OpenBrain source.');
    }
  };

  const handleApplySourceAction = async (brain: LocalOpenBrainWorkspace, action: CloudSourceActionPayload) => {
    if (mutatingSourceID) {
      return;
    }
    if (!canManageOpenBrainSource(brain)) {
      pushToast('This source is read-only on this brain.');
      return;
    }
    closeSourceActionMenu();
    setSourceActionError(null);
    setMutatingSourceID(brain.sourceID);
    try {
      const result = await applySourceAction(brain, action);
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
  };

  const handleOpenHardDeleteDialog = (brain: LocalOpenBrainWorkspace) => {
    if (mutatingSourceID) {
      return;
    }
    if (!canManageOpenBrainSource(brain)) {
      pushToast('This source is read-only on this brain.');
      return;
    }
    closeSourceActionMenu();
    setSourceActionError(null);
    setHardDeleteDialogSource(brain);
  };

  const handleDismissPendingSource = (pending: PendingOpenBrainSource) => {
    dismissPendingOpenBrainSource(pending.pendingID);
  };

  const pendingStatusLabel = (pending: PendingOpenBrainSource): string => {
    if (pending.status === 'failed') {
      return pending.error || t('openBrainSourceCreateFailed');
    }
    if (pending.rebinding) {
      return t('openBrainSourceBinding');
    }
    return provider === 'cloud'
      ? t('openBrainSourceAddingToCloud')
      : t('openBrainSourceCreating');
  };

  let body: React.ReactNode;
  if (loading && brains.length === 0 && pendingSources.length === 0) {
    body = <div className="px-3 py-2 text-xs text-secondary-text">Loading OpenBrain sources...</div>;
  } else if (authRequired && !loggedIn) {
    body = (
      <div className="px-3 py-2 text-xs">
        <button
          type="button"
          className="cursor-pointer border-0 bg-transparent p-0 text-left text-xs text-active-border underline decoration-active-border/60 underline-offset-2 hover:text-highlight focus:outline-none focus:ring-1 focus:ring-highlight disabled:cursor-wait disabled:opacity-60"
          onClick={() => void handleSignIn()}
          disabled={signingIn}
        >
          {signingIn ? t('openBrainOpeningSignIn') : t('openBrainSignIn')}
        </button>
      </div>
    );
  } else if (authRequired && loggedIn) {
    body = (
      <div className="px-3 py-2 text-xs text-accent">
        OpenBrain Cloud is not available for this account.
      </div>
    );
  } else if (error) {
    body = <div className="px-3 py-2 text-xs text-accent">{error}</div>;
  } else if (brains.length === 0 && pendingSources.length === 0) {
    body = <div className="px-3 py-2 text-xs text-secondary-text">No OpenBrain source found.</div>;
  } else {
    body = (
      <div className="flex flex-col gap-1 px-2 py-1">
        {pendingSources.map((pending) => {
          const statusLabel = pendingStatusLabel(pending);
          const failed = pending.status === 'failed';
          return (
            <div
              key={pending.pendingID}
              className={`group relative flex min-h-[44px] w-full items-center gap-1 rounded border border-transparent ${failed ? 'bg-accent/5' : 'bg-hover-bg/40'}`}
            >
              <div className="flex min-w-0 flex-1 flex-col items-start justify-center px-2 py-1.5 text-left text-sm">
                <span className={`w-full truncate font-medium ${failed ? 'text-prime-text' : 'text-secondary-text'}`}>{pending.name}</span>
                <span className={`flex w-full items-center gap-1 truncate text-xs ${failed ? 'text-accent' : 'text-secondary-text'}`}>
                  {!failed ? <RefreshIcon className="h-3 w-3 shrink-0 animate-spin text-active-border" /> : null}
                  <span className="truncate">{statusLabel}</span>
                </span>
                {!failed && pending.path ? (
                  <span className="w-full truncate text-[11px] text-secondary-text/80">{pending.path}</span>
                ) : null}
              </div>
              {failed ? (
                <button
                  type="button"
                  className="mr-1 flex h-7 w-7 shrink-0 items-center justify-center rounded text-secondary-text hover:bg-secondary-bg hover:text-prime-text focus:outline-none"
                  aria-label={t('openBrainSourceDismissFailed')}
                  title={t('openBrainSourceDismissFailed')}
                  onClick={() => handleDismissPendingSource(pending)}
                >
                  ×
                </button>
              ) : null}
            </div>
          );
        })}
        {brains.map((brain) => {
          const mutating = mutatingSourceID === brain.sourceID;
          const display = resolveOpenBrainSourceDisplayState(brain, {
            provider,
            uiLinked: isSourceLinked(brain.sourceID),
          });
          const bindingLabel = display.detail ?? display.statusText;
          return (
            <div
              key={brain.sourceID}
              className="group relative flex min-h-[44px] w-full items-center gap-1 rounded border border-transparent hover:bg-hover-bg focus-within:border-highlight"
            >
              <button
                type="button"
                className="flex min-w-0 flex-1 flex-col items-start justify-center px-2 py-1.5 text-left text-sm focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
                onClick={() => void handleSelect(brain)}
                disabled={mutating}
                title={bindingLabel}
              >
                <span className="w-full truncate font-medium text-prime-text">{brain.name}</span>
                <span className="w-full truncate text-xs text-secondary-text">{bindingLabel}</span>
              </button>
              {provider === 'cloud' ? (
                <button
                  type="button"
                  className="mr-1 flex h-7 w-7 shrink-0 items-center justify-center rounded text-secondary-text opacity-0 hover:bg-secondary-bg hover:text-prime-text focus:opacity-100 focus:outline-none group-hover:opacity-100 disabled:cursor-wait disabled:opacity-50"
                  aria-label={`Open source actions for ${brain.name}`}
                  title="Source actions"
                  disabled={mutating}
                  onClick={(event) => {
                    event.stopPropagation();
                    if (menuSourceID === brain.sourceID) {
                      closeSourceActionMenu();
                      return;
                    }
                    setMenuPosition(sourceActionMenuPosition(event.currentTarget));
                    setMenuSourceID(brain.sourceID);
                  }}
                >
                  <MoreHorizontalIcon className="h-4 w-4" />
                </button>
              ) : null}
            </div>
          );
        })}
      </div>
    );
  }

  const menuSource = menuSourceID
    ? brains.find((brain) => brain.sourceID === menuSourceID) ?? null
    : null;

  return (
    <div className="flex h-full flex-col">
      <div className="ui-tabbar sidebar-root-header openbrain-sidebar-header flex shrink-0 items-center gap-1 overflow-hidden px-2 text-secondary-text">
        <div className="flex min-w-0 flex-1 items-center">
          <span className="ui-chrome-row-label truncate">OpenBrain · {provider === 'local' ? 'Local' : 'Cloud'}</span>
        </div>
        <div className="sidebar-root-header-actions ml-auto flex shrink-0 items-center gap-0.5">
          <IconButton
            title="Create OpenBrain source"
            aria-label="Create OpenBrain source"
            disabled={creating}
            onClick={() => void handleCreate()}
          >
            <PlusIcon className="h-3.5 w-3.5" />
          </IconButton>
          <IconButton
            title="Refresh OpenBrain sources"
            aria-label="Refresh OpenBrain sources"
            disabled={loading}
            onClick={() => void refresh().catch(() => {})}
          >
            <RefreshIcon className="h-3.5 w-3.5" />
          </IconButton>
          <IconButton
            title="OpenBrain settings"
            aria-label="OpenBrain settings"
            onClick={handleOpenSettings}
          >
            <SettingsIcon className="h-3.5 w-3.5" />
          </IconButton>
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-auto">{body}</div>
      {menuSource && menuPosition ? createPortal(
        <>
          <button
            type="button"
            className="no-drag fixed inset-0 z-[60] cursor-default border-0 bg-transparent p-0"
            aria-label="Close source menu"
            onMouseDown={closeSourceActionMenu}
          />
          <PopupMenu
            className="no-drag fixed z-[70] w-72"
            style={{ left: menuPosition.left, top: menuPosition.top }}
            onMouseDown={(event) => event.stopPropagation()}
            onContextMenu={(event) => event.preventDefault()}
          >
                <CloudSourceActionMenuItems
                  source={menuSource}
                  disabled={mutatingSourceID === menuSource.sourceID}
                  onBindSource={(source) => void onBindSource(source)}
                  onApplyAction={(source, action) => void handleApplySourceAction(source, action)}
                  onOpenHardDelete={handleOpenHardDeleteDialog}
                />
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
    </div>
  );
}
