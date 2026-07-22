import type { WSConnection } from './wsConnection';

export interface OpNode {
  id: string;
  hostID?: string;
  uid: string;
  kind: string;
  uri: string;
  cwd?: string;
  tags?: string[];
  opCodes?: string[];
  run?: Record<string, unknown>;
  meta?: Record<string, unknown>;
}

export interface SystemConfigResult {
  baseDir: string;
  defaultWorkspace: string;
  hostID?: string;
  instanceID?: string;
  [key: string]: unknown;
}

export type SystemConfigRetryOptions = {
  attempts?: number;
  intervalMs?: number;
};

function waitForRetry(intervalMs: number): Promise<void> {
  if (intervalMs <= 0) {
    return Promise.resolve();
  }
  return new Promise((resolve) => {
    setTimeout(resolve, intervalMs);
  });
}

export function isCompleteSystemConfig(value: SystemConfigResult | null | undefined): value is SystemConfigResult {
  if (!value) {
    return false;
  }
  const instanceID = typeof value.hostID === 'string' && value.hostID.trim()
    ? value.hostID.trim()
    : typeof value.instanceID === 'string'
      ? value.instanceID.trim()
      : '';
  return Boolean(
    typeof value.baseDir === 'string'
    && value.baseDir.trim()
    && typeof value.defaultWorkspace === 'string'
    && value.defaultWorkspace.trim()
    && instanceID,
  );
}

const warnedMethods = {
  agentScan: false,
  nodeList: false,
  systemConfigGet: false,
};

function warnOnce(key: keyof typeof warnedMethods, message: string): void {
  if (warnedMethods[key]) {
    return;
  }
  warnedMethods[key] = true;
  console.warn(message);
}

class AgentService {
  constructor(private connection: WSConnection) {}

  async agentScan(dir: string): Promise<OpNode[]> {
    try {
      const result = await this.connection.request<OpNode[] | { nodes?: OpNode[] }>(
        'agent/scan',
        { dir },
      );
      return Array.isArray(result) ? result : result?.nodes || [];
    } catch (err) {
      warnOnce('agentScan', `[agentService] agent/scan failed: ${(err as Error)?.message || 'unknown error'}`);
      return [];
    }
  }

  async listNodes(options?: { refresh?: boolean }): Promise<OpNode[]> {
    try {
      const result = await this.connection.request<OpNode[] | { nodes?: OpNode[] }>(
        'node/list',
        options?.refresh ? { refresh: true } : {},
      );
      return Array.isArray(result) ? result : result?.nodes || [];
    } catch (err) {
      warnOnce('nodeList', `[agentService] node/list failed: ${(err as Error)?.message || 'unknown error'}`);
      return [];
    }
  }

  async getSystemConfig(options: SystemConfigRetryOptions = {}): Promise<SystemConfigResult | null> {
    const attempts = Math.max(1, options.attempts ?? 1);
    const intervalMs = Math.max(0, options.intervalMs ?? 250);
    let lastError: unknown = null;
    for (let attempt = 0; attempt < attempts; attempt += 1) {
      try {
        const result = await this.connection.request<SystemConfigResult>(
          'config/system/get',
          {},
        );
        if (isCompleteSystemConfig(result)) {
          return result;
        }
        lastError = new Error('Runtime returned an incomplete system config');
      } catch (err) {
        lastError = err;
      }
      if (attempt + 1 < attempts) {
        await waitForRetry(intervalMs);
      }
    }
    warnOnce('systemConfigGet', `[agentService] config/system/get failed: ${(lastError as Error)?.message || 'unknown error'}`);
    return null;
  }

  clearCache(): void {
    // no-op; caching is handled by appStore
  }
}

export function createAgentService(connection: WSConnection): AgentService {
  return new AgentService(connection);
}

export type { AgentService };
