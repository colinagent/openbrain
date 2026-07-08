import { authFetch } from './netFetch';

const DEFAULT_DISCOVERY_ENDPOINTS = [
  'https://api.op-agent.com/v1/info',
];

function configuredDiscoveryEndpoints(): string[] {
  const configured = String(process.env.OPENBRAIN_GATEWAY_INFO_URLS || '')
    .split(',')
    .map((value) => value.trim())
    .filter(Boolean);
  return configured.length > 0 ? configured : DEFAULT_DISCOVERY_ENDPOINTS;
}

const DISCOVERY_TIMEOUT_MS = 3000;

export type GatewayInfo = {
  version: number;
  baseUrl: string;
  gateway: string;
  aiGateway: string;
  defaultOrg?: {
    id: string;
    name?: string;
  };
};

export type LoginOptions = {
  gateway?: string;
  orgSlug?: string;
};

function normalizeAbsoluteUrl(raw: unknown): string | null {
  const value = typeof raw === 'string' ? raw.trim() : '';
  if (!value) {
    return null;
  }
  try {
    const url = new URL(value);
    if (url.protocol !== 'https:' && url.protocol !== 'http:') {
      return null;
    }
    url.pathname = url.pathname.replace(/\/+$/, '');
    url.search = '';
    url.hash = '';
    return url.toString().replace(/\/$/, '');
  } catch {
    return null;
  }
}

export function normalizeManualGateway(raw: unknown): string | null {
  return normalizeAbsoluteUrl(raw);
}

function parseGatewayInfo(payload: unknown): GatewayInfo | null {
  if (!payload || typeof payload !== 'object') {
    return null;
  }
  const record = payload as Record<string, unknown>;
  const baseUrl = normalizeAbsoluteUrl(record.baseUrl);
  const gateway = normalizeAbsoluteUrl(record.gateway);
  const aiGateway = normalizeAbsoluteUrl(record.aiGateway);
  const defaultOrgRecord =
    record.defaultOrg && typeof record.defaultOrg === 'object'
      ? (record.defaultOrg as Record<string, unknown>)
      : null;
  const defaultOrgID = normalizeOrgID(defaultOrgRecord?.id);
  const defaultOrgName = typeof defaultOrgRecord?.name === 'string' ? defaultOrgRecord.name.trim() : '';
  if (!baseUrl || !gateway || !aiGateway) {
    return null;
  }
  return {
    version: 1,
    baseUrl,
    gateway,
    aiGateway,
    defaultOrg: defaultOrgID
      ? {
          id: defaultOrgID,
          name: defaultOrgName || undefined,
        }
      : undefined,
  };
}

function normalizeOrgID(raw: unknown): string | undefined {
  const value = typeof raw === 'string' ? raw.trim().toLowerCase() : '';
  if (!value) {
    return undefined;
  }
  if (value.startsWith('org-')) {
    return undefined;
  }
  return /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(value) ? value : undefined;
}

async function fetchGatewayInfo(endpoint: string): Promise<GatewayInfo> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), DISCOVERY_TIMEOUT_MS);
  try {
    const res = await authFetch(endpoint, {
      headers: { Accept: 'application/json' },
      cache: 'no-store',
      signal: controller.signal,
    });
    if (!res.ok) {
      throw new Error(`${endpoint} responded ${res.status}`);
    }
    const parsed = parseGatewayInfo(await res.json());
    if (!parsed) {
      throw new Error(`${endpoint} returned invalid gateway info`);
    }
    return parsed;
  } finally {
    clearTimeout(timer);
  }
}

export async function discoverGatewayInfo(): Promise<GatewayInfo> {
  const discoveryEndpoints = configuredDiscoveryEndpoints();
  return new Promise((resolve, reject) => {
    let pending = discoveryEndpoints.length;
    const errors: string[] = [];

    for (const endpoint of discoveryEndpoints) {
      fetchGatewayInfo(endpoint)
        .then(resolve)
        .catch((err) => {
          errors.push(err instanceof Error ? err.message : String(err));
          pending -= 1;
          if (pending === 0) {
            reject(new Error(`Failed to discover OpenBrain gateway: ${errors.join('; ')}`));
          }
        });
    }
  });
}
