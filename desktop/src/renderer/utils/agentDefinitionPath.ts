import type { OpNode } from '../services/agentService';
import { fileUrlToAbsolutePath, parseCanonicalFileURI } from '../core/resource/uri';
import { normalizePosixPath } from './markdownMedia';

function normalizeAbsolutePath(path: string | null | undefined): string | null {
  const value = (path || '').trim();
  if (!value || !value.startsWith('/')) {
    return null;
  }
  return normalizePosixPath(value);
}

export function isAgentDefinitionFilePath(path: string | null | undefined): path is string {
  const value = (path || '').trim();
  return value.endsWith('/.agent/AGENT.md') || value.endsWith('/.agents/AGENTS.md');
}

function agentDefinitionPathFromURI(uri: string | null | undefined): string | null {
  const value = (uri || '').trim();
  if (!value) {
    return null;
  }
  try {
    const path = value.startsWith('file://')
      ? normalizePosixPath(fileUrlToAbsolutePath(value))
      : value.startsWith('opfs://')
        ? normalizePosixPath(parseCanonicalFileURI(value).path)
        : null;
    return isAgentDefinitionFilePath(path) ? path : null;
  } catch {
    return null;
  }
}

function joinAgentDefinitionPath(cwd: string): string {
  const normalized = normalizePosixPath(cwd);
  return normalized === '/' ? '/.agent/AGENT.md' : `${normalized}/.agent/AGENT.md`;
}

export function resolveAgentDefinitionPath(
  record: OpNode | null | undefined,
  indexed: { uri: string | null; path: string | null } | null | undefined,
): string | null {
  const recordURIPath = agentDefinitionPathFromURI(record?.uri);
  if (recordURIPath) {
    return recordURIPath;
  }

  const indexedURIPath = agentDefinitionPathFromURI(indexed?.uri);
  if (indexedURIPath) {
    return indexedURIPath;
  }

  const cwd = normalizeAbsolutePath(record?.cwd) || normalizeAbsolutePath(indexed?.path);
  return cwd ? joinAgentDefinitionPath(cwd) : null;
}
