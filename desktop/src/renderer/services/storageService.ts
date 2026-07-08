import type { WorkspaceStorageBinding, WorkspaceSyncPolicy } from '../types/electron';
import type { WSConnection } from './wsConnection';

export type WorkspaceStorageModelParams = {
  modelKey?: string;
  thinkingLevel?: string;
  contextWindow?: number;
  serviceTier?: string;
};

export type WorkspaceStorageStatusResult = {
  workspaceID?: string;
  path?: string;
  storage?: WorkspaceStorageBinding | null;
  policy?: WorkspaceSyncPolicy;
  status?: string;
  lastSyncAt?: string;
  lastError?: string;
  message?: string;
  error?: string;
};

class StorageService {
  constructor(private connection: WSConnection) {}

  async status(params: { workspaceID?: string; path?: string } & WorkspaceStorageModelParams): Promise<WorkspaceStorageStatusResult> {
    try {
      return await this.connection.request<WorkspaceStorageStatusResult>('storage/status', params);
    } catch (error) {
      return { status: 'error', error: (error as Error).message };
    }
  }

  async syncNow(params: { workspaceID?: string; path?: string } & WorkspaceStorageModelParams): Promise<WorkspaceStorageStatusResult> {
    try {
      return await this.connection.request<WorkspaceStorageStatusResult>('storage/syncNow', params);
    } catch (error) {
      return { status: 'error', error: (error as Error).message };
    }
  }

  async updatePolicy(params: {
    workspaceID?: string;
    path?: string;
    policy: WorkspaceSyncPolicy;
  } & WorkspaceStorageModelParams): Promise<WorkspaceStorageStatusResult> {
    try {
      return await this.connection.request<WorkspaceStorageStatusResult>('storage/updatePolicy', params);
    } catch (error) {
      return { status: 'error', error: (error as Error).message };
    }
  }

}

export function createStorageService(connection: WSConnection) {
  return new StorageService(connection);
}

export type { StorageService };
