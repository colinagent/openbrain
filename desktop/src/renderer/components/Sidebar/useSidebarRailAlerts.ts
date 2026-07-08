import { useEffect, useMemo, useState } from 'react';

import { useAppStore } from '../../store/appStore';
import {
  countCronBillingAlerts,
  type SidebarRailAlertMap,
} from './sidebarRailAlerts';

const RAIL_ALERT_POLL_MS = 5000;

/**
 * Aggregates numbered sidebar-rail attention badges from runtime-backed signals.
 * Add new alert sources here as more rail items need unread/attention badges.
 */
export function useSidebarRailAlerts(): SidebarRailAlertMap {
  const connectionState = useAppStore((state) => state.connectionState);
  const listCronTasks = useAppStore((state) => state.listCronTasks);
  const [cronBillingAlertCount, setCronBillingAlertCount] = useState(0);

  useEffect(() => {
    if (connectionState !== 'connected') {
      setCronBillingAlertCount(0);
      return undefined;
    }

    let cancelled = false;
    const refresh = async () => {
      try {
        const records = await listCronTasks();
        if (!cancelled) {
          setCronBillingAlertCount(countCronBillingAlerts(records));
        }
      } catch {
        if (!cancelled) {
          setCronBillingAlertCount(0);
        }
      }
    };

    void refresh();
    const timer = window.setInterval(() => {
      void refresh();
    }, RAIL_ALERT_POLL_MS);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [connectionState, listCronTasks]);

  return useMemo(() => {
    const alerts: SidebarRailAlertMap = {};
    if (cronBillingAlertCount > 0) {
      alerts.cron = cronBillingAlertCount;
    }
    return alerts;
  }, [cronBillingAlertCount]);
}
