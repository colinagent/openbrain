export const AGENT_LINK_PREFIX = 'agent:';

export type ParsedAgentMention = {
  agentID: string;
  from: number;
  to: number;
};

const AGENT_MENTION_PATTERN = /@agent-[A-Za-z0-9][A-Za-z0-9_-]*/g;
const AGENT_ID_PATTERN = /^agent-[A-Za-z0-9][A-Za-z0-9_-]*$/;
const AGENT_MENTION_PATTERN_EXACT = /^@agent-[A-Za-z0-9][A-Za-z0-9_-]*$/;

function isAgentMentionBoundaryChar(char: string | undefined): boolean {
  return !char || !/[A-Za-z0-9_@-]/.test(char);
}

export function normalizeAgentMentionID(value: string | null | undefined): string | null {
  const raw = (value || '').trim();
  const id = raw.startsWith('@') ? raw.slice(1).trim() : raw;
  return AGENT_ID_PATTERN.test(id) ? id : null;
}

export function parseAgentMentionValue(value: string | null | undefined): string | null {
  const raw = (value || '').trim();
  if (!AGENT_MENTION_PATTERN_EXACT.test(raw)) {
    return null;
  }
  return normalizeAgentMentionID(raw);
}

export function buildAgentLinkTarget(agentID: string | null | undefined): string | null {
  const normalized = normalizeAgentMentionID(agentID);
  return normalized ? `${AGENT_LINK_PREFIX}${normalized}` : null;
}

export function parseAgentLinkTarget(raw: string | null | undefined): string | null {
  const value = (raw || '').trim();
  if (!value.startsWith(AGENT_LINK_PREFIX)) {
    return null;
  }
  return normalizeAgentMentionID(value.slice(AGENT_LINK_PREFIX.length));
}

export function parseAgentMentionsInText(text: string): ParsedAgentMention[] {
  const source = typeof text === 'string' ? text : '';
  const mentions: ParsedAgentMention[] = [];
  let match: RegExpExecArray | null;
  AGENT_MENTION_PATTERN.lastIndex = 0;
  while ((match = AGENT_MENTION_PATTERN.exec(source)) !== null) {
    const from = match.index;
    const to = from + match[0].length;
    if (!isAgentMentionBoundaryChar(source[from - 1]) || !isAgentMentionBoundaryChar(source[to])) {
      continue;
    }
    const agentID = normalizeAgentMentionID(match[0]);
    if (!agentID) {
      continue;
    }
    mentions.push({ agentID, from, to });
  }
  return mentions;
}
