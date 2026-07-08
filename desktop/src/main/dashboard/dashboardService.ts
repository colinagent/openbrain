import type { AuthConfig } from '../auth/authStore';

export type DashboardRuntimeConnection = {
  nodeID: string;
  name: string;
  transport: 'stdio' | 'http_streamable' | string;
  daemon?: boolean;
  pid?: number;
  startedAt?: string;
  uptimeSec?: number;
  lastActiveAt?: string;
  url?: string;
};

export type DashboardRuntimeUpdater = {
  currentVersion?: string;
  targetVersion?: string;
  stagedVersion?: string;
  phase?: string;
  downloaded?: boolean;
  applying?: boolean;
  lastCheckedAt?: string;
  lastError?: string;
};

export type DashboardHost = {
  id: string;
  hostname?: string;
  env?: string;
  baseDir?: string;
  online: boolean;
  lastSeenAt?: string;
  receivedAt?: string;
  runtimeConnections: DashboardRuntimeConnection[];
  runtimeUpdater?: DashboardRuntimeUpdater;
};

type GatewayRuntimeConnection = {
  nodeID?: string;
  name?: string;
  transport?: string;
  daemon?: boolean;
  pid?: number;
  startedAt?: string;
  uptimeSec?: number;
  lastActiveAt?: string;
  url?: string;
} | null;

type GatewayHeartbeatEnvelope = {
  connections?: {
    runtime?: GatewayRuntimeConnection[];
  } | null;
  updater?: GatewayRuntimeUpdater;
} | null;

type GatewayRuntimeUpdater = {
  currentVersion?: string;
  targetVersion?: string;
  stagedVersion?: string;
  phase?: string;
  downloaded?: boolean;
  applying?: boolean;
  lastCheckedAt?: string;
  lastError?: string;
} | null;

type GatewayHeartbeatHostItem = {
  id?: string;
  hostname?: string;
  env?: string;
  baseDir?: string;
  lastSeenAt?: string;
  receivedAt?: string;
  online?: boolean;
  snapshot?: GatewayHeartbeatEnvelope;
} | null;

type GatewayHeartbeatHostsResponse = {
  items?: GatewayHeartbeatHostItem[];
  nextCursor?: string;
};

function normalizeText(value: unknown): string | undefined {
  if (typeof value !== 'string') {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed || undefined;
}

function normalizeOptionalNumber(value: unknown): number | undefined {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return undefined;
  }
  return value;
}

function normalizeOptionalBoolean(value: unknown): boolean | undefined {
  return typeof value === 'boolean' ? value : undefined;
}

function normalizeRuntimeConnections(rawRuntime: GatewayRuntimeConnection[] | undefined): DashboardRuntimeConnection[] {
  if (!Array.isArray(rawRuntime) || rawRuntime.length === 0) {
    return [];
  }
  const seen = new Set<string>();
  const runtimeConnections: DashboardRuntimeConnection[] = [];
  for (const rawConnection of rawRuntime) {
    if (!rawConnection || typeof rawConnection !== 'object') {
      continue;
    }
    const nodeID = normalizeText(rawConnection.nodeID);
    if (!nodeID) {
      continue;
    }
    const dedupeKey = `${nodeID}::${normalizeText(rawConnection.transport) || ''}`;
    if (seen.has(dedupeKey)) {
      continue;
    }
    seen.add(dedupeKey);
    runtimeConnections.push({
      nodeID,
      name: normalizeText(rawConnection.name) || nodeID,
      transport: normalizeText(rawConnection.transport) || 'unknown',
      daemon: normalizeOptionalBoolean(rawConnection.daemon),
      pid: normalizeOptionalNumber(rawConnection.pid),
      startedAt: normalizeText(rawConnection.startedAt),
      uptimeSec: normalizeOptionalNumber(rawConnection.uptimeSec),
      lastActiveAt: normalizeText(rawConnection.lastActiveAt),
      url: normalizeText(rawConnection.url),
    });
  }
  return runtimeConnections;
}

function normalizeRuntimeUpdater(rawUpdater: GatewayRuntimeUpdater | undefined): DashboardRuntimeUpdater | undefined {
  if (!rawUpdater || typeof rawUpdater !== 'object') {
    return undefined;
  }
  const currentVersion = normalizeText(rawUpdater.currentVersion);
  const targetVersion = normalizeText(rawUpdater.targetVersion);
  const stagedVersion = normalizeText(rawUpdater.stagedVersion);
  const phase = normalizeText(rawUpdater.phase);
  const lastCheckedAt = normalizeText(rawUpdater.lastCheckedAt);
  const lastError = normalizeText(rawUpdater.lastError);
  const downloaded = rawUpdater.downloaded === true ? true : undefined;
  const applying = rawUpdater.applying === true ? true : undefined;

  if (
    !currentVersion
    && !targetVersion
    && !stagedVersion
    && !phase
    && !lastCheckedAt
    && !lastError
    && downloaded !== true
    && applying !== true
  ) {
    return undefined;
  }

  return {
    currentVersion,
    targetVersion,
    stagedVersion,
    phase,
    downloaded,
    applying,
    lastCheckedAt,
    lastError,
  };
}

function normalizeDashboardHost(rawItem: GatewayHeartbeatHostItem): DashboardHost | null {
  if (!rawItem || typeof rawItem !== 'object') {
    return null;
  }
  const id = normalizeText(rawItem.id);
  if (!id) {
    return null;
  }
  return {
    id,
    hostname: normalizeText(rawItem.hostname),
    env: normalizeText(rawItem.env),
    baseDir: normalizeText(rawItem.baseDir),
    online: rawItem.online === true,
    lastSeenAt: normalizeText(rawItem.lastSeenAt),
    receivedAt: normalizeText(rawItem.receivedAt) || normalizeText(rawItem.lastSeenAt),
    runtimeConnections: normalizeRuntimeConnections(rawItem.snapshot?.connections?.runtime),
    runtimeUpdater: normalizeRuntimeUpdater(rawItem.snapshot?.updater),
  };
}

export async function fetchDashboardHosts(auth: AuthConfig): Promise<DashboardHost[]> {
  const gateway = normalizeText(auth.gateway);
  const token = normalizeText(auth.token);
  if (!gateway || !token) {
    return [];
  }

  const headers = {
    Authorization: `Bearer ${token}`,
  };
  const hostsByID = new Map<string, DashboardHost>();
  const seenCursors = new Set<string>();
  let cursor = '0';

  for (;;) {
    if (seenCursors.has(cursor)) {
      break;
    }
    seenCursors.add(cursor);

    const url = new URL('/v1/heartbeat/instances', gateway);
    url.searchParams.set('cursor', cursor);
    url.searchParams.set('limit', '1000');
    url.searchParams.set('include', 'snapshot');

    const res = await fetch(url.toString(), { headers });
    if (!res.ok) {
      throw new Error(`gateway heartbeat responded ${res.status}`);
    }
    const payload = await res.json() as GatewayHeartbeatHostsResponse;
    if (Array.isArray(payload.items)) {
      for (const rawItem of payload.items) {
        const host = normalizeDashboardHost(rawItem);
        if (!host) {
          continue;
        }
        hostsByID.set(host.id, host);
      }
    }

    const nextCursor = normalizeText(payload.nextCursor) || '0';
    if (nextCursor === '0') {
      break;
    }
    cursor = nextCursor;
  }

  return Array.from(hostsByID.values());
}
