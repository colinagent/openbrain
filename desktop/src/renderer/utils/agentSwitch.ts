import type { OpNode } from '../services/agentService';

export const PRIMARY_CHAT_CAPABLE_AGENT_OPCODE = 'thread/submit';
export const CHAT_CAPABLE_AGENT_OPCODES = [
  PRIMARY_CHAT_CAPABLE_AGENT_OPCODE,
] as const;
export const DEFAULT_AGENT_TARGET_WORKSPACE_LABEL = 'workspace';

export type AgentSwitchOption = {
  id: string;
  name: string;
  path: string | null;
  selected: boolean;
};

function normalizeDirPath(input: string | null | undefined): string {
  const value = (input || '').trim();
  if (!value) {
    return '';
  }
  if (value === '/') {
    return '/';
  }
  return value.replace(/\/+$/, '');
}

function normalizeText(value: string | null | undefined): string {
  return (value || '').trim();
}

function isPathInsideRoot(path: string | null | undefined, root: string | null | undefined): boolean {
  const normalizedPath = normalizeDirPath(path);
  const normalizedRoot = normalizeDirPath(root);
  if (!normalizedPath || !normalizedRoot) {
    return false;
  }
  return normalizedPath === normalizedRoot || normalizedPath.startsWith(`${normalizedRoot}/`);
}

function uriToPath(uri: string): string | null {
  if (!uri.startsWith('file://')) {
    return null;
  }
  const raw = uri.slice('file://'.length);
  if (!raw) {
    return null;
  }
  try {
    return decodeURIComponent(raw);
  } catch {
    return raw;
  }
}

function workdirFromUri(uri: string): string | null {
  const path = uriToPath(uri);
  if (!path) {
    return null;
  }
  if (path.endsWith('/.agent/AGENT.md')) {
    return path.slice(0, -('/.agent/AGENT.md'.length));
  }
  if (path.endsWith('/.agents/AGENTS.md')) {
    return path.slice(0, -('/.agents/AGENTS.md'.length));
  }
  return null;
}

function getNodePath(node: OpNode): string | null {
  const cwd = (node.cwd || '').trim();
  if (cwd) {
    return cwd;
  }
  const uri = (node.uri || '').trim();
  if (!uri) {
    return null;
  }
  return workdirFromUri(uri);
}

function getNodeName(node: OpNode): string {
  const meta = (node.meta as Record<string, unknown> | undefined) || {};
  const raw = typeof meta.name === 'string' ? meta.name.trim() : '';
  return raw || (node.id || '').trim();
}

function hasBindReference(node: OpNode): boolean {
  const meta = (node.meta as Record<string, unknown> | undefined) || {};
  const bind = typeof meta.bind === 'string' ? meta.bind.trim() : '';
  return Boolean(bind);
}

function getLastPathSegment(path: string | null | undefined): string {
  const normalized = normalizeDirPath(path);
  if (!normalized || normalized === '/') {
    return normalized;
  }
  const parts = normalized.split('/').filter(Boolean);
  return parts[parts.length - 1] || normalized;
}

export function normalizeAgentID(agentID: string | null | undefined): string {
  const raw = normalizeText(agentID);
  return raw.startsWith('@') ? raw.slice(1).trim() : raw;
}

export function resolveAgentRootWorkdir(path: string | null | undefined): string {
  const normalized = normalizeDirPath(path);
  if (!normalized) {
    return '';
  }
  const parts = normalized.split('/').filter(Boolean);
  for (let i = parts.length - 1; i >= 0; i -= 1) {
    if (parts[i] === '.agent') {
      return i === 0 ? '/' : `/${parts.slice(0, i).join('/')}`;
    }
  }
  return normalized;
}

export function findChatCapableAgentOpcode(node: OpNode | null | undefined): string | null {
  const rawCodes = node?.opCodes;
  const codes = Array.isArray(rawCodes) ? rawCodes : [];
  const normalized = new Set<string>();
  for (const value of codes) {
    const opcode = normalizeText(value);
    if (opcode) {
      normalized.add(opcode);
    }
  }
  if (normalized.has(PRIMARY_CHAT_CAPABLE_AGENT_OPCODE)) {
    return PRIMARY_CHAT_CAPABLE_AGENT_OPCODE;
  }
  return null;
}

export function isChatCapableAgentNode(node: OpNode | null | undefined): boolean {
  const kind = normalizeText(node?.kind || '').toLowerCase();
  if (kind !== 'agent') {
    return false;
  }
  return findChatCapableAgentOpcode(node) != null;
}

export function buildAgentSwitchOptions(params: {
  agentNodes: OpNode[];
  agentsRootDir: string | null;
  currentAgentID: string | null;
}): AgentSwitchOption[] {
  const agentsRootDir = normalizeDirPath(params.agentsRootDir);
  const currentAgentID = normalizeAgentID(params.currentAgentID);
  const seen = new Set<string>();
  const options: AgentSwitchOption[] = [];

  for (const node of params.agentNodes) {
    const id = normalizeAgentID(node?.id || '');
    if (!id || seen.has(id) || hasBindReference(node) || !isChatCapableAgentNode(node)) {
      continue;
    }

    const path = getNodePath(node);
    if (!isPathInsideRoot(path, agentsRootDir)) {
      continue;
    }

    seen.add(id);
    options.push({
      id,
      name: getNodeName(node),
      path,
      selected: id === currentAgentID,
    });
  }

  options.sort((a, b) => {
    const nameOrder = a.name.localeCompare(b.name, undefined, { sensitivity: 'base' });
    if (nameOrder !== 0) {
      return nameOrder;
    }
    const pathOrder = (a.path || '').localeCompare(b.path || '', undefined, { sensitivity: 'base' });
    if (pathOrder !== 0) {
      return pathOrder;
    }
    return a.id.localeCompare(b.id, undefined, { sensitivity: 'base' });
  });

  return options;
}

export function formatAgentTargetDisplayLabel(targetDir: string | null | undefined, agentLabel: string | null | undefined): string {
  const dirName = getLastPathSegment(targetDir);
  const label = normalizeText(agentLabel);
  if (dirName && label) {
    return `${dirName}:${label}`;
  }
  if (dirName) {
    return `${dirName}:—`;
  }
  if (label) {
    return `${DEFAULT_AGENT_TARGET_WORKSPACE_LABEL}:${label}`;
  }
  return '—';
}

export function formatAgentTargetDisplayTitle(targetDir: string | null | undefined, agentLabel: string | null | undefined): string {
  const dir = normalizeDirPath(targetDir);
  const label = normalizeText(agentLabel);
  if (dir && label) {
    return `${dir} · ${label}`;
  }
  if (dir) {
    return dir;
  }
  if (label) {
    return `${DEFAULT_AGENT_TARGET_WORKSPACE_LABEL} · ${label}`;
  }
  return '';
}
