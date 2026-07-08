import React, { useEffect, useMemo, useState } from 'react';

import {
  AgentBrainIcon,
  RefreshIcon,
  SearchIcon,
  SparkleIcon,
  TerminalIcon,
} from '../Icons';
import type { MarketplaceListItem, MarketplaceListResult } from '../../types/electron';
import { useAppStore } from '../../store/appStore';
import { useTabManagerStore } from '../../store/tabManagerStore';

type FilterKind = 'all' | 'agent' | 'skill' | 'tool';

function formatTimestamp(value: string | null): string {
  if (!value) {
    return '-';
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString();
}

function kindLabel(kind: MarketplaceListItem['kind']): string {
  switch (kind) {
    case 'agent':
      return 'Agent';
    case 'skill':
      return 'Skill';
    case 'tool':
      return 'Tool';
  }
}

function statusLabel(item: MarketplaceListItem): string {
  if (item.missingFromCatalog) {
    return 'Catalog Missing';
  }
  if (item.status === 'update_available') {
    return item.inUse ? 'Update Available' : 'Update Available';
  }
  if (item.status === 'installed') {
    return 'Installed';
  }
  return 'Install';
}

function statusClassName(item: MarketplaceListItem): string {
  if (item.missingFromCatalog) {
    return 'border-amber-300 text-amber-700 bg-amber-50';
  }
  if (item.status === 'update_available') {
    return 'border-blue-300 text-blue-700 bg-blue-50';
  }
  if (item.status === 'installed') {
    return 'border-green-300 text-green-700 bg-green-50';
  }
  return 'border-border text-secondary-text bg-editor-bg';
}

function kindPillClassName(kind: MarketplaceListItem['kind']): string {
  switch (kind) {
    case 'agent':
      return 'bg-[rgba(244,183,64,0.12)] text-prime-text';
    case 'skill':
      return 'bg-[rgba(90,151,230,0.12)] text-prime-text';
    case 'tool':
      return 'bg-[rgba(102,179,123,0.12)] text-prime-text';
  }
}

function ItemIcon({ kind }: { kind: MarketplaceListItem['kind'] }) {
  if (kind === 'agent') {
    return <AgentBrainIcon className="w-5 h-5" />;
  }
  if (kind === 'skill') {
    return <SparkleIcon className="w-5 h-5" />;
  }
  return <TerminalIcon className="w-5 h-5" />;
}

export const MarketplaceEditor: React.FC = () => {
  const activeWorkspaceTabId = useTabManagerStore((state) => state.activeTabId);
  const activeDocumentTabId = useAppStore((state) => state.activeTabId);
  const documents = useAppStore((state) => state.documents);
  const connectionState = useAppStore((state) => state.connectionState);
  const listMarketplaceItems = useAppStore((state) => state.listMarketplaceItems);
  const refreshMarketplaceItems = useAppStore((state) => state.refreshMarketplaceItems);
  const installMarketplaceItem = useAppStore((state) => state.installMarketplaceItem);
  const updateMarketplaceItem = useAppStore((state) => state.updateMarketplaceItem);
  const [result, setResult] = useState<MarketplaceListResult | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [filter, setFilter] = useState<FilterKind>('all');
  const [busyKey, setBusyKey] = useState<string | null>(null);
  const activeMarketplaceTab = useMemo(
    () => documents.find((tab) => tab.id === activeDocumentTabId) || null,
    [activeDocumentTabId, documents],
  );
  const activeEditorId = activeMarketplaceTab?.editorId || 'marketplace';
  const orgID = activeEditorId.startsWith('marketplace:')
    ? activeEditorId.slice('marketplace:'.length).trim()
    : null;
  const isOrgMarketplace = Boolean(orgID);
  const orgName = result?.items.find((item) => item.orgID === orgID && item.orgName)?.orgName
    || activeMarketplaceTab?.title
    || orgID
    || '';
  const title = isOrgMarketplace ? orgName : 'Marketplace';
  const subtitle = isOrgMarketplace
    ? 'Private agents, skills, and tools visible only to this organization.'
    : 'Browse official agents, skills, and tools from the shared catalog. Install and update actions are executed by the connected runtime.';
  const searchPlaceholder = isOrgMarketplace
    ? `Search ${orgName || 'organization'} marketplace...`
    : 'Search marketplace...';

  const loadMarketplace = async (force: boolean) => {
    if (connectionState !== 'connected') {
      setLoading(false);
      setResult(null);
      setError('Marketplace requires an active runtime connection.');
      return;
    }
    if (!force) {
      setLoading(true);
    }
    try {
      const next = force
        ? await refreshMarketplaceItems({ orgID })
        : await listMarketplaceItems({ orgID });
      setResult(next);
      setError(next.error || null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    let cancelled = false;
    const run = async () => {
      if (connectionState !== 'connected') {
        setLoading(false);
        setResult(null);
        setError('Marketplace requires an active runtime connection.');
        return;
      }
      setLoading(true);
      try {
        const next = await listMarketplaceItems({ orgID });
        if (cancelled) {
          return;
        }
        setResult(next);
        setError(next.error || null);
      } catch (cause) {
        if (!cancelled) {
          setError(cause instanceof Error ? cause.message : String(cause));
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };
    void run();
    return () => {
      cancelled = true;
    };
  }, [activeDocumentTabId, activeWorkspaceTabId, connectionState, listMarketplaceItems, orgID]);

  useEffect(() => {
    setResult(null);
    setSearch('');
    setFilter('all');
    setBusyKey(null);
  }, [activeEditorId]);

  const filteredItems = useMemo(() => {
    const normalizedQuery = search.trim().toLowerCase();
    const source = result?.items || [];
    return source.filter((item) => {
      if (filter !== 'all' && item.kind !== filter) {
        return false;
      }
      if (!normalizedQuery) {
        return true;
      }
      return item.name.toLowerCase().includes(normalizedQuery)
        || item.description.toLowerCase().includes(normalizedQuery)
        || item.id.toLowerCase().includes(normalizedQuery);
    });
  }, [filter, result?.items, search]);

  const handleAction = async (item: MarketplaceListItem) => {
    const key = `${item.scope || 'public'}:${item.orgID || ''}:${item.kind}:${item.id}`;
    setBusyKey(key);
    try {
      const result = item.status === 'not_installed'
        ? await installMarketplaceItem(item.kind, item.id, orgID)
        : await updateMarketplaceItem(item.kind, item.id, orgID);
      if (!result.success) {
        setError(result.error || 'Marketplace action failed.');
        return;
      }
      await loadMarketplace(true);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusyKey(null);
    }
  };

  return (
    <div className="flex h-full flex-col overflow-auto bg-editor-bg text-editor-fg">
      <div className="px-6 py-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div className="text-2xl font-semibold tracking-tight text-prime-text">{title}</div>
            <div className="mt-1 text-base leading-7 text-secondary-text">
              {subtitle}
            </div>
          </div>
          <button
            type="button"
            className="ui-pill-btn-secondary h-11 px-4 text-base"
            onClick={() => void loadMarketplace(true)}
            disabled={loading || busyKey != null}
          >
            <RefreshIcon className="h-[18px] w-[18px]" />
            Refresh
          </button>
        </div>
        <div className="mt-4 flex flex-wrap items-center gap-5 text-base text-secondary-text">
          <span>Catalog: {result?.catalogVersion || '-'}</span>
          <span>Updated: {formatTimestamp(result?.generatedAt || null)}</span>
        </div>
      </div>

      <div className="flex flex-1 flex-col gap-5 p-6">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="relative w-full max-w-2xl">
            <SearchIcon className="pointer-events-none absolute left-4 top-1/2 h-5 w-5 -translate-y-1/2 text-secondary-text" />
            <input
              type="text"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder={searchPlaceholder}
              className="h-14 w-full rounded-xl border border-border bg-sidebar-bg pl-14 pr-4 text-2xl text-prime-text outline-none transition-colors placeholder:text-[rgba(122,128,138,0.8)] focus:border-highlight"
            />
          </div>
          <div className="flex items-center gap-3">
            {(['all', 'agent', 'skill', 'tool'] as const).map((kind) => (
              <button
                key={kind}
                type="button"
                className={`ui-pill-btn-secondary px-5 py-2.5 text-lg ${filter === kind ? 'border-highlight text-highlight' : ''}`}
                onClick={() => setFilter(kind)}
              >
                {kind === 'all' ? 'All' : kindLabel(kind)}
              </button>
            ))}
          </div>
        </div>

        {error ? (
          <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
            {error}
          </div>
        ) : null}

        {loading && !result ? (
          <div className="rounded border border-border px-4 py-6 text-sm text-secondary-text">
            Loading marketplace...
          </div>
        ) : null}

        {!loading && filteredItems.length === 0 ? (
          <div className="rounded border border-border px-4 py-6 text-sm text-secondary-text">
            No marketplace items match the current filters.
          </div>
        ) : null}

        {filteredItems.length > 0 ? (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
            {filteredItems.map((item) => {
              const busy = busyKey === `${item.scope || 'public'}:${item.orgID || ''}:${item.kind}:${item.id}`;
              const actionLabel = item.status === 'not_installed' ? 'Install' : 'Update';
              const helperText = item.missingFromCatalog
                ? 'This managed item no longer exists in the catalog.'
                : item.status === 'update_available' && item.inUse
                  ? 'This resource is currently in use. Updating now only affects the next run.'
                  : item.status === 'installed'
                    ? 'Installed and managed by the connected runtime.'
                    : isOrgMarketplace
                      ? 'Available from this organization catalog.'
                      : 'Available from the official marketplace catalog.';

              return (
                <section
                  key={`${item.scope || 'public'}:${item.orgID || ''}:${item.kind}:${item.id}`}
                  className="flex min-h-[240px] flex-col rounded-2xl border border-border bg-sidebar-bg p-5 shadow-sm"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex items-start gap-3 min-w-0">
                      <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-editor-bg text-prime-text">
                        <ItemIcon kind={item.kind} />
                      </div>
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <h3 className="truncate text-base font-semibold text-prime-text">{item.name}</h3>
                          {item.builtin ? (
                            <span className="rounded-full bg-selection px-2 py-0.5 text-[10px] uppercase tracking-wide text-prime-text">
                              Built-in
                            </span>
                          ) : null}
                          {item.scope === 'org' ? (
                            <span className="rounded-full bg-selection px-2 py-0.5 text-[10px] uppercase tracking-wide text-prime-text">
                              Org
                            </span>
                          ) : null}
                        </div>
                        <div className="mt-2 flex flex-wrap items-center gap-2 text-[11px]">
                          <span className={`rounded-full px-2 py-0.5 ${kindPillClassName(item.kind)}`}>
                            {kindLabel(item.kind)}
                          </span>
                          <span className={`rounded-full border px-2 py-0.5 ${statusClassName(item)}`}>
                            {statusLabel(item)}
                          </span>
                        </div>
                      </div>
                    </div>
                  </div>

                  <p className="mt-4 line-clamp-3 text-sm leading-6 text-secondary-text">
                    {item.description}
                  </p>

                  <div className="mt-4 flex flex-col gap-1 text-xs text-secondary-text">
                    <div>ID: {item.id}</div>
                    <div>Catalog Version: {item.version || '-'}</div>
                    <div>Installed Version: {item.installedVersion || '-'}</div>
                    <div className="truncate" title={item.installPath}>Path: {item.installPath}</div>
                  </div>

                  <div className="mt-auto pt-5">
                    <div className="text-xs leading-5 text-secondary-text">
                      {helperText}
                    </div>
                    <div className="mt-3 flex items-center justify-between gap-3">
                      <span className="text-[11px] text-tertiary-text">
                        {item.sourceUrl
                          ? isOrgMarketplace ? 'Organization package' : 'Official package'
                          : 'Catalog entry only'}
                      </span>
                      {item.missingFromCatalog || item.status === 'installed' ? null : (
                        <button
                          type="button"
                          className="ui-pill-btn-primary op-sg-capsule op-sg-capsule--on-editor px-3 py-1.5 text-xs disabled:cursor-not-allowed disabled:opacity-60"
                          onClick={() => void handleAction(item)}
                          disabled={busy}
                        >
                          {busy ? 'Working...' : actionLabel}
                        </button>
                      )}
                    </div>
                  </div>
                </section>
              );
            })}
          </div>
        ) : null}
      </div>
    </div>
  );
};
