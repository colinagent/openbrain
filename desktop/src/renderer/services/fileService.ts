// File service - wraps WebSocket connection for file operations

import type { FileChange, WSConnection } from './wsConnection';

export type { FileChange };

export interface FileEntry {
  name: string;
  isDir: boolean;
  size: number;
  modTime: number;
}

export interface StatResult {
  path: string;
  name: string;
  size: number;
  isDir: boolean;
  modTime: number;
  mode: number;
}

export interface ReadFileResult {
  content?: string;
  encoding?: string;
  size?: number;
  tooLarge?: boolean;
  error?: string;
}

export interface WriteFileResult {
  path?: string;
  size?: number;
  modTime?: number;
  error?: string;
}

export interface ReaddirResult {
  path?: string;
  entries?: FileEntry[];
  error?: string;
}

export interface SearchMatch {
  line: number;
  column: number;
  endColumn: number;
  text: string;
}

export interface SearchFileResult {
  path: string;
  matches: SearchMatch[];
  count: number;
}

export interface SearchResult {
  files?: SearchFileResult[];
  totalCount?: number;
  truncated?: boolean;
  error?: string;
}

export interface WriteFileOptions {
  encoding?: string;
  create?: boolean;
  overwrite?: boolean;
  atomic?: boolean;
}

class FileService {
  constructor(private connection: WSConnection) {}

  async stat(path: string): Promise<StatResult & { error?: string }> {
    try {
      const result = await this.connection.request<StatResult>('fs/stat', { path });
      return result;
    } catch (error) {
      return { error: (error as Error).message } as StatResult & { error: string };
    }
  }

  async readFile(path: string, options?: { encoding?: string; limits?: { size: number } }): Promise<ReadFileResult> {
    try {
      const result = await this.connection.request<ReadFileResult>('fs/readFile', { path, options });
      return result;
    } catch (error) {
      return { error: (error as Error).message };
    }
  }

  async writeFile(path: string, content: string, options?: WriteFileOptions): Promise<WriteFileResult> {
    try {
      const result = await this.connection.request<WriteFileResult>('fs/writeFile', { 
        path, 
        content,
        options: {
          encoding: options?.encoding || 'utf8',
          create: options?.create ?? true,
          overwrite: options?.overwrite ?? true,
          atomic: options?.atomic ?? true,
        }
      });
      return result;
    } catch (error) {
      return { error: (error as Error).message };
    }
  }

  async readdir(path: string): Promise<ReaddirResult> {
    try {
      const result = await this.connection.request<ReaddirResult>('fs/readdir', { path });
      return result;
    } catch (error) {
      return { error: (error as Error).message };
    }
  }

  async search(params: {
    root: string;
    query: string;
    regex?: boolean;
    matchCase?: boolean;
    wholeWord?: boolean;
    includes?: string[];
    excludes?: string[];
    maxFiles?: number;
    maxMatches?: number;
  }): Promise<SearchResult> {
    try {
      return await this.connection.request<SearchResult>('fs/search', params);
    } catch (error) {
      return { error: (error as Error).message };
    }
  }

  async mkdir(path: string, recursive: boolean = true): Promise<{ success?: boolean; error?: string }> {
    try {
      await this.connection.request('fs/mkdir', { path, recursive });
      return { success: true };
    } catch (error) {
      return { error: (error as Error).message };
    }
  }

  async delete(
    path: string,
    recursive: boolean = false,
    useTrash: boolean = false
  ): Promise<{ success?: boolean; error?: string }> {
    try {
      await this.connection.request('fs/delete', { path, recursive, useTrash });
      return { success: true };
    } catch (error) {
      return { error: (error as Error).message };
    }
  }

  async rename(oldPath: string, newPath: string, overwrite: boolean = false): Promise<{ success?: boolean; error?: string }> {
    try {
      await this.connection.request('fs/rename', { oldPath, newPath, overwrite });
      return { success: true };
    } catch (error) {
      return { error: (error as Error).message };
    }
  }

  async copy(source: string, target: string, overwrite: boolean = false): Promise<{ success?: boolean; error?: string }> {
    try {
      await this.connection.request('fs/copy', { source, target, overwrite });
      return { success: true };
    } catch (error) {
      return { error: (error as Error).message };
    }
  }

  async watch(path: string, recursive: boolean = false, excludes?: string[]): Promise<{ watchId?: string; error?: string }> {
    try {
      const result = await this.connection.request<{ watchId: string }>('fs/watch', { path, recursive, excludes });
      return result;
    } catch (error) {
      return { error: (error as Error).message };
    }
  }

  async unwatch(watchId: string): Promise<{ success?: boolean; error?: string }> {
    try {
      await this.connection.request('fs/unwatch', { watchId });
      return { success: true };
    } catch (error) {
      return { error: (error as Error).message };
    }
  }
}

export function createFileService(connection: WSConnection) {
  return new FileService(connection);
}
