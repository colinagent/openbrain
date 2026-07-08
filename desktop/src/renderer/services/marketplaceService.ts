import type { WSConnection } from './wsConnection';
import type {
  MarketplaceActionResult,
  MarketplaceOrgListResult,
  MarketplaceListResult,
  MarketplaceStateFile,
} from '../types/electron';

export type MarketplaceUsageReport = {
  agents: string[];
  skills: string[];
  tools: string[];
};

class MarketplaceService {
  constructor(
    private connection: WSConnection,
    private getUsageReport: () => MarketplaceUsageReport,
  ) {}

  async listItems(options?: { force?: boolean; orgID?: string | null }): Promise<MarketplaceListResult> {
    try {
      return await this.connection.request<MarketplaceListResult>('marketplace/list', {
        force: options?.force === true,
        orgID: options?.orgID || undefined,
        usage: this.getUsageReport(),
      });
    } catch (error) {
      return {
        items: [],
        catalogVersion: null,
        generatedAt: null,
        error: (error as Error).message,
      };
    }
  }

  async refresh(): Promise<MarketplaceListResult> {
    return this.refreshForOrg(null);
  }

  async refreshForOrg(orgID?: string | null): Promise<MarketplaceListResult> {
    try {
      return await this.connection.request<MarketplaceListResult>('marketplace/refresh', {
        orgID: orgID || undefined,
        usage: this.getUsageReport(),
      });
    } catch (error) {
      return {
        items: [],
        catalogVersion: null,
        generatedAt: null,
        error: (error as Error).message,
      };
    }
  }

  async listOrgs(): Promise<MarketplaceOrgListResult> {
    try {
      return await this.connection.request<MarketplaceOrgListResult>('marketplace/orgs', {});
    } catch (error) {
      return {
        orgs: [],
        error: (error as Error).message,
      };
    }
  }

  async getState(): Promise<{
    state: MarketplaceStateFile;
    catalogVersion: string | null;
    generatedAt: string | null;
  }> {
    return this.connection.request('marketplace/state', {});
  }

  async installItem(kind: 'agent' | 'skill' | 'tool', id: string, orgID?: string | null): Promise<MarketplaceActionResult> {
    try {
      return await this.connection.request<MarketplaceActionResult>('marketplace/install', {
        kind,
        id,
        orgID: orgID || undefined,
        usage: this.getUsageReport(),
      });
    } catch (error) {
      return {
        success: false,
        error: (error as Error).message,
      };
    }
  }

  async updateItem(kind: 'agent' | 'skill' | 'tool', id: string, orgID?: string | null): Promise<MarketplaceActionResult> {
    try {
      return await this.connection.request<MarketplaceActionResult>('marketplace/update', {
        kind,
        id,
        orgID: orgID || undefined,
        usage: this.getUsageReport(),
      });
    } catch (error) {
      return {
        success: false,
        error: (error as Error).message,
      };
    }
  }
}

export function createMarketplaceService(
  connection: WSConnection,
  getUsageReport: () => MarketplaceUsageReport,
) {
  return new MarketplaceService(connection, getUsageReport);
}

export type { MarketplaceService };
