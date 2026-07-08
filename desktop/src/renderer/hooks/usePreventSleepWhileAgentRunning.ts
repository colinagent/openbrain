import { useEffect, useRef, useState } from 'react';

import type { IdleSleepPolicy } from '../../main/settings/settingsStore';
import { getChatWorkspaceStore } from '../store/chatWorkspaceStore';
import { useTabManagerStore } from '../store/tabManagerStore';

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

function computeAnyInProgress(tabIds: string[]): boolean {
  return tabIds.some((tabId) => getChatWorkspaceStore(tabId).getState().hasAnyInProgress());
}

export function usePreventSleepWhileAgentRunning(): void {
  const tabs = useTabManagerStore((state) => state.tabs);
  const tabIds = tabs.map((tab) => tab.id);
  const tabIdsKey = tabIds.join('|');
  const lastReportedRef = useRef<boolean | null>(null);
  const [idleSleepPolicy, setIdleSleepPolicy] = useState<IdleSleepPolicy>('off');

  useEffect(() => {
    let cancelled = false;

    const applyPolicy = (settings: unknown) => {
      if (!cancelled) {
        setIdleSleepPolicy(readIdleSleepPolicy(settings));
      }
    };

    void window.electronAPI?.settings?.get().then(applyPolicy).catch(() => {
      if (!cancelled) {
        setIdleSleepPolicy('off');
      }
    });

    const dispose = window.electronAPI?.settings?.onChanged?.(applyPolicy);
    return () => {
      cancelled = true;
      dispose?.();
    };
  }, []);

  useEffect(() => {
    if (idleSleepPolicy !== 'whileAgentRunning') {
      lastReportedRef.current = null;
      void window.electronAPI?.power?.setAgentRunning(false);
      return undefined;
    }

    const report = () => {
      const anyInProgress = computeAnyInProgress(tabIds);
      if (lastReportedRef.current === anyInProgress) {
        return;
      }
      lastReportedRef.current = anyInProgress;
      void window.electronAPI?.power?.setAgentRunning(anyInProgress);
    };

    const unsubscribers = tabIds.map((tabId) =>
      getChatWorkspaceStore(tabId).subscribe((state, previous) => {
        if (state.inProgressByTargetKey !== previous.inProgressByTargetKey) {
          report();
        }
      }),
    );

    report();

    return () => {
      unsubscribers.forEach((unsubscribe) => unsubscribe());
      lastReportedRef.current = null;
      void window.electronAPI?.power?.setAgentRunning(false);
    };
  }, [idleSleepPolicy, tabIdsKey]);
}
