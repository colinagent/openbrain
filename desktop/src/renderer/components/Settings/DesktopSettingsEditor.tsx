import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { IdleSleepPolicy } from '../../../main/settings/settingsStore';
import { CopyIcon, RefreshIcon, TrashIcon } from '../Icons';
import { IconButton } from '../IconButton';
import {
  remoteControlService,
  type RemoteControlClient,
  type RemoteControlPairing,
  type RemoteControlRegion,
  type RemoteControlStatus,
} from '../../services/remoteControlService';

type NoticeState = {
  tone: 'success' | 'warning' | 'error';
  text: string;
} | null;

const IDLE_SLEEP_POLICIES: IdleSleepPolicy[] = ['off', 'whileAgentRunning', 'whileAppRunning'];

function readIdleSleepPolicy(settings: unknown): IdleSleepPolicy {
  const raw = settings && typeof settings === 'object'
    ? settings as { system?: { power?: { idleSleepPolicy?: unknown; preventSleepWhileAgentRunning?: unknown } } }
    : null;
  const policy = raw?.system?.power?.idleSleepPolicy;
  if (policy === 'whileAgentRunning' || policy === 'whileAppRunning' || policy === 'off') {
    return policy;
  }
  if (raw?.system?.power?.preventSleepWhileAgentRunning === true) {
    return 'whileAgentRunning';
  }
  return 'off';
}

function policyLabelKey(policy: IdleSleepPolicy): `desktop.idleSleep${'Off' | 'WhileAgentRunning' | 'WhileAppRunning'}` {
  switch (policy) {
    case 'whileAgentRunning':
      return 'desktop.idleSleepWhileAgentRunning';
    case 'whileAppRunning':
      return 'desktop.idleSleepWhileAppRunning';
    default:
      return 'desktop.idleSleepOff';
  }
}

function policyHintKey(policy: IdleSleepPolicy): `desktop.idleSleep${'Off' | 'WhileAgentRunning' | 'WhileAppRunning'}Hint` {
  switch (policy) {
    case 'whileAgentRunning':
      return 'desktop.idleSleepWhileAgentRunningHint';
    case 'whileAppRunning':
      return 'desktop.idleSleepWhileAppRunningHint';
    default:
      return 'desktop.idleSleepOffHint';
  }
}

export const DesktopSettingsEditor: React.FC = () => {
  const { t } = useTranslation(['settings', 'common']);
  const [idleSleepPolicy, setIdleSleepPolicy] = useState<IdleSleepPolicy>('off');
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<NoticeState>(null);
  const [remoteStatus, setRemoteStatus] = useState<RemoteControlStatus | null>(null);
  const [regions, setRegions] = useState<RemoteControlRegion[]>([]);
  const [clients, setClients] = useState<RemoteControlClient[]>([]);
  const [pairing, setPairing] = useState<RemoteControlPairing | null>(null);
  const [selectedRegion, setSelectedRegion] = useState('');
  const [confirmingEnable, setConfirmingEnable] = useState(false);
  const [enableConfirmed, setEnableConfirmed] = useState(false);
  const [remoteBusy, setRemoteBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;

    const load = async () => {
      try {
        const settings = await window.electronAPI?.settings?.get();
        if (!cancelled) {
          setIdleSleepPolicy(readIdleSleepPolicy(settings));
        }
      } catch (error) {
        if (!cancelled) {
          setNotice({
            tone: 'error',
            text: `${t('desktop.loadFailed')}: ${(error as Error).message}`,
          });
        }
      }
    };

    void load();
    const dispose = window.electronAPI?.settings?.onChanged?.((settings) => {
      setIdleSleepPolicy(readIdleSleepPolicy(settings));
    });

    return () => {
      cancelled = true;
      dispose?.();
    };
  }, [t]);

  const loadRemoteControl = async () => {
    const status = await remoteControlService.status();
    setRemoteStatus(status);
    if (!status.available) return;
    const [nextRegions, nextClients] = await Promise.all([
      remoteControlService.regions(),
      status.enabled ? remoteControlService.clients() : Promise.resolve([]),
    ]);
    setRegions(nextRegions);
    setClients(nextClients.filter((client) => !client.revokedAt));
    setSelectedRegion(status.regionID || nextRegions[0]?.regionID || '');
  };

  useEffect(() => {
    void loadRemoteControl().catch((error) => {
      setNotice({ tone: 'error', text: `${t('desktop.remote.loadFailed')}: ${(error as Error).message}` });
    });
    const timer = window.setInterval(() => {
      void remoteControlService.status().then(setRemoteStatus).catch(() => {});
    }, 3000);
    return () => window.clearInterval(timer);
  }, [t]);

  const runRemoteAction = async (action: () => Promise<void>) => {
    setRemoteBusy(true);
    setNotice(null);
    try {
      await action();
      await loadRemoteControl();
    } catch (error) {
      setNotice({ tone: 'error', text: (error as Error).message });
    } finally {
      setRemoteBusy(false);
    }
  };

  const handlePolicyChange = async (nextPolicy: IdleSleepPolicy) => {
    setBusy(true);
    setNotice(null);
    setIdleSleepPolicy(nextPolicy);
    try {
      await window.electronAPI?.settings?.set({
        system: {
          power: {
            idleSleepPolicy: nextPolicy,
          },
        },
      });
      setNotice({ tone: 'success', text: t('desktop.saved') });
    } catch (error) {
      setNotice({
        tone: 'error',
        text: `${t('desktop.saveFailed')}: ${(error as Error).message}`,
      });
      try {
        const settings = await window.electronAPI?.settings?.get();
        setIdleSleepPolicy(readIdleSleepPolicy(settings));
      } catch {
        setIdleSleepPolicy('off');
      }
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-col h-full overflow-auto bg-editor-bg text-editor-fg">
      <div className="px-4 py-3 border-b border-border">
        <div className="text-sm font-semibold">{t('desktop.title')}</div>
        <div className="text-xs text-secondary-text">{t('desktop.subtitle')}</div>
      </div>

      <div className="flex-1 px-4 py-4 space-y-4">
        <section className="space-y-3 border-b border-border pb-5" aria-labelledby="remote-access-title">
          <div className="flex items-center gap-3">
            <span className={`h-2.5 w-2.5 shrink-0 rounded-full ${remoteStatus?.connectionState === 'online' ? 'bg-highlight' : remoteStatus?.enabled ? 'bg-accent' : 'bg-secondary-text/40'}`} />
            <div className="min-w-0 flex-1">
              <div id="remote-access-title" className="text-sm font-medium">{t('desktop.remote.title')}</div>
              <div className="truncate text-xs text-secondary-text">
                {remoteStatus?.enabled
                  ? t(`desktop.remote.state.${remoteStatus.connectionState}`)
                  : t('desktop.remote.state.off')}
              </div>
            </div>
            <button
              type="button"
              className="ui-pill-btn-secondary h-8 px-3 text-xs disabled:cursor-not-allowed disabled:opacity-50"
              disabled={remoteBusy || !remoteStatus?.available}
              onClick={() => {
                if (remoteStatus?.enabled) {
                  void runRemoteAction(async () => {
                    await remoteControlService.disable();
                    setPairing(null);
                  });
                } else {
                  setConfirmingEnable(true);
                }
              }}
            >
              {remoteStatus?.enabled ? t('desktop.remote.disable') : t('desktop.remote.enable')}
            </button>
          </div>

          {remoteStatus && !remoteStatus.available ? (
            <div className="text-xs text-secondary-text">{t('desktop.remote.unavailable')}</div>
          ) : null}

          {confirmingEnable && !remoteStatus?.enabled ? (
            <div className="space-y-3 border-l-2 border-border pl-3">
              <div className="text-xs text-secondary-text">{t('desktop.remote.confirmHint')}</div>
              <label className="flex items-start gap-2 text-xs">
                <input type="checkbox" className="mt-0.5" checked={enableConfirmed} onChange={(event) => setEnableConfirmed(event.target.checked)} />
                <span>{t('desktop.remote.confirmLabel')}</span>
              </label>
              <div className="flex flex-wrap items-center gap-2">
                <select
                  className="h-8 min-w-40 border border-border bg-editor-bg px-2 text-xs"
                  value={selectedRegion}
                  onChange={(event) => setSelectedRegion(event.target.value)}
                >
                  {regions.map((region) => <option key={region.regionID} value={region.regionID}>{region.displayName}</option>)}
                </select>
                <button
                  type="button"
                  className="ui-pill-btn-primary op-sg-capsule op-sg-capsule--on-editor h-8 px-3 text-xs disabled:cursor-not-allowed disabled:opacity-50"
                  disabled={remoteBusy || !enableConfirmed || !selectedRegion}
                  onClick={() => void runRemoteAction(async () => {
                    await remoteControlService.enable({ confirmed: true, regionID: selectedRegion });
                    setConfirmingEnable(false);
                    setEnableConfirmed(false);
                  })}
                >
                  {t('desktop.remote.confirmEnable')}
                </button>
                <button type="button" className="h-8 px-2 text-xs text-secondary-text" onClick={() => setConfirmingEnable(false)}>{t('common:cancel')}</button>
              </div>
            </div>
          ) : null}

          {remoteStatus?.enabled ? (
            <div className="space-y-4">
              <div className="grid gap-2 text-xs sm:grid-cols-[minmax(0,1fr)_minmax(12rem,auto)] sm:items-end">
                <div className="min-w-0">
                  <div className="text-secondary-text">{t('desktop.remote.computer')}</div>
                  <div className="truncate text-sm">{remoteStatus.environmentName}</div>
                </div>
                <label className="space-y-1">
                  <span className="block text-secondary-text">{t('desktop.remote.region')}</span>
                  <span className="flex gap-2">
                    <select className="h-8 min-w-36 flex-1 border border-border bg-editor-bg px-2" value={selectedRegion} onChange={(event) => setSelectedRegion(event.target.value)}>
                      {regions.map((region) => <option key={region.regionID} value={region.regionID}>{region.displayName}</option>)}
                    </select>
                    <button
                      type="button"
                      className="ui-pill-btn-secondary h-8 px-3 disabled:cursor-not-allowed disabled:opacity-50"
                      disabled={remoteBusy || selectedRegion === remoteStatus.regionID}
                      onClick={() => void runRemoteAction(async () => { await remoteControlService.switchRegion(selectedRegion); })}
                    >
                      {t('desktop.remote.switchRegion')}
                    </button>
                  </span>
                </label>
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between gap-2">
                  <div className="text-xs font-medium">{t('desktop.remote.pairDevice')}</div>
                  <button
                    type="button"
                    className="ui-pill-btn-secondary flex h-8 items-center gap-1.5 px-2 text-xs disabled:cursor-not-allowed disabled:opacity-50"
                    disabled={remoteBusy}
                    onClick={() => void runRemoteAction(async () => { setPairing(await remoteControlService.startPairing()); })}
                  >
                    <RefreshIcon className="h-3.5 w-3.5" />
                    {pairing ? t('desktop.remote.newCode') : t('desktop.remote.createCode')}
                  </button>
                </div>
                {pairing ? (
                  <div className="flex flex-wrap items-center gap-4 border-y border-border py-3">
                    <img src={pairing.qrDataURL} alt={t('desktop.remote.qrAlt')} className="h-32 w-32 bg-white p-1" />
                    <div className="min-w-0 space-y-1">
                      <div className="text-xs text-secondary-text">{t('desktop.remote.shortCode')}</div>
                      <div className="flex items-center gap-2">
                        <code className="text-lg font-semibold">{pairing.code}</code>
                        <IconButton size={28} variant="inline" title={t('desktop.remote.copyCode')} aria-label={t('desktop.remote.copyCode')} onClick={() => void navigator.clipboard.writeText(pairing.code)}>
                          <CopyIcon className="h-4 w-4" />
                        </IconButton>
                      </div>
                      <div className="text-xs text-secondary-text">{t('desktop.remote.expiresAt', { value: new Date(pairing.expiresAt).toLocaleTimeString() })}</div>
                    </div>
                  </div>
                ) : null}
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <div className="text-xs font-medium">{t('desktop.remote.devices')}</div>
                  <IconButton size={28} variant="inline" title={t('desktop.remote.refreshDevices')} aria-label={t('desktop.remote.refreshDevices')} disabled={remoteBusy} onClick={() => void runRemoteAction(async () => {})}>
                    <RefreshIcon className="h-4 w-4" />
                  </IconButton>
                </div>
                {clients.length === 0 ? <div className="text-xs text-secondary-text">{t('desktop.remote.noDevices')}</div> : clients.map((client) => (
                  <div key={client.clientID} className="flex min-h-11 items-center gap-3 border-t border-border py-2 first:border-t-0">
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm">{client.name}</div>
                      <div className="truncate text-xs text-secondary-text">
                        {client.platform} · {client.lastSeenAt ? t('desktop.remote.lastSeen', { value: new Date(client.lastSeenAt).toLocaleString() }) : t('desktop.remote.neverSeen')}
                      </div>
                    </div>
                    <IconButton size={28} variant="inline" title={t('desktop.remote.revoke')} aria-label={t('desktop.remote.revoke')} disabled={remoteBusy} onClick={() => void runRemoteAction(async () => { await remoteControlService.revokeClient(client.clientID); })}>
                      <TrashIcon className="h-4 w-4" />
                    </IconButton>
                  </div>
                ))}
              </div>
            </div>
          ) : null}
        </section>

        <div className="space-y-3">
          <div className="text-sm font-medium">{t('desktop.idleSleepPolicyLabel')}</div>
          {IDLE_SLEEP_POLICIES.map((policy) => (
            <label key={policy} className="flex items-start gap-3 cursor-pointer">
              <input
                type="radio"
                name="idleSleepPolicy"
                className="mt-1"
                checked={idleSleepPolicy === policy}
                disabled={busy}
                onChange={() => {
                  void handlePolicyChange(policy);
                }}
              />
              <span className="space-y-1">
                <span className="block text-sm">{t(policyLabelKey(policy))}</span>
                <span className="block text-xs text-secondary-text">{t(policyHintKey(policy))}</span>
              </span>
            </label>
          ))}
        </div>

        {notice ? (
          <div className={`text-xs ${notice.tone === 'error' ? 'text-error-text' : 'text-secondary-text'}`}>
            {notice.text}
          </div>
        ) : null}
      </div>
    </div>
  );
};
