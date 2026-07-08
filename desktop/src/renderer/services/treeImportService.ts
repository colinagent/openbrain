import {
  createResourceGrantToken,
  resolveWorkspaceHttpBaseUrl,
} from './resourceService';
import {
  buildTreeImportManifest,
  collectTreeImportEntriesFromFileSystemEntries,
  collectTreeImportEntriesFromFiles,
  type FileSystemEntryLike,
  type TreeImportEntry,
  type TreeImportManifestResult,
} from './treeImportManifest';

export type PreparedTreeImport = {
  conflicts: string[];
  fileCount: number;
  dirCount: number;
  targetDir: string;
  commit: (overwrite: boolean) => Promise<TreeImportCommitResult>;
  cancel: () => Promise<void>;
};

type TreeImportCommitResult = {
  importedFiles: number;
  importedDirs: number;
};

type TreeImportSessionResponse = {
  sessionId: string;
  uploadBaseUrl: string;
  conflicts: string[];
};

type DataTransferItemWithEntry = DataTransferItem & {
  webkitGetAsEntry?: () => FileSystemEntryLike | null;
};

function encodeUploadPath(relativePath: string) {
  return relativePath
    .split('/')
    .map((segment) => encodeURIComponent(segment))
    .join('/');
}

async function postJSON<T>(url: string, payload: unknown): Promise<T> {
  const response = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error(await readErrorMessage(response, `Request failed: ${response.status}`));
  }
  return response.json() as Promise<T>;
}

async function readErrorMessage(response: Response, fallback: string) {
  const text = (await response.text().catch(() => '')).trim();
  if (!text) {
    return fallback;
  }
  try {
    const payload = JSON.parse(text) as { error?: string };
    if (typeof payload?.error === 'string' && payload.error.trim()) {
      return payload.error.trim();
    }
  } catch {
    // Ignore JSON parsing failures and fall back to the raw body.
  }
  return text;
}

function isInternalTreeTransfer(dataTransfer: DataTransfer | null | undefined) {
  const text = (dataTransfer?.getData('text/plain') || '').trim();
  const types = Array.from(dataTransfer?.types || []);
  return text.startsWith('openbrain-file:')
    || text.startsWith('openbrain-dir:')
    || text === 'openbrain-tree-transfer'
    || types.includes('application/x-openbrain-file-tree-items');
}

export function hasExternalTreeImportPayload(dataTransfer: DataTransfer | null | undefined) {
  if (!dataTransfer || isInternalTreeTransfer(dataTransfer)) {
    return false;
  }
  if (Array.from(dataTransfer.items || []).some((item) => item.kind === 'file')) {
    return true;
  }
  return Array.from(dataTransfer.types || []).includes('Files');
}

async function collectDroppedEntries(dataTransfer: DataTransfer): Promise<TreeImportManifestResult> {
  const items = Array.from(dataTransfer.items || []);
  const fileSystemEntries = items
    .map((item): FileSystemEntryLike | null => {
      const entry = (item as DataTransferItemWithEntry).webkitGetAsEntry?.() || null;
      return entry as FileSystemEntryLike | null;
    })
    .filter((entry): entry is FileSystemEntryLike => entry !== null);

  if (fileSystemEntries.length > 0) {
    return buildTreeImportManifest(
      await collectTreeImportEntriesFromFileSystemEntries(fileSystemEntries),
    );
  }

  const files = Array.from(dataTransfer.files || []).filter((file) => Boolean(file));
  if (files.length === 0) {
    throw new Error('No dropped files are available for import');
  }
  return buildTreeImportManifest(collectTreeImportEntriesFromFiles(files));
}

export async function prepareTreeImport(
  targetDir: string,
  dataTransfer: DataTransfer,
  workspaceTabId?: string,
): Promise<PreparedTreeImport> {
  const manifest = await collectDroppedEntries(dataTransfer);
  const baseUrl = resolveWorkspaceHttpBaseUrl(workspaceTabId);
  const grantToken = await createResourceGrantToken([targetDir], workspaceTabId);
  const session = await postJSON<TreeImportSessionResponse>(`${baseUrl}/v1/tree-import/sessions`, {
    targetDir,
    entries: manifest.entries,
    grantToken,
  });

  const uploadBaseUrl = session.uploadBaseUrl.startsWith('http')
    ? session.uploadBaseUrl
    : `${baseUrl}${session.uploadBaseUrl}`;

  let finalized = false;

  const cancel = async () => {
    if (finalized) {
      return;
    }
    finalized = true;
    await fetch(`${baseUrl}/v1/tree-import/sessions/${session.sessionId}`, {
      method: 'DELETE',
    }).catch(() => undefined);
  };

  const commit = async (overwrite: boolean) => {
    if (finalized) {
      throw new Error('Tree import session has already been finalized');
    }
    try {
      for (const fileEntry of manifest.files) {
        const uploadResponse = await fetch(
          `${uploadBaseUrl}/${encodeUploadPath(fileEntry.relativePath)}`,
          {
            method: 'PUT',
            headers: { 'Content-Type': fileEntry.file.type || 'application/octet-stream' },
            body: fileEntry.file,
          },
        );
        if (!uploadResponse.ok) {
          throw new Error(await readErrorMessage(uploadResponse, `Upload failed: ${uploadResponse.status}`));
        }
      }

      const result = await postJSON<TreeImportCommitResult>(
        `${baseUrl}/v1/tree-import/sessions/${session.sessionId}/commit`,
        { overwrite },
      );
      finalized = true;
      return result;
    } catch (error) {
      await cancel();
      throw error;
    }
  };

  return {
    conflicts: Array.isArray(session.conflicts) ? session.conflicts : [],
    fileCount: manifest.files.length,
    dirCount: manifest.entries.filter((entry) => entry.kind === 'dir').length,
    targetDir,
    commit,
    cancel,
  };
}
