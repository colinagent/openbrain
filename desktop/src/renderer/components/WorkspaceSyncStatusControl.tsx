import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useAppStore, type WorkspaceStorageInfo } from '../store/appStore';
import { useToastStore } from '../store/toastStore';
import { useUiStore } from '../store/uiStore';
import type { CronTaskRecord } from '../services/cronService';
import { cronIntervalOptionsWithCurrent, parseCronEveryToSeconds } from '../utils/cronSchedule';
import {
  cronDataString,
  cronTaskPath,
  cronTaskSchedule,
  cronTaskStatus,
  cronTaskTitle,
  formatCronTime,
} from '../utils/cronDisplay';
import { AlertCircleIcon, CheckCircleIcon, ClockIcon, LoaderIcon, RefreshSyncIcon } from './Icons';
import { PopupMenu } from './PopupMenu';
import { BillingRestrictionCard } from './BillingRestrictionCard';
import { resolveRunErrorDisplay } from './Chat/activityErrorState';

function providerLabel(info: WorkspaceStorageInfo): string {
  const backend = (info.storage?.backend || '').trim();
  const provider = (info.storage?.provider || '').trim();
  if (backend === 'git') {
    return provider ? `Git (${provider})` : 'Git';
  }
  if (provider === 'google-drive') {
    return 'Google Drive';
  }
  if (provider === 'lark-drive' || provider === 'feishu') {
    return 'Lark Drive';
  }
  return provider || backend || 'Local';
}

function SyncPopupError({ error }: { error: string }) {
  const { billingInfo, rawError } = resolveRunErrorDisplay(error);
  if (billingInfo) {
    return (
      <div className="mt-1">
        <BillingRestrictionCard info={billingInfo} compact surface="none" />
      </div>
    );
  }
  return <div className="whitespace-pre-wrap text-accent">{rawError}</div>;
}

function statusText(info: WorkspaceStorageInfo, task: CronTaskRecord | null, taskError?: string | null): string {
  if (!info.storage?.enabled) {
    return 'Local only';
  }
  if (info.status === 'loading') {
    return 'Loading';
  }
  if ((taskError || '').trim()) {
    return 'Sync error';
  }
  if (task) {
    const status = cronTaskStatus(task);
    if (status.tone === 'running') {
      return 'Syncing';
    }
    if (status.tone === 'error') {
      return 'Sync error';
    }
    if (!task.task.enabled) {
      return 'Sync off';
    }
    return 'Sync ready';
  }
  if (info.status === 'error') {
    return 'Sync error';
  }
  return 'Sync ready';
}

function formatLastSync(value: string | null): string {
  if (!value) {
    return 'Never';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

function samePath(left: string, right: string): boolean {
  return left.trim() !== '' && left.trim() === right.trim();
}

function isOpenBrainCloudSyncTask(record: CronTaskRecord): boolean {
  return record.task.id === 'task-openbrain-cloud-sync'
    || cronDataString(record, 'managedKind') === 'openbrain-cloud-sync';
}

function cronWorkspaceList(record: CronTaskRecord): Array<Record<string, unknown>> {
  const data = record.task.payload?.data || {};
  const selectedContext = data.selectedSkillContext;
  const fromContext = selectedContext && typeof selectedContext === 'object' && !Array.isArray(selectedContext)
    ? (selectedContext as Record<string, unknown>).workspaces
    : undefined;
  const raw = Array.isArray(fromContext) ? fromContext : data.workspaces;
  return Array.isArray(raw)
    ? raw.filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === 'object' && !Array.isArray(item))
    : [];
}

function cronWorkspaceString(workspace: Record<string, unknown>, key: string): string {
  const value = workspace[key];
  return typeof value === 'string' ? value.trim() : '';
}

function findWorkspaceCronTask(records: CronTaskRecord[], info: WorkspaceStorageInfo, currentDir: string): CronTaskRecord | null {
  const workspaceID = (info.workspaceID || '').trim();
  const path = (info.path || currentDir || '').trim();
  const candidates = records.filter(isOpenBrainCloudSyncTask);
  if (workspaceID) {
    const byWorkspaceID = candidates.find((record) =>
      cronWorkspaceList(record).some((workspace) => cronWorkspaceString(workspace, 'workspaceID') === workspaceID),
    );
    if (byWorkspaceID) {
      return byWorkspaceID;
    }
  }
  return candidates.find((record) =>
    cronWorkspaceList(record).some((workspace) => samePath(cronWorkspaceString(workspace, 'workspacePath'), path))
      || samePath(cronTaskPath(record), path),
  ) || null;
}

export function WorkspaceSyncStatusControl() {
  const connectionState = useAppStore((state) => state.connectionState);
  const currentDir = useAppStore((state) => state.currentDir);
  const storageInfo = useAppStore((state) => state.storageInfo);
  const refreshStorageStatus = useAppStore((state) => state.refreshStorageStatus);
  const updateWorkspaceSyncPolicy = useAppStore((state) => state.updateWorkspaceSyncPolicy);
  const syncWorkspaceNow = useAppStore((state) => state.syncWorkspaceNow);
  const listCronTasks = useAppStore((state) => state.listCronTasks);
  const openCronTaskTab = useAppStore((state) => state.openCronTaskTab);
  const setSidebarView = useUiStore((state) => state.setSidebarView);
  const pushToast = useToastStore((state) => state.pushToast);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const [open, setOpen] = useState(false);
  const [cronTask, setCronTask] = useState<CronTaskRecord | null>(null);
  const [taskLoading, setTaskLoading] = useState(false);
  const [taskError, setTaskError] = useState<string | null>(null);

  const refreshCronTask = useCallback(async (info?: WorkspaceStorageInfo, options?: { background?: boolean }) => {
    if (connectionState !== 'connected' || !currentDir) {
      setCronTask(null);
      setTaskError(null);
      return null;
    }
    if (!options?.background) {
      setTaskLoading(true);
    }
    try {
      const records = await listCronTasks();
      const sourceInfo = info || useAppStore.getState().storageInfo;
      const task = findWorkspaceCronTask(records, sourceInfo, currentDir);
      setCronTask(task);
      setTaskError(null);
      return task;
    } catch (cause) {
      setTaskError(cause instanceof Error ? cause.message : 'Failed to load cron task.');
      return null;
    } finally {
      if (!options?.background) {
        setTaskLoading(false);
      }
    }
  }, [connectionState, currentDir, listCronTasks]);

  useEffect(() => {
    if (connectionState === 'connected' && currentDir) {
      void (async () => {
        const info = await refreshStorageStatus(currentDir);
        if (info) {
          await refreshCronTask(info);
        }
      })();
      return;
    }
    setOpen(false);
    setCronTask(null);
    setTaskError(null);
  }, [connectionState, currentDir, refreshCronTask, refreshStorageStatus]);

  useEffect(() => {
    if (connectionState !== 'connected' || !currentDir || !storageInfo.storage?.enabled) {
      return undefined;
    }
    const timer = window.setInterval(() => {
      void refreshCronTask(undefined, { background: true });
    }, 5000);
    return () => window.clearInterval(timer);
  }, [connectionState, currentDir, refreshCronTask, storageInfo.storage?.enabled, storageInfo.workspaceID, storageInfo.path]);

  useEffect(() => {
    if (!open) {
      return undefined;
    }
    const handleMouseDown = (event: MouseEvent) => {
      if (rootRef.current?.contains(event.target as Node)) {
        return;
      }
      setOpen(false);
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setOpen(false);
      }
    };
    window.addEventListener('mousedown', handleMouseDown, true);
    window.addEventListener('keydown', handleKeyDown);
    return () => {
      window.removeEventListener('mousedown', handleMouseDown, true);
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [open]);

  const taskStatus = cronTask ? cronTaskStatus(cronTask) : null;
  const busy = storageInfo.status === 'loading' || taskStatus?.tone === 'running' || taskLoading;
  const enabled = Boolean(storageInfo.storage?.enabled);
  const iconClass = storageInfo.status === 'error' || taskError || taskStatus?.tone === 'error'
    ? 'h-3.5 w-3.5 text-accent'
    : taskStatus?.tone === 'ok'
      ? 'h-3.5 w-3.5 text-health-text'
      : 'h-3.5 w-3.5';
  const intervalSec = useMemo(
    () => parseCronEveryToSeconds(cronTask?.task.schedule?.every),
    [cronTask],
  );
  const intervalOptions = useMemo(
    () => cronIntervalOptionsWithCurrent(intervalSec),
    [intervalSec],
  );

  if (connectionState !== 'connected' || !currentDir) {
    return null;
  }

  const handleSyncNow = async () => {
    const info = await syncWorkspaceNow({ reason: 'manual' });
    if (!info) {
      return;
    }
    const task = await refreshCronTask(info);
    if (info.error) {
      pushToast(info.error, { durationMs: 5000 });
      return;
    }
    if (!task && info.storage?.enabled) {
      pushToast('Cron task is not ready yet', { durationMs: 4000 });
      return;
    }
    pushToast('Workspace sync queued');
  };

  const saveTaskPatch = async (patch: { enabled?: boolean; intervalSec?: number }) => {
    const nextPolicy = {
      ...storageInfo.policy,
      autoSync: patch.enabled ?? Boolean(cronTask?.task.enabled ?? storageInfo.policy.autoSync),
      intervalSec: patch.intervalSec ?? intervalSec,
    };
    try {
      const info = await updateWorkspaceSyncPolicy(nextPolicy);
      if (!info || info.error) {
        pushToast(info?.error || 'Failed to update sync policy', { durationMs: 5000 });
        return;
      }
      const task = await refreshCronTask(info);
      if (task) {
        setCronTask(task);
      }
    } catch (cause) {
      pushToast(cause instanceof Error ? cause.message : 'Failed to update sync policy', { durationMs: 5000 });
      return;
    }
  };

  const openCron = () => {
    setOpen(false);
    setSidebarView('cron');
    if (cronTask) {
      openCronTaskTab(cronTask.task.id, cronTaskTitle(cronTask));
    }
  };

  const handleRefresh = async () => {
    const info = await refreshStorageStatus(currentDir);
    if (info) {
      await refreshCronTask(info);
    }
  };

  return (
    <div className="relative no-drag" ref={rootRef}>
      <button
        type="button"
        className="ui-statusbar-control max-w-[180px]"
        title={storageInfo.error || taskError || cronTask?.state?.lastError || storageInfo.message || statusText(storageInfo, cronTask, taskError)}
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        {busy ? (
          <LoaderIcon className="h-3.5 w-3.5 animate-spin" />
        ) : storageInfo.status === 'error' || taskError || taskStatus?.tone === 'error' ? (
          <AlertCircleIcon className={iconClass} />
        ) : taskStatus?.tone === 'ok' ? (
          <CheckCircleIcon className={iconClass} />
        ) : taskStatus?.tone === 'disabled' ? (
          <ClockIcon className={iconClass} />
        ) : (
          <RefreshSyncIcon className={iconClass} />
        )}
        <span className="truncate">{statusText(storageInfo, cronTask, taskError)}</span>
      </button>

      {open ? (
        <PopupMenu
          className="absolute bottom-0 right-full z-[60] mr-2 w-[340px] rounded-xl p-3"
          onMouseDown={(event) => event.stopPropagation()}
        >
          <div className="text-sm font-medium text-prime-text">
            {cronTask ? cronTaskTitle(cronTask) : 'Workspace sync'}
          </div>
          <div className="mt-2 space-y-1 text-xs text-secondary-text">
            <div>Storage: {providerLabel(storageInfo)}</div>
            {storageInfo.storage?.remoteName ? <div>Remote: {storageInfo.storage.remoteName}</div> : null}
            {cronTask ? <div>Schedule: {cronTaskSchedule(cronTask)}</div> : null}
            {cronTask ? <div>Next run: {formatCronTime(cronTask.state?.nextRunAtMs)}</div> : null}
            <div>Last sync: {formatLastSync(storageInfo.lastSyncAt)}</div>
            {storageInfo.error ? <SyncPopupError error={storageInfo.error} /> : null}
            {taskError ? <SyncPopupError error={taskError} /> : null}
            {cronTask?.state?.lastError ? <SyncPopupError error={cronTask.state.lastError} /> : null}
            {!enabled ? <div>This workspace is local only.</div> : null}
            {enabled && !cronTask && !taskLoading ? <div>Cron task is being prepared.</div> : null}
          </div>

          <div className="mt-3 flex items-center gap-2">
            <button
              type="button"
              className="ui-pill-btn-primary op-sg-capsule op-sg-capsule--on-editor px-3 py-1.5 text-sm disabled:cursor-not-allowed disabled:opacity-50"
              disabled={!enabled || busy}
              onClick={handleSyncNow}
            >
              {busy ? 'Syncing...' : 'Sync now'}
            </button>
            <button
              type="button"
              className="ui-pill-btn-secondary px-3 py-1.5 text-sm disabled:cursor-not-allowed disabled:opacity-50"
              onClick={() => void handleRefresh()}
              disabled={busy}
            >
              Refresh
            </button>
            <button
              type="button"
              className="ui-pill-btn-secondary px-3 py-1.5 text-sm"
              onClick={openCron}
            >
              View Cron
            </button>
          </div>

          {enabled ? (
            <div className="mt-3 space-y-2 border-t border-border pt-3 text-sm">
              <label className="flex items-center justify-between gap-3 text-secondary-text">
                <span>Enabled</span>
                <input
                  type="checkbox"
                  checked={Boolean(cronTask?.task.enabled)}
                  disabled={!cronTask}
                  onChange={(event) => void saveTaskPatch({ enabled: event.target.checked })}
                />
              </label>
              <label className="flex items-center justify-between gap-3 text-secondary-text">
                <span>Every</span>
                <select
                  className="rounded border border-border bg-editor-bg px-2 py-1 text-sm text-prime-text outline-none"
                  value={String(intervalSec)}
                  disabled={!cronTask || cronTask.task.enabled !== true}
                  onChange={(event) => void saveTaskPatch({ intervalSec: Number(event.target.value) })}
                >
                  {intervalOptions.map((option) => (
                    <option key={option.seconds} value={option.seconds}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </label>
            </div>
          ) : null}
        </PopupMenu>
      ) : null}
    </div>
  );
}
