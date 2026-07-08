import type { WSConnection } from './wsConnection';

export interface GitBranch {
  name: string;
  current: boolean;
}

export interface GitDirtySummary {
  changedFiles: number;
  addedLines: number;
  deletedLines: number;
  hasChanges: boolean;
}

export interface GitBranchesResult {
  isRepo: boolean;
  repoRoot?: string;
  currentBranch?: string;
  detached?: boolean;
  detachedLabel?: string;
  branches?: GitBranch[];
  dirty?: GitDirtySummary;
  error?: string;
}

export interface GitCheckoutResult {
  repoRoot?: string;
  currentBranch?: string;
  error?: string;
}

class GitService {
  constructor(private connection: WSConnection) {}

  async getBranches(path: string): Promise<GitBranchesResult> {
    try {
      return await this.connection.request<GitBranchesResult>('git/branches', { path });
    } catch (error) {
      return { isRepo: false, error: (error as Error).message };
    }
  }

  async checkout(params: { path: string; branch: string; create?: boolean }): Promise<GitCheckoutResult> {
    try {
      return await this.connection.request<GitCheckoutResult>('git/checkout', params);
    } catch (error) {
      return { error: (error as Error).message };
    }
  }
}

export function createGitService(connection: WSConnection) {
  return new GitService(connection);
}

export type { GitService };
