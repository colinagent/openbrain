import { resolveWorkspaceHttpBaseUrl } from './resourceService';

export type RemoteControlStatus = {
  available: boolean;
  enabled: boolean;
  connectionState: 'off' | 'connecting' | 'online' | 'reconnecting';
  environmentID?: string;
  environmentName?: string;
  regionID?: string;
  routingGeneration?: number;
  lastError?: string;
};

export type RemoteControlRegion = {
  regionID: string;
  displayName: string;
  enabled: boolean;
  sortOrder: number;
};

export type RemoteControlPairing = {
  pairingID: string;
  code: string;
  expiresAt: string;
  qrPayload: string;
  qrDataURL: string;
};

export type RemoteControlClient = {
  clientID: string;
  environmentID: string;
  name: string;
  platform: string;
  createdAt: string;
  lastSeenAt?: string;
  revokedAt?: string;
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${resolveWorkspaceHttpBaseUrl()}${path}`, {
    ...init,
    headers: init?.body ? { 'Content-Type': 'application/json', ...init.headers } : init?.headers,
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => null) as { message?: string; code?: string } | null;
    throw new Error(payload?.message || payload?.code || `Request failed: ${response.status}`);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return response.json() as Promise<T>;
}

export const remoteControlService = {
  status: () => request<RemoteControlStatus>('/v1/remote-control/status'),
  regions: async () => (await request<{ regions: RemoteControlRegion[] }>('/v1/remote-control/regions')).regions,
  enable: (input: { confirmed: boolean; name?: string; regionID: string }) => request<RemoteControlStatus>('/v1/remote-control/enable', {
    method: 'POST', body: JSON.stringify(input),
  }),
  disable: () => request<void>('/v1/remote-control/disable', { method: 'POST' }),
  switchRegion: (regionID: string) => request<RemoteControlStatus>('/v1/remote-control/region', {
    method: 'POST', body: JSON.stringify({ regionID }),
  }),
  startPairing: () => request<RemoteControlPairing>('/v1/remote-control/pairings', { method: 'POST' }),
  clients: async () => (await request<{ clients: RemoteControlClient[] }>('/v1/remote-control/clients')).clients,
  revokeClient: (clientID: string) => request<void>(`/v1/remote-control/clients/${encodeURIComponent(clientID)}`, { method: 'DELETE' }),
};
