export type ChatAgentTarget = {
  agentID: string;
  agentName: string | null;
  agentCwd: string;
};

type ResolveAgentForCwd = (cwd: string) => ChatAgentTarget | null;

type ResolveChatAgentTargetInput = {
  selectedChatPath: string | null;
  explicitAgentID: string | null;
  explicitAgentName: string | null;
  explicitAgentCwd: string | null;
  currentDir: string | null;
  resolveChatAgentForCwd: ResolveAgentForCwd;
};

export const CHAT_FILE_SEGMENT = '/.agent/chat/';
export const COMMAND_LOG_DIR_NAME = 'temp';

function normalizePath(path: string | null): string {
  return (path || '').trim().replace(/\/+$/, '');
}

function normalizeText(value: string | null | undefined): string | null {
  const normalized = (value || '').trim();
  return normalized || null;
}

export function getChatWorkdir(chatPath: string | null): string {
  const normalized = normalizePath(chatPath);
  const idx = normalized.indexOf(CHAT_FILE_SEGMENT);
  return idx > 0 ? normalized.slice(0, idx) : '';
}

export function isThreadChatPath(path: string | null | undefined): boolean {
  return normalizePath(path || null).includes(CHAT_FILE_SEGMENT);
}

export function getCommandLogDir(workspaceRoot: string | null): string {
  const normalizedRoot = normalizePath(workspaceRoot);
  return normalizedRoot ? `${normalizedRoot}/${COMMAND_LOG_DIR_NAME}` : '';
}

export function isCommandLogPath(path: string | null | undefined, workspaceRoot: string | null): boolean {
  const normalizedPath = normalizePath(path || null);
  const commandLogDir = getCommandLogDir(workspaceRoot);
  if (!normalizedPath || !commandLogDir) {
    return false;
  }
  if (!normalizedPath.endsWith('.md')) {
    return false;
  }
  return normalizedPath.startsWith(`${commandLogDir}/`);
}

export function isConversationDocumentPath(path: string | null | undefined, workspaceRoot: string | null): boolean {
  return isThreadChatPath(path) || isCommandLogPath(path, workspaceRoot);
}

export function isPathInsideRoot(path: string | null, root: string | null): boolean {
  const normalizedPath = normalizePath(path);
  const normalizedRoot = normalizePath(root);
  if (!normalizedPath || !normalizedRoot) {
    return false;
  }
  return normalizedPath === normalizedRoot || normalizedPath.startsWith(`${normalizedRoot}/`);
}

function resolveExplicitTarget(
  explicitAgentID: string | null,
  explicitAgentName: string | null,
  explicitAgentCwd: string | null,
  resolveChatAgentForCwd: ResolveAgentForCwd,
): ChatAgentTarget | null {
  const agentCwd = normalizePath(explicitAgentCwd);
  const agentID = normalizeText(explicitAgentID);
  if (!agentID) {
    return null;
  }

  const indexedTarget = agentCwd ? resolveChatAgentForCwd(agentCwd) : null;
  const indexedAgentID = normalizeText(indexedTarget?.agentID);
  if (indexedAgentID && indexedAgentID !== agentID) {
    return null;
  }

  return {
    agentID,
    agentName: normalizeText(indexedTarget?.agentName) || normalizeText(explicitAgentName),
    agentCwd,
  };
}

export function resolveChatAgentTarget(input: ResolveChatAgentTargetInput): ChatAgentTarget | null {
  const explicitTarget = resolveExplicitTarget(
    input.explicitAgentID,
    input.explicitAgentName,
    input.explicitAgentCwd,
    input.resolveChatAgentForCwd,
  );

  const selectedChatCwd = getChatWorkdir(input.selectedChatPath);

  if (selectedChatCwd) {
    const selectedTarget = input.resolveChatAgentForCwd(selectedChatCwd);
    if (selectedTarget) {
      return selectedTarget;
    }
    if (explicitTarget && !explicitTarget.agentCwd) {
      return {
        ...explicitTarget,
        agentCwd: selectedChatCwd,
      };
    }
    if (explicitTarget && explicitTarget.agentCwd === selectedChatCwd) {
      return explicitTarget;
    }
    return null;
  }

  if (explicitTarget) {
    return explicitTarget;
  }

  return null;
}
