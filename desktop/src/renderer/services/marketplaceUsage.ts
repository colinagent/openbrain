import type { OpNode } from './agentService';

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

function isPathInsideRoot(path: string | null | undefined, root: string | null | undefined): boolean {
  const normalizedPath = normalizeDirPath(path);
  const normalizedRoot = normalizeDirPath(root);
  if (!normalizedPath || !normalizedRoot) {
    return false;
  }
  return normalizedPath === normalizedRoot || normalizedPath.startsWith(`${normalizedRoot}/`);
}

function decodeFilePath(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function dirname(path: string): string {
  const normalized = normalizeDirPath(path);
  if (!normalized || normalized === '/') {
    return normalized;
  }
  const parts = normalized.split('/').filter(Boolean);
  parts.pop();
  return normalized.startsWith('/') ? `/${parts.join('/')}` : parts.join('/');
}

function basename(path: string): string {
  const normalized = normalizeDirPath(path);
  if (!normalized || normalized === '/') {
    return normalized;
  }
  const parts = normalized.split('/').filter(Boolean);
  return parts[parts.length - 1] || normalized;
}

function uriToPath(uri: string | null | undefined): string | null {
  const raw = (uri || '').trim();
  if (!raw.startsWith('file://')) {
    return null;
  }
  const filePart = raw.slice('file://'.length);
  return filePart ? decodeFilePath(filePart) : null;
}

function pathFromNode(node: OpNode | null | undefined): string | null {
  return uriToPath(node?.uri);
}

function idFromAgentNode(node: OpNode | null | undefined, agentsRootDir: string): string | null {
  const filePath = pathFromNode(node);
  if (!filePath) {
    return null;
  }
  if (filePath.endsWith('/.agent/AGENT.md')) {
    const agentDir = filePath.slice(0, -('/.agent/AGENT.md'.length));
    return isPathInsideRoot(agentDir, agentsRootDir) ? basename(agentDir) : null;
  }
  if (filePath.endsWith('/.agents/AGENTS.md')) {
    const agentDir = filePath.slice(0, -('/.agents/AGENTS.md'.length));
    return isPathInsideRoot(agentDir, agentsRootDir) ? basename(agentDir) : null;
  }
  return null;
}

function idFromSkillNode(node: OpNode | null | undefined, skillsRootDir: string): string | null {
  const filePath = pathFromNode(node);
  if (!filePath || !filePath.endsWith('/SKILL.md')) {
    return null;
  }
  const skillDir = dirname(filePath);
  return isPathInsideRoot(skillDir, skillsRootDir) ? basename(skillDir) : null;
}

function unique(values: Iterable<string>): string[] {
  const seen = new Set<string>();
  const next: string[] = [];
  for (const value of values) {
    const trimmed = value.trim();
    if (!trimmed || seen.has(trimmed)) {
      continue;
    }
    seen.add(trimmed);
    next.push(trimmed);
  }
  return next;
}

export function buildMarketplaceUsageReport(params: {
  remote: boolean;
  baseDir: string | null;
  agentsRootDir: string | null;
  effectiveAgentID: string | null;
  selectedSkillID: string | null;
  nodes: OpNode[];
}): { agents: string[]; skills: string[]; tools: string[] } {
  if (params.remote) {
    return { agents: [], skills: [], tools: [] };
  }

  const baseDir = normalizeDirPath(params.baseDir);
  const agentsRootDir = normalizeDirPath(params.agentsRootDir || (baseDir ? `${baseDir}/agents` : ''));
  const skillsRootDir = baseDir ? `${baseDir}/skills` : '';
  const toolsRootDir = baseDir ? `${baseDir}/tools` : '';
  if (!baseDir || !agentsRootDir) {
    return { agents: [], skills: [], tools: [] };
  }

  const nodeByID = new Map<string, OpNode>();
  for (const node of params.nodes) {
    const id = (node?.id || '').trim();
    if (id) {
      nodeByID.set(id, node);
    }
  }

  const activeAgentNode = nodeByID.get((params.effectiveAgentID || '').trim()) || null;
  const selectedSkillNode = nodeByID.get((params.selectedSkillID || '').trim()) || null;

  const agents = unique([
    idFromAgentNode(activeAgentNode, agentsRootDir) || '',
  ]);

  const skills = unique([
    idFromSkillNode(selectedSkillNode, skillsRootDir) || '',
  ]);

  const tools = new Set<string>();
  if (activeAgentNode) {
    const meta = (activeAgentNode.meta as Record<string, unknown> | undefined) || {};
    const toolServers = Array.isArray(meta.toolServers) ? meta.toolServers : [];
    for (const rawID of toolServers) {
      if (typeof rawID !== 'string') {
        continue;
      }
      const toolPath = pathFromNode(nodeByID.get(rawID.trim()));
      if (!toolPath || !toolPath.endsWith('/TOOL.md')) {
        continue;
      }
      const toolDir = dirname(toolPath);
      if (!isPathInsideRoot(toolDir, toolsRootDir)) {
        continue;
      }
      tools.add(basename(toolDir));
    }
    const sysTools = Array.isArray(meta.sysTools) ? meta.sysTools : [];
    if (sysTools.some((value) => typeof value === 'string' && value.trim())) {
      tools.add('systool');
    }
  }

  return {
    agents,
    skills,
    tools: Array.from(tools),
  };
}
