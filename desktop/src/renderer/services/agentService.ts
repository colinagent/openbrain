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
  baseDir?: string;
  hostID?: string;
  instanceID?: string;
  [key: string]: unknown;
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

  async getSystemConfig(): Promise<SystemConfigResult | null> {
    try {
      return await this.connection.request<SystemConfigResult>(
        'config/system/get',
        {},
      );
    } catch (err) {
      warnOnce('systemConfigGet', `[agentService] config/system/get failed: ${(err as Error)?.message || 'unknown error'}`);
      return null;
    }
  }

  clearCache(): void {
    // no-op; caching is handled by appStore
  }
}

export function createAgentService(connection: WSConnection): AgentService {
  return new AgentService(connection);
}

export type { AgentService };
