import React, { useEffect, useMemo, useState } from 'react';

import { CRON_TASK_HISTORY_LIMIT, type CronTask, type CronTaskHistoryEntry, type CronTaskRecord } from '../../services/cronService';
import { getThreadMeta } from '../../services/threadService';
import { getChatWorkspaceStore } from '../../store/chatWorkspaceStore';
import { useAppStore } from '../../store/appStore';
import { useModelsStore } from '../../store/modelsStore';
import { useTabManagerStore } from '../../store/tabManagerStore';
import { normalizeCronIntervalSec, formatCronEvery, parseCronEveryToSeconds } from '../../utils/cronSchedule';
import {
  cronDataString,
  cronPayloadData,
  cronTaskBranch,
  cronTaskPath,
  cronTaskSchedule,
  cronTaskStatus,
  cronTaskTitle,
  formatCronTime,
} from '../../utils/cronDisplay';
import { AlertCircleIcon, CheckCircleIcon, ClockIcon, LoaderIcon, ListIcon, RefreshIcon } from '../Icons';
import { BillingRestrictionCard } from '../BillingRestrictionCard';
import { resolveRunErrorDisplay } from '../Chat/activityErrorState';

function activeCronTaskID(): string {
  const state = useAppStore.getState();
  const active = state.documents.find((tab) => tab.id === state.activeTabId);
  const editorId = active?.editorId || '';
  return editorId.startsWith('cron-task:') ? editorId.slice('cron-task:'.length).trim() : '';
}

function DetailRow({ label, value, title }: { label: string; value: React.ReactNode; title?: string }) {
  return (
    <div className="grid gap-1 border-b border-border/60 py-2 last:border-b-0 sm:grid-cols-[150px_minmax(0,1fr)]">
      <div className="text-xs font-medium uppercase text-secondary-text">{label}</div>
      <div className="min-w-0 break-words text-sm text-prime-text" title={title}>
        {value || '-'}
      </div>
    </div>
  );
}

function StatusPill({ record }: { record: CronTaskRecord }) {
  const status = cronTaskStatus(record);
  const className = status.tone === 'error'
    ? 'border-accent/40 bg-accent/10 text-accent'
    : status.tone === 'running'
      ? 'border-blue-300 bg-blue-50 text-blue-700'
      : status.tone === 'ok'
        ? 'border-green-300 bg-green-50 text-green-700'
        : 'border-border bg-editor-bg text-secondary-text';
  const Icon = status.tone === 'error'
    ? AlertCircleIcon
    : status.tone === 'running'
      ? LoaderIcon
      : status.tone === 'ok'
        ? CheckCircleIcon
        : ClockIcon;
  return (
    <span className={`inline-flex items-center gap-1 rounded border px-2 py-1 text-xs ${className}`}>
      <Icon className={`h-3.5 w-3.5 ${status.tone === 'running' ? 'animate-spin' : ''}`} />
      {status.label}
    </span>
  );
}

function syncDefaultName(record: CronTaskRecord): string {
  const data = cronPayloadData(record);
  const value = data.defaultName;
  return typeof value === 'string' && value.trim() ? value.trim() : cronTaskTitle(record);
}

function formatHistoryStatus(status: string): string {
  const trimmed = (status || '').trim();
  if (!trimmed) {
    return 'Unknown';
  }
  return trimmed.charAt(0).toUpperCase() + trimmed.slice(1);
}

function formatHistoryDuration(durationMs?: number): string {
  const value = Number(durationMs || 0);
  if (value <= 0) {
    return '0s';
  }
  if (value < 1000) {
    return `${value}ms`;
  }
  return `${Math.round(value / 1000)}s`;
}

function CronRunError({ error }: { error: string }) {
  const { billingInfo, rawError } = resolveRunErrorDisplay(error);
  if (billingInfo) {
    return (
      <div className="mt-2">
        <BillingRestrictionCard info={billingInfo} compact surface="editor" />
      </div>
    );
  }
  return <div className="mt-2 whitespace-pre-wrap text-xs text-accent">{rawError}</div>;
}

function CronLastError({ error }: { error: string }) {
  const { billingInfo, rawError } = resolveRunErrorDisplay(error);
  if (billingInfo) {
    return (
      <div className="mt-4">
        <BillingRestrictionCard info={billingInfo} surface="editor" />
      </div>
    );
  }
  return (
    <div className="mt-4 rounded border border-accent/35 bg-accent/10 p-4 text-sm text-accent">
      <div className="font-semibold">Last Error</div>
      <div className="mt-2 whitespace-pre-wrap">{rawError}</div>
    </div>
  );
}

function buildUpdatedTask(record: CronTaskRecord, draft: {
  name: string;
  enabled: boolean;
  intervalSec: number;
  modelKey: string;
}): CronTask {
  const defaultName = syncDefaultName(record);
  const name = draft.name.trim() || defaultName;
  const payloadData: Record<string, unknown> = {
    ...cronPayloadData(record),
    defaultName,
    nameMode: name === defaultName ? 'auto' : 'custom',
  };
  const modelKey = draft.modelKey.trim();
  if (modelKey) {
    payloadData.modelKey = modelKey;
  } else {
    delete payloadData.modelKey;
  }
  return {
    ...record.task,
    name,
    enabled: draft.enabled,
    schedule: {
      every: formatCronEvery(draft.intervalSec),
    },
    payload: {
      ...record.task.payload,
      data: payloadData,
    },
  };
}

export const CronTaskEditor: React.FC = () => {
  const activeTabId = useAppStore((state) => state.activeTabId);
  const getCronTask = useAppStore((state) => state.getCronTask);
  const updateCronTask = useAppStore((state) => state.updateCronTask);
  const runCronTask = useAppStore((state) => state.runCronTask);
  const listCronTaskHistory = useAppStore((state) => state.listCronTaskHistory);
  const connectionState = useAppStore((state) => state.connectionState);
  const modelsConfig = useModelsStore((state) => state.config);
  const loadModels = useModelsStore((state) => state.load);
  const [record, setRecord] = useState<CronTaskRecord | null>(null);
  const [draftName, setDraftName] = useState('');
  const [draftEnabled, setDraftEnabled] = useState(true);
  const [draftIntervalSec, setDraftIntervalSec] = useState(300);
  const [draftModelKey, setDraftModelKey] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedMessage, setSavedMessage] = useState<string | null>(null);
  const [showHistory, setShowHistory] = useState(false);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState<string | null>(null);
  const [historyRuns, setHistoryRuns] = useState<CronTaskHistoryEntry[]>([]);
  const taskID = useMemo(() => activeCronTaskID(), [activeTabId]);

  const hydrateDraft = (next: CronTaskRecord) => {
    setDraftName(cronTaskTitle(next));
    setDraftEnabled(Boolean(next.task.enabled));
    setDraftIntervalSec(parseCronEveryToSeconds(next.task.schedule?.every));
    setDraftModelKey(cronDataString(next, 'modelKey'));
  };

  const refresh = async () => {
    if (!taskID) {
      setRecord(null);
      setError('Cron task id is missing.');
      setLoading(false);
      return;
    }
    if (connectionState !== 'connected') {
      setRecord(null);
      setError('Cron requires an active runtime connection.');
      setLoading(false);
      return;
    }
    setLoading(true);
    try {
      const next = await getCronTask(taskID);
      if (!next) {
        throw new Error('Cron task not found.');
      }
      setRecord(next);
      hydrateDraft(next);
      setError(null);
    } catch (cause) {
      setRecord(null);
      setError(cause instanceof Error ? cause.message : 'Failed to load cron task.');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadModels();
  }, [loadModels]);

  useEffect(() => {
    void refresh();
    setShowHistory(false);
    setHistoryRuns([]);
    setHistoryError(null);
  }, [connectionState, getCronTask, taskID]);

  useEffect(() => {
    if (!showHistory || !record || connectionState !== 'connected') {
      return;
    }
    let cancelled = false;
    const loadHistory = async () => {
      setHistoryLoading(true);
      setHistoryError(null);
      try {
        const next = await listCronTaskHistory(record.task.id, CRON_TASK_HISTORY_LIMIT);
        if (!cancelled) {
          setHistoryRuns(next);
        }
      } catch (cause) {
        if (!cancelled) {
          setHistoryError(cause instanceof Error ? cause.message : 'Failed to load cron history.');
        }
      } finally {
        if (!cancelled) {
          setHistoryLoading(false);
        }
      }
    };
    void loadHistory();
    return () => {
      cancelled = true;
    };
  }, [connectionState, listCronTaskHistory, record, showHistory]);

  const save = async () => {
    if (!record || saving) {
      return;
    }
    setSaving(true);
    setSavedMessage(null);
    try {
      const next = await updateCronTask(buildUpdatedTask(record, {
        name: draftName,
        enabled: draftEnabled,
        intervalSec: draftIntervalSec,
        modelKey: draftModelKey,
      }));
      if (!next) {
        throw new Error('Failed to save cron task.');
      }
      setRecord(next);
      hydrateDraft(next);
      setError(null);
      setSavedMessage('Saved');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to save cron task.');
    } finally {
      setSaving(false);
    }
  };

  const runNow = async () => {
    if (!taskID || running) {
      return;
    }
    setRunning(true);
    try {
      await runCronTask(taskID);
      await refresh();
      setError(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to run cron task.');
    } finally {
      setRunning(false);
    }
  };

  const resetName = () => {
    if (record) {
      setDraftName(syncDefaultName(record));
    }
  };

  const payloadData = record ? cronPayloadData(record) : {};
  const payloadJSON = record ? JSON.stringify(payloadData, null, 2) : '';
  const targetJSON = record ? JSON.stringify(record.task.target, null, 2) : '';
  const scheduleIsEvery = !record || Boolean(record.task.schedule?.every) || (!record.task.schedule?.cron && !record.task.schedule?.time);
  const defaultName = record ? syncDefaultName(record) : '';
  const intervalMinutes = Math.max(1, Math.round(draftIntervalSec / 60));
  const currentModelKey = record ? cronDataString(record, 'modelKey') : '';
  const enabledModels = useMemo(() => modelsConfig.models.filter((model) => model.enabled), [modelsConfig.models]);
  const draftModelEnabled = enabledModels.some((model) => model.key === draftModelKey.trim());
  const dirty = record
    ? draftName.trim() !== cronTaskTitle(record)
      || draftEnabled !== Boolean(record.task.enabled)
      || formatCronEvery(draftIntervalSec) !== (record.task.schedule?.every || '')
      || draftModelKey.trim() !== currentModelKey
    : false;
  const openHistoryConversation = async (run: CronTaskHistoryEntry) => {
    const threadID = (run.threadID || '').trim();
    try {
      if (!threadID) {
        throw new Error('Cron run thread id is not available.');
      }
      const workspaceTabId = useTabManagerStore.getState().activeTabId;
      const meta = await getThreadMeta({
        threadID,
        agentID: run.agentID || undefined,
      }, workspaceTabId).catch(() => null);
      const chatPath = (meta?.chatPath || run.chatPath || '').trim() || undefined;
      const chatStore = getChatWorkspaceStore(workspaceTabId).getState();
      if (meta) {
        chatStore.upsertThreadMeta(meta);
      }
      chatStore.openThreadConversation(threadID, {
        chatPath,
        title: meta?.title || threadID || 'Cron run',
        agentID: meta?.agentID || run.agentID || undefined,
      });
      setError(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to open cron conversation.');
    }
  };
  return (
    <div className="flex h-full flex-col overflow-auto bg-editor-bg text-editor-fg">
      <div className="border-b border-border px-4 py-3">
        <div className="flex min-w-0 items-start gap-3">
          <div className="min-w-0 flex-1">
            <div className="text-sm font-semibold text-prime-text">
              {record ? cronTaskTitle(record) : 'Cron Task'}
            </div>
            <div className="mt-1 truncate text-xs text-secondary-text" title={record ? cronTaskPath(record) : taskID}>
              {record ? cronTaskPath(record) : taskID}
            </div>
          </div>
          <button
            type="button"
            className="ui-pill-btn-secondary shrink-0 px-2.5 py-1.5 text-xs disabled:cursor-not-allowed disabled:opacity-50"
            onClick={() => void refresh()}
            disabled={loading}
          >
            <RefreshIcon className={`mr-1 inline h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
            Refresh
          </button>
          <button
            type="button"
            className={`ui-pill-btn-secondary shrink-0 px-2.5 py-1.5 text-xs disabled:cursor-not-allowed disabled:opacity-50 ${showHistory ? 'border-highlight text-highlight' : ''}`}
            onClick={() => setShowHistory((current) => !current)}
            disabled={!record}
          >
            <ListIcon className="mr-1 inline h-3.5 w-3.5" />
            Runs
          </button>
          <button
            type="button"
            className="ui-pill-btn-primary op-sg-capsule op-sg-capsule--on-editor shrink-0 px-2.5 py-1.5 text-xs disabled:cursor-not-allowed disabled:opacity-50"
            onClick={() => void runNow()}
            disabled={running || !record || !currentModelKey}
          >
            {running ? 'Running' : 'Run'}
          </button>
        </div>
      </div>

      {loading && !record ? (
        <div className="p-4 text-sm text-secondary-text">Loading cron task...</div>
      ) : error && !record ? (
        <div className="p-4 text-sm text-accent">{error}</div>
      ) : record ? (
        <div className="max-w-[920px] p-4">
          {showHistory ? (
            <div className="mb-4 rounded border border-border p-4">
              <div className="mb-3 flex items-center gap-2">
                <div className="text-sm font-semibold text-prime-text">Recent Runs</div>
                <div className="text-xs text-secondary-text">Latest 99</div>
                <div className="text-xs text-secondary-text">{historyRuns.length ? `${historyRuns.length} records` : 'No records yet'}</div>
              </div>
              {historyLoading ? <div className="text-sm text-secondary-text">Loading history...</div> : null}
              {historyError ? <div className="text-sm text-accent">{historyError}</div> : null}
              {!historyLoading && !historyError && historyRuns.length === 0 ? (
                <div className="text-sm text-secondary-text">No execution history recorded.</div>
              ) : null}
              <div className="space-y-2">
                {historyRuns.map((run) => (
                  <div key={run.runID} className="rounded border border-border bg-secondary-bg/40 px-3 py-2">
                    <div className="flex flex-wrap items-start gap-2">
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="text-sm font-medium text-prime-text">{formatHistoryStatus(run.status)}</span>
                          <span className="text-xs text-secondary-text">{formatHistoryStatus(run.trigger)}</span>
                          <span className="text-xs text-secondary-text">{formatCronTime(run.scheduledAtMs)}</span>
                          <span className="text-xs text-secondary-text">{formatCronTime(run.startedAtMs)}</span>
                          <span className="text-xs text-secondary-text">{formatHistoryDuration(run.durationMs)}</span>
                          {run.finishedAtMs ? <span className="text-xs text-secondary-text">{formatCronTime(run.finishedAtMs)}</span> : null}
                        </div>
                        <div className="mt-1 break-all text-xs text-secondary-text" title={run.runID}>{run.runID}</div>
                        {run.error ? <CronRunError error={run.error} /> : null}
                      </div>
                      <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
                        {run.threadID ? (
                          <button
                            type="button"
                            className="ui-pill-btn-secondary px-2 py-1 text-xs"
                            onClick={() => void openHistoryConversation(run)}
                          >
                            Conversation
                          </button>
                        ) : null}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ) : null}

          <div className="rounded border border-border p-4">
            <div className="mb-4 flex flex-wrap items-center gap-2">
              <StatusPill record={record} />
              <span className="text-xs text-secondary-text">{cronTaskSchedule(record)}</span>
              {savedMessage ? <span className="text-xs text-health-text">{savedMessage}</span> : null}
              {error ? <span className="text-xs text-accent">{error}</span> : null}
            </div>

            <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_220px]">
              <label className="block min-w-0">
                <span className="text-xs font-medium uppercase text-secondary-text">Task Name</span>
                <input
                  className="mt-1 w-full rounded border border-border bg-editor-bg px-3 py-2 text-sm text-prime-text outline-none focus:border-button-bg"
                  value={draftName}
                  onChange={(event) => {
                    setDraftName(event.target.value);
                    setSavedMessage(null);
                  }}
                />
              </label>

              <label className="flex items-end justify-between gap-3 rounded border border-border px-3 py-2 text-sm text-secondary-text">
                <span className="pb-0.5">Enabled</span>
                <input
                  type="checkbox"
                  checked={draftEnabled}
                  onChange={(event) => {
                    setDraftEnabled(event.target.checked);
                    setSavedMessage(null);
                  }}
                />
              </label>
            </div>

            <div className="mt-4 grid gap-4 sm:grid-cols-[220px_minmax(0,1fr)]">
              <label className="block">
                <span className="text-xs font-medium uppercase text-secondary-text">Interval (minutes)</span>
                <input
                  type="number"
                  min={1}
                  step={1}
                  className="mt-1 w-full rounded border border-border bg-editor-bg px-3 py-2 text-sm text-prime-text outline-none focus:border-button-bg disabled:cursor-not-allowed disabled:opacity-60"
                  value={intervalMinutes}
                  disabled={!scheduleIsEvery}
                  onChange={(event) => {
                    const minutes = Number(event.target.value);
                    setDraftIntervalSec(normalizeCronIntervalSec(minutes * 60));
                    setSavedMessage(null);
                  }}
                />
              </label>

              <label className="block min-w-0">
                <span className="text-xs font-medium uppercase text-secondary-text">Model</span>
                <select
                  className="mt-1 w-full rounded border border-border bg-editor-bg px-3 py-2 text-sm text-prime-text outline-none focus:border-button-bg"
                  value={draftModelKey}
                  onChange={(event) => {
                    setDraftModelKey(event.target.value);
                    setSavedMessage(null);
                  }}
                >
                  <option value="">Select model</option>
                  {draftModelKey && !draftModelEnabled ? (
                    <option value={draftModelKey}>{draftModelKey} (unavailable)</option>
                  ) : null}
                  {enabledModels.map((model) => (
                    <option key={model.key} value={model.key}>
                      {model.label || model.id || model.key}
                    </option>
                  ))}
                </select>
              </label>
            </div>

            <div className="mt-4 flex items-end gap-2">
              <button
                type="button"
                className="ui-pill-btn-primary op-sg-capsule op-sg-capsule--on-editor px-3 py-2 text-sm disabled:cursor-not-allowed disabled:opacity-50"
                disabled={!dirty || saving || !scheduleIsEvery || !draftModelKey.trim()}
                onClick={() => void save()}
              >
                {saving ? 'Saving...' : 'Save'}
              </button>
              <button
                type="button"
                className="ui-pill-btn-secondary px-3 py-2 text-sm disabled:cursor-not-allowed disabled:opacity-50"
                disabled={!dirty || saving}
                onClick={() => {
                  hydrateDraft(record);
                  setSavedMessage(null);
                }}
              >
                Revert
              </button>
              <button
                type="button"
                className="ui-pill-btn-secondary px-3 py-2 text-sm disabled:cursor-not-allowed disabled:opacity-50"
                disabled={draftName.trim() === defaultName}
                onClick={resetName}
              >
                Reset name
              </button>
            </div>
            {!scheduleIsEvery ? (
              <div className="mt-3 text-xs text-accent">This editor currently supports interval Cron tasks only.</div>
            ) : null}
          </div>

          <div className="mt-4 rounded border border-border p-4">
            <div className="mb-2 text-sm font-semibold text-prime-text">Status</div>
            <DetailRow label="Task ID" value={record.task.id} />
            <DetailRow label="Model" value={currentModelKey} />
            <DetailRow label="Description" value={record.task.description || '-'} />
            <DetailRow label="Next Run" value={formatCronTime(record.state?.nextRunAtMs)} />
            <DetailRow label="Last Run" value={formatCronTime(record.state?.lastRunAtMs)} />
            <DetailRow label="Running Since" value={formatCronTime(record.state?.runningAtMs)} />
            <DetailRow label="Errors" value={String(record.state?.consecutiveErrors || 0)} />
          </div>

          <div className="mt-4 rounded border border-border p-4">
            <div className="mb-2 text-sm font-semibold text-prime-text">Workspace</div>
            <DetailRow label="Path" value={cronTaskPath(record)} title={cronTaskPath(record)} />
            <DetailRow label="Branch" value={cronTaskBranch(record)} />
            <DetailRow label="Workspace ID" value={cronDataString(record, 'workspaceID')} />
            <DetailRow label="Org ID" value={cronDataString(record, 'orgID')} />
            <DetailRow label="Location" value={cronDataString(record, 'locationKind') || 'local'} />
            <DetailRow label="Repo URL" value={cronDataString(record, 'repoURL')} />
            <DetailRow label="Host" value={cronDataString(record, 'hostID') || cronDataString(record, 'hostName')} />
          </div>

          <div className="mt-4 rounded border border-border p-4">
            <div className="mb-2 text-sm font-semibold text-prime-text">Target</div>
            <DetailRow label="Agent" value={record.task.target.agentID || '-'} />
            <DetailRow label="CWD" value={record.task.target.cwd || '-'} title={record.task.target.cwd} />
            <pre className="mt-3 overflow-auto rounded bg-secondary-bg p-3 text-xs text-secondary-text">{targetJSON}</pre>
          </div>

          {record.state?.lastError ? (
            <CronLastError error={record.state.lastError} />
          ) : null}

          <div className="mt-4 rounded border border-border p-4">
            <div className="mb-2 text-sm font-semibold text-prime-text">Payload</div>
            <pre className="overflow-auto rounded bg-secondary-bg p-3 text-xs text-secondary-text">{payloadJSON}</pre>
          </div>
        </div>
      ) : null}
    </div>
  );
};
