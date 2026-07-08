import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { IdleSleepPolicy } from '../../../main/settings/settingsStore';

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
