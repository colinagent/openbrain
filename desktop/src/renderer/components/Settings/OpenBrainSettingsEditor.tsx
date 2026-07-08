import React, { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useAuthStore } from '../../store/authStore';
import { useOpenBrainStore, type OpenBrainProviderMode } from '../../store/openBrainStore';

type LocalGBrainSettings = {
  engine?: 'pglite' | 'postgres';
  databaseUrl?: string;
  databasePath?: string;
  remoteMcpUrl?: string;
  remoteMcpClientID?: string;
  remoteMcpClientSecret?: string;
  remoteMcpClientSecretEnvVar?: string;
  cliPath?: string;
};

type OpenBrainSettings = {
  provider?: OpenBrainProviderMode;
  local?: LocalGBrainSettings;
};

type NoticeState = {
  tone: 'success' | 'warning' | 'error';
  text: string;
} | null;

const emptyLocalSettings: LocalGBrainSettings = {};

function normalizeLocalSettings(value: unknown): LocalGBrainSettings {
  const raw = value && typeof value === 'object' ? value as Partial<LocalGBrainSettings> : {};
  const engine = raw.engine === 'postgres' || raw.engine === 'pglite' ? raw.engine : 'pglite';
  return {
    engine,
    databaseUrl: typeof raw.databaseUrl === 'string' ? raw.databaseUrl : '',
    databasePath: typeof raw.databasePath === 'string' ? raw.databasePath : '',
    remoteMcpUrl: typeof raw.remoteMcpUrl === 'string' ? raw.remoteMcpUrl : '',
    remoteMcpClientID: typeof raw.remoteMcpClientID === 'string' ? raw.remoteMcpClientID : '',
    remoteMcpClientSecret: typeof raw.remoteMcpClientSecret === 'string' ? raw.remoteMcpClientSecret : '',
    remoteMcpClientSecretEnvVar: typeof raw.remoteMcpClientSecretEnvVar === 'string' ? raw.remoteMcpClientSecretEnvVar : '',
    cliPath: typeof raw.cliPath === 'string' ? raw.cliPath : '',
  };
}

function normalizeOpenBrainSettings(value: unknown): Required<OpenBrainSettings> {
  const raw = value && typeof value === 'object' ? value as Partial<OpenBrainSettings> : {};
  return {
    provider: raw.provider === 'local' ? 'local' : 'cloud',
    local: normalizeLocalSettings(raw.local),
  };
}

function sanitizeLocalSettings(local: LocalGBrainSettings): LocalGBrainSettings {
  const engine = local.engine === 'postgres' ? 'postgres' : 'pglite';
  return {
    engine,
    ...(typeof local.databaseUrl === 'string' && local.databaseUrl.trim() ? { databaseUrl: local.databaseUrl.trim() } : {}),
    ...(typeof local.databasePath === 'string' && local.databasePath.trim() ? { databasePath: local.databasePath.trim() } : {}),
    ...(typeof local.remoteMcpUrl === 'string' && local.remoteMcpUrl.trim() ? { remoteMcpUrl: local.remoteMcpUrl.trim() } : {}),
    ...(typeof local.remoteMcpClientID === 'string' && local.remoteMcpClientID.trim() ? { remoteMcpClientID: local.remoteMcpClientID.trim() } : {}),
    ...(typeof local.remoteMcpClientSecret === 'string' && local.remoteMcpClientSecret.trim() ? { remoteMcpClientSecret: local.remoteMcpClientSecret.trim() } : {}),
    ...(typeof local.remoteMcpClientSecretEnvVar === 'string' && local.remoteMcpClientSecretEnvVar.trim() ? { remoteMcpClientSecretEnvVar: local.remoteMcpClientSecretEnvVar.trim() } : {}),
    ...(typeof local.cliPath === 'string' && local.cliPath.trim() ? { cliPath: local.cliPath.trim() } : {}),
  };
}

function fieldClassName(disabled = false) {
  return [
    'w-full rounded border border-border bg-editor-bg px-3 py-2 text-sm text-prime-text outline-none',
    'focus:border-active-border disabled:cursor-not-allowed disabled:opacity-60',
    disabled ? 'opacity-60' : '',
  ].filter(Boolean).join(' ');
}

export const OpenBrainSettingsEditor: React.FC = () => {
  const { t } = useTranslation(['settings', 'common']);
  const loggedIn = useAuthStore((state) => state.loggedIn);
  const startLogin = useAuthStore((state) => state.startLogin);
  const deviceCodeError = useAuthStore((state) => state.deviceCodeError);
  const refreshOpenBrains = useOpenBrainStore((state) => state.refresh);

  const [provider, setProvider] = useState<OpenBrainProviderMode>('cloud');
  const [local, setLocal] = useState<LocalGBrainSettings>(emptyLocalSettings);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState<NoticeState>(null);

  const localSettings = useMemo(() => normalizeLocalSettings(local), [local]);
  const localEnabled = provider === 'local';

  useEffect(() => {
    if (deviceCodeError) {
      setNotice({ tone: 'error', text: deviceCodeError });
    }
  }, [deviceCodeError]);

  const applySettings = (settings: unknown) => {
    const next = normalizeOpenBrainSettings((settings as { user?: { openBrain?: unknown } } | null | undefined)?.user?.openBrain);
    setProvider(next.provider);
    setLocal(next.local);
  };

  useEffect(() => {
    let cancelled = false;
    const settingsApi = window.electronAPI?.settings;
    if (!settingsApi?.get) {
      setLoading(false);
      return;
    }
    settingsApi.get()
      .then((settings) => {
        if (!cancelled) {
          applySettings(settings);
        }
      })
      .catch((error) => {
        if (!cancelled) {
          setNotice({ tone: 'error', text: error instanceof Error ? error.message : 'Failed to load OpenBrain settings.' });
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    const dispose = settingsApi.onChanged?.((settings) => {
      applySettings(settings);
    });
    return () => {
      cancelled = true;
      dispose?.();
    };
  }, []);

  const updateLocal = (patch: Partial<LocalGBrainSettings>) => {
    setLocal((current) => ({
      ...normalizeLocalSettings(current),
      ...patch,
    }));
    setNotice(null);
  };

  const handleSave = async () => {
    setSaving(true);
    setNotice(null);
    try {
      const nextLocal = sanitizeLocalSettings(localSettings);
      const result = await window.electronAPI?.openBrain?.setProvider?.({
        provider,
        local: nextLocal,
      });
      if (!result) {
        throw new Error('OpenBrain provider API is not available.');
      }
      setProvider(result.provider || provider);
      setLocal(normalizeLocalSettings(nextLocal));
      await refreshOpenBrains().catch(() => []);
      setNotice(result.authRequired
        ? { tone: 'warning', text: t('settings:openBrain.savedLoginRequired') }
        : { tone: 'success', text: t('settings:openBrain.saved') });
    } catch (error) {
      setNotice({ tone: 'error', text: error instanceof Error ? error.message : 'Failed to save OpenBrain settings.' });
    } finally {
      setSaving(false);
    }
  };

  const handleLogin = async () => {
    setNotice(null);
    const result = await startLogin();
    if (!result?.success) {
      setNotice({ tone: 'error', text: 'Failed to start login.' });
    }
  };

  const noticeClassName = notice?.tone === 'error'
    ? 'text-red-700 bg-red-50 dark:bg-red-900/20'
    : notice?.tone === 'warning'
      ? 'text-amber-800 bg-amber-50 dark:bg-amber-900/20'
      : 'text-health-text bg-green-50 dark:bg-green-900/20';

  return (
    <div className="flex h-full flex-col overflow-auto bg-editor-bg text-editor-fg">
      <div className="border-b border-border px-4 py-3">
        <div className="text-sm font-semibold">{t('settings:openBrain.title')}</div>
        <div className="text-xs text-secondary-text">{t('settings:openBrain.subtitle')}</div>
      </div>

      <div className="max-w-[860px] p-4">
        <div className="rounded border border-border p-4">
          <div className="text-sm font-semibold text-prime-text">{t('settings:openBrain.provider')}</div>
          <div className="mt-3 grid gap-2 sm:grid-cols-2">
            <button
              type="button"
              className={`ui-pill-btn-secondary w-full flex-col items-start px-3 py-2 text-left text-sm ${provider === 'cloud' ? 'border-highlight text-highlight' : ''}`}
              onClick={() => {
                setProvider('cloud');
                setNotice(null);
              }}
              disabled={loading || saving}
            >
              <div className="font-medium">{t('settings:openBrain.cloud')}</div>
              <div className="mt-1 text-xs text-secondary-text">{t('settings:openBrain.cloudHint')}</div>
            </button>
            <button
              type="button"
              className={`ui-pill-btn-secondary w-full flex-col items-start px-3 py-2 text-left text-sm ${provider === 'local' ? 'border-highlight text-highlight' : ''}`}
              onClick={() => {
                setProvider('local');
                setNotice(null);
              }}
              disabled={loading || saving}
            >
              <div className="font-medium">{t('settings:openBrain.local')}</div>
              <div className="mt-1 text-xs text-secondary-text">{t('settings:openBrain.localHint')}</div>
            </button>
          </div>

          {provider === 'cloud' && !loggedIn ? (
            <div className="mt-4 rounded border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20">
              <div className="font-medium">{t('settings:openBrain.signInRequired')}</div>
              <div className="mt-1 text-xs">{t('settings:openBrain.signInHint')}</div>
              <button
                type="button"
                className="ui-pill-btn-primary op-sg-capsule op-sg-capsule--on-editor mt-2 px-3 py-1.5 text-xs"
                onClick={() => void handleLogin()}
              >
                {t('settings:openBrain.logIn')}
              </button>
            </div>
          ) : null}
        </div>

        <div className={`mt-4 rounded border border-border p-4 ${localEnabled ? '' : 'opacity-60'}`}>
          <div className="text-sm font-semibold text-prime-text">{t('settings:openBrain.localConfiguration')}</div>
          <div className="mt-3 grid gap-4">
            <label className="flex flex-col gap-1 text-xs text-secondary-text">
              {t('settings:openBrain.engine')}
              <select
                className={fieldClassName(!localEnabled)}
                value={localSettings.engine || 'pglite'}
                onChange={(event) => updateLocal({ engine: event.target.value === 'postgres' ? 'postgres' : 'pglite' })}
                disabled={!localEnabled || loading || saving}
              >
                <option value="pglite">PGLite</option>
                <option value="postgres">Postgres</option>
              </select>
            </label>

            <label className="flex flex-col gap-1 text-xs text-secondary-text">
              {t('settings:openBrain.databasePath')}
              <input
                className={fieldClassName(!localEnabled)}
                value={localSettings.databasePath || ''}
                onChange={(event) => updateLocal({ databasePath: event.target.value })}
                placeholder="~/.openbrain/configs/gbrain/.gbrain/brain.pglite"
                disabled={!localEnabled || loading || saving}
              />
            </label>

            <label className="flex flex-col gap-1 text-xs text-secondary-text">
              {t('settings:openBrain.databaseUrl')}
              <input
                className={fieldClassName(!localEnabled)}
                value={localSettings.databaseUrl || ''}
                onChange={(event) => updateLocal({ databaseUrl: event.target.value })}
                placeholder="postgresql://..."
                disabled={!localEnabled || loading || saving}
              />
            </label>

            <label className="flex flex-col gap-1 text-xs text-secondary-text">
              {t('settings:openBrain.remoteMcpUrl')}
              <input
                className={fieldClassName(!localEnabled)}
                value={localSettings.remoteMcpUrl || ''}
                onChange={(event) => updateLocal({ remoteMcpUrl: event.target.value })}
                placeholder="https://example.com/mcp"
                disabled={!localEnabled || loading || saving}
              />
            </label>

            <label className="flex flex-col gap-1 text-xs text-secondary-text">
              {t('settings:openBrain.remoteMcpClientID')}
              <input
                className={fieldClassName(!localEnabled)}
                value={localSettings.remoteMcpClientID || ''}
                onChange={(event) => updateLocal({ remoteMcpClientID: event.target.value })}
                placeholder="openbrain-desktop"
                disabled={!localEnabled || loading || saving}
              />
            </label>

            <label className="flex flex-col gap-1 text-xs text-secondary-text">
              {t('settings:openBrain.remoteMcpClientSecret')}
              <input
                className={fieldClassName(!localEnabled)}
                value={localSettings.remoteMcpClientSecret || ''}
                onChange={(event) => updateLocal({ remoteMcpClientSecret: event.target.value })}
                placeholder={t('settings:openBrain.remoteMcpClientSecretPlaceholder')}
                type="password"
                disabled={!localEnabled || loading || saving}
              />
            </label>

            <label className="flex flex-col gap-1 text-xs text-secondary-text">
              {t('settings:openBrain.remoteMcpClientSecretEnvVar')}
              <input
                className={fieldClassName(!localEnabled)}
                value={localSettings.remoteMcpClientSecretEnvVar || ''}
                onChange={(event) => updateLocal({ remoteMcpClientSecretEnvVar: event.target.value })}
                placeholder="GBRAIN_REMOTE_CLIENT_SECRET"
                disabled={!localEnabled || loading || saving}
              />
            </label>

            <label className="flex flex-col gap-1 text-xs text-secondary-text">
              {t('settings:openBrain.cliPath')}
              <input
                className={fieldClassName(!localEnabled)}
                value={localSettings.cliPath || ''}
                onChange={(event) => updateLocal({ cliPath: event.target.value })}
                placeholder="gbrain"
                disabled={!localEnabled || loading || saving}
              />
            </label>
          </div>
        </div>

        {notice ? (
          <div className={`mt-4 rounded px-3 py-2 text-xs ${noticeClassName}`}>{notice.text}</div>
        ) : null}

        <div className="mt-4 flex gap-2">
          <button
            type="button"
            className="ui-pill-btn-primary op-sg-capsule op-sg-capsule--on-editor px-3 py-1.5 text-sm disabled:cursor-not-allowed disabled:opacity-60"
            onClick={() => void handleSave()}
            disabled={loading || saving}
          >
            {saving ? t('settings:openBrain.saving') : t('common:save', { ns: 'common' })}
          </button>
        </div>
      </div>
    </div>
  );
};
