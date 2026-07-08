import React, { useEffect, useRef, useState } from 'react';

import type { DashboardHost, DashboardRuntimeConnection, DashboardRuntimeUpdater } from '../../types/electron';
import { useAuthStore } from '../../store/authStore';

const DASHBOARD_REFRESH_INTERVAL_MS = 15_000;

function compareHosts(a: DashboardHost, b: DashboardHost) {
  if (a.online !== b.online) {
    return a.online ? -1 : 1;
  }
  const aTime = Date.parse(a.lastSeenAt || a.receivedAt || '') || 0;
  const bTime = Date.parse(b.lastSeenAt || b.receivedAt || '') || 0;
  if (aTime !== bTime) {
    return bTime - aTime;
  }
  const aLabel = (a.hostname || a.id).toLowerCase();
  const bLabel = (b.hostname || b.id).toLowerCase();
  return aLabel.localeCompare(bLabel);
}

function formatTimestamp(value: string | undefined): string {
  if (!value) {
    return '-';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

function compareRuntimeConnections(a: DashboardRuntimeConnection, b: DashboardRuntimeConnection) {
  if (a.transport !== b.transport) {
    if (a.transport === 'stdio') {
      return -1;
    }
    if (b.transport === 'stdio') {
      return 1;
    }
    if (a.transport === 'http_streamable') {
      return -1;
    }
    if (b.transport === 'http_streamable') {
      return 1;
    }
    return a.transport.localeCompare(b.transport);
  }
  const aLabel = (a.name || a.nodeID).toLowerCase();
  const bLabel = (b.name || b.nodeID).toLowerCase();
  return aLabel.localeCompare(bLabel);
}

function formatDuration(seconds: number | undefined): string {
  if (seconds == null || !Number.isFinite(seconds) || seconds < 0) {
    return '-';
  }
  const total = Math.floor(seconds);
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const secs = total % 60;
  if (hours > 0) {
    return `${hours}h ${minutes}m ${secs}s`;
  }
  if (minutes > 0) {
    return `${minutes}m ${secs}s`;
  }
  return `${secs}s`;
}

function transportLabel(value: string): string {
  if (value === 'stdio') {
    return 'stdio';
  }
  if (value === 'http_streamable') {
    return 'http';
  }
  return value || 'unknown';
}

function formatRuntimeUpdaterPhase(updater: DashboardRuntimeUpdater | undefined): string {
  const phase = (updater?.phase || '').trim();
  if (!phase) {
    return '-';
  }
  if (phase === 'staged') {
    return 'staged';
  }
  if (phase === 'checking') {
    return 'checking';
  }
  if (phase === 'downloading') {
    return 'downloading';
  }
  if (phase === 'applying') {
    return 'applying';
  }
  if (phase === 'idle') {
    return 'idle';
  }
  if (phase === 'disabled') {
    return 'disabled';
  }
  if (phase === 'error') {
    return 'error';
  }
  return phase;
}

export const DashboardEditor: React.FC = () => {
  const loggedIn = useAuthStore((state) => state.loggedIn);
  const [hosts, setHosts] = useState<DashboardHost[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [lastLoadedAt, setLastLoadedAt] = useState<number | null>(null);
  const inFlightRef = useRef(false);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const loadHosts = async (silent: boolean) => {
    if (!loggedIn) {
      if (mountedRef.current) {
        setHosts([]);
        setError(null);
        setLoading(false);
        setLastLoadedAt(null);
      }
      return;
    }
    if (!window.electronAPI?.dashboard?.getHosts) {
      if (mountedRef.current) {
        setError('Dashboard API is unavailable.');
        setLoading(false);
      }
      return;
    }
    if (inFlightRef.current) {
      return;
    }

    inFlightRef.current = true;
    if (!silent) {
      setLoading(true);
    }

    try {
      const nextHosts = await window.electronAPI.dashboard.getHosts();
      if (mountedRef.current) {
        setHosts(nextHosts || []);
        setError(null);
        setLastLoadedAt(Date.now());
      }
    } catch (err) {
      if (mountedRef.current) {
        setError((err as Error).message || 'Failed to load dashboard hosts.');
      }
    } finally {
      inFlightRef.current = false;
      if (!silent && mountedRef.current) {
        setLoading(false);
      }
    }
  };

  useEffect(() => {
    if (!loggedIn) {
      void loadHosts(false);
      return;
    }

    let cancelled = false;
    const run = async (silent: boolean) => {
      if (cancelled) {
        return;
      }
      await loadHosts(silent);
    };

    void run(false);
    const intervalID = window.setInterval(() => {
      void run(true);
    }, DASHBOARD_REFRESH_INTERVAL_MS);

    return () => {
      cancelled = true;
      window.clearInterval(intervalID);
    };
  }, [loggedIn]);

  const sortedHosts = [...hosts].sort(compareHosts);

  return (
    <div className="flex h-full flex-col overflow-auto bg-editor-bg text-editor-fg">
      <div className="border-b border-border px-4 py-3">
        <div className="text-sm font-semibold">Dashboard</div>
        <div className="text-xs text-secondary-text">
          All hosts and their active runtime connections. Auto-refreshes every 15 seconds.
        </div>
      </div>

      <div className="flex flex-1 flex-col gap-4 p-4">
        <div className="flex flex-wrap items-center gap-2">
          <button
            className="ui-pill-btn-primary op-sg-capsule op-sg-capsule--on-editor px-3 py-1 text-xs disabled:opacity-60"
            onClick={() => {
              void loadHosts(false);
            }}
            disabled={loading}
          >
            Refresh
          </button>
          <span className="text-xs text-secondary-text">
            {lastLoadedAt ? `Last updated ${new Date(lastLoadedAt).toLocaleTimeString()}` : 'Waiting for first sync'}
          </span>
        </div>

        {!loggedIn ? (
          <div className="rounded border border-border px-4 py-6 text-sm text-secondary-text">
            Log in to load the host dashboard.
          </div>
        ) : (
          <>
            {error && (
              <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-600">
                {error}
              </div>
            )}

            {loading && hosts.length === 0 ? (
              <div className="rounded border border-border px-4 py-6 text-sm text-secondary-text">
                Loading hosts...
              </div>
            ) : null}

            {!loading && sortedHosts.length === 0 ? (
              <div className="rounded border border-border px-4 py-6 text-sm text-secondary-text">
                No hosts have reported heartbeat data yet.
              </div>
            ) : null}

            {sortedHosts.length > 0 ? (
              <div className="space-y-3">
                {sortedHosts.map((host) => (
                  <section key={host.id} className="rounded border border-border bg-sidebar-bg">
                    <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-4 py-3">
                      <div className="min-w-0">
                        <div className="truncate text-sm font-medium text-prime-text">
                          {host.hostname || host.id}
                        </div>
                        <div className="truncate font-mono text-[11px] text-secondary-text">
                          {host.id}
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <span
                          className={`rounded border px-2 py-0.5 text-[10px] uppercase tracking-wide ${
                            host.online
                              ? 'border-green-300 text-health-text'
                              : 'border-border text-secondary-text'
                          }`}
                        >
                          {host.online ? 'online' : 'offline'}
                        </span>
                        <span className="rounded border border-border px-2 py-0.5 text-[10px] uppercase tracking-wide text-secondary-text">
                          {host.runtimeConnections.length} connections
                        </span>
                      </div>
                    </div>

                    <div className="grid gap-3 px-4 py-3 sm:grid-cols-2">
                      <div>
                        <div className="text-[11px] text-secondary-text">Environment</div>
                        <div className="text-sm">{host.env || '-'}</div>
                      </div>
                      <div>
                        <div className="text-[11px] text-secondary-text">Last heartbeat</div>
                        <div className="text-sm">{formatTimestamp(host.receivedAt || host.lastSeenAt)}</div>
                      </div>
                      <div className="sm:col-span-2">
                        <div className="text-[11px] text-secondary-text">Base directory</div>
                        <div className="truncate font-mono text-xs text-prime-text">{host.baseDir || '-'}</div>
                      </div>
                      <div>
                        <div className="text-[11px] text-secondary-text">Runtime updater</div>
                        <div className="text-sm">{formatRuntimeUpdaterPhase(host.runtimeUpdater)}</div>
                      </div>
                      <div>
                        <div className="text-[11px] text-secondary-text">Runtime version</div>
                        <div className="text-sm">
                          {host.runtimeUpdater?.currentVersion || '-'}
                          {host.runtimeUpdater?.stagedVersion ? ` -> ${host.runtimeUpdater.stagedVersion}` : ''}
                        </div>
                      </div>
                      <div>
                        <div className="text-[11px] text-secondary-text">Last updater check</div>
                        <div className="text-sm">{formatTimestamp(host.runtimeUpdater?.lastCheckedAt)}</div>
                      </div>
                      <div>
                        <div className="text-[11px] text-secondary-text">Updater error</div>
                        <div className="truncate text-sm text-accent">{host.runtimeUpdater?.lastError || '-'}</div>
                      </div>
                    </div>

                    <div className="border-t border-border px-4 py-3">
                      <div className="mb-2 text-[11px] uppercase tracking-wide text-secondary-text">Runtime connections</div>
                      {host.runtimeConnections.length === 0 ? (
                        <div className="text-sm text-secondary-text">No active runtime connections.</div>
                      ) : (
                        <div className="space-y-2">
                          {[...host.runtimeConnections].sort(compareRuntimeConnections).map((connection) => (
                            <div key={connection.nodeID} className="rounded border border-border px-3 py-2">
                              <div className="flex flex-wrap items-center gap-2">
                                <div className="truncate text-sm text-prime-text">{connection.name}</div>
                                <span className="rounded border border-border px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-secondary-text">
                                  {transportLabel(connection.transport)}
                                </span>
                                {connection.daemon && (
                                  <span className="rounded border border-border px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-secondary-text">
                                    daemon
                                  </span>
                                )}
                              </div>
                              <div className="truncate font-mono text-[11px] text-secondary-text">
                                {connection.nodeID}
                              </div>
                              {connection.transport === 'stdio' ? (
                                <div className="mt-2 grid gap-2 text-[11px] text-secondary-text sm:grid-cols-3">
                                  <div>
                                    <div className="uppercase tracking-wide">PID</div>
                                    <div className="font-mono text-prime-text">{connection.pid ?? '-'}</div>
                                  </div>
                                  <div>
                                    <div className="uppercase tracking-wide">Started</div>
                                    <div className="text-prime-text">{formatTimestamp(connection.startedAt)}</div>
                                  </div>
                                  <div>
                                    <div className="uppercase tracking-wide">Uptime</div>
                                    <div className="text-prime-text">{formatDuration(connection.uptimeSec)}</div>
                                  </div>
                                </div>
                              ) : (
                                <div className="mt-2 grid gap-2 text-[11px] text-secondary-text sm:grid-cols-2">
                                  <div>
                                    <div className="uppercase tracking-wide">Last active</div>
                                    <div className="text-prime-text">{formatTimestamp(connection.lastActiveAt)}</div>
                                  </div>
                                  <div>
                                    <div className="uppercase tracking-wide">Endpoint</div>
                                    <div className="truncate font-mono text-prime-text">{connection.url || '-'}</div>
                                  </div>
                                </div>
                              )}
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  </section>
                ))}
              </div>
            ) : null}
          </>
        )}
      </div>
    </div>
  );
};
